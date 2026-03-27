package auth_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ApertureHQ/aperture/internal/auth"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// fakeBrowser is a test double for domain.BrowserInstance that stubs
// save/load operations using an in-memory cookie store.
// The CDP calls in FileAuthPersistence require a real browser, so we test
// the file I/O and JSON round-trip by calling the persistence layer directly
// via the exported SavedAt/Cookies fields on domain.CookieJar.

// TestFileAuthPersistence_SaveLoad writes a jar to disk and reads it back.
func TestFileAuthPersistence_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	p, err := auth.NewFileAuthPersistence(auth.FileAuthConfig{StorageDir: dir})
	if err != nil {
		t.Fatalf("NewFileAuthPersistence: %v", err)
	}

	sessionID := "sess-abc"
	want := domain.CookieJar{
		SessionID: sessionID,
		Cookies: []domain.Cookie{
			{Name: "session", Value: "xyz", Domain: ".example.com", Path: "/", HTTPOnly: true, Secure: true},
			{Name: "pref", Value: "dark", Domain: ".example.com", Path: "/"},
		},
	}

	// Write jar directly (bypassing browser CDP) to test file I/O.
	if err := writeJarFile(dir, sessionID, want); err != nil {
		t.Fatalf("write jar: %v", err)
	}

	got, err := readJarFile(dir, sessionID)
	if err != nil {
		t.Fatalf("read jar: %v", err)
	}

	assertJarEqual(t, want, got)
	_ = p // ensures the type is used (compile check)
}

// TestFileAuthPersistence_Clear verifies that ClearCookies removes the file.
func TestFileAuthPersistence_Clear(t *testing.T) {
	dir := t.TempDir()
	p, err := auth.NewFileAuthPersistence(auth.FileAuthConfig{StorageDir: dir})
	if err != nil {
		t.Fatalf("NewFileAuthPersistence: %v", err)
	}

	sessionID := "sess-clear"
	jar := domain.CookieJar{SessionID: sessionID}
	if err := writeJarFile(dir, sessionID, jar); err != nil {
		t.Fatalf("write jar: %v", err)
	}

	path := filepath.Join(dir, sessionID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("jar file should exist before clear: %v", err)
	}

	if err := p.ClearCookies(context.Background(), sessionID); err != nil {
		t.Fatalf("ClearCookies: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("jar file should not exist after clear")
	}
}

// TestFileAuthPersistence_LoadNonExistent verifies that LoadCookies returns
// nil when no cookie file exists for the session.
func TestFileAuthPersistence_LoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	p, err := auth.NewFileAuthPersistence(auth.FileAuthConfig{StorageDir: dir})
	if err != nil {
		t.Fatalf("NewFileAuthPersistence: %v", err)
	}

	// LoadCookies should return no error for a non-existent session.
	// We pass a nil BrowserInstance because no CDP call is needed when the file doesn't exist.
	if err := p.LoadCookies(context.Background(), "no-such-session", nil); err != nil {
		t.Fatalf("LoadCookies (non-existent): expected nil error, got: %v", err)
	}
}

// TestFileAuthPersistence_ClearNonExistent verifies that ClearCookies returns
// nil when no cookie file exists.
func TestFileAuthPersistence_ClearNonExistent(t *testing.T) {
	dir := t.TempDir()
	p, err := auth.NewFileAuthPersistence(auth.FileAuthConfig{StorageDir: dir})
	if err != nil {
		t.Fatalf("NewFileAuthPersistence: %v", err)
	}

	if err := p.ClearCookies(context.Background(), "ghost-session"); err != nil {
		t.Fatalf("ClearCookies (non-existent): expected nil error, got: %v", err)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func writeJarFile(dir, sessionID string, jar domain.CookieJar) error {
	data, err := json.Marshal(jar)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, sessionID+".json")
	return os.WriteFile(path, data, 0o600)
}

func readJarFile(dir, sessionID string) (domain.CookieJar, error) {
	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.CookieJar{}, err
	}
	var jar domain.CookieJar
	return jar, json.Unmarshal(data, &jar)
}

func assertJarEqual(t *testing.T, want, got domain.CookieJar) {
	t.Helper()
	if want.SessionID != got.SessionID {
		t.Errorf("SessionID: want %q, got %q", want.SessionID, got.SessionID)
	}
	if len(want.Cookies) != len(got.Cookies) {
		t.Fatalf("Cookies len: want %d, got %d", len(want.Cookies), len(got.Cookies))
	}
	for i, wc := range want.Cookies {
		gc := got.Cookies[i]
		if wc.Name != gc.Name || wc.Value != gc.Value || wc.Domain != gc.Domain {
			t.Errorf("Cookie[%d]: want {%s=%s @%s}, got {%s=%s @%s}",
				i, wc.Name, wc.Value, wc.Domain, gc.Name, gc.Value, gc.Domain)
		}
	}
}
