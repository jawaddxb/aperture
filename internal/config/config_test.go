package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ApertureHQ/aperture/internal/config"
)

// writeYAML writes content to a temp file and returns its path.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "aperture-*.yaml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing temp file: %v", err)
	}
	return f.Name()
}

// TestConfigLoad verifies that a YAML file is parsed into the Config struct correctly.
func TestConfigLoad(t *testing.T) {
	yaml := `
server:
  port: 9090
  host: "127.0.0.1"
browser:
  pool_size: 3
  chromium_path: "/usr/bin/chromium"
redis:
  url: "redis://redis:6379"
sqlite:
  path: "/tmp/test.db"
log:
  level: "debug"
api:
  key_prefix: "apt_sk_"
`
	path := writeYAML(t, yaml)
	cfg, err := config.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected server.port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected server.host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Browser.PoolSize != 3 {
		t.Errorf("expected browser.pool_size 3, got %d", cfg.Browser.PoolSize)
	}
	if cfg.Browser.ChromiumPath != "/usr/bin/chromium" {
		t.Errorf("expected browser.chromium_path /usr/bin/chromium, got %s", cfg.Browser.ChromiumPath)
	}
	if cfg.Redis.URL != "redis://redis:6379" {
		t.Errorf("expected redis.url redis://redis:6379, got %s", cfg.Redis.URL)
	}
	if cfg.SQLite.Path != "/tmp/test.db" {
		t.Errorf("expected sqlite.path /tmp/test.db, got %s", cfg.SQLite.Path)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log.level debug, got %s", cfg.Log.Level)
	}
	if cfg.API.KeyPrefix != "apt_sk_" {
		t.Errorf("expected api.key_prefix apt_sk_, got %s", cfg.API.KeyPrefix)
	}
}

// TestConfigEnvOverride verifies that APERTURE_* env vars override YAML values.
func TestConfigEnvOverride(t *testing.T) {
	yaml := `
server:
  port: 8080
  host: "0.0.0.0"
browser:
  chromium_path: "/usr/bin/chromium"
`
	path := writeYAML(t, yaml)

	t.Setenv("APERTURE_SERVER_PORT", "9999")
	t.Setenv("APERTURE_SERVER_HOST", "127.0.0.1")

	cfg, err := config.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Errorf("expected env-overridden port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected env-overridden host 127.0.0.1, got %s", cfg.Server.Host)
	}
}

// TestConfigValidation verifies that missing required fields return a clear error.
func TestConfigValidation(t *testing.T) {
	// Config without browser.chromium_path should fail validation.
	yaml := `
server:
  port: 8080
`
	path := writeYAML(t, yaml)

	// Ensure the env var isn't set from a prior test.
	t.Setenv("APERTURE_BROWSER_CHROMIUM_PATH", "")

	_, err := config.LoadFromFile(path)
	if err == nil {
		t.Fatal("expected validation error for missing browser.chromium_path, got nil")
	}

	_ = filepath.Base(path) // suppress unused import warning
}
