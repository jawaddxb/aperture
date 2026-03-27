package browser_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/browser"
)

// findTestChromePath returns a chrome path for tests, or skips if unavailable.
func findTestChromePath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	// Check absolute paths.
	if _, err := exec.LookPath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}
	t.Skip("Chrome not available")
	return ""
}

// TestChromeTabManager_OpenNewTab verifies that a new tab appears in ListTabs.
func TestChromeTabManager_OpenNewTab(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tab test in short mode")
	}
	chromePath := findTestChromePath(t)

	pool, err := browser.NewPool(browser.Config{PoolSize: 1, ChromiumPath: chromePath})
	if err != nil {
		t.Skipf("pool creation failed (Chrome unavailable?): %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	inst, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer pool.Release(inst)

	mgr := browser.NewChromeTabManager(inst.Context())

	before, err := mgr.ListTabs(ctx)
	if err != nil {
		t.Fatalf("ListTabs before: %v", err)
	}

	tab, err := mgr.NewTab(ctx, "about:blank")
	if err != nil {
		t.Fatalf("NewTab: %v", err)
	}
	if tab.ID == "" {
		t.Fatal("NewTab returned empty ID")
	}

	after, err := mgr.ListTabs(ctx)
	if err != nil {
		t.Fatalf("ListTabs after: %v", err)
	}

	if len(after) <= len(before) {
		t.Errorf("expected more tabs after NewTab: before=%d, after=%d", len(before), len(after))
	}
}

// TestChromeTabManager_SwitchTab verifies that SwitchTab activates the target tab.
func TestChromeTabManager_SwitchTab(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tab test in short mode")
	}
	chromePath := findTestChromePath(t)

	pool, err := browser.NewPool(browser.Config{PoolSize: 1, ChromiumPath: chromePath})
	if err != nil {
		t.Skipf("pool creation failed: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	inst, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer pool.Release(inst)

	mgr := browser.NewChromeTabManager(inst.Context())

	tab, err := mgr.NewTab(ctx, "about:blank")
	if err != nil {
		t.Fatalf("NewTab: %v", err)
	}

	if err := mgr.SwitchTab(ctx, tab.ID); err != nil {
		t.Fatalf("SwitchTab: %v", err)
	}

	tabs, err := mgr.ListTabs(ctx)
	if err != nil {
		t.Fatalf("ListTabs: %v", err)
	}

	found := false
	for _, tk := range tabs {
		if tk.ID == tab.ID && tk.Active {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("tab %q should be active after SwitchTab", tab.ID)
	}
}

// TestChromeTabManager_CloseTab verifies that CloseTab removes the tab from ListTabs.
func TestChromeTabManager_CloseTab(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tab test in short mode")
	}
	chromePath := findTestChromePath(t)

	pool, err := browser.NewPool(browser.Config{PoolSize: 1, ChromiumPath: chromePath})
	if err != nil {
		t.Skipf("pool creation failed: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inst, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer pool.Release(inst)

	mgr := browser.NewChromeTabManager(inst.Context())

	tab, err := mgr.NewTab(ctx, "about:blank")
	if err != nil {
		t.Fatalf("NewTab: %v", err)
	}

	if err := mgr.CloseTab(ctx, tab.ID); err != nil {
		t.Fatalf("CloseTab: %v", err)
	}

	tabs, err := mgr.ListTabs(ctx)
	if err != nil {
		t.Fatalf("ListTabs after close: %v", err)
	}

	for _, tk := range tabs {
		if tk.ID == tab.ID {
			t.Errorf("closed tab %q still appears in ListTabs", tab.ID)
		}
	}
}
