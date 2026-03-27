package browser_test

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// TestDownloadManager_Basic verifies that a download can be tracked.
func TestDownloadManager_Basic(t *testing.T) {
	// We use the same pool-based setup as in executor tests for reliability.
	p, err := browser.NewPool(browser.Config{
		PoolSize:     1,
		ChromiumPath: chromiumPath(t),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer p.Close()

	inst, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer p.Release(inst)

	tmpDir := t.TempDir()
	dm := inst.Downloads()
	if err := dm.SetDownloadBehavior(inst.Context(), tmpDir); err != nil {
		t.Fatalf("SetDownloadBehavior failed: %v", err)
	}

	// Trigger a download by navigating to a data URL with a download header.
	html := `data:text/html,<html><body><a id="dl" href="data:application/octet-stream,hello" download="test.txt">Download</a></body></html>`
	
	err = chromedp.Run(inst.Context(), 
		chromedp.Navigate(html),
		chromedp.Click("#dl", chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("failed to trigger download: %v", err)
	}

	// Wait for the download to appear in the manager.
	var downloads []domain.Download
	for i := 0; i < 50; i++ {
		downloads = dm.ListDownloads()
		if len(downloads) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(downloads) == 0 {
		t.Fatal("no downloads tracked after 5 seconds")
	}

	d := downloads[0]
	if d.Filename != "test.txt" {
		t.Errorf("filename = %q, want %q", d.Filename, "test.txt")
	}

	// Wait for completion.
	for i := 0; i < 50; i++ {
		if d.State == "completed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
		d, _ = dm.GetDownload(d.GUID)
	}

	if d.State != "completed" {
		t.Errorf("final state = %q, want %q", d.State, "completed")
	}
}
