// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file contains handlers for xBPP policy CRUD endpoints.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
)

// PolicyHandlers groups HTTP handlers for policy CRUD endpoints.
type PolicyHandlers struct {
	engine domain.PolicyEngine
}

// NewPolicyHandlers constructs PolicyHandlers.
func NewPolicyHandlers(engine domain.PolicyEngine) *PolicyHandlers {
	return &PolicyHandlers{engine: engine}
}

// Get handles GET /api/v1/policies/{agent_id}.
func (h *PolicyHandlers) Get(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	pol := h.engine.GetPolicy(agentID)
	if pol == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"agent_id": agentID,
			"policy":   nil,
			"message":  "no policy set, agent operates in open/dev mode",
		})
		return
	}
	writeJSON(w, http.StatusOK, pol)
}

// Upsert handles PUT /api/v1/policies/{agent_id}.
func (h *PolicyHandlers) Upsert(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")

	var pol domain.AgentPolicy
	if err := json.NewDecoder(r.Body).Decode(&pol); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := h.engine.SetPolicy(agentID, pol); err != nil {
		writeError(w, http.StatusInternalServerError, "SET_POLICY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"agent_id": agentID,
	})
}

// Delete handles DELETE /api/v1/policies/{agent_id}.
func (h *PolicyHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	if err := h.engine.DeletePolicy(agentID); err != nil {
		writeError(w, http.StatusInternalServerError, "DELETE_POLICY_FAILED", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
