// Package e2e contains end-to-end integration tests for Aperture.
// Tests use real Chrome and local HTTP test servers.
// All tests are skipped when testing.Short() is true or Chrome is unavailable.
package e2e_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	browserpool "github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/ApertureHQ/aperture/internal/resolver"
	"github.com/ApertureHQ/aperture/internal/sequencer"
)

// ─── test infrastructure ──────────────────────────────────────────────────────

// testdataDir returns the absolute path to the testdata directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("testdata dir: %v", err)
	}
	return dir
}

// serveTestdata starts an httptest.Server serving the testdata directory.
func serveTestdata(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.FileServer(http.Dir(testdataDir(t))))
}

// findChromium locates a Chrome binary or skips the test.
func findChromium(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	t.Skip("Chrome not available")
	return ""
}

// newPool creates a 1-instance pool or skips on failure.
func newPool(t *testing.T, chromePath string) *browserpool.Pool {
	t.Helper()
	pool, err := browserpool.NewPool(browserpool.Config{PoolSize: 1, ChromiumPath: chromePath})
	if err != nil {
		t.Skipf("pool creation failed (Chrome unavailable?): %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// acquireInst acquires a browser instance from pool or fails the test.
func acquireInst(t *testing.T, pool *browserpool.Pool) domain.BrowserInstance {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	inst, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(func() { pool.Release(inst) })
	return inst
}

// buildRegistry builds the standard executor registry.
func buildRegistry() map[string]domain.Executor {
	res := resolver.NewUnifiedResolver()
	return map[string]domain.Executor{
		"navigate":   executor.NewNavigateExecutor(),
		"click":      executor.NewClickExecutor(res),
		"type":       executor.NewTypeExecutor(res),
		"screenshot": executor.NewScreenshotExecutor(),
		"wait":       executor.NewWaitExecutor(),
		"select":     executor.NewSelectExecutor(res),
	}
}

// newSequencer returns a DefaultSequencer with the standard registry.
func newSequencer() *sequencer.DefaultSequencer {
	return sequencer.NewDefaultSequencer(sequencer.Config{Registry: buildRegistry()})
}

// testCtx returns a 30-second context for a single test.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// navigate is a helper to navigate to a URL and fail fast on error.
func navigate(t *testing.T, ctx context.Context, inst domain.BrowserInstance, url string) {
	t.Helper()
	navExec := executor.NewNavigateExecutor()
	res, err := navExec.Execute(ctx, inst, map[string]interface{}{"url": url})
	if err != nil {
		t.Fatalf("navigate error: %v", err)
	}
	if !res.Success {
		t.Fatalf("navigate failed: %s", res.Error)
	}
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestE2E_NavigateAndScreenshot navigates to login.html and takes a screenshot.
func TestE2E_NavigateAndScreenshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e in short mode")
	}
	chromePath := findChromium(t)
	pool := newPool(t, chromePath)
	inst := acquireInst(t, pool)
	ctx := testCtx(t)

	srv := serveTestdata(t)
	defer srv.Close()

	navigate(t, ctx, inst, srv.URL+"/login.html")

	ssExec := executor.NewScreenshotExecutor()
	res, err := ssExec.Execute(ctx, inst, map[string]interface{}{"fullPage": false, "format": "png"})
	if err != nil {
		t.Fatalf("screenshot error: %v", err)
	}
	if !res.Success {
		t.Fatalf("screenshot failed: %s", res.Error)
	}
	if len(res.Data) == 0 {
		t.Fatal("screenshot returned empty data")
	}
	// Verify PNG magic bytes.
	if !bytes.HasPrefix(res.Data, []byte("\x89PNG")) {
		t.Errorf("expected PNG, got prefix: %x", res.Data[:4])
	}
}

// TestE2E_FillLoginForm types email/password and clicks submit.
func TestE2E_FillLoginForm(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e in short mode")
	}
	chromePath := findChromium(t)
	pool := newPool(t, chromePath)
	inst := acquireInst(t, pool)
	ctx := testCtx(t)

	srv := serveTestdata(t)
	defer srv.Close()

	navigate(t, ctx, inst, srv.URL+"/login.html")

	res := resolver.NewUnifiedResolver()

	// Type email.
	typeExec := executor.NewTypeExecutor(res)
	r, err := typeExec.Execute(ctx, inst, map[string]interface{}{
		"selector": "#email",
		"input":    "user@example.com",
	})
	if err != nil || !r.Success {
		t.Fatalf("type email: err=%v result=%+v", err, r)
	}

	// Type password.
	r, err = typeExec.Execute(ctx, inst, map[string]interface{}{
		"selector": "#password",
		"input":    "secret123",
	})
	if err != nil || !r.Success {
		t.Fatalf("type password: err=%v result=%+v", err, r)
	}

	// Click submit.
	clickExec := executor.NewClickExecutor(res)
	r, err = clickExec.Execute(ctx, inst, map[string]interface{}{
		"selector": "#submitBtn",
	})
	if err != nil || !r.Success {
		t.Fatalf("click submit: err=%v result=%+v", err, r)
	}

	// Wait for success message to appear.
	waitExec := executor.NewWaitExecutor()
	r, err = waitExec.Execute(ctx, inst, map[string]interface{}{
		"strategy": "selector",
		"selector": "#success",
	})
	if err != nil || !r.Success {
		t.Fatalf("wait for success: err=%v result=%+v", err, r)
	}
}

// TestE2E_SelectDropdown selects an option in the form.html dropdown.
func TestE2E_SelectDropdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e in short mode")
	}
	chromePath := findChromium(t)
	pool := newPool(t, chromePath)
	inst := acquireInst(t, pool)
	ctx := testCtx(t)

	srv := serveTestdata(t)
	defer srv.Close()

	navigate(t, ctx, inst, srv.URL+"/form.html")

	res := resolver.NewUnifiedResolver()
	selExec := executor.NewSelectExecutor(res)
	r, err := selExec.Execute(ctx, inst, map[string]interface{}{
		"target": "#color",
		"value":  "blue",
	})
	if err != nil || !r.Success {
		t.Fatalf("select dropdown: err=%v result=%+v", err, r)
	}
}

