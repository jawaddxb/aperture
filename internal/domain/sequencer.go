// Package domain defines core interfaces for Aperture.
// This file defines the Sequencer interface and related result types.
package domain

import (
	"context"
	"time"
)

// StepResult holds the outcome of a single Step execution.
type StepResult struct {
	// Step is the plan step that was executed.
	Step Step `json:"step"`

	// Result is the ActionResult returned by the executor.
	Result *ActionResult `json:"result,omitempty"`

	// Index is the zero-based position of this step within the plan.
	Index int `json:"index"`

	// Duration is the wall-clock time spent executing this step (including recovery).
	Duration time.Duration `json:"duration_ns"`

	// Cost is the credit cost consumed by this action.
	Cost int `json:"cost"`
}

// RunResult holds the overall outcome of running an entire Plan.
type RunResult struct {
	// Plan is the plan that was executed.
	Plan *Plan `json:"plan,omitempty"`

	// Steps contains one StepResult per executed step (may be partial on failure).
	Steps []StepResult `json:"steps"`

	// Success is true when all required steps completed without error.
	Success bool `json:"success"`

	// FailedStep is the zero-based index of the first failed step, or -1 on success.
	FailedStep int `json:"failed_step"`

	// Duration is the total wall-clock time for the run.
	Duration time.Duration `json:"duration_ns"`

	// TotalCost is the sum of all step costs in credits.
	TotalCost int `json:"total_cost"`
}

// Sequencer executes a Plan against a BrowserInstance and returns a RunResult.
type Sequencer interface {
	// Run executes all steps in plan against inst.
	// ctx controls the overall deadline.
	Run(ctx context.Context, inst BrowserInstance, plan *Plan) (*RunResult, error)
}
