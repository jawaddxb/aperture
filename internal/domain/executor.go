// Package domain defines core interfaces for Aperture.
// This file defines the Executor interface and shared action result types.
package domain

import (
	"context"
	"time"
)

// ActionResult holds the outcome of any executed browser action.
// Error is empty on success; inspect Success to branch without type-asserting.
type ActionResult struct {
	// Action is the name of the executed action: "navigate", "click", "type", etc.
	Action string

	// Success reports whether the action completed without error.
	Success bool

	// Element is the resolved element that was acted on.
	// Nil for navigate actions which target a URL rather than a DOM element.
	Element *Candidate

	// PageState describes the browser page after the action completes.
	PageState *PageState

	// Duration is the wall-clock time from action start to completion.
	Duration time.Duration

	// Error is the human-readable failure reason. Empty on success.
	Error string

	// Data holds raw binary output from the action (e.g. screenshot bytes).
	// Populated by ScreenshotExecutor; nil for other action types.
	Data []byte
}

// PageState captures a snapshot of browser page metadata after an action.
type PageState struct {
	// URL is the final URL after any redirects.
	URL string

	// Title is the document.title at the time of capture.
	Title string

	// StatusCode is the HTTP status code of the main frame navigation.
	// May be 0 when the page was not navigated (e.g. after a click that
	// does not trigger a new navigation).
	StatusCode int

	// ProfileMatched is the domain pattern of the matched site profile, if any.
	ProfileMatched string `json:"profile_matched,omitempty"`

	// StructuredData holds extracted fields from a matched site profile.
	StructuredData map[string]interface{} `json:"structured_data,omitempty"`

	// AvailableActions lists semantic actions available on the matched page.
	AvailableActions []string `json:"available_actions,omitempty"`
}

// Executor executes a single browser action inside a live BrowserInstance.
// Implementations are created per-action-type (navigate, click, type, etc.)
// and receive dependencies via their constructor (dependency inversion).
//
// params keys are action-specific; each executor documents its own parameter set.
type Executor interface {
	// Execute runs the action and returns its result.
	// ctx controls the overall deadline; implementations should respect it.
	// Returns a non-nil *ActionResult even on failure (Success=false, Error set).
	Execute(ctx context.Context, inst BrowserInstance, params map[string]interface{}) (*ActionResult, error)
}
