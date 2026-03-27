package executor_test

import (
	"context"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/executor"
)

// TestWait_SelectorExists verifies that waiting for a selector that already
// exists in the DOM resolves immediately (within a short timeout).
func TestWait_SelectorExists(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body><div id="ready">ready</div></body></html>`
	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html,
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	e := executor.NewWaitExecutor()
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"strategy": "selector",
		"selector": "#ready",
		"timeout":  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("wait error: %v", err)
	}
	if !result.Success {
		t.Fatalf("wait failed: %s", result.Error)
	}
	if result.Duration > 2*time.Second {
		t.Errorf("wait took too long for existing selector: %s", result.Duration)
	}
	t.Logf("wait selector: duration=%s", result.Duration)
}

// TestWait_SelectorTimeout verifies that waiting for a non-existent selector
// returns a failure after the configured timeout.
func TestWait_SelectorTimeout(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body><p>nothing here</p></body></html>`
	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html,
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	e := executor.NewWaitExecutor()
	start := time.Now()
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"strategy": "selector",
		"selector": "#does-not-exist",
		"timeout":  500 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false on timeout")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error on timeout")
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("timeout fired too early: elapsed=%s", elapsed)
	}
	t.Logf("timeout result: error=%s elapsed=%s", result.Error, elapsed)
}

// TestWait_TextAppearing verifies that waiting for text that exists on the page
// resolves successfully.
func TestWait_TextAppearing(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body><p>unique-text-aperture-42</p></body></html>`
	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html,
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	e := executor.NewWaitExecutor()
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"strategy": "text",
		"text":     "unique-text-aperture-42",
		"timeout":  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("wait error: %v", err)
	}
	if !result.Success {
		t.Fatalf("wait for text failed: %s", result.Error)
	}
	t.Logf("wait text: duration=%s", result.Duration)
}

// TestWait_FixedTimeout verifies that the "timeout" strategy waits approximately
// the specified number of milliseconds.
func TestWait_FixedTimeout(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	nav := executor.NewNavigateExecutor()
	nav.Execute(context.Background(), inst, map[string]interface{}{ //nolint:errcheck
		"url": "data:text/html,<html><body></body></html>",
	})

	e := executor.NewWaitExecutor()
	start := time.Now()
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"strategy": "timeout",
		"ms":       200,
		"timeout":  5 * time.Second,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("wait error: %v", err)
	}
	if !result.Success {
		t.Fatalf("wait failed: %s", result.Error)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("wait ended too early: elapsed=%s", elapsed)
	}
	t.Logf("fixed timeout: elapsed=%s", elapsed)
}
