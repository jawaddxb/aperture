// Package sequencer provides a Sequencer that executes a Plan step-by-step.
package sequencer

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ProgressFunc is an optional callback invoked after each step completes.
// It receives the StepResult of the just-completed step.
type ProgressFunc func(result domain.StepResult)

// DefaultSequencer executes Plan steps in order using a registry of Executors.
// On failure it consults a RecoveryStrategy (if configured) before aborting.
// Implements domain.Sequencer conceptually; the interface lives in domain but
// the concrete type lives here to keep domain free of implementations.
type DefaultSequencer struct {
	registry  map[string]domain.Executor
	recovery  domain.RecoveryStrategy // optional; nil = no recovery
	progress  ProgressFunc            // optional; nil = no callback
	validator domain.StepValidator    // optional; nil = no validation
}

// Config holds constructor parameters for DefaultSequencer.
type Config struct {
	// Registry maps action names to Executor implementations.
	// Required.
	Registry map[string]domain.Executor

	// Recovery is the strategy consulted on step failure.
	// Optional; nil disables recovery and fails immediately.
	Recovery domain.RecoveryStrategy

	// Progress is called after each step with its result.
	// Optional; nil means no progress notifications.
	Progress ProgressFunc

	// Validator performs pre-flight checks before each step executes.
	// Optional; nil disables validation.
	Validator domain.StepValidator
}

// NewDefaultSequencer returns a DefaultSequencer ready for use.
func NewDefaultSequencer(cfg Config) *DefaultSequencer {
	return &DefaultSequencer{
		registry:  cfg.Registry,
		recovery:  cfg.Recovery,
		progress:  cfg.Progress,
		validator: cfg.Validator,
	}
}

// Run executes all steps in plan against inst.
// It stops on the first unrecoverable failure (unless the step is Optional).
// ctx controls the overall deadline; individual steps inherit it.
func (s *DefaultSequencer) Run(ctx context.Context, inst domain.BrowserInstance, plan *domain.Plan) (*domain.RunResult, error) {
	start := time.Now()
	result := &domain.RunResult{
		Plan:       plan,
		Steps:      make([]domain.StepResult, 0, len(plan.Steps)),
		Success:    true,
		FailedStep: -1,
	}

	for i, step := range plan.Steps {
		sr, stop := s.runStep(ctx, inst, step, i)
		result.Steps = append(result.Steps, sr)

		if s.progress != nil {
			s.progress(sr)
		}

		if stop {
			result.Success = false
			result.FailedStep = i
			break
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// runStep executes a single step with recovery, returning the StepResult and
// whether the sequencer should stop (true = halt the run).
func (s *DefaultSequencer) runStep(
	ctx context.Context,
	inst domain.BrowserInstance,
	step domain.Step,
	idx int,
) (domain.StepResult, bool) {
	start := time.Now()

	// Pre-flight validation (if configured).
	if s.validator != nil {
		vr, vErr := s.validator.Validate(ctx, step, nil)
		if vErr != nil {
			ar := &domain.ActionResult{Action: step.Action, Success: false, Error: vErr.Error()}
			return domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}, !step.Optional
		}
		if !vr.Valid {
			msg := fmt.Sprintf("validation failed: %v", vr.Errors)
			ar := &domain.ActionResult{Action: step.Action, Success: false, Error: msg}
			return domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}, !step.Optional
		}
	}

	ar, execErr := s.executeStep(ctx, inst, step)
	if execErr == nil && ar.Success {
		return domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}, false
	}

	// Determine the error for recovery.
	err := execErr
	if err == nil {
		err = fmt.Errorf("%s", ar.Error)
	}

	// Attempt recovery if strategy is configured.
	if s.recovery != nil {
		pageState := pageStateFromResult(ar)
		action, recoveryErr := s.recovery.Recover(ctx, step, err, pageState)
		if recoveryErr == nil && action != nil {
			updatedStep, recovered := s.applyRecovery(ctx, inst, step, action, idx)
			if recovered {
				return domain.StepResult{Step: updatedStep, Result: &domain.ActionResult{Action: step.Action, Success: true}, Index: idx, Duration: time.Since(start)}, false
			}
		}
	}

	// No recovery succeeded.
	if step.Optional {
		return domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}, false
	}
	return domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}, true
}

// executeStep dispatches a step to its executor.
func (s *DefaultSequencer) executeStep(ctx context.Context, inst domain.BrowserInstance, step domain.Step) (*domain.ActionResult, error) {
	exec, ok := s.registry[step.Action]
	if !ok {
		return &domain.ActionResult{
			Action:  step.Action,
			Success: false,
			Error:   fmt.Sprintf("no executor registered for action %q", step.Action),
		}, fmt.Errorf("no executor for action %q", step.Action)
	}
	return exec.Execute(ctx, inst, step.Params)
}

// applyRecovery applies a RecoveryAction and returns the final step + success flag.
func (s *DefaultSequencer) applyRecovery(
	ctx context.Context,
	inst domain.BrowserInstance,
	step domain.Step,
	action *domain.RecoveryAction,
	idx int,
) (domain.Step, bool) {
	switch action.Strategy {
	case "retry", "screenshot_and_retry":
		ar, err := s.executeStep(ctx, inst, step)
		if err == nil && ar.Success {
			return step, true
		}
		return step, false

	case "replan":
		for _, newStep := range action.NewSteps {
			ar, err := s.executeStep(ctx, inst, newStep)
			if err != nil || !ar.Success {
				return newStep, false
			}
		}
		return step, true

	case "skip":
		return step, true

	default: // "abort" and unknown strategies
		return step, false
	}
}

// pageStateFromResult extracts PageState from an ActionResult safely.
func pageStateFromResult(ar *domain.ActionResult) *domain.PageState {
	if ar == nil {
		return nil
	}
	return ar.PageState
}

// compile-time interface check — DefaultSequencer satisfies domain.Sequencer.
var _ interface {
	Run(ctx context.Context, inst domain.BrowserInstance, plan *domain.Plan) (*domain.RunResult, error)
} = (*DefaultSequencer)(nil)
