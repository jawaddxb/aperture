package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// ScreenshotService implements domain.ScreenshotService using a BrowserPool.
type ScreenshotService struct {
	pool domain.BrowserPool
}

// NewScreenshotService constructs a ScreenshotService backed by pool.
func NewScreenshotService(pool domain.BrowserPool) *ScreenshotService {
	return &ScreenshotService{pool: pool}
}

// Screenshot acquires a browser instance, navigates to url, and captures a PNG.
// If fullPage is true, the full scrollable page is captured.
func (s *ScreenshotService) Screenshot(ctx context.Context, url string, fullPage bool) ([]byte, error) {
	inst, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire: %w", err)
	}
	defer s.pool.Release(inst)

	var buf []byte
	bctx := inst.Context()

	if err := chromedp.Run(bctx, chromedp.Navigate(url)); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

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
