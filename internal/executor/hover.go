package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// HoverExecutor moves the mouse over an element to trigger :hover state.
// It implements domain.Executor.
type HoverExecutor struct {
	resolver domain.UnifiedResolver
}

// NewHoverExecutor constructs a HoverExecutor with the given resolver.
// resolver is injected (DI) so callers can substitute mocks in tests.
func NewHoverExecutor(resolver domain.UnifiedResolver) *HoverExecutor {
	return &HoverExecutor{resolver: resolver}
}

// Execute resolves the target element and dispatches a mousemove event to its center.
//
// Supported params:
//   - "target"  string — CSS selector, text, or role to resolve (required)
//   - "timeout" time.Duration — override default 30 s timeout
//
// Returns a non-nil *ActionResult with the hovered element details.
// Implements domain.Executor.
func (e *HoverExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "hover"}

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout(params))
	defer cancel()

	sel, candidate, err := e.resolveSelector(ctx, inst, params)
	if err != nil {
		return failResult(result, start, fmt.Errorf("hover: %w", err)), nil
	}

	if err := hoverElement(ctx, inst.Context(), sel); err != nil {
		return failResult(result, start, fmt.Errorf("hover: %w", err)), nil
	}

	pageState, err := capturePageState(inst.Context())
	if err != nil {
		return failResult(result, start, fmt.Errorf("hover: page state: %w", err)), nil
	}

	result.Success = true
	result.Element = &candidate
	result.PageState = pageState
	result.Duration = time.Since(start)
	return result, nil
}

// resolveSelector obtains a CSS selector and Candidate for the target element.
func (e *HoverExecutor) resolveSelector(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (string, domain.Candidate, error) {
	if target, ok := params["target"].(string); ok && target != "" {
		c := domain.Candidate{SemanticID: rawSelectorPrefix + target}
		return target, c, nil
	}
	resolution, err := e.resolver.Resolve(ctx, buildResolutionTarget(params), inst)
	if err != nil {
		return "", domain.Candidate{}, fmt.Errorf("resolve: %w", err)
	}
	if len(resolution.Candidates) == 0 {
		return "", domain.Candidate{}, fmt.Errorf("no candidates found")
	}
	c := resolution.Candidates[0]
	return selectorForCandidate(c), c, nil
}

// hoverElement moves the mouse to the center of the element via CDP mouse events.
func hoverElement(ctx, browserCtx context.Context, sel string) error {
	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()

	var cx, cy float64
	centerScript := fmt.Sprintf(
		`(function(){
			var el=document.querySelector(%q);
			if(!el){return null;}
			var r=el.getBoundingClientRect();
			return [r.left+r.width/2, r.top+r.height/2];
		})()`,
		sel,
	)

	var coords []float64
	if err := chromedp.Run(runCtx, chromedp.Evaluate(centerScript, &coords)); err != nil {
		return fmt.Errorf("get element center: %w", err)
	}
	if len(coords) < 2 {
		return fmt.Errorf("element %q not found or has no bounding box", sel)
	}
	cx, cy = coords[0], coords[1]

	return chromedp.Run(runCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, cx, cy).Do(ctx)
	}))
}
