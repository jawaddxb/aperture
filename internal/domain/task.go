// Package domain defines core interfaces for Aperture.
// This file defines the TaskPlanner interface and TaskContext type.
package domain

import (
	"context"
	"encoding/json"
	"time"
)

// TaskEvent is a progress/data event emitted during task execution.
type TaskEvent struct {
	Type        string            `json:"type"`                   // "progress", "data", "replan", "error", "complete"
	Step        int               `json:"step"`
	TotalSteps  int               `json:"total_steps"`
	Message     string            `json:"message,omitempty"`
	Extracted   []json.RawMessage `json:"extracted,omitempty"`
	Count       int               `json:"count,omitempty"`
	Error       string            `json:"error,omitempty"`
	CreditsUsed int               `json:"credits_used,omitempty"`
}

// TaskContext holds stateful context across a multi-step task execution.
type TaskContext struct {
	ID     string `json:"id"`
	Goal   string `json:"goal"`
	Mode   string `json:"mode"`   // research|hardened|max
	Status string `json:"status"` // planning|executing|extracting|paginating|completed|failed

	// Progress
	CurrentStep int          `json:"current_step"`
	TotalSteps  int          `json:"total_steps"`
	StepResults []StepResult `json:"step_results"`

	// Data accumulator
	Extracted    []json.RawMessage `json:"extracted"`
	ExtractCount int               `json:"extract_count"`

	// Pagination
	CurrentPage int  `json:"current_page"`
	TotalPages  int  `json:"total_pages"`
	HasMore     bool `json:"has_more"`

	// Page context for re-planning
	LastPageURL   string `json:"last_page_url"`
	LastPageTitle string `json:"last_page_title"`
	LastAXSummary string `json:"last_ax_summary"`

	// Checkpoint
	CheckpointID string    `json:"checkpoint_id"`
	CheckpointAt time.Time `json:"checkpoint_at"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskPlanner executes multi-step goals with stateful context, progress tracking,
// pagination handling, and re-planning on unexpected page states.
type TaskPlanner interface {
	// PlanAndExecute decomposes a goal into steps and executes them,
	// emitting TaskEvents on the events channel as progress is made.
	// The browser instance is used for all browser operations.
	PlanAndExecute(ctx context.Context, goal string, mode string, inst BrowserInstance, events chan<- TaskEvent) (*TaskContext, error)
}
