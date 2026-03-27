package browser

import (
	"context"
	"os"
	"runtime"
	"testing"

	cdpnetwork "github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chromiumPath resolves the Chromium binary path for integration tests.
// Tests are skipped when no binary is found.
func chromiumPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("APERTURE_CHROMIUM_PATH"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	case "linux":
		for _, p := range []string{
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/google-chrome",
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	t.Skip("no Chromium binary found; set APERTURE_CHROMIUM_PATH to enable")
	return ""
}

// TestProfileManager_CreateDelete verifies profile creation and deletion via FileProfileManager.
func TestProfileManager_CreateDelete(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "aperture-profiles-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	m, err := NewFileProfileManager(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	profileID := "test-user"

	profile, err := m.CreateProfile(ctx, profileID)
	require.NoError(t, err)
	assert.Equal(t, profileID, profile.ID)
	assert.DirExists(t, profile.Path)

	err = m.DeleteProfile(ctx, profileID)
	require.NoError(t, err)
	assert.NoDirExists(t, profile.Path)
}

// TestProfileManager keeps the original name for backward compat.
func TestProfileManager(t *testing.T) { TestProfileManager_CreateDelete(t) }

// TestPool_ProfileIsolation verifies that two pool instances using different
// profile IDs cannot see each other's in-session cookies.
// This test requires a real Chrome binary (skipped in -short mode).
func TestPool_ProfileIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "aperture-profiles-pool-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	m, err := NewFileProfileManager(tempDir)
	require.NoError(t, err)

	// Use PoolSize=2 so both profiles can be live simultaneously.
	cfg := Config{
		PoolSize:       2,
		ChromiumPath:   chromiumPath(t),
		SkipPreWarm:    false,
		ProfileManager: m,
	}

	p, err := NewPool(cfg)
	require.NoError(t, err)
	defer p.Close()

	ctx := context.Background()

	// Acquire two instances with distinct profiles concurrently.
	instA, err := p.Acquire(ctx, "user-a")
	require.NoError(t, err)
	defer p.Release(instA)

	instB, err := p.Acquire(ctx, "user-b")
	require.NoError(t, err)
	defer p.Release(instB)

	// Verify the instances are genuinely distinct.
	assert.NotEqual(t, instA.ID(), instB.ID(), "two acquired instances must be different")

	// Set a CDP session cookie in profile A's tab.
	const testURL = "https://example.com"
	err = chromedp.Run(instA.Context(),
		chromedp.Navigate(testURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return cdpnetwork.SetCookie("aperture-profile-test", "val-a").
				WithDomain("example.com").
				Do(ctx)
		}),
	)
	require.NoError(t, err)

	// Profile B must not see profile A's cookie.
	var cookiesB []*cdpnetwork.Cookie
	err = chromedp.Run(instB.Context(),
		chromedp.Navigate(testURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookiesB, err = cdpnetwork.GetCookies().Do(ctx)
			return err
		}),
	)
	require.NoError(t, err)

	for _, c := range cookiesB {
		if c.Name == "aperture-profile-test" {
			t.Errorf("profile B should not see cookie from profile A, but found value=%q", c.Value)
		}
	}

	// Profile A must still have its own cookie within the same session.
	var cookiesA []*cdpnetwork.Cookie
	err = chromedp.Run(instA.Context(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookiesA, err = cdpnetwork.GetCookies().Do(ctx)
			return err
		}),
	)
	require.NoError(t, err)

	foundA := false
	for _, c := range cookiesA {
		if c.Name == "aperture-profile-test" && c.Value == "val-a" {
			foundA = true
		}
	}
	assert.True(t, foundA, "profile A must retain its own cookie within the same session")
}
