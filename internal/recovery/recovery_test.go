package recovery_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/recovery"
)

// stepWith creates a Step with optional initial params.
func stepWith(action string, params map[string]interface{}) domain.Step {
	if params == nil {
		params = map[string]interface{}{}
	}
	return domain.Step{Action: action, Params: params}
}

// ─── element not found ────────────────────────────────────────────────────────

func TestDefaultRecovery_ElementNotFound_RetriesOnce(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("click", nil)
	err := errors.New("element not found: submit button")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action.Strategy != "retry" {
		t.Errorf("expected retry, got %q", action.Strategy)
	}
}

func TestDefaultRecovery_ElementNotFound_NoMatchSentinel(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("click", nil)
	err := errors.New("resolver: no match found")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action.Strategy != "retry" {
		t.Errorf("expected retry, got %q", action.Strategy)
	}
}

// ─── timeout ──────────────────────────────────────────────────────────────────

func TestDefaultRecovery_Timeout_DoublesTimeout(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("navigate", map[string]interface{}{"url": "https://example.com", "timeout": 10.0})
	err := errors.New("context deadline exceeded (timeout)")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action.Strategy != "retry" {
		t.Errorf("expected retry, got %q", action.Strategy)
	}
	if len(action.NewSteps) == 0 {
		t.Fatal("expected replacement step in NewSteps")
	}
	newTimeout := action.NewSteps[0].Params["timeout"]
	if newTimeout != 20.0 {
		t.Errorf("expected doubled timeout=20, got %v", newTimeout)
	}
}

func TestDefaultRecovery_Timeout_CapsAtMax(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("navigate", map[string]interface{}{"timeout": 40.0})
	err := errors.New("timed out waiting for element")

	action, _ := r.Recover(context.Background(), step, err, nil)
	if len(action.NewSteps) == 0 {
		t.Fatal("expected replacement step")
	}
	timeout := action.NewSteps[0].Params["timeout"].(float64)
	if timeout > 60.0 {
		t.Errorf("expected timeout capped at 60, got %v", timeout)
	}
}

// ─── max retries ──────────────────────────────────────────────────────────────

func TestDefaultRecovery_MaxAttemptsExceeded_Aborts(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	// Simulate 3 prior attempts.
	step := stepWith("click", map[string]interface{}{"_recoveryAttempts": 3})
	err := errors.New("element not found")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action.Strategy != "abort" {
		t.Errorf("expected abort after max attempts, got %q", action.Strategy)
	}
}

// ─── navigation error ─────────────────────────────────────────────────────────

func TestDefaultRecovery_NavigationError_ScreenshotAndAbort(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("navigate", map[string]interface{}{"url": "https://bad.example"})
	err := errors.New("failed to navigate: net::err_name_not_resolved")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action.Strategy != "abort" {
		t.Errorf("expected abort for navigation error, got %q", action.Strategy)
	}
	// NewSteps should include a screenshot step.
	if len(action.NewSteps) == 0 {
		t.Fatal("expected screenshot step in NewSteps")
	}
	if action.NewSteps[0].Action != "screenshot" {
		t.Errorf("expected first NewStep to be screenshot, got %q", action.NewSteps[0].Action)
	}
}

// ─── click intercepted ────────────────────────────────────────────────────────

func TestDefaultRecovery_ClickIntercepted_JSFallback(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("click", map[string]interface{}{"target": "submit"})
	err := errors.New("click intercepted by overlay element")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action.Strategy != "retry" {
		t.Errorf("expected retry, got %q", action.Strategy)
	}
	if len(action.NewSteps) == 0 {
		t.Fatal("expected replacement step")
	}
	if action.NewSteps[0].Params["jsClick"] != true {
		t.Errorf("expected jsClick=true in replacement step params")
	}
}

// ─── unknown error ────────────────────────────────────────────────────────────

func TestDefaultRecovery_UnknownError_ScreenshotAndAbort(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("type", map[string]interface{}{"target": "email", "text": "a@b.com"})
	err := errors.New("something completely unexpected happened")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action.Strategy != "abort" {
		t.Errorf("expected abort for unknown error, got %q", action.Strategy)
	}
	if len(action.NewSteps) == 0 || action.NewSteps[0].Action != "screenshot" {
		t.Error("expected screenshot step in NewSteps for unknown error")
	}
}

// ─── nil pageState ────────────────────────────────────────────────────────────

func TestDefaultRecovery_NilPageState_DoesNotPanic(t *testing.T) {
	r := recovery.NewDefaultRecovery()
	step := stepWith("click", nil)
	err := errors.New("element not found")

	action, recErr := r.Recover(context.Background(), step, err, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
}
