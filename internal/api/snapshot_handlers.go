// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file implements GET /api/v1/sessions/{id}/snapshot (Spec §5.4).
package api

import (
	"errors"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
)

// SnapshotResponse is the JSON body returned by GET /sessions/{id}/snapshot.
type SnapshotResponse struct {
	SessionID        string                 `json:"session_id"`
	Status           string                 `json:"status"`
	URL              string                 `json:"url"`
	Title            string                 `json:"title"`
	ProfileMatched   string                 `json:"profile_matched"`
	StructuredData   map[string]interface{} `json:"structured_data"`
	AvailableActions []string               `json:"available_actions"`
}

// Snapshot handles GET /api/v1/sessions/{id}/snapshot.
// Returns the current semantic page state of a session.
func (h *SessionHandlers) Snapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s, err := h.manager.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "GET_FAILED", err.Error())
		return
	}

	resp := SnapshotResponse{
		SessionID:        s.ID,
		Status:           s.Status,
		URL:              s.CurrentURL,
		Title:            s.CurrentTitle,
		StructuredData:   make(map[string]interface{}),
		AvailableActions: []string{},
	}

	// Extract profile and structured data from the most recent result.
	if s.Plan != nil && len(s.Results) > 0 {
		last := s.Results[len(s.Results)-1]
		if last.Result != nil && last.Result.PageState != nil {
			ps := last.Result.PageState
			resp.ProfileMatched = ps.ProfileMatched
			if ps.StructuredData != nil {
				resp.StructuredData = ps.StructuredData
			}
			if ps.AvailableActions != nil {
				resp.AvailableActions = ps.AvailableActions
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
