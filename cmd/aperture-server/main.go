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
	"github.com/ApertureHQ/aperture/internal/checkpoint"
	"github.com/ApertureHQ/aperture/internal/config"
	"github.com/ApertureHQ/aperture/internal/credentials"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
	"github.com/ApertureHQ/aperture/internal/llm"
	"github.com/ApertureHQ/aperture/internal/captcha"
	"github.com/ApertureHQ/aperture/internal/memory"
	"github.com/ApertureHQ/aperture/internal/observe"
	"github.com/ApertureHQ/aperture/internal/store"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/ApertureHQ/aperture/internal/policy"
	"github.com/ApertureHQ/aperture/internal/profiles"
	"github.com/ApertureHQ/aperture/internal/resolver"
	"github.com/ApertureHQ/aperture/internal/sequencer"
	"github.com/ApertureHQ/aperture/internal/session"
	"github.com/ApertureHQ/aperture/internal/stealth"
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
		proxyMode := cfg.Stealth.UTLS.Mode
		if proxyMode == "" {
			proxyMode = "relay"
		}
		var proxy *stealth.Proxy
		var proxyErr error
		if proxyMode == "mitm" {
			proxy, proxyErr = stealth.NewMITMProxy(fp)
		} else {
			proxy, proxyErr = stealth.NewProxy(fp)
		}
		if proxyErr != nil {
			slog.Error("failed to start utls proxy", "error", proxyErr)
			os.Exit(1)
		}
		proxy.Start(context.Background())
		stealthCfg.UTLSProxyAddr = proxy.Addr()
		stealthCfg.UTLSEnabled = true
		stealthCfg.UTLSFingerprint = fp
		slog.Info("utls proxy started", "addr", proxy.Addr(), "fingerprint", fp, "mode", proxyMode)
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

	// xBPP Policy Engine: starts in-memory, upgraded to persistent after store init.
	// Replaced below once persistStore is available.
	var policyEngine domain.PolicyEngine = policy.NewInMemoryPolicyEngine()
	slog.Info("xBPP policy engine enabled (in-memory, will upgrade to persistent)")

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

	// Billing: SQLite-backed credit system.
	billingDB, err := billing.InitDB("aperture.db")
	if err != nil {
		slog.Error("failed to initialize billing database", "error", err)
		os.Exit(1)
	}
	defer billingDB.Close()
	accountService := billing.NewAccountService(billingDB)
	slog.Info("billing system initialized")

	// Persistent store: SQLite-backed KV, policy, and session metadata.
	// Uses the same aperture.db file as billing (separate tables, WAL mode).
	persistStore, persistErr := store.NewSQLiteStore("aperture.db")
	if persistErr != nil {
		slog.Warn("persistent store unavailable, falling back to in-memory", "error", persistErr)
	} else {
		defer persistStore.Close()
		// Upgrade policy engine to persistent
		if pe, err := policy.NewStorePolicyEngine(persistStore); err != nil {
			slog.Warn("persistent policy engine failed, staying in-memory", "error", err)
		} else {
			policyEngine = pe
			slog.Info("xBPP policy engine upgraded to persistent SQLite")
		}
	}

	// Agent state KV store: persistent if store is available, in-memory otherwise.
	var agentStateStore domain.AgentStateStore
	if persistStore != nil {
		agentStateStore = memory.NewStoreBackedKV(persistStore)
		slog.Info("agent state KV store: persistent SQLite")
	} else {
		agentStateStore = memory.NewInMemoryKV()
		slog.Info("agent state KV store: in-memory (no persistent store)")
	}

	// Session recovery: any session marked "active" in the DB was interrupted by
	// a prior unclean shutdown. Mark them "interrupted" so callers can detect and
	// retry them rather than seeing stale "active" entries.
	if persistStore != nil {
		recoverInterruptedSessions(persistStore)
	}

	// Progress emitter for WebSocket streaming.
	emitter := stream.NewChannelEmitter()

	// CAPTCHA solving: chain solver (CapSolver → 2Captcha → HITL fallback).
	// Only enabled when captcha.enabled=true and at least one API key is configured.
	var captchaSolver domain.CaptchaSolver
	captchaDetector := captcha.NewDetector()
	captchaInjector := captcha.NewInjector()
	if cfg.Captcha.Enabled {
		var solvers []domain.CaptchaSolver
		if cfg.Captcha.Primary == "capsolver" && cfg.Captcha.CapSolverAPIKey != "" {
			solvers = append(solvers, captcha.NewCapSolverClient(cfg.Captcha.CapSolverAPIKey))
			slog.Info("captcha: CapSolver enabled")
		}
		if cfg.Captcha.Fallback == "2captcha" && cfg.Captcha.TwoCaptchaAPIKey != "" {
			solvers = append(solvers, captcha.NewTwoCaptchaClient(cfg.Captcha.TwoCaptchaAPIKey))
			slog.Info("captcha: 2Captcha fallback enabled")
		}
		if len(solvers) > 0 || cfg.Captcha.Fallback == "hitl" {
			captchaSolver = captcha.NewChainSolver(solvers, hitlMgr)
			slog.Info("captcha solver chain enabled",
				"primary", cfg.Captcha.Primary,
				"fallback", cfg.Captcha.Fallback,
			)
		} else {
			slog.Warn("captcha.enabled=true but no API keys configured — CAPTCHA detection will log warnings only")
		}
	}

	sessionMgr := session.NewDefaultSessionManager(session.Config{
		Pool:            pool,
		Planner:         p,
		Sequencer:       seq,
		AuthPersistence: authPersist,
		PolicyEngine:    policyEngine,
		Billing:         accountService,
		MaxPerAccount:   cfg.API.MaxSessionsPerAccount,
		CaptchaSolver:   captchaSolver,
		CaptchaDetector: captchaDetector,
		CaptchaInjector: captchaInjector,
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
		taskPlannerSvc = planner.NewStatefulTaskPlanner(
			llmClient, reg, checkpointDir,
			cfg.LLM.MaxStepsPerTask, cfg.LLM.MaxCallsPerSession,
		)
		slog.Info("stateful task planner enabled", "checkpoint_dir", checkpointDir)
	} else {
		slog.Warn("stateful task planner disabled: no LLM client configured")
	}

	// Checkpoint TTL cleaner: remove expired checkpoint files hourly.
	ttlHours := cfg.CheckpointTTLHours
	if ttlHours <= 0 {
		ttlHours = 24
	}
	checkpoint.StartCleaner(checkpointDir, time.Duration(ttlHours)*time.Hour, 1*time.Hour)
	slog.Info("checkpoint cleaner started", "ttl_hours", ttlHours, "dir", checkpointDir)

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
		Auth:                  buildAuthConfig(cfg),
		RateLimit:             api.RateLimitConfig{RequestsPerMinute: cfg.API.RateLimitRPM},
		CORSOrigins:           cfg.API.CORSOrigins,
		CORSRejectUnknown:     cfg.API.CORSRejectUnknown,
		MaxBodyBytes:          cfg.API.MaxBodyBytes,
		RequestTimeoutSeconds: cfg.API.RequestTimeoutSeconds,
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

// recoverInterruptedSessions marks any session that was "active" in the persistent
// store as "interrupted". These sessions were executing when the previous server
// process died (crash, OOM, SIGKILL) and their browser instances no longer exist.
// Callers can detect "interrupted" status and retry or surface the error to the user.
func recoverInterruptedSessions(s *store.SQLiteStore) {
	ctx := context.Background()
	sessions, err := s.ListSessions(ctx, "") // "" = all accounts
	if err != nil {
		slog.Warn("session recovery: could not list sessions", "error", err)
		return
	}
	recovered := 0
	for _, rec := range sessions {
		if rec.Status == "active" {
			if err := s.UpdateSessionStatus(ctx, rec.ID, "interrupted"); err != nil {
				slog.Warn("session recovery: failed to mark session interrupted",
					"session_id", rec.ID, "error", err)
			} else {
				recovered++
				slog.Info("session recovery: marked interrupted", "session_id", rec.ID, "goal", rec.Goal)
			}
		}
	}
	if recovered > 0 {
		slog.Warn("session recovery: interrupted sessions detected on startup",
			"count", recovered,
			"reason", "server restarted while sessions were active",
		)
	} else {
		slog.Info("session recovery: no interrupted sessions")
	}
}
