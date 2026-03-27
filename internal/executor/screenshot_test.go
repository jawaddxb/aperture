package executor_test

import (
	"context"
	"testing"

	"github.com/ApertureHQ/aperture/internal/executor"
)

// TestScreenshot_PNG verifies that a viewport screenshot returns non-empty PNG bytes.
func TestScreenshot_PNG(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": "data:text/html,<html><head><title>Shot</title></head><body><h1>Hello Screenshot</h1></body></html>",
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	e := executor.NewScreenshotExecutor()
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"format": "png",
	})
	if err != nil {
		t.Fatalf("screenshot error: %v", err)
	}
	if !result.Success {
		t.Fatalf("screenshot failed: %s", result.Error)
	}
	if len(result.Data) == 0 {
		t.Fatal("expected non-empty PNG bytes in result.Data")
	}
	// Verify PNG magic header: 89 50 4E 47
	if result.Data[0] != 0x89 || result.Data[1] != 0x50 {
		t.Errorf("expected PNG header, got %x %x", result.Data[0], result.Data[1])
	}
	if result.PageState == nil {
		t.Error("PageState should not be nil")
	}
	t.Logf("PNG screenshot: %d bytes", len(result.Data))
}

// TestScreenshot_ElementClip verifies that an element screenshot is smaller
// than the full viewport screenshot (clip to bounding box).
func TestScreenshot_ElementClip(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body style="margin:0;padding:0">` +
		`<div id="box" style="width:50px;height:50px;background:red"></div>` +
		`<div style="width:800px;height:800px;background:blue">large content area</div>` +
		`</body></html>`

	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html,
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	e := executor.NewScreenshotExecutor()

	// Full viewport screenshot.
	fullResult, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"format": "png",
	})
	if err != nil || !fullResult.Success {
		t.Fatalf("full screenshot failed: err=%v result=%+v", err, fullResult)
	}

	// Element-clipped screenshot.
	elemResult, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"format":   "png",
		"selector": "#box",
	})
	if err != nil {
		t.Fatalf("element screenshot error: %v", err)
	}
	if !elemResult.Success {
		t.Fatalf("element screenshot failed: %s", elemResult.Error)
	}
	if len(elemResult.Data) == 0 {
		t.Fatal("expected non-empty element screenshot bytes")
	}
	if len(elemResult.Data) >= len(fullResult.Data) {
		t.Errorf("element screenshot (%d bytes) should be smaller than full viewport (%d bytes)",
			len(elemResult.Data), len(fullResult.Data))
	}
	t.Logf("full=%d bytes, element=%d bytes", len(fullResult.Data), len(elemResult.Data))
}
