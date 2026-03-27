// Package validator provides DefaultValidator, a pre-flight checker for plan steps.
package validator

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// knownWaitStrategies is the set of accepted wait strategy names.
var knownWaitStrategies = map[string]bool{
	"visible":     true,
	"hidden":      true,
	"network_idle": true,
	"load":        true,
	"dom_ready":   true,
	"delay":       true,
}

// knownScrollDirections is the set of accepted scroll direction values.
var knownScrollDirections = map[string]bool{
	"up":    true,
	"down":  true,
	"left":  true,
	"right": true,
}

// DefaultValidator checks each step's parameters before execution.
// It satisfies domain.StepValidator.
type DefaultValidator struct{}

// NewDefaultValidator constructs a ready-to-use DefaultValidator.
func NewDefaultValidator() *DefaultValidator {
	return &DefaultValidator{}
}

// Validate performs pre-flight checks on step before execution.
// It always returns a non-nil *ValidationResult.
// The returned error is reserved for unexpected internal failures.
func (v *DefaultValidator) Validate(
	ctx context.Context,
	step domain.Step,
	pageState *domain.PageState,
) (*domain.ValidationResult, error) {
	res := &domain.ValidationResult{Valid: true}

	// Per-action checks.
	switch step.Action {
	case "navigate":
		v.checkNavigate(step.Params, res)
	case "click", "hover":
		v.checkTarget(step.Params, res)
	case "type":
		v.checkTarget(step.Params, res)
		v.checkNonEmptyString(step.Params, "text", res)
	case "select":
		v.checkTarget(step.Params, res)
		v.checkSelectChoice(step.Params, res)
	case "screenshot":
		v.checkScreenshotFormat(step.Params, res)
	case "wait":
		v.checkWait(step.Params, res)
	case "scroll":
		v.checkScroll(step.Params, res)
	}

	// Common warning: missing timeout.
	if _, ok := step.Params["timeout"]; !ok {
		res.Warnings = append(res.Warnings, "missing timeout param; using executor default")
	}

	// Flip Valid to false when any errors were recorded.
	if len(res.Errors) > 0 {
		res.Valid = false
	}

	return res, nil
}

// checkNavigate validates the navigate action's url param.
func (v *DefaultValidator) checkNavigate(params map[string]interface{}, res *domain.ValidationResult) {
	raw, ok := params["url"]
	if !ok || raw == nil {
		res.Errors = append(res.Errors, "navigate: missing required param \"url\"")
		return
	}
	str, ok := raw.(string)
	if !ok || strings.TrimSpace(str) == "" {
		res.Errors = append(res.Errors, "navigate: param \"url\" must be a non-empty string")
		return
	}
	u, err := url.Parse(str)
	if err != nil || u.Scheme == "" {
		res.Errors = append(res.Errors, fmt.Sprintf("navigate: invalid URL %q (must include scheme)", str))
		return
	}
	if u.Scheme == "http" {
		res.Warnings = append(res.Warnings, fmt.Sprintf("navigate: URL %q is non-HTTPS; prefer https://", str))
	}
}

// checkTarget validates that the "target" param is a non-empty string.
func (v *DefaultValidator) checkTarget(params map[string]interface{}, res *domain.ValidationResult) {
	v.checkNonEmptyString(params, "target", res)
}

// checkNonEmptyString validates that the named param exists and is a non-empty string.
func (v *DefaultValidator) checkNonEmptyString(params map[string]interface{}, key string, res *domain.ValidationResult) {
	raw, ok := params[key]
	if !ok || raw == nil {
		res.Errors = append(res.Errors, fmt.Sprintf("missing required param %q", key))
		return
	}
	str, ok := raw.(string)
	if !ok || strings.TrimSpace(str) == "" {
		res.Errors = append(res.Errors, fmt.Sprintf("param %q must be a non-empty string", key))
	}
}

// checkSelectChoice ensures at least one of "value" or "text" is present.
func (v *DefaultValidator) checkSelectChoice(params map[string]interface{}, res *domain.ValidationResult) {
	_, hasValue := params["value"]
	_, hasText := params["text"]
	if !hasValue && !hasText {
		res.Errors = append(res.Errors, "select: requires at least one of \"value\" or \"text\" params")
	}
}

// checkScreenshotFormat validates the optional "format" param.
func (v *DefaultValidator) checkScreenshotFormat(params map[string]interface{}, res *domain.ValidationResult) {
	raw, ok := params["format"]
	if !ok || raw == nil {
		return // format is optional
	}
	str, ok := raw.(string)
	if !ok || (str != "png" && str != "jpeg") {
		res.Errors = append(res.Errors, fmt.Sprintf("screenshot: param \"format\" must be \"png\" or \"jpeg\", got %q", raw))
	}
}

// checkWait validates the wait action's strategy and required sub-params.
func (v *DefaultValidator) checkWait(params map[string]interface{}, res *domain.ValidationResult) {
	raw, ok := params["strategy"]
	if !ok || raw == nil {
		res.Errors = append(res.Errors, "wait: missing required param \"strategy\"")
		return
	}
	str, ok := raw.(string)
	if !ok || !knownWaitStrategies[str] {
		res.Errors = append(res.Errors,
			fmt.Sprintf("wait: unknown strategy %q; must be one of: %s", raw, joinKeys(knownWaitStrategies)))
		return
	}
	// delay strategy requires "ms" sub-param.
	if str == "delay" {
		if _, hasDuration := params["ms"]; !hasDuration {
			res.Errors = append(res.Errors, "wait: strategy \"delay\" requires param \"ms\"")
		}
	}
}

// checkScroll validates the scroll action's direction param.
func (v *DefaultValidator) checkScroll(params map[string]interface{}, res *domain.ValidationResult) {
	raw, ok := params["direction"]
	if !ok || raw == nil {
		res.Errors = append(res.Errors, "scroll: missing required param \"direction\"")
		return
	}
	str, ok := raw.(string)
	if !ok || !knownScrollDirections[str] {
		res.Errors = append(res.Errors,
			fmt.Sprintf("scroll: invalid direction %q; must be one of: up, down, left, right", raw))
	}
}

// joinKeys returns the sorted keys of a bool map as a comma-separated string.
func joinKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// compile-time interface check.
var _ domain.StepValidator = (*DefaultValidator)(nil)
