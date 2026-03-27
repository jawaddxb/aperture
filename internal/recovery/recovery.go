// Package recovery provides RecoveryStrategy implementations for Aperture.
// When a sequencer step fails, a RecoveryStrategy decides the next action:
// retry, replan, skip, or abort.
package recovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

const (
	maxAttempts    = 3
	retryWait      = 2 * time.Second
	maxTimeout     = 60.0
	defaultTimeout = 10.0
)

// attemptKey is the key used to track retry count in step params.
const attemptKey = "_recoveryAttempts"

// DefaultRecovery classifies errors and returns a recovery action.
// It implements domain.RecoveryStrategy.
type DefaultRecovery struct{}

// NewDefaultRecovery returns a DefaultRecovery ready for use.
func NewDefaultRecovery() *DefaultRecovery {
	return &DefaultRecovery{}
}

// Recover analyses the error and returns a RecoveryAction.
// It respects a maximum of maxAttempts recovery attempts per step.
func (r *DefaultRecovery) Recover(
	ctx context.Context,
	failedStep domain.Step,
	err error,
	pageState *domain.PageState,
) (*domain.RecoveryAction, error) {
	attempts := currentAttempts(failedStep)
	if attempts >= maxAttempts {
		return &domain.RecoveryAction{
			Strategy: "abort",
			Reason:   fmt.Sprintf("max recovery attempts (%d) exceeded", maxAttempts),
		}, nil
	}

	errMsg := strings.ToLower(err.Error())

	switch {
	case isElementNotFound(errMsg):
		return r.recoverElementNotFound(ctx, failedStep, attempts)
	case isTimeout(errMsg):
		return r.recoverTimeout(failedStep, attempts)
	case isNavigationError(errMsg):
		return r.recoverNavigation(failedStep)
	case isClickIntercepted(errMsg):
		return r.recoverClickIntercepted(failedStep, attempts)
	default:
		return r.recoverUnknown(failedStep)
	}
}

// recoverElementNotFound waits 2s then retries (stale DOM).
func (r *DefaultRecovery) recoverElementNotFound(
	ctx context.Context,
	step domain.Step,
	attempts int,
) (*domain.RecoveryAction, error) {
	select {
	case <-time.After(retryWait):
	case <-ctx.Done():
		return &domain.RecoveryAction{Strategy: "abort", Reason: "context cancelled during wait"}, nil
	}
	updated := incrementAttempts(step, attempts)
	return &domain.RecoveryAction{
		Strategy: "retry",
		NewSteps: []domain.Step{updated},
		Reason:   "element not found; retrying after 2s wait (stale DOM)",
	}, nil
}

// recoverTimeout doubles the timeout and retries (up to maxTimeout).
func (r *DefaultRecovery) recoverTimeout(step domain.Step, attempts int) (*domain.RecoveryAction, error) {
	updated := incrementAttempts(step, attempts)
	updated.Params = cloneParams(step.Params)

	current := paramFloat(step.Params, "timeout", defaultTimeout)
	doubled := current * 2
	if doubled > maxTimeout {
		doubled = maxTimeout
	}
	updated.Params["timeout"] = doubled

	return &domain.RecoveryAction{
		Strategy: "retry",
		NewSteps: []domain.Step{updated},
		Reason:   fmt.Sprintf("timeout; retrying with doubled timeout (%.0fs)", doubled),
	}, nil
}

// recoverNavigation takes a screenshot and aborts.
func (r *DefaultRecovery) recoverNavigation(step domain.Step) (*domain.RecoveryAction, error) {
	screenshot := domain.Step{
		Action:    "screenshot",
		Params:    map[string]interface{}{"fullPage": true},
		Reasoning: "capture state after navigation error",
	}
	return &domain.RecoveryAction{
		Strategy: "abort",
		NewSteps: []domain.Step{screenshot, step},
		Reason:   "navigation error; taking screenshot before abort",
	}, nil
}

// recoverClickIntercepted retries with JS click fallback.
func (r *DefaultRecovery) recoverClickIntercepted(step domain.Step, attempts int) (*domain.RecoveryAction, error) {
	updated := incrementAttempts(step, attempts)
	updated.Params = cloneParams(step.Params)
	updated.Params["jsClick"] = true

	return &domain.RecoveryAction{
		Strategy: "retry",
		NewSteps: []domain.Step{updated},
		Reason:   "click intercepted; retrying with JS click fallback",
	}, nil
}

// recoverUnknown takes a screenshot and aborts.
func (r *DefaultRecovery) recoverUnknown(step domain.Step) (*domain.RecoveryAction, error) {
	screenshot := domain.Step{
		Action:    "screenshot",
		Params:    map[string]interface{}{"fullPage": true},
		Reasoning: "capture state after unknown error",
	}
	return &domain.RecoveryAction{
		Strategy: "abort",
		NewSteps: []domain.Step{screenshot, step},
		Reason:   "unknown error; taking screenshot before abort",
	}, nil
}

// ─── error classifiers ────────────────────────────────────────────────────────

func isElementNotFound(msg string) bool {
	return containsAny(msg, "no match", "not found", "element not found", "no element", "errnotmatch")
}

func isTimeout(msg string) bool {
	return containsAny(msg, "timeout", "timed out", "deadline exceeded", "context deadline")
}

func isNavigationError(msg string) bool {
	return containsAny(msg, "navigation", "net::err", "failed to navigate", "page not loaded")
}

func isClickIntercepted(msg string) bool {
	return containsAny(msg, "intercepted", "covered by", "other element", "not clickable")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ─── param helpers ────────────────────────────────────────────────────────────

func currentAttempts(step domain.Step) int {
	if step.Params == nil {
		return 0
	}
	v, ok := step.Params[attemptKey]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func incrementAttempts(step domain.Step, current int) domain.Step {
	updated := step
	updated.Params = cloneParams(step.Params)
	updated.Params[attemptKey] = current + 1
	return updated
}

func cloneParams(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func paramFloat(params map[string]interface{}, key string, def float64) float64 {
	if params == nil {
		return def
	}
	v, ok := params[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return def
}
