package executor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
)

// ─── DefaultHITLManager tests ────────────────────────────────────────────────

// TestHITLManager_ResolveUnblocks verifies that ResolveIntervention delivers
// the response to the goroutine blocked in RequestIntervention.
func TestHITLManager_ResolveUnblocks(t *testing.T) {
	mgr := executor.NewDefaultHITLManager()

	req := &domain.InterventionRequest{
		ID:        "req-1",
		SessionID: "sess-1",
		Type:      "captcha",
		Prompt:    "Solve the CAPTCHA",
		CreatedAt: time.Now(),
	}

	resultCh := make(chan *domain.InterventionResponse, 1)
	errCh := make(chan error, 1)

	go func() {
		resp, err := mgr.RequestIntervention(context.Background(), req)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- resp
	}()

	// Give the goroutine time to register.
	time.Sleep(20 * time.Millisecond)

	want := &domain.InterventionResponse{ID: "req-1", Success: true, Data: "solved-text"}
	if err := mgr.ResolveIntervention(context.Background(), "req-1", want); err != nil {
		t.Fatalf("ResolveIntervention: %v", err)
	}

	select {
	case resp := <-resultCh:
		if resp.Data != "solved-text" {
			t.Errorf("Data = %q, want %q", resp.Data, "solved-text")
		}
		if !resp.Success {
			t.Error("Success must be true")
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response")
	}
}

// TestHITLManager_Timeout verifies that a context deadline causes
// RequestIntervention to return an error.
func TestHITLManager_Timeout(t *testing.T) {
	mgr := executor.NewDefaultHITLManager()

	req := &domain.InterventionRequest{
		ID:        "req-timeout",
		Type:      "confirmation",
		Prompt:    "Confirm?",
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := mgr.RequestIntervention(ctx, req)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

// TestHITLManager_Cancel verifies that CancelIntervention unblocks the caller.
func TestHITLManager_Cancel(t *testing.T) {
	mgr := executor.NewDefaultHITLManager()

	req := &domain.InterventionRequest{
		ID:        "req-cancel",
		Type:      "input",
		Prompt:    "Enter value",
		CreatedAt: time.Now(),
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := mgr.RequestIntervention(context.Background(), req)
		errCh <- err
	}()

	time.Sleep(20 * time.Millisecond)

	if err := mgr.CancelIntervention(context.Background(), "req-cancel"); err != nil {
		t.Fatalf("CancelIntervention: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected cancellation error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancel to propagate")
	}
}

// TestHITLManager_ResolveUnknown verifies that resolving an unknown ID returns an error.
func TestHITLManager_ResolveUnknown(t *testing.T) {
	mgr := executor.NewDefaultHITLManager()
	resp := &domain.InterventionResponse{ID: "nope", Success: true}
	err := mgr.ResolveIntervention(context.Background(), "nope", resp)
	if err == nil {
		t.Fatal("expected error for unknown ID, got nil")
	}
}

// ─── PauseExecutor tests ─────────────────────────────────────────────────────

// stubHITLManager is a test double for domain.HITLManager.
type stubHITLManager struct {
	requestFn func(ctx context.Context, req *domain.InterventionRequest) (*domain.InterventionResponse, error)
}

func (s *stubHITLManager) RequestIntervention(ctx context.Context, req *domain.InterventionRequest) (*domain.InterventionResponse, error) {
	return s.requestFn(ctx, req)
}
func (s *stubHITLManager) ResolveIntervention(_ context.Context, _ string, _ *domain.InterventionResponse) error {
	return nil
}
func (s *stubHITLManager) CancelIntervention(_ context.Context, _ string) error { return nil }

// TestPauseExecutor_Success verifies that PauseExecutor forwards the response data.
func TestPauseExecutor_Success(t *testing.T) {
	mgr := &stubHITLManager{
		requestFn: func(_ context.Context, req *domain.InterventionRequest) (*domain.InterventionResponse, error) {
			if req.Type != "captcha" {
				t.Errorf("Type = %q, want captcha", req.Type)
			}
			if req.Prompt != "Solve the CAPTCHA please" {
				t.Errorf("Prompt = %q", req.Prompt)
			}
			return &domain.InterventionResponse{ID: req.ID, Success: true, Data: "abc123"}, nil
		},
	}

	e := executor.NewPauseExecutor(mgr)
	inst := &stubBrowserInstance{ctx: inst_unused()}

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"type":      "captcha",
		"prompt":    "Solve the CAPTCHA please",
		"sessionID": "sess-42",
		"timeout":   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if string(result.Data) != "abc123" {
		t.Errorf("Data = %q, want abc123", string(result.Data))
	}
}

// TestPauseExecutor_MissingType verifies that a missing "type" param fails gracefully.
func TestPauseExecutor_MissingType(t *testing.T) {
	e := executor.NewPauseExecutor(executor.NewDefaultHITLManager())
	inst := &stubBrowserInstance{ctx: inst_unused()}

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"prompt": "something",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for missing type param")
	}
}

// TestPauseExecutor_Timeout verifies that a short timeout causes failure.
func TestPauseExecutor_Timeout(t *testing.T) {
	mgr := &stubHITLManager{
		requestFn: func(ctx context.Context, _ *domain.InterventionRequest) (*domain.InterventionResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	e := executor.NewPauseExecutor(mgr)
	inst := &stubBrowserInstance{ctx: inst_unused()}

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"type":    "captcha",
		"prompt":  "take your time",
		"timeout": 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure on timeout")
	}
}