// TestE2E_DynamicContent clicks the reveal button and verifies hidden div appears.
func TestE2E_DynamicContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e in short mode")
	}
	chromePath := findChromium(t)
	pool := newPool(t, chromePath)
	inst := acquireInst(t, pool)
	ctx := testCtx(t)

	srv := serveTestdata(t)
	defer srv.Close()

	navigate(t, ctx, inst, srv.URL+"/dynamic.html")

	res := resolver.NewUnifiedResolver()

	// Click the reveal button.
	clickExec := executor.NewClickExecutor(res)
	r, err := clickExec.Execute(ctx, inst, map[string]interface{}{
		"selector": "#revealBtn",
	})
	if err != nil || !r.Success {
		t.Fatalf("click reveal: err=%v result=%+v", err, r)
	}

	// Wait for the hidden div to become visible.
	waitExec := executor.NewWaitExecutor()
	r, err = waitExec.Execute(ctx, inst, map[string]interface{}{
		"strategy": "selector",
		"selector": "#hidden",
	})
	if err != nil || !r.Success {
		t.Fatalf("wait for hidden div: err=%v result=%+v", err, r)
	}
}

// TestE2E_StaticPlanner plans "navigate to URL" and executes it via sequencer.
func TestE2E_StaticPlanner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e in short mode")
	}
	chromePath := findChromium(t)
	pool := newPool(t, chromePath)
	inst := acquireInst(t, pool)
	ctx := testCtx(t)

	srv := serveTestdata(t)
	defer srv.Close()

	goal := "navigate to " + srv.URL + "/login.html"
	p := planner.NewStaticPlanner()
	plan, err := p.Plan(ctx, goal, nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("plan has no steps")
	}

	seq := newSequencer()
	result, err := seq.Run(ctx, inst, plan)
	if err != nil {
		t.Fatalf("sequencer.Run: %v", err)
	}
	if !result.Success {
		t.Fatalf("run failed: step %d", result.FailedStep)
	}
}
