// Package auth provides AuthPersistence implementations for Aperture.
// FileAuthPersistence saves and restores browser cookies as JSON files.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	cdpnetwork "github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	// defaultCookieDir is the default storage directory for cookie jars.
	defaultCookieDir = "~/.aperture/cookies"
)

// FileAuthPersistence implements domain.AuthPersistence by storing
// CookieJar instances as JSON files under a configurable directory.
// One file per session: {storageDir}/{sessionID}.json.
type FileAuthPersistence struct {
	storageDir string
}

// FileAuthConfig holds constructor parameters for FileAuthPersistence.
type FileAuthConfig struct {
	// StorageDir is the directory where cookie JSON files are stored.
	// Defaults to ~/.aperture/cookies when empty.
	StorageDir string
}

// NewFileAuthPersistence constructs a FileAuthPersistence.
// It creates storageDir if it does not exist.
func NewFileAuthPersistence(cfg FileAuthConfig) (*FileAuthPersistence, error) {
	dir := resolveDir(cfg.StorageDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("auth: create storage dir %q: %w", dir, err)
	}
	return &FileAuthPersistence{storageDir: dir}, nil
}

// SaveCookies exports all cookies from inst and persists them for sessionID.
// Overwrites any previously saved jar for that session.
func (f *FileAuthPersistence) SaveCookies(ctx context.Context, sessionID string, inst domain.BrowserInstance) error {
	cookies, err := exportCookies(inst.Context())
	if err != nil {
		return fmt.Errorf("auth.SaveCookies: export: %w", err)
	}

	jar := domain.CookieJar{
		SessionID: sessionID,
		Cookies:   cookies,
		SavedAt:   time.Now().UTC(),
	}

	data, err := json.Marshal(jar)
	if err != nil {
		return fmt.Errorf("auth.SaveCookies: marshal: %w", err)
	}

	path := f.jarPath(sessionID)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("auth.SaveCookies: write %q: %w", path, err)
	}
	return nil
}

// LoadCookies imports previously persisted cookies for sessionID into inst.
// Returns nil (no error) when no persisted cookies exist for the session.
func (f *FileAuthPersistence) LoadCookies(ctx context.Context, sessionID string, inst domain.BrowserInstance) error {
	path := f.jarPath(sessionID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil // no saved cookies — not an error
	}
	if err != nil {
		return fmt.Errorf("auth.LoadCookies: read %q: %w", path, err)
	}

	var jar domain.CookieJar
	if err := json.Unmarshal(data, &jar); err != nil {
		return fmt.Errorf("auth.LoadCookies: unmarshal: %w", err)
	}

	return importCookies(inst.Context(), jar.Cookies)
}

// ClearCookies removes the persisted cookie file for sessionID.
// Returns nil when no file exists for that session.
func (f *FileAuthPersistence) ClearCookies(_ context.Context, sessionID string) error {
	path := f.jarPath(sessionID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("auth.ClearCookies: remove %q: %w", path, err)
	}
	return nil
}

// jarPath returns the storage path for a session's cookie jar.
func (f *FileAuthPersistence) jarPath(sessionID string) string {
	return filepath.Join(f.storageDir, sessionID+".json")
}

// exportCookies retrieves all cookies from the browser via CDP.
func exportCookies(browserCtx context.Context) ([]domain.Cookie, error) {
	var cdpCookies []*cdpnetwork.Cookie
	err := chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cdpCookies, err = cdpnetwork.GetCookies().Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return convertFromCDP(cdpCookies), nil
}

// importCookies sets cookies in the browser via CDP SetCookies.
func importCookies(browserCtx context.Context, cookies []domain.Cookie) error {
	if len(cookies) == 0 {
		return nil
	}
	params := convertToCDPParams(cookies)
	return chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return cdpnetwork.SetCookies(params).Do(ctx)
		}),
	)
}

// convertFromCDP maps CDP cookie structs to domain.Cookie values.
func convertFromCDP(src []*cdpnetwork.Cookie) []domain.Cookie {
	out := make([]domain.Cookie, 0, len(src))
	for _, c := range src {
		out = append(out, domain.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  int64(c.Expires),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
		})
	}
	return out
}

// convertToCDPParams maps domain.Cookie values to CDP CookieParam structs.
func convertToCDPParams(src []domain.Cookie) []*cdpnetwork.CookieParam {
	out := make([]*cdpnetwork.CookieParam, 0, len(src))
	for _, c := range src {
		p := &cdpnetwork.CookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
		}
		out = append(out, p)
	}
	return out
}

// resolveDir expands ~ in dir and returns the result.
func resolveDir(dir string) string {
	if dir == "" {
		dir = defaultCookieDir
	}
	if len(dir) >= 2 && dir[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	return dir
}

// compile-time interface assertion.
var _ domain.AuthPersistence = (*FileAuthPersistence)(nil)
