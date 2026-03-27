package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// SelectExecutor selects an option in a <select> dropdown element.
// It implements domain.Executor.
type SelectExecutor struct {
	resolver domain.UnifiedResolver
}

// NewSelectExecutor constructs a SelectExecutor with the given resolver.
// resolver is injected (DI) so callers can substitute mocks in tests.
func NewSelectExecutor(resolver domain.UnifiedResolver) *SelectExecutor {
	return &SelectExecutor{resolver: resolver}
}

// Execute resolves a <select> element and sets its value.
//
// Supported params:
//   - "target"  string — CSS selector, text, or role to resolve the <select>
//   - "value"   string — option value to select (preferred)
//   - "text"    string — option text label (fallback when value is absent)
//   - "timeout" time.Duration — override default 30 s timeout
//
// Returns a non-nil *ActionResult with the selected value.
// Implements domain.Executor.
func (e *SelectExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "select"}

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout(params))
	defer cancel()

	sel, candidate, err := e.resolveSelector(ctx, inst, params)
	if err != nil {
		return failResult(result, start, fmt.Errorf("select: %w", err)), nil
	}

	value, err := selectOption(ctx, inst.Context(), sel, params)
	if err != nil {
		return failResult(result, start, fmt.Errorf("select: %w", err)), nil
	}

	pageState, err := capturePageState(inst.Context())
	if err != nil {
		return failResult(result, start, fmt.Errorf("select: page state: %w", err)), nil
	}

	result.Success = true
	result.Element = &candidate
	result.PageState = pageState
	result.Duration = time.Since(start)
	_ = value
	return result, nil
}

// resolveSelector obtains a CSS selector and Candidate for the <select> element.
// It first checks for a "target" string as a direct CSS selector; otherwise it
// uses the UnifiedResolver.
func (e *SelectExecutor) resolveSelector(
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

// selectOption sets the dropdown value via JavaScript and dispatches a change event.
// It prefers "value" param; falls back to matching by option text.
func selectOption(
	ctx context.Context,
	browserCtx context.Context,
	sel string,
	params map[string]interface{},
) (string, error) {
	optValue, _ := params["value"].(string)
	optText, _ := params["text"].(string)

	if optValue == "" && optText == "" {
		return "", fmt.Errorf("one of 'value' or 'text' params required")
	}

	script := buildSelectScript(sel, optValue, optText)

	var selected string
	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()

	if err := chromedp.Run(runCtx, chromedp.Evaluate(script, &selected)); err != nil {
		return "", fmt.Errorf("set value: %w", err)
	}
	if selected == "" {
		return "", fmt.Errorf("option not found in select element")
	}
	return selected, nil
}

// buildSelectScript returns JS that sets the select value and dispatches events.
func buildSelectScript(sel, optValue, optText string) string {
	if optValue != "" {
		return fmt.Sprintf(
			`(function(){
				var el=document.querySelector(%q);
				if(!el){return '';}
				el.value=%q;
				el.dispatchEvent(new Event('input',{bubbles:true}));
				el.dispatchEvent(new Event('change',{bubbles:true}));
				return el.value;
			})()`,
			sel, optValue,
		)
	}
	return fmt.Sprintf(
		`(function(){
			var el=document.querySelector(%q);
			if(!el){return '';}
			var opts=Array.from(el.options);
			var found=opts.find(function(o){return o.text===%q;});
			if(!found){return '';}
			el.value=found.value;
			el.dispatchEvent(new Event('input',{bubbles:true}));
			el.dispatchEvent(new Event('change',{bubbles:true}));
			return el.value;
		})()`,
		sel, optText,
	)
}
