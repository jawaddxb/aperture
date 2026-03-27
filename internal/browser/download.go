package browser

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ChromeDownloadManager implements domain.DownloadManager for Chrome.
// It tracks file downloads via CDP Page events.
type ChromeDownloadManager struct {
	mu           sync.RWMutex
	downloads    map[string]domain.Download
	downloadPath string
}

// NewChromeDownloadManager constructs a new ChromeDownloadManager.
func NewChromeDownloadManager() *ChromeDownloadManager {
	return &ChromeDownloadManager{
		downloads: make(map[string]domain.Download),
	}
}

// SetDownloadBehavior configures the browser to save downloads to downloadPath.
// It also sets up listeners for download progress events.
func (m *ChromeDownloadManager) SetDownloadBehavior(ctx context.Context, downloadPath string) error {
	m.mu.Lock()
	m.downloadPath = downloadPath
	m.mu.Unlock()

	// Enable browser.SetDownloadBehavior to intercept downloads.
	err := chromedp.Run(ctx, browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllow).
		WithDownloadPath(downloadPath).
		WithEventsEnabled(true))
	if err != nil {
		return fmt.Errorf("set download behavior: %w", err)
	}

	// Listen for browser.EventDownloadWillBegin and browser.EventDownloadProgress.
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *browser.EventDownloadWillBegin:
			m.handleDownloadWillBegin(e)
		case *browser.EventDownloadProgress:
			m.handleDownloadProgress(e)
		}
	})

	return nil
}

// ListDownloads returns all tracked downloads.
func (m *ChromeDownloadManager) ListDownloads() []domain.Download {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]domain.Download, 0, len(m.downloads))
	for _, d := range m.downloads {
		res = append(res, d)
	}
	return res
}

// GetDownload returns a download by its GUID.
func (m *ChromeDownloadManager) GetDownload(guid string) (domain.Download, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.downloads[guid]
	return d, ok
}

// handleDownloadWillBegin initializes a new download record.
func (m *ChromeDownloadManager) handleDownloadWillBegin(e *browser.EventDownloadWillBegin) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.downloads[e.GUID] = domain.Download{
		GUID:      e.GUID,
		URL:       e.URL,
		Filename:  e.SuggestedFilename,
		LocalPath: filepath.Join(m.downloadPath, e.SuggestedFilename),
		State:     "in_progress",
	}
}

// handleDownloadProgress updates an existing download record.
func (m *ChromeDownloadManager) handleDownloadProgress(e *browser.EventDownloadProgress) {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.downloads[e.GUID]
	if !ok {
		return // should not happen
	}

	d.TotalBytes = int64(e.TotalBytes)
	d.ReceivedBytes = int64(e.ReceivedBytes)

	switch e.State {
	case browser.DownloadProgressStateInProgress:
		d.State = "in_progress"
	case browser.DownloadProgressStateCompleted:
		d.State = "completed"
	case browser.DownloadProgressStateCanceled:
		d.State = "canceled"
	}

	m.downloads[e.GUID] = d
}
