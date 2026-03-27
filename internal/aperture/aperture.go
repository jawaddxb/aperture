// Package aperture provides the top-level Aperture orchestrator.
// New() wires the full dependency graph from config and returns a ready-to-start
// instance. Callers should defer Shutdown() for graceful cleanup.
package aperture

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ApertureHQ/aperture/internal/api"
	"github.com/ApertureHQ/aperture/internal/bridge"
	browserpool "github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/config"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
	"github.com/ApertureHQ/aperture/internal/llm"
	"github.com/ApertureHQ/aperture/internal/observe"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/ApertureHQ/aperture/internal/recovery"
	"github.com/ApertureHQ/aperture/internal/resolver"
	"github.com/ApertureHQ/aperture/internal/sequencer"
	"github.com/ApertureHQ/aperture/internal/session"
	"github.com/ApertureHQ/aperture/internal/vision"
)

const (
	// shutdownTimeout is the max time allowed for graceful drain on SIGTERM.
	shutdownTimeout = 30 * time.Second

	// serverReadTimeout is the HTTP server read timeout.
	serverReadTimeout = 15 * time.Second

	// serverWriteTimeout is the HTTP server write timeout.
	serverWriteTimeout = 60 * time.Second
)

// Aperture is the top-level orchestrator that owns all long-lived components.
type Aperture struct {
	Config    *config.Config
	Pool      domain.BrowserPool
	Planner   domain.Planner
	Sequencer domain.Sequencer
	Bridge    domain.Bridge
	Logger    domain.ActionLogger
	Metrics   domain.MetricsCollector
	Router    http.Handler

	server *http.Server
}

// New creates a fully wired Aperture instance from cfg.
// Returns an error if any required component fails to initialise.
// When cfg.LLM.APIKey is empty, a StaticPlanner is used (no LLM calls).
func New(cfg *config.Config) (*Aperture, error) {
	logger := buildLogger()
	metrics := observe.NewInMemoryMetrics()

	pool, err := buildPool(cfg)
	if err != nil {
		return nil, fmt.Errorf("aperture: browser pool: %w", err)
	}

	p := buildPlanner(cfg, logger)
	seq := buildSequencer(metrics)
	mgr := buildSessionManager(pool, p, seq)
	br := buildBridge(mgr, cfg)
	router := buildRouter(cfg, mgr, br, pool, logger, metrics)

	return &Aperture{
		Config:    cfg,
		Pool:      pool,
		Planner:   p,
		Sequencer: seq,
		Bridge:    br,
		Logger:    logger,
		Metrics:   metrics,
		Router:    router,
	}, nil
}

