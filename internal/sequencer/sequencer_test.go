package sequencer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/sequencer"
)

// alwaysInvalidValidator always marks steps as invalid.
type alwaysInvalidValidator struct{}

func (v *alwaysInvalidValidator) Validate(_ context.Context, _ domain.Step, _ *domain.PageState) (*domain.ValidationResult, error) {
	return &domain.ValidationResult{Valid: false, Errors: []string{"always invalid"}}, nil
}

// alwaysValidValidator always passes.
type alwaysValidValidator struct{}

func (v *alwaysValidValidator) Validate(_ context.Context, _ domain.Step, _ *domain.PageState) (*domain.ValidationResult, error) {
	return &domain.ValidationResult{Valid: true}, nil
}

// ─── stubs ────────────────────────────────────────────────────────────────────

// stubBrowserInstance satisfies domain.BrowserInstance without a real Chromium.
type stubBrowserInstance struct{ ctx context.Context }

func (s *stubBrowserInstance) Context() context.Context { return s.ctx }
func (s *stubBrowserInstance) ID() string               { return "stub-0" }
func (s *stubBrowserInstance) CreatedAt() time.Time     { return time.Time{} }
func (s *stubBrowserInstance) IsAlive() bool            { return true }
func (s *stubBrowserInstance) Close() error             { return nil }
func (s *stubBrowserInstance) Downloads() domain.DownloadManager { return nil }
func (s *stubBrowserInstance) Network() domain.NetworkManager   { return nil }

func newStubInst() domain.BrowserInstance {
	return &stubBrowserInstance{ctx: context.Background()}
}

// successExecutor always returns a successful ActionResult.
type successExecutor struct{}

func (e *successExecutor) Execute(_ context.Context, _ domain.BrowserInstance, _ map[string]interface{}) (*domain.ActionResult, error) {
	return &domain.ActionResult{Action: "ok", Success: true}, nil
}

// failExecutor always returns a failed ActionResult.
type failExecutor struct{ msg string }

func (e *failExecutor) Execute(_ context.Context, _ domain.BrowserInstance, _ map[string]interface{}) (*domain.ActionResult, error) {
	return &domain.ActionResult{Action: "fail", Success: false, Error: e.msg}, errors.New(e.msg)
}

// stepPlan creates a Plan with n steps all using the given action name.
func stepPlan(action string, n int) *domain.Plan {
	steps := make([]domain.Step, n)
	for i := range steps {
		steps[i] = domain.Step{Action: action, Params: map[string]interface{}{}}
	}
	return &domain.Plan{Goal: "test", Steps: steps}
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestSequencer_AllStepsSucceed(t *testing.T) {
	reg := map[string]domain.Executor{"ok": &successExecutor{}}
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg})

	result, err := seq.Run(context.Background(), newStubInst(), stepPlan("ok", 3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.FailedStep != -1 {
		t.Errorf("expected FailedStep=-1, got %d", result.FailedStep)
	}
	if len(result.Steps) != 3 {
		t.Errorf("expected 3 step results, got %d", len(result.Steps))
	}
}

func TestSequencer_StopOnFirstFailure(t *testing.T) {
	reg := map[string]domain.Executor{
		"ok":   &successExecutor{},
		"fail": &failExecutor{msg: "boom"},
	}
	plan := &domain.Plan{
		Goal: "test",
		Steps: []domain.Step{
			{Action: "ok", Params: map[string]interface{}{}},
			{Action: "fail", Params: map[string]interface{}{}},
			{Action: "ok", Params: map[string]interface{}{}},
		},
	}
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg})

	result, err := seq.Run(context.Background(), newStubInst(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected Success=false")
	}
	if result.FailedStep != 1 {
		t.Errorf("expected FailedStep=1, got %d", result.FailedStep)
	}
	// Only steps 0 and 1 should be in results (step 2 never ran).
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 step results, got %d", len(result.Steps))
	}
	if !result.Steps[0].Result.Success {
		t.Error("step 0 should have succeeded")
	}
}

func TestSequencer_OptionalStepFailureContinues(t *testing.T) {
	reg := map[string]domain.Executor{
		"ok":   &successExecutor{},
		"fail": &failExecutor{msg: "optional-fail"},
	}
	plan := &domain.Plan{
		Goal: "test",
		Steps: []domain.Step{
			{Action: "ok", Params: map[string]interface{}{}},
			{Action: "fail", Params: map[string]interface{}{}, Optional: true},
			{Action: "ok", Params: map[string]interface{}{}},
		},
	}
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg})

	result, err := seq.Run(context.Background(), newStubInst(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true (optional step failure should not affect overall success)")
	}
	if result.FailedStep != -1 {
		t.Errorf("expected FailedStep=-1, got %d", result.FailedStep)
	}
	if len(result.Steps) != 3 {
		t.Errorf("expected 3 step results, got %d", len(result.Steps))
	}
}

func TestSequencer_ProgressCallback(t *testing.T) {
	reg := map[string]domain.Executor{"ok": &successExecutor{}}
	var called int
	progress := func(_ domain.StepResult) { called++ }
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg, Progress: progress})

	_, err := seq.Run(context.Background(), newStubInst(), stepPlan("ok", 3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 3 {
		t.Errorf("expected progress called 3 times, got %d", called)
	}
}

func TestSequencer_UnknownActionFails(t *testing.T) {
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: map[string]domain.Executor{}})
	result, err := seq.Run(context.Background(), newStubInst(), stepPlan("ghost", 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for unknown action")
	}
}

func TestSequencer_Duration(t *testing.T) {
	reg := map[string]domain.Executor{"ok": &successExecutor{}}
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg})
	result, _ := seq.Run(context.Background(), newStubInst(), stepPlan("ok", 1))
	if result.Duration <= 0 {
		t.Error("expected positive Duration")
	}
}

func TestSequencer_EmptyPlan(t *testing.T) {
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: map[string]domain.Executor{}})
	result, err := seq.Run(context.Background(), newStubInst(), &domain.Plan{Goal: "noop"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("empty plan should succeed")
	}
	if result.FailedStep != -1 {
		t.Errorf("expected FailedStep=-1, got %d", result.FailedStep)
	}
}

func TestSequencer_ValidatorSkipsInvalidStep(t *testing.T) {
	reg := map[string]domain.Executor{"ok": &successExecutor{}}
	seq := sequencer.NewDefaultSequencer(sequencer.Config{
		Registry:  reg,
		Validator: &alwaysInvalidValidator{},
	})
	result, err := seq.Run(context.Background(), newStubInst(), stepPlan("ok", 2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First step fails validation → run stops.
	if result.Success {
		t.Error("expected Success=false when validator rejects step")
	}
	if result.FailedStep != 0 {
		t.Errorf("expected FailedStep=0, got %d", result.FailedStep)
	}
	if len(result.Steps) < 1 {
		t.Fatal("expected at least one step result")
	}
	if result.Steps[0].Result.Success {
		t.Error("step 0 result should be failure")
	}
}

func TestSequencer_ValidatorPassesValidStep(t *testing.T) {
	reg := map[string]domain.Executor{"ok": &successExecutor{}}
	seq := sequencer.NewDefaultSequencer(sequencer.Config{
		Registry:  reg,
		Validator: &alwaysValidValidator{},
	})
	result, err := seq.Run(context.Background(), newStubInst(), stepPlan("ok", 3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true with always-valid validator")
	}
	if len(result.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(result.Steps))
	}
}
