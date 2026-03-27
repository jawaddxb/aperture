package executor_test

import (
	"context"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
)

// TestType_BasicInput verifies that typing into a text input sets the value.
func TestType_BasicInput(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body><input id="name" type="text"></body></html>`
	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html, "wait": "load",
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	resolver := newCannedResolver(domain.Candidate{Confidence: 1.0}, "#name")
	e := executor.NewTypeExecutor(resolver)

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"selector": "#name",
		"input":    "hello world",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Type failed: %s", result.Error)
	}

	// Verify the input value via JS.
	val := assertJSString(t, inst, `document.querySelector("#name").value`)
	if val != "hello world" {
		t.Errorf("input value = %q, want %q", val, "hello world")
	}
}

// TestType_ClearBeforeType verifies that clear=true replaces existing text.
func TestType_ClearBeforeType(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	// Pre-fill the input with existing text via value attribute.
	html := `data:text/html,<html><body><input id="inp" type="text" value="old text"></body></html>`
	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html, "wait": "load",
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	resolver := newCannedResolver(domain.Candidate{Confidence: 1.0}, "#inp")
	e := executor.NewTypeExecutor(resolver)

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"selector": "#inp",
		"input":    "new text",
		"clear":    true,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Type with clear failed: %s", result.Error)
	}

	val := assertJSString(t, inst, `document.querySelector("#inp").value`)
	if val != "new text" {
		t.Errorf("value after clear+type = %q, want %q", val, "new text")
	}
}

// TestType_PressEnter verifies that pressEnter=true submits the form.
func TestType_PressEnter(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body>` +
		`<form id="frm" onsubmit="window._submitted=true;return false;">` +
		`<input id="q" type="text">` +
		`</form></body></html>`

	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html, "wait": "load",
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	resolver := newCannedResolver(domain.Candidate{Confidence: 1.0}, "#q")
	e := executor.NewTypeExecutor(resolver)

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"selector":   "#q",
		"input":      "aperture",
		"pressEnter": true,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Type with pressEnter failed: %s", result.Error)
	}

	submitted := assertJSBool(t, inst, "window._submitted === true")
	if !submitted {
		t.Error("form was not submitted after pressEnter")
	}
}

// TestType_MissingInput verifies that omitting the "input" param fails gracefully.
func TestType_MissingInput(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	resolver := newCannedResolver(domain.Candidate{Confidence: 1.0}, "#x")
	e := executor.NewTypeExecutor(resolver)

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"selector": "#x",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false when input param is missing")
	}
}
