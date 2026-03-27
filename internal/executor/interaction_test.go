package executor_test

import (
	"context"
	"testing"

	"github.com/ApertureHQ/aperture/internal/executor"
)

// TestScroll_Down verifies that scrolling down increases the page's scrollY.
func TestScroll_Down(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	// Page tall enough to scroll.
	html := `data:text/html,<html><body style="height:3000px"><p id="top">top</p></body></html>`
	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html,
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	scrollY0 := assertJSString(t, inst, "String(window.scrollY)")

	e := executor.NewScrollExecutor()
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"direction": "down",
		"amount":    300,
	})
	if err != nil {
		t.Fatalf("scroll error: %v", err)
	}
	if !result.Success {
		t.Fatalf("scroll failed: %s", result.Error)
	}

	scrollY1 := assertJSString(t, inst, "String(window.scrollY)")
	if scrollY1 == scrollY0 {
		t.Errorf("scrollY did not change: before=%s after=%s", scrollY0, scrollY1)
	}
	t.Logf("scrollY: %s → %s", scrollY0, scrollY1)
}

// TestSelect_ByValue verifies selecting a dropdown option by value.
func TestSelect_ByValue(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body>` +
		`<select id="sel">` +
		`<option value="a">Apple</option>` +
		`<option value="b">Banana</option>` +
		`<option value="c">Cherry</option>` +
		`</select></body></html>`

	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html,
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	e := executor.NewSelectExecutor(nil) // nil resolver — using direct target selector
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"target": "#sel",
		"value":  "b",
	})
	if err != nil {
		t.Fatalf("select error: %v", err)
	}
	if !result.Success {
		t.Fatalf("select failed: %s", result.Error)
	}

	val := assertJSString(t, inst, `document.querySelector('#sel').value`)
	if val != "b" {
		t.Errorf("selected value = %q, want %q", val, "b")
	}
	t.Logf("selected value: %s", val)
}

// TestHover_Element verifies that hovering an element triggers CSS :hover.
// We detect hover by listening to mouseenter via JS.
func TestHover_Element(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body>` +
		`<div id="target" onmouseenter="window._hovered=true" style="width:100px;height:100px;background:green">hover me</div>` +
		`</body></html>`

	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html,
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	e := executor.NewHoverExecutor(nil) // nil resolver — using direct target selector
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"target": "#target",
	})
	if err != nil {
		t.Fatalf("hover error: %v", err)
	}
	if !result.Success {
		t.Fatalf("hover failed: %s", result.Error)
	}

	hovered := assertJSBool(t, inst, "window._hovered === true")
	if !hovered {
		t.Error("window._hovered not set — mouseenter was not triggered")
	}
	t.Logf("hover result: element=%+v", result.Element)
}
