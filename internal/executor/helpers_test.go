// Package executor_test provides shared test helpers for the executor package.
package executor_test

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// ─── browser helpers ──────────────────────────────────────────────────────────

// chromiumPath returns the path to a Chromium/Chrome binary for tests.
// Override via APERTURE_CHROMIUM_PATH environment variable.
func chromiumPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("APERTURE_CHROMIUM_PATH"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	case "linux":
		for _, p := range []string{"/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome"} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	t.Skip("no Chromium binary found; set APERTURE_CHROMIUM_PATH to enable")
	return ""
}

// newTestBrowserInstance creates a single browser instance for use in tests.
func newTestBrowserInstance(t *testing.T) domain.BrowserInstance {
	t.Helper()
	p, err := browser.NewPool(browser.Config{
		PoolSize:     1,
		ChromiumPath: chromiumPath(t),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	inst, err := p.Acquire(context.Background())
	if err != nil {
		_ = p.Close()
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(func() {
		p.Release(inst)
		_ = p.Close()
	})
	return inst
}

// ─── JS assertion helpers ─────────────────────────────────────────────────────

// assertJSBool evaluates a JS expression and asserts it is truthy.
// Returns false without failing when the expression returns false.
func assertJSBool(t *testing.T, inst domain.BrowserInstance, expr string) bool {
	t.Helper()
	var val bool
	err := chromedp.Run(inst.Context(), chromedp.Evaluate(expr, &val))
	if err != nil {
		t.Errorf("JS eval %q: %v", expr, err)
		return false
	}
	return val
}

// assertJSString evaluates a JS expression and returns the string result.
func assertJSString(t *testing.T, inst domain.BrowserInstance, expr string) string {
	t.Helper()
	var val string
	err := chromedp.Run(inst.Context(), chromedp.Evaluate(expr, &val))
	if err != nil {
		t.Errorf("JS eval %q: %v", expr, err)
	}
	return val
}

// ─── stub BrowserInstance ─────────────────────────────────────────────────────

// stubBrowserInstance satisfies domain.BrowserInstance for unit tests that do
// not require a real Chromium process (e.g. resolver-only tests).
type stubBrowserInstance struct {
	ctx context.Context
}

func (s *stubBrowserInstance) Context() context.Context { return s.ctx }
func (s *stubBrowserInstance) ID() string               { return "stub-0" }
func (s *stubBrowserInstance) CreatedAt() time.Time     { return time.Time{} }
func (s *stubBrowserInstance) IsAlive() bool            { return false }
func (s *stubBrowserInstance) Close() error             { return nil }

// inst_unused returns a cancelled context for tests that never use the browser.
func inst_unused() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// ─── canned resolver ──────────────────────────────────────────────────────────

// cannedUnifiedResolver is a test-only domain.UnifiedResolver that returns a
// pre-configured Candidate (or error) regardless of the target.
// selector overrides the candidate's SemanticID so selectorForCandidate returns
// a valid CSS selector (#id or [selector]).
type cannedUnifiedResolver struct {
	candidate domain.Candidate
	selector  string
	err       error
}

// newCannedResolver builds a resolver that always returns the given candidate.
// selector is stored in the candidate's SemanticID field using the "raw:" sentinel
// prefix recognised by selectorForCandidate, so the executor uses it verbatim as
// a CSS selector when acting on the element (e.g. "#btn").
func newCannedResolver(c domain.Candidate, selector string) domain.UnifiedResolver {
	// "raw:<selector>" is the sentinel prefix handled by selectorForCandidate.
	c.SemanticID = "raw:" + selector
	return &cannedUnifiedResolver{candidate: c, selector: selector}
}

// newFailingResolver builds a resolver that always returns the given error.
func newFailingResolver(err error) domain.UnifiedResolver {
	return &cannedUnifiedResolver{err: err}
}

// Resolve implements domain.UnifiedResolver.
func (c *cannedUnifiedResolver) Resolve(
	_ context.Context,
	_ domain.ResolutionTarget,
	_ domain.BrowserInstance,
) (*domain.Resolution, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &domain.Resolution{
		Tier:       domain.TierAXTree,
		Confidence: c.candidate.Confidence,
		Candidates: []domain.Candidate{c.candidate},
	}, nil
}


