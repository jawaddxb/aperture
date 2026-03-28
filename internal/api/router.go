// Package api provides HTTP routing and handler wiring for the Aperture server.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/billing"
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

	// VisionAnalyzer, when set, enables POST /api/v1/actions/vision.
	VisionAnalyzer domain.VisionAnalyzer

	// HITLManager, when set, enables HITL intervention endpoints.
	HITLManager domain.HITLManager

	// PolicyEngine, when set, enables xBPP policy CRUD endpoints.
	PolicyEngine domain.PolicyEngine

	// ProfileManager, when set, enables site profile listing endpoint.
	ProfileManager domain.SiteProfileManager

	// CredentialVault, when set, enables credential CRUD endpoints.
	CredentialVault domain.CredentialVault

	// AgentStateStore, when set, enables agent memory KV endpoints.
	AgentStateStore domain.AgentStateStore

	// AccountService, when set, enables billing-aware auth and admin endpoints.
	AccountService *billing.AccountService

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
		// Use billing-aware auth when AccountService is available, else legacy key map.
		if cfg.AccountService != nil {
			r.Use(billing.AuthMiddleware(cfg.AccountService, []string{"/health", "/website"}))
		} else {
			r.Use(APIKeyAuth(cfg.Auth))
		}
		sh := NewSessionHandlers(cfg.SessionManager)
		r.Post("/sessions", sh.Create)
		r.Get("/sessions", sh.List)
		r.Get("/sessions/{id}", sh.GetByID)
		r.Get("/sessions/{id}/snapshot", sh.Snapshot)
		r.Post("/sessions/{id}/execute", sh.Execute)
		r.Delete("/sessions/{id}", sh.Delete)

		ah := NewActionHandlers(cfg.SessionManager, cfg.ScreenshotService)
		r.Post("/actions/execute", ah.ExecuteAction)
		r.Post("/actions/screenshot", ah.Screenshot)

		if cfg.VisionAnalyzer != nil {
			vh := NewVisionHandlers(cfg.ScreenshotService, cfg.VisionAnalyzer)
			r.Post("/actions/vision", vh.Analyze)
		}

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

		if cfg.PolicyEngine != nil {
			ph := NewPolicyHandlers(cfg.PolicyEngine)
			r.Get("/policies/{agent_id}", ph.Get)
			r.Put("/policies/{agent_id}", ph.Upsert)
			r.Delete("/policies/{agent_id}", ph.Delete)
		}

		if cfg.ProfileManager != nil {
			prh := NewProfileHandlers(cfg.ProfileManager)
			r.Get("/profiles", prh.List)
		}

		if cfg.CredentialVault != nil {
			ch := NewCredentialHandlers(cfg.CredentialVault)
			r.Get("/agents/{agent_id}/credentials", ch.List)
			r.Put("/agents/{agent_id}/credentials/{domain}", ch.Store)
			r.Delete("/agents/{agent_id}/credentials/{domain}", ch.Delete)
		}

		if cfg.AgentStateStore != nil {
			mh := NewMemoryHandlers(cfg.AgentStateStore)
			r.Get("/agents/{agent_id}/memory", mh.List)
			r.Put("/agents/{agent_id}/memory/{key}", mh.SetKey)
			r.Get("/agents/{agent_id}/memory/{key}", mh.GetKey)
			r.Delete("/agents/{agent_id}/memory/{key}", mh.DeleteKey)
		}

		RegisterBridgeRoutes(r, BridgeConfig{
			Bridge:  cfg.Bridge,
			Pool:    cfg.BrowserPool,
			Logger:  cfg.Logger,
			Metrics: cfg.MetricsCollector,
		})

		// Admin routes (requires is_admin on API key).
		if cfg.AccountService != nil {
			ah := NewAdminHandlers(cfg.AccountService)
			r.Route("/admin", func(r chi.Router) {
				r.Use(billing.AdminOnly(cfg.AccountService))
				r.Post("/accounts", ah.CreateAccount)
				r.Get("/accounts", ah.ListAccounts)
				r.Get("/accounts/{id}", ah.GetAccount)
				r.Post("/accounts/{id}/credits", ah.AddCredits)
				r.Get("/accounts/{id}/usage", ah.GetUsage)
				r.Post("/accounts/{id}/keys", ah.CreateAPIKey)
				r.Delete("/accounts/{id}/keys/{key}", ah.RevokeAPIKey)
				r.Get("/stats", ah.GetStats)
			})
		}
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
