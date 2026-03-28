// Package main is the entrypoint for the Aperture server binary.
// It performs bootstrap wiring only — no business logic lives here.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ApertureHQ/aperture/internal/api"
	"github.com/ApertureHQ/aperture/internal/auth"
	"github.com/ApertureHQ/aperture/internal/bridge"
	browserpool "github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/config"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
	"github.com/ApertureHQ/aperture/internal/llm"
	"github.com/ApertureHQ/aperture/internal/observe"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/ApertureHQ/aperture/internal/policy"
	"github.com/ApertureHQ/aperture/internal/resolver"
	"github.com/ApertureHQ/aperture/internal/sequencer"
	"github.com/ApertureHQ/aperture/internal/session"
	"github.com/ApertureHQ/aperture/internal/stream"
	"github.com/ApertureHQ/aperture/internal/vision"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	pool, err := browserpool.NewPool(browserpool.Config{
		PoolSize:     cfg.Browser.PoolSize,
		ChromiumPath: cfg.Browser.ChromiumPath,
		Stealth:      mapStealthConfig(cfg),
	})
	if err != nil {
		slog.Error("failed to create browser pool", "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := pool.Close(); closeErr != nil {
			slog.Warn("browser pool close error", "error", closeErr)
		}
	}()

	slog.Info("browser pool ready", "size", pool.Size(), "available", pool.Available())

	// Bootstrap the full session manager with planner + sequencer.
	llmClient := buildLLMClient(cfg)
	hitlMgr := executor.NewDefaultHITLManager()
	reg := buildRegistry(pool, llmClient, hitlMgr)
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg})
	p := buildPlanner(cfg)

	// VisionAnalyzer wraps the LLM client for screenshot analysis.
	// LLMVisionAnalyzer needs a VisionLLMClient — check if our client implements it.
	var visionAnalyzer domain.VisionAnalyzer
	if visionClient, ok := llmClient.(domain.VisionLLMClient); ok {
		visionAnalyzer = vision.NewLLMVisionAnalyzer(visionClient)
		slog.Info("vision analyzer enabled")
	}

	// Auth persistence: save/restore cookies across restarts.
	var authPersist domain.AuthPersistence
	if ap, err := auth.NewFileAuthPersistence(auth.FileAuthConfig{}); err != nil {
		slog.Warn("auth persistence unavailable, cookies will not survive restarts", "error", err)
	} else {
		authPersist = ap
		slog.Info("auth persistence enabled")
	}

	// xBPP Policy Engine: gates every agent action against configurable rules.
	policyEngine := policy.NewInMemoryPolicyEngine()
	slog.Info("xBPP policy engine enabled")

	// Metrics collector for action timing and success rates.
	metrics := observe.NewInMemoryMetrics()

	// Progress emitter for WebSocket streaming.
	emitter := stream.NewChannelEmitter()

	sessionMgr := session.NewDefaultSessionManager(session.Config{
		Pool:            pool,
		Planner:         p,
		Sequencer:       seq,
		AuthPersistence: authPersist,
		PolicyEngine:    policyEngine,
	})

	screenshotSrv := browserpool.NewScreenshotService(pool)

	bridgeSrv := bridge.NewOpenClawBridge(bridge.Config{
		SessionManager: sessionMgr,
		MaxConcurrent:  cfg.Bridge.MaxConcurrentTasks,
	})

	router := api.NewRouter(api.RouterConfig{
		SessionManager:    sessionMgr,
		ScreenshotService: screenshotSrv,
		VisionAnalyzer:    visionAnalyzer,
		Bridge:            bridgeSrv,
		BrowserPool:       pool,
		HITLManager:       hitlMgr,
		ProgressEmitter:   emitter,
		MetricsCollector:  metrics,
		PolicyEngine:      policyEngine,
		Auth:              buildAuthConfig(cfg),
		RateLimit:         api.RateLimitConfig{RequestsPerMinute: cfg.API.RateLimitRPM},
		CORSOrigins:       cfg.API.CORSOrigins,
	})

	// Railway sets PORT env var. Use it if present, else use config.
	port := cfg.Server.Port
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := fmt.Sscanf(envPort, "%d", &port); p == 1 && err == nil {
			slog.Info("using PORT env var", "port", port)
		}
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, port)
	slog.Info("aperture server starting", "addr", addr)

	srv := &http.Server{Addr: addr, Handler: router}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("shutdown signal received", "signal", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("graceful shutdown failed", "error", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped gracefully")
}

// buildRegistry constructs the default executor registry wired to the pool.
// If llm is non-nil, the extract executor is included for structured data extraction.
func buildRegistry(pool domain.BrowserPool, llm domain.LLMClient, hitlMgr *executor.DefaultHITLManager) map[string]domain.Executor {
	res := resolver.NewUnifiedResolver()
	tabMgr := browserpool.NewPoolTabProxy(pool)
	reg := map[string]domain.Executor{
		"navigate":   executor.NewNavigateExecutor(),
		"click":      executor.NewClickExecutor(res),
		"type":       executor.NewTypeExecutor(res),
		"screenshot": executor.NewScreenshotExecutor(),
		"scroll":     executor.NewScrollExecutor(),
		"hover":      executor.NewHoverExecutor(res),
		"select":     executor.NewSelectExecutor(res),
		"wait":       executor.NewWaitExecutor(),
		"pause":      executor.NewPauseExecutor(hitlMgr),
		"upload":     executor.NewUploadExecutor(res),
	}
	reg["new_tab"] = executor.NewNewTabExecutor(tabMgr)
	reg["switch_tab"] = executor.NewSwitchTabExecutor(tabMgr)
	if llm != nil {
		reg["extract"] = executor.NewExtractExecutor(llm)
	}
	return reg
}

// buildPlanner returns an LLMPlanner if an API key is configured, else a
// no-op fallback planner so the server starts without LLM credentials.
func buildPlanner(cfg *config.Config) domain.Planner {
	slog.Info("LLM config", "provider", cfg.LLM.Provider, "model", cfg.LLM.Model, "base_url", cfg.LLM.BaseURL, "has_key", cfg.LLM.APIKey != "")
	if cfg.LLM.APIKey == "" {
		slog.Warn("no LLM API key configured — bridge execute will use fallback planner")
		return planner.NewFallbackPlanner()
	}
	p, err := planner.NewLLMPlannerFromConfig(planner.PlannerConfig{
		Provider: cfg.LLM.Provider,
		APIKey:   cfg.LLM.APIKey,
		Model:    cfg.LLM.Model,
		BaseURL:  cfg.LLM.BaseURL,
	})
	if err != nil {
		slog.Warn("failed to build LLM planner, using fallback", "error", err)
		return planner.NewFallbackPlanner()
	}
	return p
}

// buildLLMClient returns an LLMClient if credentials are configured, nil otherwise.
// Used to wire the extract executor and any other LLM-dependent components.
func buildLLMClient(cfg *config.Config) domain.LLMClient {
	if cfg.LLM.APIKey == "" {
		return nil
	}
	client, err := llm.NewClient(llm.Config{
		Provider: cfg.LLM.Provider,
		Model:    cfg.LLM.Model,
		APIKey:   cfg.LLM.APIKey,
		BaseURL:  cfg.LLM.BaseURL,
	})
	if err != nil {
		slog.Warn("failed to build LLM client for extraction", "error", err)
		return nil
	}
	return client
}

// buildAuthConfig constructs API key auth config from the app config.
func buildAuthConfig(cfg *config.Config) api.AuthConfig {
	keys := make(map[string]bool, len(cfg.API.Keys))
	for _, k := range cfg.API.Keys {
		keys[k] = true
	}
	return api.AuthConfig{
		Keys:      keys,
		KeyPrefix: cfg.API.KeyPrefix,
		SkipPaths: []string{"/health", "/website"},
	}
}

// mapStealthConfig converts YAML stealth settings to domain.StealthConfig.
func mapStealthConfig(cfg *config.Config) domain.StealthConfig {
	s := cfg.Stealth
	return domain.StealthConfig{
		Enabled:       s.Enabled,
		HideWebDriver: s.HideWebDriver,
		CanvasNoise:   s.CanvasNoise,
		BlockWebRTC:   s.BlockWebRTC,
		RandomView:    s.RandomViewport,
		MockPlugins:   s.MockPlugins,
		Timezone:      s.Timezone,
		GeoLatitude:   s.GeoLatitude,
		GeoLongitude:  s.GeoLongitude,
	}
}
