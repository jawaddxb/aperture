package executor_test

import (
	"context"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/executor"
)

// TestNavigate_DataURL verifies that navigating to a data: URL returns the
// correct title and URL in the ActionResult.
func TestNavigate_DataURL(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	e := executor.NewNavigateExecutor()
	ctx := context.Background()

	html := `data:text/html,<html><head><title>Hello Aperture</title></head><body>ok</body></html>`

	result, err := e.Execute(ctx, inst, map[string]interface{}{
		"url":  html,
		"wait": "load",
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Execute failed: %s", result.Error)
	}
	if result.PageState == nil {
		t.Fatal("PageState is nil")
	}
	if result.PageState.Title != "Hello Aperture" {
		t.Errorf("Title = %q, want %q", result.PageState.Title, "Hello Aperture")
	}
	if result.PageState.URL == "" {
		t.Error("URL is empty after navigation")
	}
	if result.Duration <= 0 {
		t.Error("Duration must be > 0")
	}
	t.Logf("Navigate result: URL=%s title=%q duration=%s", result.PageState.URL, result.PageState.Title, result.Duration)
}

// TestNavigate_NetworkIdle verifies that the networkidle wait strategy completes
// successfully on a simple data: page.
func TestNavigate_NetworkIdle(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	e := executor.NewNavigateExecutor()
	ctx := context.Background()

	html := `data:text/html,<html><head><title>Idle</title></head><body>idle</body></html>`

	result, err := e.Execute(ctx, inst, map[string]interface{}{
		"url":  html,
		"wait": "networkidle",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Execute failed: %s", result.Error)
	}
	if result.PageState.Title != "Idle" {
		t.Errorf("Title = %q, want %q", result.PageState.Title, "Idle")
	}
}

// TestNavigate_SelectorWait verifies that wait=selector waits for a CSS selector.
func TestNavigate_SelectorWait(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	e := executor.NewNavigateExecutor()
	ctx := context.Background()

	html := `data:text/html,<html><body><div id="ready">ready</div></body></html>`

	result, err := e.Execute(ctx, inst, map[string]interface{}{
		"url":      html,
		"wait":     "selector",
		"selector": "#ready",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Execute failed: %s", result.Error)
	}
}

// TestNavigate_Timeout verifies that a very short timeout causes a failure result
// when the page cannot load in time.
func TestNavigate_Timeout(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	e := executor.NewNavigateExecutor()
	ctx := context.Background()

	// Use a 1 ns timeout — guaranteed to expire before any navigation completes.
	result, err := e.Execute(ctx, inst, map[string]interface{}{
		"url":     "https://example.com",
		"wait":    "load",
		"timeout": time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected Go error (want nil, wrapped in result): %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false on timeout, got true")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error on timeout")
	}
	t.Logf("Timeout result error: %s", result.Error)
}

// TestNavigate_MissingURL verifies that omitting the required "url" param returns
// a failure ActionResult without panicking.
func TestNavigate_MissingURL(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	e := executor.NewNavigateExecutor()
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false when url param is missing")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error when url param is missing")
	}
}
