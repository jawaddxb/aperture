package resolver_test

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/resolver"
	"github.com/chromedp/chromedp"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// chromiumBinary returns the path to a Chrome binary for integration tests.
// Override via APERTURE_CHROMIUM_PATH environment variable.
func chromiumBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("APERTURE_CHROMIUM_PATH"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "darwin":
		const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(mac); err == nil {
			return mac
		}
	case "linux":
		for _, p := range []string{
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/google-chrome",
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	t.Skip("no Chromium binary found; set APERTURE_CHROMIUM_PATH to enable browser tests")
	return ""
}

// newBrowserResolver creates a single-instance browser pool, acquires an instance,
// navigates to url, and returns an AXTreeResolver ready to snapshot.
// The returned cleanup func must be deferred by the caller.
func newBrowserResolver(t *testing.T, url string) (*resolver.AXTreeResolver, func()) {
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

	r := resolver.NewAXTreeResolver(inst.Context())

	return r, func() {
		cancel()
		pool.Release(inst)
		_ = pool.Close()
	}
}

// navigateTo runs a chromedp Navigate on the instance's tab context.
func navigateTo(inst domain.BrowserInstance, url string) error {
	return chromedp.Run(inst.Context(), chromedp.Navigate(url))
}

// ─── unit tests (no browser required) ────────────────────────────────────────

// TestSemanticID_Hash verifies that SemanticID returns exactly 16 hex characters.
func TestSemanticID_Hash(t *testing.T) {
	cases := []struct {
		role, name, path string
	}{
		{"button", "Buy Now", "webarea/main/section"},
		{"link", "Home", "webarea/nav"},
		{"textbox", "", "webarea/form"},
		{"", "", ""},
	}

	for _, tc := range cases {
		id := resolver.SemanticID(tc.role, tc.name, tc.path)
		if len(id) != 16 {
			t.Errorf("SemanticID(%q,%q,%q) = %q (len %d), want 16 chars",
				tc.role, tc.name, tc.path, id, len(id))
		}
		for _, ch := range id {
			if !isHexChar(ch) {
				t.Errorf("SemanticID returned non-hex char %q in %q", ch, id)
				break
			}
		}
	}
}

func isHexChar(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// TestSemanticID_Deterministic verifies that the same inputs always produce the same ID.
func TestSemanticID_Deterministic(t *testing.T) {
	const (
		role = "button"
		name = "Buy Now"
		path = "webarea/main/section:product card"
	)
	first := resolver.SemanticID(role, name, path)
	for i := 0; i < 100; i++ {
		got := resolver.SemanticID(role, name, path)
		if got != first {
			t.Fatalf("SemanticID not deterministic at call %d: got %q, want %q", i, got, first)
		}
	}
}

// TestSemanticID_Distinct verifies that different inputs yield different IDs.
func TestSemanticID_Distinct(t *testing.T) {
	id1 := resolver.SemanticID("button", "Buy Now", "webarea/main")
	id2 := resolver.SemanticID("button", "Cancel", "webarea/main")
	id3 := resolver.SemanticID("link", "Buy Now", "webarea/main")

	if id1 == id2 {
		t.Errorf("different names produced same SemanticID: %q", id1)
	}
	if id1 == id3 {
		t.Errorf("different roles produced same SemanticID: %q", id1)
	}
}

// ─── integration tests (require Chrome) ──────────────────────────────────────

// testPageHTML is a minimal page with interactive and non-interactive elements.
const testPageHTML = `data:text/html,<html><body>
<nav><a href="#">Home</a></nav>
<main>
  <button id="buy">Buy Now</button>
  <button id="cancel">Cancel</button>
  <input type="text" aria-label="Search" />
  <a href="/product">Product link</a>
  <p>Some static text paragraph</p>
</main>
</body></html>`

// TestAXTree_Snapshot verifies Snapshot returns a non-nil tree with indexed nodes.
func TestAXTree_Snapshot(t *testing.T) {
	r, cleanup := newBrowserResolver(t, testPageHTML)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tree, err := r.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if tree == nil {
		t.Fatal("Snapshot returned nil tree")
	}
	if tree.Root == nil {
		t.Fatal("tree.Root is nil")
	}
	if len(tree.Index) == 0 {
		t.Fatal("tree.Index is empty — no nodes were indexed")
	}

	// Every indexed node must have a 16-char semantic ID.
	for id, node := range tree.Index {
		if len(id) != 16 {
			t.Errorf("Index key %q has len %d, want 16", id, len(id))
		}
		if node.SemanticID != id {
			t.Errorf("node.SemanticID %q does not match index key %q", node.SemanticID, id)
		}
	}

	t.Logf("Snapshot indexed %d nodes", len(tree.Index))
}

// TestAXTree_SemanticID_Deterministic navigates to the same page twice and
// verifies that the same nodes receive the same semantic IDs.
func TestAXTree_SemanticID_Deterministic(t *testing.T) {
	chromePath := chromiumBinary(t)

	pool, err := browser.NewPool(browser.Config{PoolSize: 1, ChromiumPath: chromePath})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer func() { _ = pool.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inst, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer pool.Release(inst)

	r := resolver.NewAXTreeResolver(inst.Context())

	// First snapshot.
	if err := navigateTo(inst, testPageHTML); err != nil {
		t.Fatalf("navigate (first): %v", err)
	}
	tree1, err := r.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot (first): %v", err)
	}

	// Second snapshot of the same page.
	if err := navigateTo(inst, testPageHTML); err != nil {
		t.Fatalf("navigate (second): %v", err)
	}
	tree2, err := r.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot (second): %v", err)
	}

	// Every semantic ID in tree1 should also be in tree2.
	mismatches := 0
	for id := range tree1.Index {
		if _, ok := tree2.Index[id]; !ok {
			t.Errorf("SemanticID %q present in first snapshot but missing in second", id)
			mismatches++
			if mismatches >= 5 {
				t.Log("(additional mismatches truncated)")
				break
			}
		}
	}
}

// TestAXTree_FindByRole verifies FindByRole locates a button by role+name and
// returns the correct confidence scores.
func TestAXTree_FindByRole(t *testing.T) {
	r, cleanup := newBrowserResolver(t, testPageHTML)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := r.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Exact match.
	matches, err := r.FindByRole("button", "Buy Now")
	if err != nil {
		t.Fatalf("FindByRole: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("FindByRole(button, 'Buy Now') returned no matches")
	}

	best := bestMatch(matches)
	if best.Confidence != 1.0 {
		t.Errorf("exact match confidence = %.2f, want 1.0", best.Confidence)
	}
	if !strings.EqualFold(best.Node.Role, "button") {
		t.Errorf("matched node role = %q, want button", best.Node.Role)
	}

	// Role-only search — no name provided.
	roleOnly, err := r.FindByRole("button", "")
	if err != nil {
		t.Fatalf("FindByRole role-only: %v", err)
	}
	if len(roleOnly) == 0 {
		t.Fatal("FindByRole(button, '') returned no matches")
	}
	for _, m := range roleOnly {
		if m.Confidence != 0.3 {
			t.Errorf("role-only match confidence = %.2f, want 0.3 (name=%q)", m.Confidence, m.Node.Name)
		}
	}

	// No snapshot → should return empty, not error.
	fresh := resolver.NewAXTreeResolver(inst_unused())
	empty, err := fresh.FindByRole("button", "X")
	if err != nil {
		t.Errorf("FindByRole before Snapshot returned error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("FindByRole before Snapshot returned %d results, want 0", len(empty))
	}
}

// TestAXTree_FindInteractable verifies that only clickable/typeable elements
// are returned and non-interactive nodes are excluded.
func TestAXTree_FindInteractable(t *testing.T) {
	r, cleanup := newBrowserResolver(t, testPageHTML)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := r.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	interactable, err := r.FindInteractable()
	if err != nil {
		t.Fatalf("FindInteractable: %v", err)
	}
	if len(interactable) == 0 {
		t.Fatal("FindInteractable returned no elements for test page")
	}

	// These roles must NOT appear in the results.
	nonInteractable := map[string]bool{
		"webarea":          true,
		"rootwebarea":      true,
		"document":         true,
		"genericcontainer": true,
		"statictext":       true,
		"heading":          true,
		"paragraph":        true,
		"none":             true,
		"group":            true,
	}

	for _, node := range interactable {
		role := strings.ToLower(strings.TrimSpace(node.Role))
		if nonInteractable[role] {
			t.Errorf("FindInteractable returned non-interactable role %q (name=%q)", role, node.Name)
		}
	}

	t.Logf("FindInteractable returned %d elements", len(interactable))

	// No snapshot → empty, no error.
	fresh := resolver.NewAXTreeResolver(inst_unused())
	empty, err := fresh.FindInteractable()
	if err != nil {
		t.Errorf("FindInteractable before Snapshot returned error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("FindInteractable before Snapshot returned %d results, want 0", len(empty))
	}
}

// ─── test helpers ─────────────────────────────────────────────────────────────

// bestMatch returns the MatchResult with the highest confidence.
func bestMatch(matches []domain.MatchResult) domain.MatchResult {
	best := matches[0]
	for _, m := range matches[1:] {
		if m.Confidence > best.Confidence {
			best = m
		}
	}
	return best
}

// inst_unused returns a context that cannot communicate with any browser.
// Used to construct resolvers that have no snapshot, purely to test no-snapshot behaviour.
func inst_unused() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled — no browser behind it
	return ctx
}
