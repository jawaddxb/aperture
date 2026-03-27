package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ScreenshotService implements domain.ScreenshotService using a BrowserPool.
// It detects Cloudflare Turnstile challenges and automatically falls back
// to Scrapling's StealthyFetcher when the native browser is blocked.
type ScreenshotService struct {
	pool     domain.BrowserPool
	toolsDir string // path to tools/ directory containing scrapling_fallback.py
}

// NewScreenshotService constructs a ScreenshotService backed by pool.
func NewScreenshotService(pool domain.BrowserPool) *ScreenshotService {
	return &ScreenshotService{
		pool:     pool,
		toolsDir: resolveToolsDir(),
	}
}

// Screenshot acquires a browser instance, navigates to url, and captures a PNG.
// If a Cloudflare Turnstile challenge is detected, it falls back to Scrapling.
func (s *ScreenshotService) Screenshot(ctx context.Context, url string, fullPage bool) ([]byte, error) {
	inst, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire: %w", err)
	}
	defer s.pool.Release(inst)

	bctx := inst.Context()

	if err := chromedp.Run(bctx, chromedp.Navigate(url)); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Wait a moment for any Turnstile challenge to appear.
	time.Sleep(2 * time.Second)

	// Check for Cloudflare challenge — if detected, try waiting then fallback.
	if blocked, _ := isTurnstilePresent(bctx); blocked {
		slog.Info("cloudflare challenge detected, attempting wait", "url", url)

		// Give Turnstile up to 10s to auto-resolve with our stealth.
		waitErr := WaitForTurnstile(bctx, 10*time.Second)
		if waitErr != nil {
			slog.Info("turnstile did not resolve, falling back to scrapling", "url", url)
			return s.scraplingFallback(ctx, url)
		}
		slog.Info("turnstile resolved natively", "url", url)
	}

	var buf []byte
	if fullPage {
		if err := chromedp.Run(bctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
			return nil, fmt.Errorf("full screenshot: %w", err)
		}
	} else {
		if err := chromedp.Run(bctx, chromedp.CaptureScreenshot(&buf)); err != nil {
			return nil, fmt.Errorf("screenshot: %w", err)
		}
	}

	return buf, nil
}

// scraplingFallback calls the Python Scrapling script and returns the screenshot bytes.
func (s *ScreenshotService) scraplingFallback(ctx context.Context, url string) ([]byte, error) {
	result, err := FallbackFetch(ctx, s.toolsDir, url, true, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("scrapling fallback: %w", err)
	}

	if result.ScreenshotB64 == "" {
		return nil, fmt.Errorf("scrapling fallback: no screenshot returned")
	}

	data, err := base64.StdEncoding.DecodeString(result.ScreenshotB64)
	if err != nil {
		return nil, fmt.Errorf("scrapling fallback decode: %w", err)
	}

	slog.Info("scrapling fallback succeeded", "url", url, "bytes", len(data))
	return data, nil
}

// resolveToolsDir finds the tools/ directory relative to this source file.
func resolveToolsDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "tools"
	}
	// screenshot.go is in internal/browser/, tools/ is at repo root
	return filepath.Join(filepath.Dir(filename), "..", "..", "tools")
}
