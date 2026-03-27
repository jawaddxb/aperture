// Package api provides HTTP routing and handler wiring for the Aperture server.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// HealthResponse is the JSON body returned by the health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// RouterConfig holds optional dependencies for registering API routes.
// Fields are optional; omitting them means the corresponding routes return 501.
type RouterConfig struct {
	// SessionManager, when set, enables all /api/v1/sessions endpoints.
	SessionManager domain.SessionManager

	// ScreenshotService, when set, enables POST /api/v1/actions/screenshot.
	ScreenshotService domain.ScreenshotService

	// ProgressEmitter, when set, enables GET /api/v1/sessions/:id/stream.
	ProgressEmitter domain.ProgressEmitter

	// MetricsCollector, when set, enables GET /api/v1/metrics.
	MetricsCollector domain.MetricsCollector

	// Bridge, when set, enables all /api/v1/bridge/* endpoints.
	Bridge domain.Bridge

	// BrowserPool, when set, is reported in /api/v1/bridge/health.
	BrowserPool domain.BrowserPool

	// Logger, when set, is used for bridge request/response logging.
	Logger domain.ActionLogger

	// HITLManager, when set, enables HITL intervention endpoints.
	HITLManager domain.HITLManager

	// Auth configures API key authentication. Empty Keys = dev mode (no auth).
	Auth AuthConfig

	// RateLimit configures per-IP rate limiting. 0 RPM = unlimited.
	RateLimit RateLimitConfig

	// CORSOrigins is a list of allowed CORS origins. Empty = allow all.
	CORSOrigins []string
}

// NewRouter constructs and returns the root chi router with all routes registered.
// Pass a non-nil RouterConfig to enable the full API surface.
func NewRouter(cfgs ...RouterConfig) http.Handler {
	var cfg RouterConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(CORSMiddleware(cfg.CORSOrigins))
	r.Use(RateLimit(cfg.RateLimit))

	// Health and website are always public.
	r.Get("/health", HealthHandler)

	registerV1Routes(r, cfg)

	// Serve the Aperture website from the static directory.
	fs := http.FileServer(http.Dir("internal/api/static"))
	r.Handle("/website/*", http.StripPrefix("/website", fs))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/website/", http.StatusMovedPermanently)
	})

	return r
}

// registerV1Routes mounts all /api/v1/* routes with optional API key auth.
func registerV1Routes(r chi.Router, cfg RouterConfig) {
	r.Route("/api/v1", func(r chi.Router) {
		// Apply API key auth to all API routes (skips health/website).
		r.Use(APIKeyAuth(cfg.Auth))
		sh := NewSessionHandlers(cfg.SessionManager)
		r.Post("/sessions", sh.Create)
		r.Get("/sessions", sh.List)
		r.Get("/sessions/{id}", sh.GetByID)
		r.Post("/sessions/{id}/execute", sh.Execute)
		r.Delete("/sessions/{id}", sh.Delete)

		ah := NewActionHandlers(cfg.SessionManager, cfg.ScreenshotService)
		r.Post("/actions/execute", ah.ExecuteAction)
		r.Post("/actions/screenshot", ah.Screenshot)

		if cfg.ProgressEmitter != nil {
			wsh := NewStreamHandler(cfg.ProgressEmitter)
			r.Get("/sessions/{id}/stream", wsh.Stream)
		}

		if cfg.MetricsCollector != nil {
			mh := NewMetricsHandler(cfg.MetricsCollector)
			r.Get("/metrics", mh.GetMetrics)
		}

		if cfg.HITLManager != nil {
			hh := NewHITLHandlers(cfg.HITLManager)
			r.Post("/hitl/{id}/resolve", hh.Resolve)
			r.Delete("/hitl/{id}", hh.Cancel)
		}

		RegisterBridgeRoutes(r, BridgeConfig{
			Bridge:  cfg.Bridge,
			Pool:    cfg.BrowserPool,
			Logger:  cfg.Logger,
			Metrics: cfg.MetricsCollector,
		})
	})
}

// HealthHandler handles GET /health and returns {"status":"ok"}.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(HealthResponse{Status: "ok"}); err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
	}
}
