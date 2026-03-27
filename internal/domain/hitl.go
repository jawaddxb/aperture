// Package domain defines core interfaces for Aperture.
// This file defines the HITLManager interface and associated types used for
// human-in-the-loop interventions (e.g., CAPTCHA solving, confirmations).
package domain

import (
	"context"
	"time"
)

// InterventionRequest describes a pause point that requires human action.
type InterventionRequest struct {
	// ID is a unique identifier for this intervention request.
	ID string

	// SessionID is the browser session that triggered this request.
	SessionID string

	// Type categorises the intervention: "captcha", "confirmation", or "input".
	Type string

	// Prompt is the human-readable description of what is required.
	Prompt string

	// Screenshot holds an optional PNG snapshot of the page at pause time.
	Screenshot []byte

	// CreatedAt is the UTC time when the request was created.
	CreatedAt time.Time
}

// InterventionResponse carries the human's reply to an InterventionRequest.
type InterventionResponse struct {
	// ID must match the InterventionRequest.ID being resolved.
	ID string

	// Success indicates whether the human completed the intervention.
	Success bool

	// Data holds the human-supplied value (e.g. solved captcha text or typed input).
	Data string
}

// HITLManager coordinates human-in-the-loop interventions.
// Implementations must be safe for concurrent use.
type HITLManager interface {
	// RequestIntervention pauses execution and waits until the intervention is
	// resolved, cancelled, or the context deadline is exceeded.
	// Returns the human's response or an error if the request timed out/cancelled.
	RequestIntervention(ctx context.Context, req *InterventionRequest) (*InterventionResponse, error)

	// ResolveIntervention delivers resp to the goroutine blocked in RequestIntervention.
	// Returns an error if no pending intervention with that ID exists.
	ResolveIntervention(ctx context.Context, id string, resp *InterventionResponse) error

	// CancelIntervention cancels a pending intervention, unblocking its caller with an error.
	// Returns an error if no pending intervention with that ID exists.
	CancelIntervention(ctx context.Context, id string) error
}