// Start runs the HTTP server and blocks until ctx is cancelled or a fatal error
// occurs. It also listens for SIGINT/SIGTERM and triggers graceful shutdown.
func (a *Aperture) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", a.Config.Server.Host, a.Config.Server.Port)

	a.server = &http.Server{
		Addr:         addr,
		Handler:      a.Router,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	srvErr := make(chan error, 1)
	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
	}()

	select {
	case <-ctx.Done():
	case sig := <-stopCh:
		_ = sig
	case err := <-srvErr:
		return fmt.Errorf("aperture: server: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return a.Shutdown(shutdownCtx)
}

// Shutdown gracefully stops the HTTP server and closes the browser pool.
// The caller should pass a context with an appropriate deadline.
func (a *Aperture) Shutdown(ctx context.Context) error {
	var srvErr error
	if a.server != nil {
		srvErr = a.server.Shutdown(ctx)
	}

	poolErr := a.Pool.Close()

	if srvErr != nil {
		return fmt.Errorf("aperture shutdown: server: %w", srvErr)
	}
	if poolErr != nil {
		return fmt.Errorf("aperture shutdown: pool: %w", poolErr)
	}
	return nil
}

// ─── builder helpers ──────────────────────────────────────────────────────────

// buildLogger constructs a JSON logger writing to stderr.
func buildLogger() domain.ActionLogger {
	return observe.NewJSONLogger(os.Stderr)
}

// buildPool creates the browser pool; pre-warming is skipped when cfg.Browser.SkipPreWarm is true.
func buildPool(cfg *config.Config) (domain.BrowserPool, error) {
	return browserpool.NewPool(browserpool.Config{
		PoolSize:     cfg.Browser.PoolSize,
		ChromiumPath: cfg.Browser.ChromiumPath,
		SkipPreWarm:  cfg.Browser.SkipPreWarm,
	})
}

// buildPlanner returns an LLM-backed planner when APIKey is set, otherwise
// a StaticPlanner. This satisfies the "missing API key still works" requirement.
func buildPlanner(cfg *config.Config, _ domain.ActionLogger) domain.Planner {
	if cfg.LLM.APIKey == "" {
		return planner.NewStaticPlanner()
	}

	client, err := llm.NewClient(llm.Config{
		Provider:  cfg.LLM.Provider,
		Model:     cfg.LLM.Model,
		APIKey:    cfg.LLM.APIKey,
		BaseURL:   cfg.LLM.BaseURL,
	})
	if err != nil {
		// Fall back gracefully.
		return planner.NewStaticPlanner()
	}

	visionAnalyzer := vision.NewLLMVisionAnalyzer(wrapAsVision(client))
	return planner.NewLLMPlannerWithVision(client, visionAnalyzer)
}

// buildSequencer wires executors, recovery, and metrics into a DefaultSequencer.
func buildSequencer(metrics domain.MetricsCollector) domain.Sequencer {
	res := resolver.NewUnifiedResolver()
	registry := buildExecutorRegistry(res)
	rec := recovery.NewDefaultRecovery()

	return sequencer.NewDefaultSequencer(sequencer.Config{
		Registry: registry,
		Recovery: rec,
		Progress: func(sr domain.StepResult) {
			success := sr.Result != nil && sr.Result.Success
			metrics.RecordAction(sr.Step.Action, sr.Duration, success)
		},
	})
}

// buildExecutorRegistry maps action names to their executors.
func buildExecutorRegistry(res domain.UnifiedResolver) map[string]domain.Executor {
	return map[string]domain.Executor{
		"navigate":   executor.NewNavigateExecutor(),
		"click":      executor.NewClickExecutor(res),
		"type":       executor.NewTypeExecutor(res),
		"screenshot": executor.NewScreenshotExecutor(),
		"scroll":     executor.NewScrollExecutor(),
		"wait":       executor.NewWaitExecutor(),
	}
}

// buildSessionManager wires pool, planner, and sequencer into a session manager.
func buildSessionManager(
	pool domain.BrowserPool,
	p domain.Planner,
	seq domain.Sequencer,
) domain.SessionManager {
	return session.NewDefaultSessionManager(session.Config{
		Pool:      pool,
		Planner:   p,
		Sequencer: seq,
	})
}

// buildBridge wires the session manager into the OpenClaw bridge.
func buildBridge(mgr domain.SessionManager, cfg *config.Config) domain.Bridge {
	return bridge.NewOpenClawBridge(bridge.Config{
		SessionManager: mgr,
		MaxConcurrent:  cfg.Bridge.MaxConcurrentTasks,
	})
}

// buildRouter creates the chi router with all routes registered.
func buildRouter(
	_ *config.Config,
	mgr domain.SessionManager,
	br domain.Bridge,
	pool domain.BrowserPool,
	logger domain.ActionLogger,
	metrics domain.MetricsCollector,
) http.Handler {
	return api.NewRouter(api.RouterConfig{
		SessionManager:   mgr,
		MetricsCollector: metrics,
		Bridge:           br,
		BrowserPool:      pool,
		Logger:           logger,
	})
}

// ─── interface adapters ───────────────────────────────────────────────────────

// visionAdapter adapts an LLMClient to VisionLLMClient by delegating image
// calls to the text Complete method (for providers that don't support vision).
// Real vision-capable clients (OpenAI, Anthropic) already implement the full
// interface when returned by llm.NewClient.
type visionAdapter struct {
	domain.LLMClient
}

// CompleteWithImage falls back to text-only completion, discarding the image.
// This enables graceful degradation when the configured model lacks vision.
func (v *visionAdapter) CompleteWithImage(ctx context.Context, prompt, _ string, _ string) (string, error) {
	return v.Complete(ctx, prompt)
}

// wrapAsVision ensures an LLMClient satisfies VisionLLMClient.
// If client already implements VisionLLMClient it is returned as-is.
func wrapAsVision(client domain.LLMClient) domain.VisionLLMClient {
	if vc, ok := client.(domain.VisionLLMClient); ok {
		return vc
	}
	return &visionAdapter{client}
}
