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
	"github.com/ApertureHQ/aperture/internal/billing"
	"github.com/ApertureHQ/aperture/internal/bridge"
	browserpool "github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/config"
	"github.com/ApertureHQ/aperture/internal/stealth"
	"github.com/ApertureHQ/aperture/internal/credentials"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
	"github.com/ApertureHQ/aperture/internal/llm"
	"github.com/ApertureHQ/aperture/internal/memory"
	"github.com/ApertureHQ/aperture/internal/observe"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/ApertureHQ/aperture/internal/policy"
	"github.com/ApertureHQ/aperture/internal/profiles"
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

	stealthCfg := mapStealthConfig(cfg)

	// Start uTLS proxy before the browser pool so the pool gets the proxy address.
	if cfg.Stealth.UTLS.Enabled {
		fp := cfg.Stealth.UTLS.Fingerprint
		if fp == "" {
			fp = stealth.DefaultFingerprint
		}
		proxy, proxyErr := stealth.NewProxy(fp)
		if proxyErr != nil {
			slog.Error("failed to start utls proxy", "error", proxyErr)
			os.Exit(1)
		}
		proxy.Start(context.Background())
		stealthCfg.UTLSProxyAddr = proxy.Addr()
		stealthCfg.UTLSEnabled = true
		stealthCfg.UTLSFingerprint = fp
		slog.Info("utls proxy started", "addr", proxy.Addr(), "fingerprint", fp)
		defer proxy.Close()
	}

	pool, err := browserpool.NewPool(browserpool.Config{
		PoolSize:     cfg.Browser.PoolSize,
		ChromiumPath: cfg.Browser.ChromiumPath,
		Stealth:      stealthCfg,
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

	// xBPP Policy Engine: gates every agent action against configurable rules.
	policyEngine := policy.NewInMemoryPolicyEngine()
	slog.Info("xBPP policy engine enabled")

	// Site profiles: YAML-defined domain intelligence for structured extraction.
	var profileMgr domain.SiteProfileManager
	if pm, err := profiles.NewYAMLProfileManager(); err != nil {
		slog.Warn("profile manager failed to load", "error", err)
		profileMgr = profiles.NewNoopProfileManager()
	} else {
		profileMgr = pm
		slog.Info("site profiles loaded", "count", len(pm.Profiles()))
	}

	// Auth persistence: save/restore cookies across restarts.
	var authPersist domain.AuthPersistence
	if ap, err := auth.NewFileAuthPersistence(auth.FileAuthConfig{}); err != nil {
		slog.Warn("auth persistence unavailable, cookies will not survive restarts", "error", err)
	} else {
		authPersist = ap
		slog.Info("auth persistence enabled")
	}

	// Credential vault: encrypted per-agent, per-domain credentials.
	var vault domain.CredentialVault
	if v, err := credentials.NewEncryptedFileVault(); err != nil {
		slog.Warn("credential vault unavailable", "error", err)
	} else {
		vault = v
		slog.Info("credential vault enabled")
	}

	// Bootstrap the full session manager with planner + sequencer.
	llmClient := buildLLMClient(cfg)
	hitlMgr := executor.NewDefaultHITLManager()
	reg := buildRegistry(pool, llmClient, hitlMgr, profileMgr, vault)
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg})
	p := buildPlanner(cfg)

	// VisionAnalyzer wraps the LLM client for screenshot analysis.
	// LLMVisionAnalyzer needs a VisionLLMClient — check if our client implements it.
	var visionAnalyzer domain.VisionAnalyzer
	if visionClient, ok := llmClient.(domain.VisionLLMClient); ok {
		visionAnalyzer = vision.NewLLMVisionAnalyzer(visionClient)
		slog.Info("vision analyzer enabled")
	}

	// Metrics collector for action timing and success rates.
	metrics := observe.NewInMemoryMetrics()

	// Agent state KV store for per-agent memory.
	agentStateStore := memory.NewInMemoryKV()
	slog.Info("agent state KV store enabled")

	// Billing: SQLite-backed credit system.
	billingDB, err := billing.InitDB("aperture.db")
	if err != nil {
		slog.Error("failed to initialize billing database", "error", err)
		os.Exit(1)
	}
	defer billingDB.Close()
	accountService := billing.NewAccountService(billingDB)
	slog.Info("billing system initialized")

	// Progress emitter for WebSocket streaming.
	emitter := stream.NewChannelEmitter()

	sessionMgr := session.NewDefaultSessionManager(session.Config{
		Pool:            pool,
		Planner:         p,
		Sequencer:       seq,
		AuthPersistence: authPersist,
		PolicyEngine:    policyEngine,
		Billing:         accountService,
	})

	screenshotSrv := browserpool.NewScreenshotService(pool)

	bridgeSrv := bridge.NewOpenClawBridge(bridge.Config{
		SessionManager: sessionMgr,
		MaxConcurrent:  cfg.Bridge.MaxConcurrentTasks,
	})

	// Stateful task planner: parallel execution path with SSE streaming.
	checkpointDir := cfg.CheckpointDir
	if checkpointDir == "" {
		checkpointDir = "checkpoints"
	}
	var taskPlannerSvc domain.TaskPlanner
	if llmClient != nil {
		taskPlannerSvc = planner.NewStatefulTaskPlanner(llmClient, reg, checkpointDir)
		slog.Info("stateful task planner enabled", "checkpoint_dir", checkpointDir)
	} else {
		slog.Warn("stateful task planner disabled: no LLM client configured")
	}

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
		ProfileManager:    profileMgr,
		CredentialVault:   vault,
		AgentStateStore:   agentStateStore,
		TaskPlanner:       taskPlannerSvc,
		TaskCheckpointDir: checkpointDir,
		AccountService:    accountService,
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
func buildRegistry(pool domain.BrowserPool, llm domain.LLMClient, hitlMgr *executor.DefaultHITLManager, profileMgr domain.SiteProfileManager, credVault domain.CredentialVault) map[string]domain.Executor {
	res := resolver.NewUnifiedResolver()
	tabMgr := browserpool.NewPoolTabProxy(pool)
	var navOpts []executor.NavigateOption
	if profileMgr != nil {
		navOpts = append(navOpts, executor.WithProfileManager(profileMgr))
	}
	if credVault != nil {
		navOpts = append(navOpts, executor.WithCredentialVault(credVault))
	}
	reg := map[string]domain.Executor{
		"navigate":   executor.NewNavigateExecutor(navOpts...),
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
// UTLSProxyAddr is NOT set here — it is populated at runtime after the proxy starts.
func mapStealthConfig(cfg *config.Config) domain.StealthConfig {
	s := cfg.Stealth
	webgl := s.WebGL
	if webgl == "" {
		webgl = "swiftshader"
	}
	// When SwiftShader is active, canvas noise is redundant and counterproductive.
	// SwiftShader produces identical output across all instances (crowd-blend).
	// Canvas noise adds random per-session deltas that are ML-detectable.
	canvasNoise := s.CanvasNoise
	if webgl == "swiftshader" {
		canvasNoise = false
	}
	fp := s.UTLS.Fingerprint
	if fp == "" {
		fp = "chrome_120"
	}
	return domain.StealthConfig{
		Enabled:         s.Enabled,
		HideWebDriver:   s.HideWebDriver,
		CanvasNoise:     canvasNoise,
		BlockWebRTC:     s.BlockWebRTC,
		RandomView:      s.RandomViewport,
		MockPlugins:     s.MockPlugins,
		Timezone:        s.Timezone,
		GeoLatitude:     s.GeoLatitude,
		GeoLongitude:    s.GeoLongitude,
		WebGL:           webgl,
		UTLSEnabled:     s.UTLS.Enabled,
		UTLSFingerprint: fp,
		// UTLSProxyAddr is set later in main() after the proxy starts.
	}
}
