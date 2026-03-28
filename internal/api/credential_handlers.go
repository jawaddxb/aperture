// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file contains handlers for credential vault CRUD endpoints.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
)

// CredentialHandlers groups HTTP handlers for credential vault endpoints.
type CredentialHandlers struct {
	vault domain.CredentialVault
}

// NewCredentialHandlers constructs CredentialHandlers.
func NewCredentialHandlers(vault domain.CredentialVault) *CredentialHandlers {
	return &CredentialHandlers{vault: vault}
}

// storeCredentialRequest is the JSON body for PUT /api/v1/agents/{agent_id}/credentials/{domain}.
type storeCredentialRequest struct {
	Username  string            `json:"username"`
	Password  string            `json:"password"`
	TOTPSeed  string            `json:"totp_seed,omitempty"`
	AutoLogin bool              `json:"auto_login"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// List handles GET /api/v1/agents/{agent_id}/credentials.
func (h *CredentialHandlers) List(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	summaries, err := h.vault.List(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_CREDENTIALS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

// Store handles PUT /api/v1/agents/{agent_id}/credentials/{domain}.
func (h *CredentialHandlers) Store(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	domainName := chi.URLParam(r, "domain")

	var req storeCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	cred := domain.Credential{
		AgentID:   agentID,
		Domain:    domainName,
		Username:  req.Username,
		Password:  req.Password,
		TOTPSeed:  req.TOTPSeed,
		AutoLogin: req.AutoLogin,
		Metadata:  req.Metadata,
	}

	if err := h.vault.Store(r.Context(), cred); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_CREDENTIAL_FAILED", err.Error())
		return
	}

	// Response never includes password.
	writeJSON(w, http.StatusOK, domain.CredentialSummary{
		Domain:      domainName,
		Username:    req.Username,
		HasPassword: req.Password != "",
		HasTOTP:     req.TOTPSeed != "",
		AutoLogin:   req.AutoLogin,
		Metadata:    req.Metadata,
	})
}

// Delete handles DELETE /api/v1/agents/{agent_id}/credentials/{domain}.
func (h *CredentialHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	domainName := chi.URLParam(r, "domain")

	if err := h.vault.Delete(r.Context(), agentID, domainName); err != nil {
		writeError(w, http.StatusInternalServerError, "DELETE_CREDENTIAL_FAILED", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
