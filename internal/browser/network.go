package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ChromeNetworkManager implements domain.NetworkManager for Chrome.
// It uses the CDP Fetch domain to intercept and control network requests.
type ChromeNetworkManager struct {
	mu    sync.RWMutex
	rules []domain.NetworkRule
}

// NewChromeNetworkManager constructs a new ChromeNetworkManager.
func NewChromeNetworkManager() *ChromeNetworkManager {
	return &ChromeNetworkManager{}
}

// SetRules configures the manager with a set of interception rules.
// It enables the Fetch domain and sets up a listener for requestPaused events.
func (m *ChromeNetworkManager) SetRules(ctx context.Context, rules []domain.NetworkRule) error {
	m.mu.Lock()
	m.rules = rules
	m.mu.Unlock()

	// Enable Fetch.enable with all resource types.
	// We use patterns to match everything and then filter in the listener.
	err := chromedp.Run(ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
		{URLPattern: "*"},
	}))
	if err != nil {
		return fmt.Errorf("network: enable fetch: %w", err)
	}

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if e, ok := ev.(*fetch.EventRequestPaused); ok {
			go m.handleRequestPaused(ctx, e)
		}
	})

	return nil
}

// handleRequestPaused decides whether to continue, block, or mock a request.
func (m *ChromeNetworkManager) handleRequestPaused(ctx context.Context, e *fetch.EventRequestPaused) {
	m.mu.RLock()
	rules := m.rules
	m.mu.RUnlock()

	for _, rule := range rules {
		if !strings.Contains(e.Request.URL, rule.URLPattern) {
			continue
		}

		switch rule.Action {
		case "block":
			_ = chromedp.Run(ctx, fetch.FailRequest(e.RequestID, network.ErrorReasonAborted))
			return
		case "mock":
			_ = chromedp.Run(ctx, fetch.FulfillRequest(e.RequestID, 200).
				WithBody(base64.StdEncoding.EncodeToString([]byte(rule.MockBody))).
				WithResponseHeaders([]*fetch.HeaderEntry{
					{Name: "Content-Type", Value: "application/json"},
				}))
			return
		}
	}

	// No rule matched; continue normally.
	_ = chromedp.Run(ctx, fetch.ContinueRequest(e.RequestID))
}
