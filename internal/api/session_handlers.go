// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file contains handlers for session lifecycle endpoints.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ApertureHQ/aperture/internal/billing"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
)

// checkSessionOwnership verifies the caller owns the session.
// Returns nil if ownership is valid or billing is not active (dev mode).
// Returns an error string if access should be denied.
func checkSessionOwnership(ctx context.Context, session *domain.Session) bool {
	acct := billing.AccountFromContext(ctx)
	// Dev mode: no billing context → allow all.
	if acct == nil {
		return true
	}
	// Session has no account (created in dev mode) → allow all.
	if session.AccountID == "" {
		return true
	}
	return acct.ID == session.AccountID
}

// SessionHandlers groups HTTP handlers for session lifecycle endpoints.
// All handlers receive a SessionManager via constructor (dependency inversion).
type SessionHandlers struct {
	manager domain.SessionManager
}

// NewSessionHandlers constructs SessionHandlers.
func NewSessionHandlers(manager domain.SessionManager) *SessionHandlers {
	return &SessionHandlers{manager: manager}
}

// CreateSessionRequest is the JSON body for POST /api/v1/sessions.
type CreateSessionRequest struct {
	Goal    string                 `json:"goal"`
	AgentID string                 `json:"agent_id,omitempty"` // optional; used for xBPP policy lookup
	Config  map[string]interface{} `json:"config,omitempty"`
}

// CreateSessionResponse is the JSON body returned for a created session.
type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

// SessionDetailResponse is the JSON body returned for a single session.
type SessionDetailResponse struct {
	SessionID string               `json:"session_id"`
	Status    string               `json:"status"`
	Goal      string               `json:"goal"`
	BrowserID string               `json:"browser_id"`
	Results   []*domain.StepResult `json:"results,omitempty"`
}

// ExecuteSessionResponse is the JSON body returned after executing a session.
type ExecuteSessionResponse struct {
	Success           bool                `json:"success"`
	Steps             []domain.StepResult `json:"steps"`
	DurationMS        int64               `json:"duration_ms"`
	TotalCost         int                 `json:"total_cost"`
	CreditsRemaining  int                 `json:"credits_remaining,omitempty"`
}

// Create handles POST /api/v1/sessions.
func (h *SessionHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Goal == "" {
		writeError(w, http.StatusBadRequest, "MISSING_GOAL", "field 'goal' is required")
		return
	}

	meta := map[string]string{}
	if req.AgentID != "" {
		meta["agent_id"] = req.AgentID
	}
	s, err := h.manager.Create(r.Context(), req.Goal, meta)
	if err != nil {
		if errors.Is(err, domain.ErrConcurrentLimitExceeded) {
			writeError(w, http.StatusTooManyRequests, "LIMIT_EXCEEDED", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "CREATE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, CreateSessionResponse{
		SessionID: s.ID,
		Status:    s.Status,
	})
}

// List handles GET /api/v1/sessions.
func (h *SessionHandlers) List(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.manager.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}

	// Filter sessions by account ownership.
	acct := billing.AccountFromContext(r.Context())
	out := make([]SessionDetailResponse, 0, len(sessions))
	for _, s := range sessions {
		// Dev mode (no billing): show all.
		// Billing active: only show sessions owned by this account.
		if acct == nil || s.AccountID == "" || s.AccountID == acct.ID {
			out = append(out, sessionToDetail(s))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// GetByID handles GET /api/v1/sessions/:id.
func (h *SessionHandlers) GetByID(w http.ResponseWriter, r *http.Request) {
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

	if !checkSessionOwnership(r.Context(), s) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	writeJSON(w, http.StatusOK, sessionToDetail(s))
}

// Execute handles POST /api/v1/sessions/:id/execute.
func (h *SessionHandlers) Execute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Ownership check: verify caller owns this session.
	s, err := h.manager.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "GET_FAILED", err.Error())
		return
	}
	if !checkSessionOwnership(r.Context(), s) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	// Hard timeout: 60s for the entire execution (plan + all steps).
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	result, err := h.manager.Execute(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "EXECUTE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ExecuteSessionResponse{
		Success:          result.Success,
		Steps:            result.Steps,
		DurationMS:       result.Duration.Milliseconds(),
		TotalCost:        result.TotalCost,
		CreditsRemaining: result.CreditsRemaining,
	})
}

// Delete handles DELETE /api/v1/sessions/:id.
func (h *SessionHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Ownership check.
	s, err := h.manager.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "GET_FAILED", err.Error())
		return
	}
	if !checkSessionOwnership(r.Context(), s) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	if err := h.manager.Delete(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// sessionToDetail maps a domain.Session to a SessionDetailResponse.
func sessionToDetail(s *domain.Session) SessionDetailResponse {
	return SessionDetailResponse{
		SessionID: s.ID,
		Status:    s.Status,
		Goal:      s.Goal,
		BrowserID: s.BrowserID,
		Results:   s.Results,
	}
}

// decodeJSON decodes the request body into v.
func decodeJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ErrorResponse is the standard JSON error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// writeError writes a standardised JSON error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: message, Code: code})
}
