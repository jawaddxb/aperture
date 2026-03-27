// Package domain defines core interfaces for Aperture.
// This file defines the Bridge interface and associated request/response types
// used by the OpenClaw integration layer.
package domain

import (
	"context"
	"time"
)

// TaskRequest is what OpenClaw sends to Aperture.
type TaskRequest struct {
	ID          string            `json:"id"`
	Goal        string            `json:"goal"`
	URL         string            `json:"url,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Config      TaskConfig        `json:"config,omitempty"`
	Screenshots bool              `json:"screenshots,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
}

// TaskConfig carries per-task tuning knobs sent by OpenClaw.
type TaskConfig struct {
	Timeout     int    `json:"timeout,omitempty"`
	MaxSteps    int    `json:"max_steps,omitempty"`
	Model       string `json:"model,omitempty"`
	Headless    bool   `json:"headless"`
	RecordSteps bool   `json:"record_steps,omitempty"`
}

// TaskResponse is what Aperture returns to OpenClaw.
type TaskResponse struct {
	ID         string        `json:"id"`
	Success    bool          `json:"success"`
	Goal       string        `json:"goal"`
	Steps      []StepSummary `json:"steps"`
	FinalURL   string        `json:"final_url"`
	FinalTitle string        `json:"final_title"`
	Screenshot []byte        `json:"screenshot,omitempty"`
	Duration   time.Duration `json:"duration_ms"`
	Error      string        `json:"error,omitempty"`
}

// StepSummary is a compact per-step result for the TaskResponse.
type StepSummary struct {
	Index      int           `json:"index"`
	Action     string        `json:"action"`
	Target     string        `json:"target,omitempty"`
	Success    bool          `json:"success"`
	Duration   time.Duration `json:"duration_ms"`
	Screenshot []byte        `json:"screenshot,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// Bridge is the top-level interface for the OpenClaw integration layer.
// Implementations must be safe for concurrent use.
type Bridge interface {
	// ExecuteTask runs a task described by req and returns the full result.
	// The call blocks until the task completes or ctx is cancelled.
	ExecuteTask(ctx context.Context, req *TaskRequest) (*TaskResponse, error)

	// GetStatus returns the current or final result for the given task ID.
	// Returns ErrTaskNotFound when no task with that ID is known.
	GetStatus(ctx context.Context, taskID string) (*TaskResponse, error)

	// CancelTask requests cancellation of a running task.
	// Returns ErrTaskNotFound when no task with that ID is known.
	CancelTask(ctx context.Context, taskID string) error
}

// ErrTaskNotFound is returned by GetStatus and CancelTask when the task ID
// is unknown.
const ErrTaskNotFound bridgeError = "task not found"

// bridgeError is a sentinel error type for Bridge errors.
type bridgeError string

func (e bridgeError) Error() string { return string(e) }
