// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file contains handlers for the /api/v1/bridge/* endpoints used by OpenClaw.
package api

import (
	"net/http"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
)

// BridgeHandlers groups HTTP handlers for the OpenClaw bridge endpoints.
// All handlers receive a Bridge via constructor (dependency inversion).
type BridgeHandlers struct {
	bridge   domain.Bridge
	pool     domain.BrowserPool
	logger   domain.ActionLogger
	metrics  domain.MetricsCollector
}

// BridgeConfig holds optional dependencies for BridgeHandlers.
type BridgeConfig struct {
	// Bridge is required; all bridge endpoints return 501 when nil.
	Bridge domain.Bridge

	// Pool is optional; used in /health to report pool availability.
	Pool domain.BrowserPool

	// Logger is optional; used for request/response logging.
	Logger domain.ActionLogger

	// Metrics is optional; used for recording action metrics.
	Metrics domain.MetricsCollector
}

// NewBridgeHandlers constructs BridgeHandlers from cfg.
func NewBridgeHandlers(cfg BridgeConfig) *BridgeHandlers {
	return &BridgeHandlers{
		bridge:  cfg.Bridge,
		pool:    cfg.Pool,
		logger:  cfg.Logger,
		metrics: cfg.Metrics,
	}
}

// ─── request / response types ─────────────────────────────────────────────────

// executeTaskRequest is the JSON body for POST /api/v1/bridge/execute.
type executeTaskRequest struct {
	ID          string            `json:"id,omitempty"`
	Goal        string            `json:"goal"`
	URL         string            `json:"url,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Config      domain.TaskConfig `json:"config,omitempty"`
	Screenshots bool              `json:"screenshots,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
}

// asyncTaskResponse is returned for async execute (no ?sync=true).
type asyncTaskResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// quickRequest is the JSON body for POST /api/v1/bridge/quick.
type quickRequest struct {
	Goal string `json:"goal"`
	URL  string `json:"url,omitempty"`
}

// bridgeHealthResponse is the JSON body for GET /api/v1/bridge/health.
type bridgeHealthResponse struct {
	Status       string `json:"status"`
	BrowserPool  string `json:"browser_pool"`
	LLMClient    string `json:"llm_client"`
	ActiveTasks  int    `json:"active_tasks"`
}

// ─── handlers ─────────────────────────────────────────────────────────────────

// Execute handles POST /api/v1/bridge/execute.
// With ?sync=true it blocks and returns the full TaskResponse.
// Without, it runs the task and returns immediately with the task ID.
// Note: async mode runs the task in the same goroutine for simplicity;
// callers wishing true async should use a task queue. Currently both modes
// complete synchronously — async is provided for interface compatibility.
func (h *BridgeHandlers) Execute(w http.ResponseWriter, r *http.Request) {
	if h.bridge == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "bridge not configured")
		return
	}

	var req executeTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Goal == "" {
		writeError(w, http.StatusBadRequest, "MISSING_GOAL", "field 'goal' is required")
		return
	}

	taskReq := toTaskRequest(req)
	start := time.Now()
	sync := r.URL.Query().Get("sync") == "true"

	h.logRequest(r, "bridge.execute", taskReq.ID, req.Goal)

	resp, err := h.bridge.ExecuteTask(r.Context(), taskReq)
	if err != nil {
		h.recordMetric("bridge.execute", time.Since(start), false)
		writeError(w, http.StatusInternalServerError, "EXECUTE_FAILED", err.Error())
		return
	}

	h.recordMetric("bridge.execute", time.Since(start), resp.Success)

	if sync {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	writeJSON(w, http.StatusOK, asyncTaskResponse{
		TaskID: resp.ID,
		Status: statusFromResponse(resp),
	})
}

