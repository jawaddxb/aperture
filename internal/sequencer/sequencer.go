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
type DefaultSequencer struct {
	registry  map[string]domain.Executor
	recovery  domain.RecoveryStrategy // optional; nil = no recovery
	progress  ProgressFunc            // optional; nil = no callback
	validator domain.StepValidator    // optional; nil = no validation
}

// Config holds constructor parameters for DefaultSequencer.
type Config struct {
	Registry  map[string]domain.Executor
	Recovery  domain.RecoveryStrategy
	Progress  ProgressFunc
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

	if s.validator != nil {
		if stop, res := s.validateStep(ctx, step, idx, start); stop {
			return res, true
		}
	}

	// Hard timeout per step: 30s. chromedp may not always respect context
	// cancellation for certain page load states (e.g., infinite JS loading,
	// CAPTCHA walls that load body but never settle). This goroutine wrapper
	// guarantees we return within the timeout.
	stepTimeout := 30 * time.Second
	type execResult struct {
		ar      *domain.ActionResult
		execErr error
	}
	ch := make(chan execResult, 1)
	stepCtx, stepCancel := context.WithTimeout(ctx, stepTimeout)
	go func() {
		ar, err := s.executeStep(stepCtx, inst, step)
		ch <- execResult{ar: ar, execErr: err}
	}()

	var ar *domain.ActionResult
	var execErr error
	select {
	case res := <-ch:
		ar = res.ar
		execErr = res.execErr
	case <-stepCtx.Done():
		ar = &domain.ActionResult{
			Action:   step.Action,
			Success:  false,
			Error:    fmt.Sprintf("step timed out after %v", stepTimeout),
			Duration: time.Since(start),
		}
		execErr = nil
	}
	stepCancel()

	if execErr == nil && ar.Success {
		return domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}, false
	}

	if s.recovery != nil {
		if ok, res := s.tryRecover(ctx, inst, step, ar, execErr, idx, start); ok {
			return res, false
		}
	}

	return domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}, !step.Optional
}

func (s *DefaultSequencer) validateStep(ctx context.Context, step domain.Step, idx int, start time.Time) (bool, domain.StepResult) {
	vr, vErr := s.validator.Validate(ctx, step, nil)
	if vErr != nil || !vr.Valid {
		msg := "validation failed"
		if vErr != nil {
			msg = vErr.Error()
		} else if len(vr.Errors) > 0 {
			msg = fmt.Sprintf("validation failed: %v", vr.Errors)
		}
		ar := &domain.ActionResult{Action: step.Action, Success: false, Error: msg}
		return !step.Optional, domain.StepResult{Step: step, Result: ar, Index: idx, Duration: time.Since(start)}
	}
	return false, domain.StepResult{}
}

func (s *DefaultSequencer) tryRecover(
	ctx context.Context,
	inst domain.BrowserInstance,
	step domain.Step,
	ar *domain.ActionResult,
	execErr error,
	idx int,
	start time.Time,
) (bool, domain.StepResult) {
	err := execErr
	if err == nil {
		err = fmt.Errorf("%s", ar.Error)
	}

	pageState := pageStateFromResult(ar)
	action, recoveryErr := s.recovery.Recover(ctx, step, err, pageState)
	if recoveryErr == nil && action != nil {
		updatedStep, recovered := s.applyRecovery(ctx, inst, step, action, idx)
		if recovered {
			res := domain.StepResult{
				Step:     updatedStep,
				Result:   &domain.ActionResult{Action: step.Action, Success: true},
				Index:    idx,
				Duration: time.Since(start),
			}
			return true, res
		}
	}
	return false, domain.StepResult{}
}

// executeStep dispatches a step to its executor.
func (s *DefaultSequencer) executeStep(ctx context.Context, inst domain.BrowserInstance, step domain.Step) (*domain.ActionResult, error) {
	exec, ok := s.registry[step.Action]
	if !ok {
		msg := fmt.Sprintf("no executor registered for action %q", step.Action)
		return &domain.ActionResult{Action: step.Action, Success: false, Error: msg}, fmt.Errorf("%s", msg)
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
		return step, err == nil && ar.Success

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
