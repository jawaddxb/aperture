// Package domain defines core interfaces for Aperture.
// This file defines the StepValidator interface for pre-flight step validation.
package domain

import "context"

// ValidationResult holds the outcome of validating a Step before execution.
type ValidationResult struct {
	// Valid is true when the step can proceed; false aborts the step.
	Valid bool

	// Errors contains blocking issues that prevent execution.
	Errors []string

	// Warnings contains non-blocking advisories (e.g. missing timeout, HTTP URL).
	Warnings []string
}

// StepValidator performs pre-flight checks on a Step before execution.
// Implementations must be safe for concurrent use.
type StepValidator interface {
	// Validate checks the step against the current page state.
	// Returns a non-nil *ValidationResult even on error.
	Validate(ctx context.Context, step Step, pageState *PageState) (*ValidationResult, error)
}
