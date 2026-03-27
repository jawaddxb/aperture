package executor_test

import (
	"context"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
)

// TestClick_Button verifies that clicking a visible button on a test page
// registers the click (detected via a JS side-effect).
func TestClick_Button(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	// Navigate to a test page with a clickable button that sets window._clicked.
	html := `data:text/html,<html><body>` +
		`<button id="btn" onclick="window._clicked=true">Click Me</button>` +
		`</body></html>`

	nav := executor.NewNavigateExecutor()
	navResult, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html, "wait": "load",
	})
	if err != nil || !navResult.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, navResult)
	}

	// Use a canned resolver that returns the #btn selector.
	resolver := newCannedResolver(domain.Candidate{
		SemanticID: "", // empty — selectorForCandidate will build from role+name
		Role:       "",
		Name:       "",
		Confidence: 1.0,
	}, "#btn")

	e := executor.NewClickExecutor(resolver)
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"selector": "#btn",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Click failed: %s", result.Error)
	}

	// Verify the click was registered via JS.
	clicked := assertJSBool(t, inst, "window._clicked === true")
	if !clicked {
		t.Error("window._clicked not set — click was not registered")
	}
	t.Logf("Click result: element=%+v pageState=%+v", result.Element, result.PageState)
}

// TestClick_HiddenElement verifies that clicking a hidden (off-screen) element
// causes it to be scrolled into view before clicking.
func TestClick_HiddenElement(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	// Button is placed 5000px below the fold.
	html := `data:text/html,<html><body style="height:6000px">` +
		`<button id="far" style="position:absolute;top:5000px" onclick="window._farClicked=true">Far</button>` +
		`</body></html>`

	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html, "wait": "load",
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	resolver := newCannedResolver(domain.Candidate{Confidence: 1.0}, "#far")

	e := executor.NewClickExecutor(resolver)
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"selector": "#far",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Click on hidden element failed: %s", result.Error)
	}

	clicked := assertJSBool(t, inst, "window._farClicked === true")
	if !clicked {
		t.Error("window._farClicked not set — far element click was not registered")
	}
}

// TestClick_NoMatch verifies that a click on an unresolvable target fails gracefully.
func TestClick_NoMatch(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	nav := executor.NewNavigateExecutor()
	nav.Execute(context.Background(), inst, map[string]interface{}{ //nolint:errcheck
		"url": `data:text/html,<html><body>empty</body></html>`, "wait": "load",
	})

	resolver := newFailingResolver(&domain.ErrNoMatch{Target: domain.ResolutionTarget{Text: "ghost"}})
	e := executor.NewClickExecutor(resolver)

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"text": "ghost",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for no-match click")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for no-match click")
	}
}
