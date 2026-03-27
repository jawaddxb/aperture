package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// UploadExecutor sets local file paths on a file input element resolved
// by a domain.UnifiedResolver. Implements domain.Executor.
type UploadExecutor struct {
	resolver domain.UnifiedResolver
}

// NewUploadExecutor constructs an UploadExecutor with the given resolver.
func NewUploadExecutor(resolver domain.UnifiedResolver) *UploadExecutor {
	return &UploadExecutor{resolver: resolver}
}

// Execute resolves the target file input and sets its files.
//
// Supported params:
//   - "target"   string (required) — visible text / accessible name to resolve
//   - "role"     string — optional WAI-ARIA role filter (defaults to "file")
//   - "selector" string — optional CSS selector override
//   - "files"    []string (required) — local file paths to upload
//   - "timeout"  time.Duration — override default 30 s timeout
//
// Returns a non-nil *ActionResult on both success and failure.
// Implements domain.Executor.
func (e *UploadExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "upload"}

	files, err := stringSliceParam(params, "files")
	if err != nil {
		return failResult(result, start, fmt.Errorf("upload: %w", err)), nil
	}

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout(params))
	defer cancel()

	candidate, err := e.resolveTarget(ctx, inst, params)
	if err != nil {
		return failResult(result, start, fmt.Errorf("upload: resolve: %w", err)), nil
	}

	sel := selectorForCandidate(*candidate)
	if err := chromedp.Run(inst.Context(), chromedp.SetUploadFiles(sel, files, chromedp.ByQuery)); err != nil {
		return failResult(result, start, fmt.Errorf("upload: set files: %w", err)), nil
	}

	pageState, err := capturePageState(inst.Context())
	if err != nil {
		return failResult(result, start, fmt.Errorf("upload: capture page state: %w", err)), nil
	}

	result.Success = true
	result.Element = candidate
	result.PageState = pageState
	result.Duration = time.Since(start)
	return result, nil
}

// resolveTarget resolves the target file input element.
func (e *UploadExecutor) resolveTarget(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.Candidate, error) {
	t := buildResolutionTarget(params)
	if target, ok := params["target"].(string); ok && t.Text == "" && t.Selector == "" {
		t.Text = target
	}
	if t.Role == "" {
		t.Role = "file"
	}

	resolution, err := e.resolver.Resolve(ctx, t, inst)
	if err != nil {
		return nil, err
	}
	if len(resolution.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates for %+v", t)
	}
	return &resolution.Candidates[0], nil
}

// stringSliceParam extracts a required string slice from params.
func stringSliceParam(params map[string]interface{}, key string) ([]string, error) {
	v, ok := params[key]
	if !ok {
		return nil, fmt.Errorf("missing required param %q", key)
	}

	switch val := v.(type) {
	case []string:
		if len(val) == 0 {
			return nil, fmt.Errorf("param %q must be a non-empty string slice", key)
		}
		return val, nil
	case []interface{}:
		if len(val) == 0 {
			return nil, fmt.Errorf("param %q must be a non-empty slice", key)
		}
		res := make([]string, len(val))
		for i, v := range val {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("param %q item %d must be a string", key, i)
			}
			res[i] = s
		}
		return res, nil
	default:
		return nil, fmt.Errorf("param %q must be a string slice", key)
	}
}
