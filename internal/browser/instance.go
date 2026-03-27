package browser

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// instance implements domain.BrowserInstance.
// Each instance owns exactly one Chromium tab (one chromedp context chain).
type instance struct {
	id          string
	createdAt   time.Time
	allocCtx    context.Context // allocator-level context (owns the OS process)
	allocCancel context.CancelFunc
	tabCtx      context.Context // tab-level context (single tab)
	tabCancel   context.CancelFunc
	closed      atomic.Bool
	stealth     domain.StealthConfig
	profileID   string
	downloads   domain.DownloadManager
	network     domain.NetworkManager

	proxyMu   sync.Mutex
	proxyUser string
	proxyPass string
}

// newInstance launches a single Chromium process and opens one tab.
// allocCtx must be a chromedp allocator context.
func newInstance(allocCtx context.Context, allocCancel context.CancelFunc, id string, stealth domain.StealthConfig, profileID string) (*instance, error) {
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)

	// Apply stealth before initial navigation.
	if err := ApplyStealth(tabCtx, stealth); err != nil {
		tabCancel()
		allocCancel()
		return nil, fmt.Errorf("instance %s: failed to apply stealth: %w", id, err)
	}

	// Navigate to blank page to confirm the browser is alive.
	if err := chromedp.Run(tabCtx, chromedp.Navigate("about:blank")); err != nil {
		tabCancel()
		allocCancel()
		return nil, fmt.Errorf("instance %s: initial navigation failed: %w", id, err)
	}

	return &instance{
		id:          id,
		createdAt:   time.Now(),
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		tabCtx:      tabCtx,
		tabCancel:   tabCancel,
		stealth:     stealth,
		profileID:   profileID,
		downloads:   NewChromeDownloadManager(),
		network:     NewChromeNetworkManager(),
	}, nil
}

// Context returns the chromedp tab context for this instance.
func (i *instance) Context() context.Context {
	return i.tabCtx
}

// ID returns the unique identifier for this instance.
func (i *instance) ID() string {
	return i.id
}

// CreatedAt returns when this instance was launched.
func (i *instance) CreatedAt() time.Time {
	return i.createdAt
}

// IsAlive reports whether the underlying Chromium process is still running.
// It checks if the allocator context has been cancelled or the OS process is gone.
func (i *instance) IsAlive() bool {
	if i.closed.Load() {
		return false
	}
	select {
	case <-i.allocCtx.Done():
		return false
	default:
		return true
	}
}

// Downloads returns the download manager for this instance.
func (i *instance) Downloads() domain.DownloadManager {
	return i.downloads
}

// Network returns the network manager for this instance.
func (i *instance) Network() domain.NetworkManager {
	return i.network
}

// Close terminates the Chromium process and releases all associated resources.
// Safe to call multiple times; subsequent calls are no-ops.
func (i *instance) Close() error {
	if i.closed.Swap(true) {
		return nil // already closed
	}
	i.tabCancel()
	i.allocCancel()
	return nil
}

// reset clears tab state so the instance can be safely reused.
// It replaces the tab context with a fresh one, discarding previous session data.
func (i *instance) reset() error {
	i.tabCancel()
	tabCtx, tabCancel := chromedp.NewContext(i.allocCtx)

	// Re-apply stealth to new context.
	if err := ApplyStealth(tabCtx, i.stealth); err != nil {
		tabCancel()
		return fmt.Errorf("instance %s: failed to re-apply stealth on reset: %w", i.id, err)
	}

	if err := chromedp.Run(tabCtx, chromedp.Navigate("about:blank")); err != nil {
		tabCancel()
		return fmt.Errorf("instance %s: reset navigation failed: %w", i.id, err)
	}
	i.tabCtx = tabCtx
	i.tabCancel = tabCancel
	return nil
}

// setProxyAuth configures proxy credentials for this instance.
func (i *instance) setProxyAuth(user, pass string) {
	i.proxyMu.Lock()
	i.proxyUser = user
	i.proxyPass = pass
	i.proxyMu.Unlock()

	// Enable Fetch.enable to intercept authentication requests.
	chromedp.ListenTarget(i.tabCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			go func() {
				i.proxyMu.Lock()
				user := i.proxyUser
				pass := i.proxyPass
				i.proxyMu.Unlock()

				if user == "" && pass == "" {
					_ = chromedp.Run(i.tabCtx, fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
						Response: fetch.AuthChallengeResponseResponseDefault,
					}))
					return
				}

				_ = chromedp.Run(i.tabCtx, fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
					Response: fetch.AuthChallengeResponseResponseProvideCredentials,
					Username: user,
					Password: pass,
				}))
			}()
		}
	})

	_ = chromedp.Run(i.tabCtx, fetch.Enable().WithHandleAuthRequests(true))
}
