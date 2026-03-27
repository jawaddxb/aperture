package browser_test

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// TestNetworkManager_Block verifies that a specific URL pattern can be blocked.
func TestNetworkManager_Block(t *testing.T) {
	p, err := browser.NewPool(browser.Config{
		PoolSize:     1,
		ChromiumPath: chromiumPath(t),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer p.Close()

	inst, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer p.Release(inst)

	nm := inst.Network()
	err = nm.SetRules(inst.Context(), []domain.NetworkRule{
		{URLPattern: "blocked-url", Action: "block"},
	})
	if err != nil {
		t.Fatalf("SetRules failed: %v", err)
	}

	// Navigate to a real page first to establish an origin.
	if err := chromedp.Run(inst.Context(), chromedp.Navigate("http://example.com")); err != nil {
		t.Fatalf("failed to navigate to example.com: %v", err)
	}

	// Inject a script that fetches the blocked URL.
	script := `fetch('/blocked-url').catch(e => window._blocked = true)`
	if err := chromedp.Run(inst.Context(), chromedp.Evaluate(script, nil)); err != nil {
		t.Fatalf("failed to evaluate script: %v", err)
	}

	// Check if the fetch was blocked.
	var blocked bool
	for i := 0; i < 50; i++ {
		err = chromedp.Run(inst.Context(), chromedp.Evaluate("window._blocked || false", &blocked))
		if err == nil && blocked {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !blocked {
		t.Error("fetch was not blocked")
	}
}

// TestNetworkManager_Mock verifies that an API response can be mocked.
func TestNetworkManager_Mock(t *testing.T) {
	p, err := browser.NewPool(browser.Config{
		PoolSize:     1,
		ChromiumPath: chromiumPath(t),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer p.Close()

	inst, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer p.Release(inst)

	nm := inst.Network()
	mockBody := `{"status":"ok", "data": "mocked"}`
	err = nm.SetRules(inst.Context(), []domain.NetworkRule{
		{URLPattern: "/api/data", Action: "mock", MockBody: mockBody},
	})
	if err != nil {
		t.Fatalf("SetRules failed: %v", err)
	}

	// Navigate to a real page first to establish an origin.
	if err := chromedp.Run(inst.Context(), chromedp.Navigate("http://example.com")); err != nil {
		t.Fatalf("failed to navigate to example.com: %v", err)
	}

	// Inject a script that fetches the mocked URL.
	script := `fetch('/api/data').then(r => r.json()).then(d => window._data = d.data).catch(e => window._data = 'error: ' + e)`
	if err := chromedp.Run(inst.Context(), chromedp.Evaluate(script, nil)); err != nil {
		t.Fatalf("failed to evaluate script: %v", err)
	}

	// Check if the mocked data was received.
	var data string
	for i := 0; i < 50; i++ {
		err = chromedp.Run(inst.Context(), chromedp.Evaluate("window._data || ''", &data))
		if err == nil && data != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if data != "mocked" {
		t.Errorf("received data = %q, want %q", data, "mocked")
	}
}
