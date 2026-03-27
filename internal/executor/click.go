package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// maxClickRetries is the number of JS-fallback attempts after interception.
const maxClickRetries = 2

// ClickExecutor clicks an element resolved by a domain.UnifiedResolver.
// It scrolls the element into view, waits for visibility, and falls back to
// a JS click if a native click is intercepted. Implements domain.Executor.
type ClickExecutor struct {
	resolver domain.UnifiedResolver
}

// NewClickExecutor constructs a ClickExecutor with the given resolver.
// resolver is injected (DI) so callers can substitute mocks in tests.
func NewClickExecutor(resolver domain.UnifiedResolver) *ClickExecutor {
	return &ClickExecutor{resolver: resolver}
}

// Execute resolves the target element and clicks it.
//
// Supported params:
//   - "text"     string — visible text / accessible name to resolve
//   - "role"     string — optional WAI-ARIA role filter
//   - "selector" string — optional CSS selector override
//   - "timeout"  time.Duration — override default 30 s timeout
//
// Returns a non-nil *ActionResult on both success and failure.
// Implements domain.Executor.
func (e *ClickExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "click"}

	timeout := defaultNavigateTimeout
	if v, ok := params["timeout"]; ok {
		if d, ok := v.(time.Duration); ok {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := buildResolutionTarget(params)

	resolution, err := e.resolver.Resolve(ctx, target, inst)
	if err != nil {
		return failResult(result, start, fmt.Errorf("click: resolve: %w", err)), nil
	}

	if len(resolution.Candidates) == 0 {
		return failResult(result, start, fmt.Errorf("click: no candidates for %+v", target)), nil
	}

	candidate := resolution.Candidates[0]

	if err := clickElement(ctx, inst.Context(), candidate); err != nil {
		return failResult(result, start, fmt.Errorf("click: %w", err)), nil
	}

	pageState, err := capturePageState(inst.Context())
	if err != nil {
		return failResult(result, start, fmt.Errorf("click: capture page state: %w", err)), nil
	}

	result.Success = true
	result.Element = &candidate
	result.PageState = pageState
	result.Duration = time.Since(start)
	return result, nil
}

// clickElement scrolls to the candidate, waits for visibility, then clicks.
// Falls back to a JS click if a native click fails (overlay interception).
func clickElement(ctx context.Context, browserCtx context.Context, candidate domain.Candidate) error {
	sel := selectorForCandidate(candidate)

	if err := scrollIntoView(browserCtx, sel); err != nil {
		return fmt.Errorf("scroll into view: %w", err)
	}

	if err := waitVisible(ctx, browserCtx, sel); err != nil {
		return fmt.Errorf("wait visible: %w", err)
	}

	for attempt := 0; attempt < maxClickRetries; attempt++ {
		err := chromedp.Run(browserCtx, chromedp.Click(sel, chromedp.ByQuery))
		if err == nil {
			return nil
		}
		// Retry with JS click as fallback (handles overlay interception).
		jsErr := jsClick(browserCtx, sel)
		if jsErr == nil {
			return nil
		}
		if attempt == maxClickRetries-1 {
			return fmt.Errorf("native click failed (%v) and js fallback failed (%v)", err, jsErr)
		}
	}
	return nil
}

// scrollIntoView executes scrollIntoView on the element via JavaScript.
func scrollIntoView(browserCtx context.Context, sel string) error {
	script := fmt.Sprintf(
		`(function(){var el=document.querySelector(%q);if(el){el.scrollIntoView({block:"center",behavior:"instant"});}})()`,
		sel,
	)
	return chromedp.Run(browserCtx, chromedp.Evaluate(script, nil))
}

// waitVisible waits until the element is visible and enabled.
func waitVisible(ctx context.Context, browserCtx context.Context, sel string) error {
	done := make(chan error, 1)
	go func() {
		done <- chromedp.Run(browserCtx, chromedp.WaitVisible(sel, chromedp.ByQuery))
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for element to be visible: %w", ctx.Err())
	}
}

// jsClick dispatches a synthetic click event via JavaScript.
func jsClick(browserCtx context.Context, sel string) error {
	script := fmt.Sprintf(
		`(function(){var el=document.querySelector(%q);if(el){el.click();return true;}return false;})()`,
		sel,
	)
	var clicked bool
	err := chromedp.Run(browserCtx, chromedp.Evaluate(script, &clicked))
	if err != nil {
		return err
	}
	if !clicked {
		return fmt.Errorf("js click: element %q not found", sel)
	}
	return nil
}

// capturePageState reads URL and title from the browser after an action.
func capturePageState(browserCtx context.Context) (*domain.PageState, error) {
	var url, title string
	err := chromedp.Run(browserCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)
	if err != nil {
		return nil, err
	}
	return &domain.PageState{URL: url, Title: title}, nil
}

// buildResolutionTarget extracts a ResolutionTarget from executor params.
func buildResolutionTarget(params map[string]interface{}) domain.ResolutionTarget {
	var t domain.ResolutionTarget
	if v, ok := params["text"].(string); ok {
		t.Text = v
	}
	if v, ok := params["role"].(string); ok {
		t.Role = v
	}
	if v, ok := params["selector"].(string); ok {
		t.Selector = v
	}
	return t
}
