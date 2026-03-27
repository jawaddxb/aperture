// Package domain defines core interfaces for Aperture.
// All services depend on these interfaces, never on concrete implementations.
package domain

import (
	"context"
	"time"
)

// BrowserPool manages a pool of headless Chromium instances.
// Implementations must be safe for concurrent use.
type BrowserPool interface {
	// Acquire returns an available BrowserInstance from the pool.
	// Blocks up to 10 seconds if the pool is exhausted.
	// Returns ErrPoolExhausted if no instance becomes available in time.
	Acquire(ctx context.Context) (BrowserInstance, error)

	// Release returns a BrowserInstance to the pool after clearing its state.
	// The instance must have been obtained from this pool via Acquire.
	Release(instance BrowserInstance)

	// Size returns the configured maximum pool size.
	Size() int

	// Available returns the number of instances currently available for acquisition.
	Available() int

	// Close shuts down all instances in the pool and releases resources.
	Close() error
}

// BrowserInstance wraps a single headless Chromium context.
// Callers interact exclusively with this interface; raw chromedp types are hidden.
type BrowserInstance interface {
	// Context returns the chromedp context for this instance.
	// Use this context with chromedp.Run(...) to execute browser actions.
	Context() context.Context

	// ID returns a unique identifier for this instance.
	ID() string

	// CreatedAt returns when this instance was launched.
	CreatedAt() time.Time

	// IsAlive reports whether the underlying Chromium process is healthy.
	IsAlive() bool

	// Close terminates the Chromium process and releases all associated resources.
	Close() error
}

// ErrPoolExhausted is returned by BrowserPool.Acquire when all instances
// are in use and no instance becomes available within the timeout.
type ErrPoolExhausted struct {
	PoolSize int
	Waited   time.Duration
}

func (e *ErrPoolExhausted) Error() string {
	return "browser pool exhausted: all " +
		itoa(e.PoolSize) + " instances are in use (waited " + e.Waited.String() + ")"
}

// itoa converts an int to string without importing strconv in the domain layer.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// Tab represents a single browser tab (CDP target).
type Tab struct {
	// ID is the CDP Target ID for this tab.
	ID string
	// URL is the current URL of the tab.
	URL string
	// Title is the document title of the tab.
	Title string
	// Active reports whether this tab is the currently focused tab.
	Active bool
}

// TabManager manages browser tabs via CDP.
// Implementations must be safe for concurrent use.
type TabManager interface {
	// ListTabs returns all open tabs in the browser.
	ListTabs(ctx context.Context) ([]Tab, error)

	// NewTab opens a new tab navigated to url.
	// Pass an empty url to open a blank tab.
	NewTab(ctx context.Context, url string) (*Tab, error)

	// SwitchTab makes the tab with the given tabID the active focused tab.
	SwitchTab(ctx context.Context, tabID string) error

	// CloseTab closes the tab identified by tabID.
	CloseTab(ctx context.Context, tabID string) error

	// WaitForNewTab waits up to timeout for a new tab to open and returns it.
	// Returns an error if no new tab appears before timeout.
	WaitForNewTab(ctx context.Context, timeout time.Duration) (*Tab, error)
}
