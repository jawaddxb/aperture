package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// scrollDefaultAmount is the default scroll distance in pixels.
const scrollDefaultAmount = 300

// ScrollExecutor scrolls the viewport or a specific element.
// It implements domain.Executor.
type ScrollExecutor struct{}

// NewScrollExecutor constructs a ScrollExecutor.
func NewScrollExecutor() *ScrollExecutor {
	return &ScrollExecutor{}
}

// Execute scrolls the page or a target element in the given direction.
//
// Supported params:
//   - "direction" string — "up"|"down"|"left"|"right" (required)
//   - "amount"    int    — pixels to scroll (default 300)
//   - "target"    string — optional CSS selector to scroll a specific element
//   - "timeout"   time.Duration — override default 30 s timeout
//
// Returns a non-nil *ActionResult with scroll position in PageState.
// Implements domain.Executor.
func (e *ScrollExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "scroll"}

	direction, err := stringParam(params, "direction")
	if err != nil {
		return failResult(result, start, fmt.Errorf("scroll: %w", err)), nil
	}

	amount := scrollDefaultAmount
	if v, ok := params["amount"].(int); ok && v > 0 {
		amount = v
	}

	target, _ := params["target"].(string)

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout(params))
	defer cancel()

	runCtx, cancelRun := makeRunContext(ctx, inst.Context())
	defer cancelRun()

	script := buildScrollScript(direction, amount, target)
	if err := chromedp.Run(runCtx, chromedp.Evaluate(script, nil)); err != nil {
		return failResult(result, start, fmt.Errorf("scroll: %w", err)), nil
	}

	pageState, err := capturePageState(inst.Context())
	if err != nil {
		return failResult(result, start, fmt.Errorf("scroll: page state: %w", err)), nil
	}

	result.Success = true
	result.PageState = pageState
	result.Duration = time.Since(start)
	return result, nil
}

// buildScrollScript returns a JS snippet that scrolls in the given direction.
func buildScrollScript(direction string, amount int, target string) string {
	var dx, dy int
	switch direction {
	case "down":
		dy = amount
	case "up":
		dy = -amount
	case "right":
		dx = amount
	case "left":
		dx = -amount
	}

	if target != "" {
		return fmt.Sprintf(
			`(function(){
				var el=document.querySelector(%q);
				if(el){el.scrollBy(%d,%d);}
			})()`,
			target, dx, dy,
		)
	}
	return fmt.Sprintf(`window.scrollBy(%d,%d)`, dx, dy)
}
