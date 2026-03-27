// Package api provides HTTP handlers and middleware for Aperture.
// This file provides handlers for human-in-the-loop (HITL) interventions.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
)

// HITLHandlers manages HITL intervention endpoints.
type HITLHandlers struct {
	hitl domain.HITLManager
}

// NewHITLHandlers constructs HITLHandlers.
func NewHITLHandlers(hitl domain.HITLManager) *HITLHandlers {
	return &HITLHandlers{hitl: hitl}
}

// resolveRequest is the JSON body for POST /api/v1/hitl/{id}/resolve.
type resolveRequest struct {
	Success bool   `json:"success"` // Whether the human completed the task
	Data    string `json:"data"`    // Optional data (e.g., CAPTCHA solution)
}

// Resolve handles POST /api/v1/hitl/{id}/resolve.
func (h *HITLHandlers) Resolve(w http.ResponseWriter, r *http.Request) {
	if h.hitl == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "HITL not configured")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "MISSING_ID", "intervention ID required")
		return
	}

	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	resp := &domain.InterventionResponse{
		ID:      id,
		Success: req.Success,
		Data:    req.Data,
	}

	if err := h.hitl.ResolveIntervention(r.Context(), id, resp); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// Cancel handles DELETE /api/v1/hitl/{id}.
func (h *HITLHandlers) Cancel(w http.ResponseWriter, r *http.Request) {
	if h.hitl == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "HITL not configured")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.hitl.CancelIntervention(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
