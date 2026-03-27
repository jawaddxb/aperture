package executor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
)

// TestUploadExecutor_Basic verifies that setting files on an input works.
func TestUploadExecutor_Basic(t *testing.T) {
	inst := newTestBrowserInstance(t)
	// We don't call inst.Close() because newTestBrowserInstance uses t.Cleanup to release and close the pool.

	// Create a temporary file to upload.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	html := `data:text/html,<html><body><input id="file-input" type="file"></body></html>`
	nav := executor.NewNavigateExecutor()
	if r, err := nav.Execute(context.Background(), inst, map[string]interface{}{
		"url": html, "wait": "load",
	}); err != nil || !r.Success {
		t.Fatalf("navigate failed: err=%v result=%+v", err, r)
	}

	resolver := newCannedResolver(domain.Candidate{Confidence: 1.0, Role: "file"}, "#file-input")
	e := executor.NewUploadExecutor(resolver)

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"target": "#file-input",
		"files":  []string{tmpFile},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Upload failed: %s", result.Error)
	}

	// Verify the file was set.
	val := assertJSString(t, inst, `document.querySelector("#file-input").files[0].name`)
	if val != "test.txt" {
		t.Errorf("file name = %q, want %q", val, "test.txt")
	}
}

// TestUploadExecutor_MissingFiles verifies failure when files param is missing.
func TestUploadExecutor_MissingFiles(t *testing.T) {
	inst := newTestBrowserInstance(t)

	resolver := newCannedResolver(domain.Candidate{Confidence: 1.0}, "#x")
	e := executor.NewUploadExecutor(resolver)

	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"target": "#x",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false when files param is missing")
	}
}
