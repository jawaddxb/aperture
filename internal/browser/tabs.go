// Package browser provides Chromium lifecycle management for Aperture.
// This file implements ChromeTabManager: open, switch, close, and list tabs.
package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// ChromeTabManager implements domain.TabManager via Chrome DevTools Protocol.
// It operates on the allocator-level context (browser-level), not a single tab.
type ChromeTabManager struct {
	// browserCtx is the chromedp allocator context (browser-level).
	browserCtx context.Context
	// activeTabID tracks the currently active tab ID.
	activeTabID string
}

// NewChromeTabManager constructs a ChromeTabManager for the given browser context.
// browserCtx must be a chromedp allocator (or browser-level) context.
func NewChromeTabManager(browserCtx context.Context) *ChromeTabManager {
	return &ChromeTabManager{browserCtx: browserCtx}
}

// ListTabs returns all open "page" tabs in the browser.
func (m *ChromeTabManager) ListTabs(ctx context.Context) ([]domain.Tab, error) {
	infos, err := m.getTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListTabs: %w", err)
	}
	return convertInfosToTabs(infos, m.activeTabID), nil
}

// NewTab opens a new browser tab navigated to url.
// Pass an empty string for a blank tab (about:blank).
func (m *ChromeTabManager) NewTab(ctx context.Context, url string) (*domain.Tab, error) {
	if url == "" {
		url = "about:blank"
	}

	var targetID cdptarget.ID
	err := chromedp.Run(m.browserCtx,
		chromedp.ActionFunc(func(c context.Context) error {
			var err error
			targetID, err = cdptarget.CreateTarget(url).Do(c)
			return err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("NewTab: create target: %w", err)
	}

	tab := &domain.Tab{
		ID:  string(targetID),
		URL: url,
	}
	return tab, nil
}

// SwitchTab activates (focuses) the tab with the given tabID.
func (m *ChromeTabManager) SwitchTab(ctx context.Context, tabID string) error {
	err := chromedp.Run(m.browserCtx,
		chromedp.ActionFunc(func(c context.Context) error {
			return cdptarget.ActivateTarget(cdptarget.ID(tabID)).Do(c)
		}),
	)
	if err != nil {
		return fmt.Errorf("SwitchTab %q: %w", tabID, err)
	}
	m.activeTabID = tabID
	return nil
}

// CloseTab closes the tab with the given tabID.
func (m *ChromeTabManager) CloseTab(ctx context.Context, tabID string) error {
	err := chromedp.Run(m.browserCtx,
		chromedp.ActionFunc(func(c context.Context) error {
			return cdptarget.CloseTarget(cdptarget.ID(tabID)).Do(c)
		}),
	)
	if err != nil {
		return fmt.Errorf("CloseTab %q: %w", tabID, err)
	}
	if m.activeTabID == tabID {
		m.activeTabID = ""
	}
	return nil
}

// WaitForNewTab blocks until a new tab is created or timeout elapses.
// It uses chromedp's target discovery event stream.
func (m *ChromeTabManager) WaitForNewTab(ctx context.Context, timeout time.Duration) (*domain.Tab, error) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ch := make(chan *domain.Tab, 1)
	listenCtx, stopListen := context.WithCancel(m.browserCtx)
	defer stopListen()

	chromedp.ListenTarget(listenCtx, func(ev interface{}) {
		e, ok := ev.(*cdptarget.EventTargetCreated)
		if !ok {
			return
		}
		if e.TargetInfo.Type != "page" {
			return
		}
		tab := &domain.Tab{
			ID:    string(e.TargetInfo.TargetID),
			URL:   e.TargetInfo.URL,
			Title: e.TargetInfo.Title,
		}
		select {
		case ch <- tab:
		default:
		}
	})

	// Enable target discovery so events are emitted.
	if err := enableTargetDiscovery(m.browserCtx); err != nil {
		return nil, fmt.Errorf("WaitForNewTab: enable discovery: %w", err)
	}

	select {
	case tab := <-ch:
		return tab, nil
	case <-tctx.Done():
		return nil, fmt.Errorf("WaitForNewTab: timeout after %s", timeout)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// getTargets retrieves all CDP "page" targets.
func (m *ChromeTabManager) getTargets(ctx context.Context) ([]*cdptarget.Info, error) {
	var infos []*cdptarget.Info
	err := chromedp.Run(m.browserCtx,
		chromedp.ActionFunc(func(c context.Context) error {
			var err error
			infos, err = cdptarget.GetTargets().Do(c)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}

	pages := make([]*cdptarget.Info, 0, len(infos))
	for _, info := range infos {
		if info.Type == "page" {
			pages = append(pages, info)
		}
	}
	return pages, nil
}

// enableTargetDiscovery sends the SetDiscoverTargets CDP command.
func enableTargetDiscovery(browserCtx context.Context) error {
	return chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(c context.Context) error {
			return cdptarget.SetDiscoverTargets(true).Do(c)
		}),
	)
}

// convertInfosToTabs maps CDP target infos to domain.Tab values.
func convertInfosToTabs(infos []*cdptarget.Info, activeID string) []domain.Tab {
	tabs := make([]domain.Tab, 0, len(infos))
	for _, info := range infos {
		tabs = append(tabs, domain.Tab{
			ID:     string(info.TargetID),
			URL:    info.URL,
			Title:  info.Title,
			Active: string(info.TargetID) == activeID,
		})
	}
	return tabs
}

// compile-time interface assertion.
var _ domain.TabManager = (*ChromeTabManager)(nil)
