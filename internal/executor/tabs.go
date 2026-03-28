// Package executor implements browser action executors for Aperture.
// This file provides NewTabExecutor and SwitchTabExecutor for multi-tab support.
package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// NewTabExecutor opens a new browser tab and returns the tab ID in ActionResult.
// It requires a domain.TabManager injected at construction time.
// Implements domain.Executor.
type NewTabExecutor struct {
	tabs domain.TabManager
}

// NewNewTabExecutor constructs a NewTabExecutor.
// tabs is injected (DI) so callers can substitute mocks in tests.
func NewNewTabExecutor(tabs domain.TabManager) *NewTabExecutor {
	return &NewTabExecutor{tabs: tabs}
}

// Execute opens a new tab.
//
// Supported params:
//   - "url" string — URL to navigate the new tab to (optional; defaults to about:blank)
//
// Returns a non-nil *ActionResult on both success and failure.
// Implements domain.Executor.
func (e *NewTabExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "newTab"}

	url, _ := params["url"].(string)

	// SSRF protection: validate URL before opening new tab.
	if url != "" {
		if err := validateNavigateURL(url); err != nil {
			return failResult(result, start, fmt.Errorf("url_blocked: %w", err)), nil
		}
	}

	tab, err := e.tabs.NewTab(ctx, url)
	if err != nil {
		return failResult(result, start, fmt.Errorf("newTab: %w", err)), nil
	}

	result.Success = true
	result.PageState = &domain.PageState{URL: tab.URL}
	result.Duration = time.Since(start)
	return result, nil
}

// SwitchTabExecutor switches the active browser tab.
// It requires a domain.TabManager injected at construction time.
// Implements domain.Executor.
type SwitchTabExecutor struct {
	tabs domain.TabManager
}

// NewSwitchTabExecutor constructs a SwitchTabExecutor.
// tabs is injected (DI) so callers can substitute mocks in tests.
func NewSwitchTabExecutor(tabs domain.TabManager) *SwitchTabExecutor {
	return &SwitchTabExecutor{tabs: tabs}
}

// Execute switches the focused tab by ID or index.
//
// Supported params:
//   - "tabId"    string — direct target ID of the tab to switch to
//   - "tabIndex" int    — zero-based index into ListTabs result (used when tabId absent)
//
// Returns a non-nil *ActionResult on both success and failure.
// Implements domain.Executor.
func (e *SwitchTabExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "switchTab"}

	tabID, err := resolveTabID(ctx, e.tabs, params)
	if err != nil {
		return failResult(result, start, err), nil
	}

	if err := e.tabs.SwitchTab(ctx, tabID); err != nil {
		return failResult(result, start, fmt.Errorf("switchTab: %w", err)), nil
	}

	result.Success = true
	result.Duration = time.Since(start)
	return result, nil
}

// resolveTabID extracts the tab ID from params["tabId"] or looks up the tab
// at params["tabIndex"] in the current tab list.
func resolveTabID(ctx context.Context, tabs domain.TabManager, params map[string]interface{}) (string, error) {
	if id, ok := params["tabId"].(string); ok && id != "" {
		return id, nil
	}

	idx := 0
	if v, ok := params["tabIndex"].(int); ok {
		idx = v
	}

	list, err := tabs.ListTabs(ctx)
	if err != nil {
		return "", fmt.Errorf("resolveTabID: list tabs: %w", err)
	}
	if idx < 0 || idx >= len(list) {
		return "", fmt.Errorf("resolveTabID: tabIndex %d out of range (have %d tabs)", idx, len(list))
	}
	return list[idx].ID, nil
}
