package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ApertureHQ/aperture/internal/api"
)

// TestHealthEndpoint verifies GET /health returns 200 with body containing "ok".
func TestHealthEndpoint(t *testing.T) {
	router := api.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "ok") {
		t.Fatalf("expected body to contain \"ok\", got: %s", body)
	}
}
