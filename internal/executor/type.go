package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// TypeOptions configures the behaviour of the type action.
type TypeOptions struct {
	// Clear removes existing text before typing when true.
	Clear bool

	// PressEnter submits the form by pressing Enter after typing when true.
	PressEnter bool
}

// TypeExecutor types text into an input or textarea resolved by a
// domain.UnifiedResolver. It focuses the element, dispatches DOM events,
// optionally clears existing content, and optionally submits via Enter.
// Implements domain.Executor.
type TypeExecutor struct {
	resolver domain.UnifiedResolver
}

// NewTypeExecutor constructs a TypeExecutor with the given resolver.
// resolver is injected (DI) so callers can substitute mocks in tests.
func NewTypeExecutor(resolver domain.UnifiedResolver) *TypeExecutor {
	return &TypeExecutor{resolver: resolver}
}

// Execute resolves the target element and types text into it.
//
// Supported params:
//   - "text"        string           — visible text / accessible name to resolve
//   - "role"        string           — optional WAI-ARIA role filter
//   - "selector"    string           — optional CSS selector override
//   - "input"       string (required) — text to type
//   - "clear"       bool             — clear existing text first (default false)
//   - "pressEnter"  bool             — press Enter after typing (default false)
//   - "timeout"     time.Duration    — override default 30 s timeout
//
// Returns a non-nil *ActionResult on both success and failure.
// Implements domain.Executor.
func (e *TypeExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "type"}

	input, err := stringParam(params, "input")
	if err != nil {
		return failResult(result, start, fmt.Errorf("type: %w", err)), nil
	}

	opts := typeOptionsFromParams(params)
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout(params))
	defer cancel()

	candidate, sel, err := e.resolveCandidate(ctx, inst, params)
	if err != nil {
		return failResult(result, start, err), nil
	}

	finalValue, err := typeIntoElement(ctx, inst.Context(), sel, input, opts)
	if err != nil {
		return failResult(result, start, fmt.Errorf("type: %w", err)), nil
	}
	_ = finalValue // available to callers who need to assert the typed value

	pageState, err := capturePageState(inst.Context())
	if err != nil {
		return failResult(result, start, fmt.Errorf("type: capture page state: %w", err)), nil
	}

	result.Success = true
	result.Element = &candidate
	result.PageState = pageState
	result.Duration = time.Since(start)
	return result, nil
}

// resolveCandidate resolves params to the first matching Candidate and its selector.
func (e *TypeExecutor) resolveCandidate(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (domain.Candidate, string, error) {
	target := buildResolutionTarget(params)
	resolution, err := e.resolver.Resolve(ctx, target, inst)
	if err != nil {
		return domain.Candidate{}, "", fmt.Errorf("type: resolve: %w", err)
	}
	if len(resolution.Candidates) == 0 {
		return domain.Candidate{}, "", fmt.Errorf("type: no candidates for %+v", target)
	}
	c := resolution.Candidates[0]
	return c, selectorForCandidate(c), nil
}

// resolveTimeout returns the timeout from params or the default.
func resolveTimeout(params map[string]interface{}) time.Duration {
	if v, ok := params["timeout"]; ok {
		if d, ok := v.(time.Duration); ok {
			return d
		}
	}
	return defaultNavigateTimeout
}

// typeIntoElement focuses the element, optionally clears it, types text,
// dispatches DOM events (focus → input → change), and returns the final value.
func typeIntoElement(
	ctx context.Context,
	browserCtx context.Context,
	sel string,
	input string,
	opts TypeOptions,
) (string, error) {
	if err := scrollIntoView(browserCtx, sel); err != nil {
		return "", fmt.Errorf("scroll into view: %w", err)
	}

	if err := waitVisible(ctx, browserCtx, sel); err != nil {
		return "", fmt.Errorf("wait visible: %w", err)
	}

	if err := focusElement(browserCtx, sel); err != nil {
		return "", fmt.Errorf("focus: %w", err)
	}

	if opts.Clear {
		if err := clearElement(browserCtx, sel); err != nil {
			return "", fmt.Errorf("clear: %w", err)
		}
	}

	if err := chromedp.Run(browserCtx, chromedp.SendKeys(sel, input, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("send keys: %w", err)
	}

	if err := dispatchChangeEvent(browserCtx, sel); err != nil {
		return "", fmt.Errorf("dispatch change: %w", err)
	}

	if opts.PressEnter {
		if err := chromedp.Run(browserCtx, chromedp.SendKeys(sel, "\r", chromedp.ByQuery)); err != nil {
			return "", fmt.Errorf("press enter: %w", err)
		}
	}

	return readElementValue(browserCtx, sel)
}

// focusElement focuses the element via JavaScript to trigger the focus event.
func focusElement(browserCtx context.Context, sel string) error {
	script := fmt.Sprintf(
		`(function(){var el=document.querySelector(%q);if(el){el.focus();}})()`,
		sel,
	)
	return chromedp.Run(browserCtx, chromedp.Evaluate(script, nil))
}

// clearElement clears an input's value and dispatches an input event.
func clearElement(browserCtx context.Context, sel string) error {
	script := fmt.Sprintf(
		`(function(){
			var el=document.querySelector(%q);
			if(!el){return;}
			var nativeInputValueSetter=Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,'value');
			if(nativeInputValueSetter&&nativeInputValueSetter.set){
				nativeInputValueSetter.set.call(el,'');
			}else{
				el.value='';
			}
			el.dispatchEvent(new Event('input',{bubbles:true}));
		})()`,
		sel,
	)
	return chromedp.Run(browserCtx, chromedp.Evaluate(script, nil))
}

// dispatchChangeEvent dispatches a 'change' event on the element.
// This ensures frameworks that listen to change (not input) see the update.
func dispatchChangeEvent(browserCtx context.Context, sel string) error {
	script := fmt.Sprintf(
		`(function(){
			var el=document.querySelector(%q);
			if(el){el.dispatchEvent(new Event('change',{bubbles:true}));}
		})()`,
		sel,
	)
	return chromedp.Run(browserCtx, chromedp.Evaluate(script, nil))
}

// readElementValue returns the current .value of an input element.
func readElementValue(browserCtx context.Context, sel string) (string, error) {
	var value string
	script := fmt.Sprintf(
		`(function(){var el=document.querySelector(%q);return el?el.value:'';})()`,
		sel,
	)
	err := chromedp.Run(browserCtx, chromedp.Evaluate(script, &value))
	return value, err
}

// typeOptionsFromParams extracts TypeOptions from executor params.
func typeOptionsFromParams(params map[string]interface{}) TypeOptions {
	var opts TypeOptions
	if v, ok := params["clear"].(bool); ok {
		opts.Clear = v
	}
	if v, ok := params["pressEnter"].(bool); ok {
		opts.PressEnter = v
	}
	return opts
}
