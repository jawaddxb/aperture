package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ApertureHQ/aperture/internal/api"
	"github.com/ApertureHQ/aperture/internal/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAdminRouter creates a router with billing and returns a helper to make
// authenticated requests using the admin key from the first account created.
func setupAdminRouter(t *testing.T) (http.Handler, *billing.AccountService) {
	t.Helper()
	dir := t.TempDir()
	db, err := billing.InitDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	svc := billing.NewAccountService(db)
	router := api.NewRouter(api.RouterConfig{
		AccountService: svc,
	})
	return router, svc
}

// createAdminAccount creates the first account (dev mode, no auth needed) and
// returns the account ID and admin API key for subsequent authenticated requests.
func createAdminAccount(t *testing.T, router http.Handler) (acctID, apiKey string) {
	t.Helper()
	body := `{"name":"Admin","plan":"starter","initial_credits":10000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	acctID = resp["account"].(map[string]interface{})["id"].(string)
	apiKey = resp["api_key"].(string)
	return
}

func authedRequest(method, url string, body string, apiKey string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return req
}

func TestAdminCreateAccount(t *testing.T) {
	router, _ := setupAdminRouter(t)

	// First account (dev mode — no auth).
	body := `{"name":"Acme Corp","email":"admin@acme.com","plan":"starter","initial_credits":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotEmpty(t, resp["api_key"])
	assert.NotNil(t, resp["account"])
	assert.Equal(t, "Account created. Save this API key — it won't be shown again.", resp["message"])

	acct := resp["account"].(map[string]interface{})
	assert.Equal(t, "Acme Corp", acct["name"])
	assert.Equal(t, float64(5000), acct["credit_balance"])
}

func TestAdminListAccounts(t *testing.T) {
	router, _ := setupAdminRouter(t)
	_, adminKey := createAdminAccount(t, router)

	// Create a second account (now requires auth).
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedRequest(http.MethodPost, "/api/v1/admin/accounts",
		`{"name":"Beta","plan":"free","initial_credits":100}`, adminKey))
	require.Equal(t, http.StatusCreated, w.Code)

	// List accounts.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authedRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&per_page=10", "", adminKey))

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["total"])
	accounts := resp["accounts"].([]interface{})
	assert.Len(t, accounts, 2)
}

func TestAdminAddCredits(t *testing.T) {
	router, _ := setupAdminRouter(t)
	acctID, adminKey := createAdminAccount(t, router)

	// Add credits.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedRequest(http.MethodPost, "/api/v1/admin/accounts/"+acctID+"/credits",
		`{"amount":500,"reason":"bonus"}`, adminKey))

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(10000), resp["previous_balance"])
	assert.Equal(t, float64(10500), resp["new_balance"])
}

func TestAdminGetStats(t *testing.T) {
	router, _ := setupAdminRouter(t)
	_, adminKey := createAdminAccount(t, router)

	// Get stats.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedRequest(http.MethodGet, "/api/v1/admin/stats", "", adminKey))

	assert.Equal(t, http.StatusOK, w.Code)

	var stats map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	assert.Equal(t, float64(1), stats["total_accounts"])
}
