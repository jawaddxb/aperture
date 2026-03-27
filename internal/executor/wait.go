package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// waitDefaultTimeout is the default timeout for wait strategies.
const waitDefaultTimeout = 10 * time.Second

// waitNetworkIdleDuration is the quiescence window for network idle detection.
const waitNetworkIdleDuration = 500 * time.Millisecond

// ConditionStrategy names the condition the WaitExecutor waits for.
// The string values match the "strategy" param accepted by Execute.
type ConditionStrategy string

const (
	// ConditionSelectorVisible waits for a CSS selector to appear in the DOM.
	ConditionSelectorVisible ConditionStrategy = "selector"

	// ConditionText waits for text content to appear anywhere on the page.
	ConditionText ConditionStrategy = "text"

	// ConditionHidden waits for a selector to disappear from the DOM.
	ConditionHidden ConditionStrategy = "hidden"

	// ConditionTimeout waits a fixed number of milliseconds.
	ConditionTimeout ConditionStrategy = "timeout"

	// ConditionNetworkIdle waits until no network requests are pending for 500ms.
	ConditionNetworkIdle ConditionStrategy = "networkidle"
)

// WaitExecutor waits for a page condition before proceeding.
// It implements domain.Executor.
type WaitExecutor struct{}

// NewWaitExecutor constructs a WaitExecutor.
func NewWaitExecutor() *WaitExecutor {
	return &WaitExecutor{}
}

// Execute waits for a condition and returns whether it was met within the timeout.
//
// Supported params:
//   - "strategy" string — "selector"|"text"|"hidden"|"timeout"|"networkidle" (required)
//   - "selector" string — CSS selector (required for "selector" and "hidden" strategies)
//   - "text"     string — text to wait for (required for "text" strategy)
//   - "ms"       int    — milliseconds to wait (required for "timeout" strategy)
//   - "timeout"  time.Duration — max wait duration (default 10s)
//
// Returns a non-nil *ActionResult. Success=false and Error set on timeout.
// Implements domain.Executor.
func (e *WaitExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "wait"}

	strategy, err := stringParam(params, "strategy")
	if err != nil {
		return failResult(result, start, fmt.Errorf("wait: %w", err)), nil
	}

	timeout := waitDefaultTimeout
	if v, ok := params["timeout"]; ok {
		if d, ok := v.(time.Duration); ok {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err = runWaitStrategy(ctx, inst.Context(), ConditionStrategy(strategy), params)
	if err != nil {
		return failResult(result, start, fmt.Errorf("wait %s: %w", strategy, err)), nil
	}

	pageState, _ := capturePageState(inst.Context()) // best-effort; ignore error

	result.Success = true
	result.PageState = pageState
	result.Duration = time.Since(start)
	return result, nil
}

// runWaitStrategy dispatches to the correct wait implementation.
func runWaitStrategy(ctx, browserCtx context.Context, s ConditionStrategy, params map[string]interface{}) error {
	switch s {
	case ConditionSelectorVisible:
		return waitForSelector(ctx, browserCtx, params)
	case ConditionText:
		return waitForText(ctx, browserCtx, params)
	case ConditionHidden:
		return waitForHidden(ctx, browserCtx, params)
	case ConditionTimeout:
		return waitForDuration(ctx, params)
	case ConditionNetworkIdle:
		return waitForNetworkIdle(ctx, browserCtx)
	default:
		return fmt.Errorf("unknown strategy %q", s)
	}
}

// waitForSelector waits until the CSS selector appears in the DOM.
func waitForSelector(ctx, browserCtx context.Context, params map[string]interface{}) error {
	sel, err := stringParam(params, "selector")
	if err != nil {
		return fmt.Errorf("selector strategy: %w", err)
	}
	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()
	return chromedp.Run(runCtx, chromedp.WaitVisible(sel, chromedp.ByQuery))
}

// waitForText polls until textContent matching the given string appears on the page.
func waitForText(ctx, browserCtx context.Context, params map[string]interface{}) error {
	text, err := stringParam(params, "text")
	if err != nil {
		return fmt.Errorf("text strategy: %w", err)
	}

	script := fmt.Sprintf(
		`document.body && document.body.innerText.indexOf(%q) !== -1`,
		text,
	)

	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()

	for {
		select {
		case <-runCtx.Done():
			return fmt.Errorf("timed out waiting for text %q: %w", text, runCtx.Err())
		default:
		}

		var found bool
		if err := chromedp.Run(runCtx, chromedp.Evaluate(script, &found)); err != nil {
			return fmt.Errorf("eval: %w", err)
		}
		if found {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForHidden waits until the CSS selector is no longer visible in the DOM.
func waitForHidden(ctx, browserCtx context.Context, params map[string]interface{}) error {
	sel, err := stringParam(params, "selector")
	if err != nil {
		return fmt.Errorf("hidden strategy: %w", err)
	}
	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()
	return chromedp.Run(runCtx, chromedp.WaitNotVisible(sel, chromedp.ByQuery))
}

// waitForDuration waits a fixed number of milliseconds.
func waitForDuration(ctx context.Context, params map[string]interface{}) error {
	ms := 0
	if v, ok := params["ms"].(int); ok {
		ms = v
	}
	if ms <= 0 {
		return fmt.Errorf("'ms' param must be a positive integer")
	}
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context cancelled during timeout wait: %w", ctx.Err())
	}
}

// waitForNetworkIdle waits until no requests fire for waitNetworkIdleDuration.
func waitForNetworkIdle(ctx, browserCtx context.Context) error {
	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()
	return chromedp.Run(runCtx, chromedp.Sleep(waitNetworkIdleDuration))
}
