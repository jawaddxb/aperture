// Package domain defines core interfaces for Aperture.
// This file defines the Planner interface and related types.
package domain

import "context"

// Step is a single concrete browser action in a Plan.
type Step struct {
	// Action is the executor action name: "navigate", "click", "type", "screenshot", etc.
	Action string

	// Params are the executor-specific parameters for this step.
	Params map[string]interface{}

	// Reasoning explains why this step was chosen (for debugging/logging).
	Reasoning string

	// Optional indicates that a failure of this step should not halt the plan.
	Optional bool
}

// Plan is an ordered sequence of Steps that together accomplish a high-level Goal.
type Plan struct {
	// Goal is the original natural-language intent supplied by the caller.
	Goal string

	// Steps is the ordered sequence of executor actions.
	Steps []Step

	// Metadata holds arbitrary key-value annotations (e.g. "complexity": "simple").
	Metadata map[string]string
}

// Planner decomposes a natural-language goal into a concrete Plan.
type Planner interface {
	// Plan returns an ordered list of Steps that fulfil the given goal.
	// pageState is the current browser state; may be nil if the browser is fresh.
	Plan(ctx context.Context, goal string, pageState *PageState) (*Plan, error)
}

// LLMClient is the minimal interface for calling a language model.
// Implementations may wrap any SDK (OpenAI, Anthropic, local models, etc.).
type LLMClient interface {
	// Complete sends prompt to the model and returns the completion text.
	Complete(ctx context.Context, prompt string) (string, error)
}
