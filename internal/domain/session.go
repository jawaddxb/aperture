// Package domain defines core interfaces for Aperture.
// This file defines Session, SessionManager and ScreenshotService.
package domain

import (
	"context"
	"errors"
	"time"
)

// ErrSessionNotFound is returned when the requested session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// ErrConcurrentLimitExceeded is returned when the concurrent session limit is reached.
var ErrConcurrentLimitExceeded = errors.New("concurrent session limit exceeded")

// ErrAccountSessionLimit is returned when a single account exceeds its per-account session limit.
var ErrAccountSessionLimit = errors.New("account session limit exceeded")

// Session tracks browser automation state for a single goal.
type Session struct {
	// ID is the UUID v4 identifier for this session.
	ID string

	// AccountID is the billing account that owns this session.
	// Empty in dev mode (no billing). When set, access is restricted to this account.
	AccountID string

	// Status is one of: "active", "paused", "completed", "failed".
	Status string

	// BrowserID is the ID of the browser instance from the pool.
	BrowserID string

	// Goal is the original natural-language intent.
	Goal string

	// Plan is the decomposed execution plan (nil until Execute is called).
	Plan *Plan

	// Results holds per-step outcomes (nil until Execute is called).
	Results []*StepResult

	// CurrentURL is the URL of the last successfully navigated page.
	CurrentURL string

	// CurrentTitle is the title of the last successfully navigated page.
	CurrentTitle string

	// CreatedAt records when the session was created.
	CreatedAt time.Time

	// UpdatedAt records the last modification time.
	UpdatedAt time.Time

	// Metadata holds arbitrary key-value annotations.
	Metadata map[string]string

	// TrustMode controls fingerprint randomisation for this session.
	// "standard" applies normal stealth; "preserve" locks the imported profile's fingerprint.
	TrustMode TrustMode

	// ImportedProfile is the profile ID created during cookie import, e.g. "imported-linkedin-1234".
	ImportedProfile string

	// ImportedDomains lists the domains covered by the imported cookies.
	ImportedDomains []string

	// LLMCallCount tracks the total number of LLM API calls made during this session.
	LLMCallCount int
}

// SessionManager manages the lifecycle of browser sessions.
// Implementations must be safe for concurrent use.
type SessionManager interface {
	// Create creates a new session with the given goal and acquires a browser.
	// meta is optional key-value annotations (e.g. "agent_id" for xBPP policy lookup).
	Create(ctx context.Context, goal string, meta map[string]string) (*Session, error)

	// Get retrieves a session by ID.
	Get(ctx context.Context, id string) (*Session, error)

	// List returns all managed sessions.
	List(ctx context.Context) ([]*Session, error)

	// Update persists changes to an existing session.
	Update(ctx context.Context, session *Session) error

	// Delete removes a session and releases its browser back to the pool.
	Delete(ctx context.Context, id string) error

	// Execute runs the full goal: plan → sequence → update status.
	Execute(ctx context.Context, id string) (*RunResult, error)
}

// ScreenshotService captures a screenshot of a URL.
// Implementations drive a real or stubbed browser.
type ScreenshotService interface {
	// Screenshot navigates to url and returns PNG bytes.
	Screenshot(ctx context.Context, url string, fullPage bool) ([]byte, error)
}
