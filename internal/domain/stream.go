// Package domain defines core interfaces for Aperture.
// This file defines the ProgressEmitter interface for real-time step streaming.
package domain

import "time"

// ProgressEvent carries state updates about plan execution to subscribers.
type ProgressEvent struct {
	// SessionID identifies the session this event belongs to.
	SessionID string `json:"session_id"`

	// Type is the event kind: "step_start", "step_complete", "step_failed",
	// "plan_ready", or "done".
	Type string `json:"type"`

	// StepIndex is the zero-based position of the step this event describes.
	StepIndex int `json:"step_index"`

	// Action is the executor action name (e.g. "click", "navigate").
	Action string `json:"action,omitempty"`

	// Success reports whether the step succeeded (populated on completion events).
	Success bool `json:"success,omitempty"`

	// Message is a human-readable description of the event.
	Message string `json:"message,omitempty"`

	// Timestamp records when the event was emitted.
	Timestamp time.Time `json:"timestamp"`
}

// ProgressEmitter publishes ProgressEvents and allows clients to subscribe
// to per-session event streams. Implementations must be safe for concurrent use.
type ProgressEmitter interface {
	// Emit publishes an event to all subscribers of the event's SessionID.
	// Non-blocking: slow subscribers may miss events.
	Emit(event ProgressEvent)

	// Subscribe returns a channel that receives events for the given sessionID.
	// The caller must eventually call Unsubscribe to free resources.
	Subscribe(sessionID string) <-chan ProgressEvent

	// Unsubscribe stops delivering events to ch and removes it from the fan-out.
	Unsubscribe(sessionID string, ch <-chan ProgressEvent)
}
