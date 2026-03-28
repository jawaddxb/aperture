package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ApertureHQ/aperture/internal/billing"
	"github.com/go-chi/chi/v5"
)

// AdminHandlers exposes HTTP handlers for the admin API.
type AdminHandlers struct {
	svc *billing.AccountService
}

// NewAdminHandlers creates admin handlers backed by the given AccountService.
func NewAdminHandlers(svc *billing.AccountService) *AdminHandlers {
	return &AdminHandlers{svc: svc}
}

// CreateAccount handles POST /admin/accounts.
func (h *AdminHandlers) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		Email          string `json:"email"`
		Plan           string `json:"plan"`
		InitialCredits int    `json:"initial_credits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "name is required")
		return
	}
	if req.Plan == "" {
		req.Plan = "free"
	}

	acct, key, err := h.svc.CreateAccount(req.Name, req.Email, req.Plan, req.InitialCredits)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"account": acct,
		"api_key": key,
		"message": "Account created. Save this API key — it won't be shown again.",
	})
}

// ListAccounts handles GET /admin/accounts.
func (h *AdminHandlers) ListAccounts(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	accounts, total, err := h.svc.ListAccounts(offset, perPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"accounts": accounts,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// GetAccount handles GET /admin/accounts/{id}.
func (h *AdminHandlers) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acct, err := h.svc.GetAccount(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	keys, _ := h.svc.GetAPIKeys(id)
	maskedKeys := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		maskedKeys = append(maskedKeys, map[string]interface{}{
			"key":        maskKey(k.Key),
			"label":      k.Label,
			"is_admin":   k.IsAdmin,
			"created_at": k.CreatedAt,
			"revoked_at": k.RevokedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"account":        acct,
		"credit_balance": acct.CreditBalance,
		"api_keys":       maskedKeys,
	})
}

// AddCredits handles POST /admin/accounts/{id}/credits.
func (h *AdminHandlers) AddCredits(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Amount int    `json:"amount"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "amount must be positive")
		return
	}
	if req.Reason == "" {
		req.Reason = "manual top-up"
	}

	// Get previous balance for the response.
	acct, err := h.svc.GetAccount(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	prevBalance := acct.CreditBalance

	newBalance, err := h.svc.AddCredits(id, req.Amount, req.Reason)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"previous_balance": prevBalance,
		"new_balance":      newBalance,
	})
}

// GetUsage handles GET /admin/accounts/{id}/usage.
func (h *AdminHandlers) GetUsage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		from = "2000-01-01"
	}
	if to == "" {
		to = "2099-12-31"
	}

	usage, err := h.svc.GetUsage(id, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	totalCost := 0
	for _, u := range usage {
		totalCost += u.TotalCost
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"usage":      usage,
		"total_cost": totalCost,
	})
}

// CreateAPIKey handles POST /admin/accounts/{id}/keys.
func (h *AdminHandlers) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Label   string `json:"label"`
		IsAdmin bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	// Verify account exists.
	if _, err := h.svc.GetAccount(id); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	key, err := h.svc.CreateAPIKey(id, req.Label, req.IsAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"key":     key,
		"label":   req.Label,
		"message": "Save this API key — it won't be shown again.",
	})
}

// RevokeAPIKey handles DELETE /admin/accounts/{id}/keys/{key}.
func (h *AdminHandlers) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := h.svc.RevokeAPIKey(key); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetStats handles GET /admin/stats.
func (h *AdminHandlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// maskKey returns a partially masked API key: first 8 + "..." + last 4 chars.
func maskKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:8] + "..." + key[len(key)-4:]
}
