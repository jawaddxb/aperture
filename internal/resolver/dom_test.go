package resolver_test

import (
	"context"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/resolver"
)

// ─── DOMResolver unit tests (no browser required) ─────────────────────────────

// TestDOM_FindByText_Empty verifies that an empty text query returns nil, not error.
func TestDOM_FindByText_Empty(t *testing.T) {
	d := resolver.NewDOMResolver(inst_unused())
	results, err := d.FindByText(context.Background(), "")
	if err != nil {
		t.Fatalf("FindByText empty: unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("FindByText empty: want nil, got %v", results)
	}
}

// TestDOM_FindBySelector_Empty verifies that an empty selector returns nil, not error.
func TestDOM_FindBySelector_Empty(t *testing.T) {
	d := resolver.NewDOMResolver(inst_unused())
	results, err := d.FindBySelector(context.Background(), "")
	if err != nil {
		t.Fatalf("FindBySelector empty: unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("FindBySelector empty: want nil, got %v", results)
	}
}

// TestDOM_FindByPattern_Empty verifies that an empty patterns slice returns nil, not error.
func TestDOM_FindByPattern_Empty(t *testing.T) {
	d := resolver.NewDOMResolver(inst_unused())
	results, err := d.FindByPattern(context.Background(), nil)
	if err != nil {
		t.Fatalf("FindByPattern nil: unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("FindByPattern nil: want nil, got %v", results)
	}
}

// TestDOM_Snapshot verifies that Snapshot returns an empty but non-nil tree.
func TestDOM_Snapshot(t *testing.T) {
	d := resolver.NewDOMResolver(inst_unused())
	tree, err := d.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("DOMResolver.Snapshot: %v", err)
	}
	if tree == nil {
		t.Fatal("Snapshot returned nil")
	}
	if tree.Index == nil {
		t.Fatal("Snapshot.Index is nil")
	}
}

// ─── DOMResolver integration tests (require Chrome) ───────────────────────────

// testDOMPageHTML is a simple page with a variety of interactive elements.
const testDOMPageHTML = `data:text/html,<html><body>
<button type="submit" id="submit-btn">Submit Form</button>
<button type="button" id="cancel-btn">Cancel</button>
<input type="text" id="search" placeholder="Search here" aria-label="Search" />
<input type="email" id="email" placeholder="Email address" />
<a href="/home">Go Home</a>
<a href="/about">About Us</a>
<select id="country"><option>UK</option><option>US</option></select>
<p>Static paragraph text</p>
</body></html>`

// TestDOM_FindByText verifies that FindByText locates elements by visible text content.
func TestDOM_FindByText(t *testing.T) {
	d, cleanup := newBrowserDOMResolver(t, testDOMPageHTML)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	results, err := d.FindByText(ctx, "Submit Form")
	if err != nil {
		t.Fatalf("FindByText: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FindByText('Submit Form') returned no results")
	}

	best := bestMatch(results)
	if best.Confidence < 0.60 {
		t.Errorf("FindByText confidence = %.2f, want >= 0.60", best.Confidence)
	}
	if best.Confidence > 0.80 {
		t.Errorf("FindByText confidence = %.2f, want <= 0.80", best.Confidence)
	}
	t.Logf("FindByText('Submit Form'): %d results, best confidence=%.2f name=%q",
		len(results), best.Confidence, best.Node.Name)
}

// TestDOM_FindBySelector verifies that FindBySelector matches by CSS selector.
func TestDOM_FindBySelector(t *testing.T) {
	d, cleanup := newBrowserDOMResolver(t, testDOMPageHTML)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	results, err := d.FindBySelector(ctx, "input[type=email]")
	if err != nil {
		t.Fatalf("FindBySelector: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FindBySelector('input[type=email]') returned no results")
	}
	for _, r := range results {
		if r.Confidence != 0.70 {
			t.Errorf("FindBySelector confidence = %.2f, want 0.70", r.Confidence)
		}
	}
	t.Logf("FindBySelector('input[type=email]'): %d results, confidence=%.2f",
		len(results), results[0].Confidence)
}

// TestDOM_FindByPattern verifies that FindByPattern returns results for common patterns.
func TestDOM_FindByPattern(t *testing.T) {
	d, cleanup := newBrowserDOMResolver(t, testDOMPageHTML)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	results, err := d.FindByPattern(ctx, resolver.CommonPatterns)
	if err != nil {
		t.Fatalf("FindByPattern: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FindByPattern(CommonPatterns) returned no results for test page")
	}

	for _, r := range results {
		if r.Confidence != 0.65 {
			t.Errorf("FindByPattern confidence = %.2f, want 0.65 (name=%q)", r.Confidence, r.Node.Name)
		}
		if r.Node.SemanticID == "" {
			t.Errorf("result has empty SemanticID (name=%q)", r.Node.Name)
		}
		if len(r.Node.SemanticID) != 16 {
			t.Errorf("SemanticID len=%d, want 16 (name=%q)", len(r.Node.SemanticID), r.Node.Name)
		}
	}
	t.Logf("FindByPattern(CommonPatterns): %d results", len(results))
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// newBrowserDOMResolver creates a DOMResolver backed by a real browser tab.
func newBrowserDOMResolver(t *testing.T, url string) (*resolver.DOMResolver, func()) {
	t.Helper()
	chromePath := chromiumBinary(t)

	pool, err := browser.NewPool(browser.Config{
		PoolSize:     1,
		ChromiumPath: chromePath,
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	inst, err := pool.Acquire(ctx)
	if err != nil {
		cancel()
		_ = pool.Close()
		t.Fatalf("Acquire: %v", err)
	}
	if err := navigateTo(inst, url); err != nil {
		cancel()
		pool.Release(inst)
		_ = pool.Close()
		t.Fatalf("navigate to %q: %v", url, err)
	}
	d := resolver.NewDOMResolver(inst.Context())
	return d, func() {
		cancel()
		pool.Release(inst)
		_ = pool.Close()
	}
}
