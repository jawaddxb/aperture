package validator_test

import (
	"context"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/validator"
)

func newStep(action string, params map[string]interface{}) domain.Step {
	return domain.Step{Action: action, Params: params}
}

func TestValidateNavigate_Valid(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("navigate", map[string]interface{}{
		"url":     "https://example.com",
		"timeout": 5000,
	})
	res, err := v.Validate(context.Background(), step, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected Valid=true, got errors: %v", res.Errors)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", res.Warnings)
	}
}

func TestValidateNavigate_MissingURL(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("navigate", map[string]interface{}{})
	res, err := v.Validate(context.Background(), step, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Error("expected Valid=false for missing URL")
	}
	if len(res.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

func TestValidateNavigate_InvalidURL(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("navigate", map[string]interface{}{"url": "not-a-url"})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false for URL without scheme")
	}
}

func TestValidateNavigate_HTTPWarning(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("navigate", map[string]interface{}{
		"url":     "http://example.com",
		"timeout": 5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if !res.Valid {
		t.Errorf("expected Valid=true, got errors: %v", res.Errors)
	}
	found := false
	for _, w := range res.Warnings {
		if len(w) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected HTTP warning in Warnings")
	}
}

func TestValidateScroll_InvalidDirection(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("scroll", map[string]interface{}{"direction": "sideways"})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false for invalid scroll direction")
	}
}

func TestValidateScroll_ValidDirections(t *testing.T) {
	v := validator.NewDefaultValidator()
	for _, dir := range []string{"up", "down", "left", "right"} {
		step := newStep("scroll", map[string]interface{}{
			"direction": dir,
			"timeout":   5000,
		})
		res, _ := v.Validate(context.Background(), step, nil)
		if !res.Valid {
			t.Errorf("direction %q should be valid, got errors: %v", dir, res.Errors)
		}
	}
}

func TestValidateClick_MissingTarget(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("click", map[string]interface{}{})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false for missing target")
	}
}

func TestValidateType_MissingText(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("type", map[string]interface{}{
		"target":  "#input",
		"timeout": 5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false for missing text param")
	}
}

func TestValidateSelect_NeitherValueNorText(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("select", map[string]interface{}{
		"target":  "#sel",
		"timeout": 5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false when neither value nor text is set")
	}
}

func TestValidateSelect_WithValue(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("select", map[string]interface{}{
		"target":  "#sel",
		"value":   "opt1",
		"timeout": 5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if !res.Valid {
		t.Errorf("expected Valid=true, got: %v", res.Errors)
	}
}

func TestValidateScreenshot_InvalidFormat(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("screenshot", map[string]interface{}{
		"format":  "gif",
		"timeout": 5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false for invalid screenshot format")
	}
}

func TestValidateScreenshot_ValidFormats(t *testing.T) {
	v := validator.NewDefaultValidator()
	for _, fmt := range []string{"png", "jpeg"} {
		step := newStep("screenshot", map[string]interface{}{
			"format":  fmt,
			"timeout": 5000,
		})
		res, _ := v.Validate(context.Background(), step, nil)
		if !res.Valid {
			t.Errorf("format %q should be valid", fmt)
		}
	}
}

func TestValidateWait_InvalidStrategy(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("wait", map[string]interface{}{
		"strategy": "unknown_strategy",
		"timeout":  5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false for unknown wait strategy")
	}
}

func TestValidateWait_DelayMissingMs(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("wait", map[string]interface{}{
		"strategy": "delay",
		"timeout":  5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if res.Valid {
		t.Error("expected Valid=false for delay without ms param")
	}
}

func TestValidateWait_DelayValid(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("wait", map[string]interface{}{
		"strategy": "delay",
		"ms":       500,
		"timeout":  5000,
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if !res.Valid {
		t.Errorf("expected Valid=true, got: %v", res.Errors)
	}
}

func TestValidate_MissingTimeoutWarning(t *testing.T) {
	v := validator.NewDefaultValidator()
	step := newStep("navigate", map[string]interface{}{
		"url": "https://example.com",
	})
	res, _ := v.Validate(context.Background(), step, nil)
	if !res.Valid {
		t.Errorf("expected Valid=true")
	}
	found := false
	for _, w := range res.Warnings {
		if w != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected missing timeout warning")
	}
}
