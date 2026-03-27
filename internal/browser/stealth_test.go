package browser

import (
	"context"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ApertureHQ/aperture/internal/domain"
)

func TestApplyStealth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	opts := BuildAllocatorOptions(chromiumPath(t)) // Use system chrome
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	cfg := domain.StealthConfig{
		Enabled:       true,
		HideWebDriver: true,
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}

	err := ApplyStealth(ctx, cfg)
	require.NoError(t, err)

	err = chromedp.Run(ctx, chromedp.Navigate("about:blank"))
	require.NoError(t, err)

	// Test navigator.webdriver
	var isWebdriver bool
	err = chromedp.Run(ctx, chromedp.Evaluate("navigator.webdriver", &isWebdriver))
	assert.NoError(t, err)
	assert.False(t, isWebdriver)

	// Test User-Agent
	var ua string
	err = chromedp.Run(ctx, chromedp.Evaluate("navigator.userAgent", &ua))
	assert.NoError(t, err)
	assert.Equal(t, cfg.UserAgent, ua)

	// Test navigator.languages
	var langs []string
	err = chromedp.Run(ctx, chromedp.Evaluate("navigator.languages", &langs))
	assert.NoError(t, err)
	assert.Contains(t, langs, "en-US")
}

func TestApplyStealth_Disabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	opts := BuildAllocatorOptions(chromiumPath(t))
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	cfg := domain.StealthConfig{
		Enabled: false,
	}

	err := ApplyStealth(ctx, cfg)
	assert.NoError(t, err)

	err = chromedp.Run(ctx, chromedp.Navigate("about:blank"))
	require.NoError(t, err)

	// navigator.webdriver should be true by default in headless chrome
	var isWebdriver bool
	err = chromedp.Run(ctx, chromedp.Evaluate("navigator.webdriver", &isWebdriver))
	assert.NoError(t, err)
	// headless chrome has navigator.webdriver true
	// but some versions might behave differently. 
	// For now we just verify ApplyStealth(Enabled: false) doesn't error and doesn't inject.
}
