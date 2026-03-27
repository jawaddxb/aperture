// Package domain defines core interfaces for Aperture.
// This file defines ActionLogger and MetricsCollector for observability.
package domain

import "time"

// LogEntry is a single structured log record.
type LogEntry struct {
	// Level is one of: "debug", "info", "warn", "error".
	Level string `json:"level"`

	// Message is the human-readable log text.
	Message string `json:"message"`

	// SessionID scopes the entry to a session, if set.
	SessionID string `json:"session_id,omitempty"`

	// Action is the executor action name this entry describes, if set.
	Action string `json:"action,omitempty"`

	// StepIndex is the zero-based plan step index, if applicable.
	StepIndex int `json:"step_index,omitempty"`

	// Duration is the wall-clock time for the logged operation.
	Duration time.Duration `json:"duration_ms,omitempty"`

	// Error is a human-readable error description, if applicable.
	Error string `json:"error,omitempty"`

	// Fields holds additional arbitrary key-value pairs.
	Fields map[string]string `json:"fields,omitempty"`

	// Timestamp records when the entry was created.
	Timestamp time.Time `json:"timestamp"`
}

// ActionLogger writes structured log entries. Implementations are chainable:
// WithSession and WithAction return a new logger with pre-set fields so
// callers don't repeat session/action names on every call.
// Implementations must be safe for concurrent use.
type ActionLogger interface {
	// Log writes the entry to the underlying output.
	Log(entry LogEntry)

	// WithSession returns a new logger with SessionID pre-set to id.
	WithSession(sessionID string) ActionLogger

	// WithAction returns a new logger with Action pre-set to action.
	WithAction(action string) ActionLogger
}

// MetricsSnapshot is a point-in-time copy of collected metrics.
type MetricsSnapshot struct {
	// TotalActions is the total number of actions recorded across all sessions.
	TotalActions int64 `json:"total_actions"`

	// TotalSessions is the number of sessions recorded.
	TotalSessions int64 `json:"total_sessions"`

	// ActionCounts maps action name to invocation count.
	ActionCounts map[string]int64 `json:"action_counts"`

	// AvgDurationMs maps action name to average duration in milliseconds.
	AvgDurationMs map[string]float64 `json:"avg_duration_ms"`

	// ErrorRate is the fraction of failed actions in [0, 1].
	ErrorRate float64 `json:"error_rate"`
}

// MetricsCollector accumulates runtime statistics. Implementations must be
// safe for concurrent use and must not hold locks during Snapshot().
type MetricsCollector interface {
	// RecordAction records one action execution with its duration and outcome.
	RecordAction(action string, duration time.Duration, success bool)

	// RecordSession records session-level aggregates at completion.
	RecordSession(duration time.Duration, stepsTotal int, stepsFailed int)

	// Snapshot returns a consistent point-in-time copy of all metrics.
	Snapshot() *MetricsSnapshot
}
