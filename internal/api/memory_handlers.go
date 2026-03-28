// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file implements agent state KV CRUD endpoints.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
)

// MemoryHandlers groups HTTP handlers for agent state KV endpoints.
type MemoryHandlers struct {
	store domain.AgentStateStore
}

// NewMemoryHandlers constructs MemoryHandlers.
func NewMemoryHandlers(store domain.AgentStateStore) *MemoryHandlers {
	return &MemoryHandlers{store: store}
}

// setMemoryRequest is the JSON body for PUT /agents/{agent_id}/memory/{key}.
type setMemoryRequest struct {
	Value interface{} `json:"value"`
}

// List handles GET /api/v1/agents/{agent_id}/memory.
// Optional query param: ?prefix=X
func (h *MemoryHandlers) List(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	prefix := r.URL.Query().Get("prefix")

	entries, err := h.store.List(r.Context(), agentID, prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// GetKey handles GET /api/v1/agents/{agent_id}/memory/{key}.
func (h *MemoryHandlers) GetKey(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	key := chi.URLParam(r, "key")

	entry, err := h.store.Get(r.Context(), agentID, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GET_FAILED", err.Error())
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "key not found")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// SetKey handles PUT /api/v1/agents/{agent_id}/memory/{key}.
func (h *MemoryHandlers) SetKey(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	key := chi.URLParam(r, "key")

	var req setMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := h.store.Set(r.Context(), agentID, key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "SET_FAILED", err.Error())
		return
	}

	// Return the stored entry.
	entry := &domain.MemoryEntry{
		Key:       key,
		Value:     req.Value,
		UpdatedAt: time.Now().UTC(),
	}
	writeJSON(w, http.StatusOK, entry)
}

// DeleteKey handles DELETE /api/v1/agents/{agent_id}/memory/{key}.
func (h *MemoryHandlers) DeleteKey(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	key := chi.URLParam(r, "key")

	if err := h.store.Delete(r.Context(), agentID, key); err != nil {
		writeError(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
