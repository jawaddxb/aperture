package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ApertureHQ/aperture/internal/api"
)

// TestActionsExecute_ReturnsRunResult verifies POST /api/v1/actions/execute.
func TestActionsExecute_ReturnsRunResult(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	rec := postJSON(router, "/api/v1/actions/execute", map[string]interface{}{
		"goal": "click the submit button",
		"url":  "https://example.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
}

// TestActionsExecute_MissingGoalReturns400 verifies missing goal returns 400.
func TestActionsExecute_MissingGoalReturns400(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	rec := postJSON(router, "/api/v1/actions/execute", map[string]interface{}{
		"url": "https://example.com",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestErrorFormat verifies the error response format is consistent.
func TestErrorFormat(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	rec := getReq(router, "/api/v1/sessions/nonexistent")

	var resp api.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
	if resp.Code == "" {
		t.Error("expected non-empty error code")
	}
}
