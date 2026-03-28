// Package checkpoint provides maintenance utilities for task checkpoint files.
package checkpoint

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// StartCleaner spawns a background goroutine that periodically removes
// checkpoint files older than ttl from dir.
func StartCleaner(dir string, ttl time.Duration, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			clean(dir, ttl)
		}
	}()
}

// clean removes files in dir whose modification time is older than ttl.
func clean(dir string, ttl time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // directory may not exist yet
	}
	cutoff := time.Now().Add(-ttl)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, e.Name())
			if err := os.Remove(path); err != nil {
				slog.Warn("checkpoint cleaner: failed to remove", "path", path, "error", err)
			} else {
				slog.Debug("checkpoint cleaner: removed expired", "path", path)
			}
		}
	}
}