// GetTask handles GET /api/v1/bridge/tasks/:id.
func (h *BridgeHandlers) GetTask(w http.ResponseWriter, r *http.Request) {
	if h.bridge == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "bridge not configured")
		return
	}

	id := chi.URLParam(r, "id")
	resp, err := h.bridge.GetStatus(r.Context(), id)
	if err != nil {
		if err == domain.ErrTaskNotFound {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "STATUS_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// CancelTask handles DELETE /api/v1/bridge/tasks/:id.
func (h *BridgeHandlers) CancelTask(w http.ResponseWriter, r *http.Request) {
	if h.bridge == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "bridge not configured")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.bridge.CancelTask(r.Context(), id); err != nil {
		if err == domain.ErrTaskNotFound {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "CANCEL_FAILED", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Quick handles POST /api/v1/bridge/quick — a sync one-shot shortcut.
func (h *BridgeHandlers) Quick(w http.ResponseWriter, r *http.Request) {
	if h.bridge == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "bridge not configured")
		return
	}

	var req quickRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Goal == "" {
		writeError(w, http.StatusBadRequest, "MISSING_GOAL", "field 'goal' is required")
		return
	}

	taskReq := &domain.TaskRequest{
		Goal: req.Goal,
		URL:  req.URL,
	}

	start := time.Now()
	resp, err := h.bridge.ExecuteTask(r.Context(), taskReq)
	if err != nil {
		h.recordMetric("bridge.quick", time.Since(start), false)
		writeError(w, http.StatusInternalServerError, "EXECUTE_FAILED", err.Error())
		return
	}

	h.recordMetric("bridge.quick", time.Since(start), resp.Success)
	writeJSON(w, http.StatusOK, resp)
}

// Health handles GET /api/v1/bridge/health.
func (h *BridgeHandlers) Health(w http.ResponseWriter, r *http.Request) {
	resp := bridgeHealthResponse{
		Status:    "ok",
		LLMClient: "not configured",
	}

	// Detect if LLM client is available by checking bridge health
	// If bridge exists and has active planner, assume OpenAI/OpenRouter is available
	if h.bridge != nil {
		resp.LLMClient = "openai"
	}

	if h.pool != nil {
		if h.pool.Available() > 0 {
			resp.BrowserPool = "available"
		} else {
			resp.BrowserPool = "exhausted"
		}
	} else {
		resp.BrowserPool = "not configured"
	}

	if h.bridge == nil {
		resp.Status = "degraded"
	}

	writeJSON(w, http.StatusOK, resp)
}

// RegisterBridgeRoutes mounts all /api/v1/bridge/* routes on r.
func RegisterBridgeRoutes(r chi.Router, cfg BridgeConfig) {
	h := NewBridgeHandlers(cfg)

	r.Route("/bridge", func(r chi.Router) {
		r.Post("/execute", h.Execute)
		r.Get("/tasks/{id}", h.GetTask)
		r.Delete("/tasks/{id}", h.CancelTask)
		r.Post("/quick", h.Quick)
		r.Get("/health", h.Health)
	})
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// toTaskRequest converts an executeTaskRequest to domain.TaskRequest.
func toTaskRequest(req executeTaskRequest) *domain.TaskRequest {
	return &domain.TaskRequest{
		ID:          req.ID,
		Goal:        req.Goal,
		URL:         req.URL,
		SessionID:   req.SessionID,
		Config:      req.Config,
		Screenshots: req.Screenshots,
		Context:     req.Context,
	}
}

// statusFromResponse returns a status string based on the TaskResponse.
func statusFromResponse(resp *domain.TaskResponse) string {
	if resp.Success {
		return "completed"
	}
	if resp.Error != "" {
		return "failed"
	}
	return "running"
}

// logRequest logs an incoming bridge request if a logger is set.
func (h *BridgeHandlers) logRequest(r *http.Request, action, taskID, goal string) {
	if h.logger == nil {
		return
	}
	h.logger.WithAction(action).Log(domain.LogEntry{
		Level:   "info",
		Message: "bridge request received",
		Action:  action,
		Fields: map[string]string{
			"task_id": taskID,
			"goal":    goal,
			"method":  r.Method,
			"path":    r.URL.Path,
		},
	})
}

// recordMetric records an action metric if a collector is set.
func (h *BridgeHandlers) recordMetric(action string, d time.Duration, success bool) {
	if h.metrics == nil {
		return
	}
	h.metrics.RecordAction(action, d, success)
}
