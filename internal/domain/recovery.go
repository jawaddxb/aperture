// Package domain defines core interfaces for Aperture.
// This file defines the RecoveryStrategy interface and related types.
package domain

import "context"

// RecoveryAction describes what the sequencer should do after a step failure.
type RecoveryAction struct {
	// Strategy names the recovery approach:
	//   "retry"               – re-run the failed step as-is
	//   "screenshot_and_retry"– take a screenshot, then re-run the step
	//   "replan"              – replace the failed step with NewSteps
	//   "skip"                – skip the step and continue
	//   "abort"               – stop the run immediately
	Strategy string

	// NewSteps are replacement steps used when Strategy == "replan".
	NewSteps []Step

	// Reason is a human-readable explanation for the chosen strategy.
	Reason string
}

// RecoveryStrategy decides how to handle a failed step.
// Implementations receive their dependencies via constructor (DI).
type RecoveryStrategy interface {
	// Recover analyses the failure and returns the action to take.
	// pageState is the browser state at the moment of failure; may be nil.
	Recover(ctx context.Context, failedStep Step, err error, pageState *PageState) (*RecoveryAction, error)
}
