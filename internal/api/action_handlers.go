// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file contains handlers for one-shot action endpoints.
package api

import (
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ActionHandlers groups HTTP handlers for one-shot action endpoints.
type ActionHandlers struct {
	manager    domain.SessionManager
	screenshot domain.ScreenshotService
}

// NewActionHandlers constructs ActionHandlers.
func NewActionHandlers(manager domain.SessionManager, screenshot domain.ScreenshotService) *ActionHandlers {
	return &ActionHandlers{manager: manager, screenshot: screenshot}
}

// ExecuteActionRequest is the JSON body for POST /api/v1/actions/execute.
type ExecuteActionRequest struct {
	Goal string `json:"goal"`
	URL  string `json:"url,omitempty"`
}

// ScreenshotRequest is the JSON body for POST /api/v1/actions/screenshot.
type ScreenshotRequest struct {
	URL      string `json:"url"`
	FullPage bool   `json:"fullPage,omitempty"`
}

// ExecuteAction handles POST /api/v1/actions/execute.
// Creates a session, executes it, deletes it, and returns the RunResult.
func (h *ActionHandlers) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	var req ExecuteActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Goal == "" {
		writeError(w, http.StatusBadRequest, "MISSING_GOAL", "field 'goal' is required")
		return
	}

	ctx := r.Context()

	s, err := h.manager.Create(ctx, req.Goal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CREATE_FAILED", err.Error())
		return
	}

	result, execErr := h.manager.Execute(ctx, s.ID)

	// Best-effort cleanup: always delete regardless of execution result.
	_ = h.manager.Delete(ctx, s.ID)

	if execErr != nil {
		writeError(w, http.StatusInternalServerError, "EXECUTE_FAILED", execErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// Screenshot handles POST /api/v1/actions/screenshot.
// Navigates to the provided URL and returns PNG bytes.
func (h *ActionHandlers) Screenshot(w http.ResponseWriter, r *http.Request) {
	var req ScreenshotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "MISSING_URL", "field 'url' is required")
		return
	}

	if h.screenshot == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "screenshot service not configured")
		return
	}

	buf, err := h.screenshot.Screenshot(r.Context(), req.URL, req.FullPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SCREENSHOT_FAILED", err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf)
}
