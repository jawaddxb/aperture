package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// screenshotDefaultFormat is used when "format" param is absent.
const screenshotDefaultFormat = "png"

// ScreenshotExecutor captures a screenshot of the current page or a specific
// element. Implements domain.Executor.
type ScreenshotExecutor struct{}

// NewScreenshotExecutor constructs a ScreenshotExecutor.
func NewScreenshotExecutor() *ScreenshotExecutor {
	return &ScreenshotExecutor{}
}

// Execute captures a screenshot and returns it in ActionResult.Data.
//
// Supported params:
//   - "fullPage"  bool   — capture the full scrollable page (default false)
//   - "selector" string  — CSS selector of element to clip to (optional)
//   - "format"   string  — "png" (default) or "jpeg"
//   - "quality"  int     — JPEG quality 0–100 (jpeg only)
//
// Returns a non-nil *ActionResult on both success and failure.
// Implements domain.Executor.
func (e *ScreenshotExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "screenshot"}

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout(params))
	defer cancel()

	opts := screenshotOptsFromParams(params)

	var buf []byte
	var err error

	if opts.selector != "" {
		buf, err = captureElementScreenshot(ctx, inst.Context(), opts)
	} else {
		buf, err = captureViewportScreenshot(ctx, inst.Context(), opts)
	}
	if err != nil {
		return failResult(result, start, fmt.Errorf("screenshot: %w", err)), nil
	}

	pageState, err := capturePageState(inst.Context())
	if err != nil {
		return failResult(result, start, fmt.Errorf("screenshot: page state: %w", err)), nil
	}

	result.Success = true
	result.PageState = pageState
	result.Data = buf
	result.Duration = time.Since(start)
	return result, nil
}

// screenshotOpts holds parsed screenshot parameters.
type screenshotOpts struct {
	fullPage bool
	selector string
	format   string
	quality  int
}

// screenshotOptsFromParams parses executor params into screenshotOpts.
func screenshotOptsFromParams(params map[string]interface{}) screenshotOpts {
	opts := screenshotOpts{
		format:  screenshotDefaultFormat,
		quality: 80,
	}
	if v, ok := params["fullPage"].(bool); ok {
		opts.fullPage = v
	}
	if v, ok := params["selector"].(string); ok {
		opts.selector = v
	}
	if v, ok := params["format"].(string); ok {
		opts.format = v
	}
	if v, ok := params["quality"].(int); ok {
		opts.quality = v
	}
	return opts
}

// captureViewportScreenshot takes a viewport or full-page screenshot.
func captureViewportScreenshot(ctx, browserCtx context.Context, opts screenshotOpts) ([]byte, error) {
	var buf []byte

	if opts.fullPage {
		return captureFullPageScreenshot(ctx, browserCtx, opts)
	}

	format := page.CaptureScreenshotFormatPng
	if opts.format == "jpeg" {
		format = page.CaptureScreenshotFormatJpeg
	}

	action := chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().
			WithFormat(format).
			WithQuality(int64(opts.quality)).
			Do(ctx)
		return err
	})

	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()

	if err := chromedp.Run(runCtx, action); err != nil {
		return nil, err
	}
	return buf, nil
}

// captureFullPageScreenshot expands the viewport to full page size and screenshots.
func captureFullPageScreenshot(ctx, browserCtx context.Context, opts screenshotOpts) ([]byte, error) {
	var buf []byte

	format := page.CaptureScreenshotFormatPng
	if opts.format == "jpeg" {
		format = page.CaptureScreenshotFormatJpeg
	}

	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()

	err := chromedp.Run(runCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Get full page dimensions.
			var width, height int64
			err := chromedp.Run(ctx, chromedp.Evaluate(
				`[document.documentElement.scrollWidth, document.documentElement.scrollHeight]`,
				&[]int64{width, height},
			))
			if err != nil {
				return err
			}
			// Override viewport to full page size.
			return emulation.SetDeviceMetricsOverride(width, height, 1, false).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().
				WithFormat(format).
				WithQuality(int64(opts.quality)).
				Do(ctx)
			return err
		}),
		// Restore default viewport.
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.ClearDeviceMetricsOverride().Do(ctx)
		}),
	)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// elementClipViewport resolves the bounding box of the element matching
// selector and returns it as a page.Viewport clip region.
func elementClipViewport(ctx context.Context, selector string) (*page.Viewport, error) {
	var box []float64
	script := fmt.Sprintf(
		`(function(){
			var el=document.querySelector(%q);
			if(!el){return null;}
			var r=el.getBoundingClientRect();
			return [r.left+window.scrollX, r.top+window.scrollY, r.width, r.height];
		})()`,
		selector,
	)
	if err := chromedp.Evaluate(script, &box).Do(ctx); err != nil {
		return nil, err
	}
	if len(box) < 4 {
		return nil, fmt.Errorf("element %q not found or has no bounding box", selector)
	}
	return &page.Viewport{
		X: box[0], Y: box[1], Width: box[2], Height: box[3], Scale: 1,
	}, nil
}

// captureElementScreenshot clips the screenshot to a specific element's bounding box.
func captureElementScreenshot(ctx, browserCtx context.Context, opts screenshotOpts) ([]byte, error) {
	var buf []byte

	format := page.CaptureScreenshotFormatPng
	if opts.format == "jpeg" {
		format = page.CaptureScreenshotFormatJpeg
	}

	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()

	err := chromedp.Run(runCtx,
		chromedp.WaitVisible(opts.selector, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			clip, err := elementClipViewport(ctx, opts.selector)
			if err != nil {
				return err
			}
			buf, err = page.CaptureScreenshot().
				WithFormat(format).
				WithQuality(int64(opts.quality)).
				WithClip(clip).
				Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
