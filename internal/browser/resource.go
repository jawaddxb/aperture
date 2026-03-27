package browser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// DefaultResourceManager monitors browser resources and cleans up temp files.
type DefaultResourceManager struct {
	pool      domain.BrowserPool
	tempDir   string
	maxTTL    time.Duration
}

// NewDefaultResourceManager creates a resource manager for the given pool.
func NewDefaultResourceManager(pool domain.BrowserPool, tempDir string, maxTTL time.Duration) *DefaultResourceManager {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &DefaultResourceManager{
		pool:    pool,
		tempDir: tempDir,
		maxTTL:  maxTTL,
	}
}

// GetStats returns current resource usage statistics.
func (m *DefaultResourceManager) GetStats(ctx context.Context) (*domain.ResourceStats, error) {
	stats := &domain.ResourceStats{
		InstanceCount: m.pool.Size() - m.pool.Available(),
	}

	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		// Best-effort memory/CPU using ps
		out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "rss,pcpu,comm").Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, "chrome") || strings.Contains(line, "Chromium") || strings.Contains(line, "Google Chrome") {
					parts := strings.Fields(line)
					if len(parts) >= 3 {
						rss, _ := strconv.ParseInt(parts[0], 10, 64)
						pcpu, _ := strconv.ParseFloat(parts[1], 64)
						stats.MemoryUsageBytes += rss * 1024 // rss is in KB
						stats.CPUUsagePercent += pcpu
					}
				}
			}
		}
	}

	// Disk usage of temp directories
	stats.DiskUsageBytes = m.calculateDiskUsage(m.tempDir)

	return stats, nil
}

// Cleanup removes stale temporary data and expired resources.
func (m *DefaultResourceManager) Cleanup(ctx context.Context) error {
	// 1. Scan and delete old chromedp temp dirs
	files, err := os.ReadDir(m.tempDir)
	if err == nil {
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "chromedp-run") {
				info, err := f.Info()
				if err == nil && time.Since(info.ModTime()) > m.maxTTL {
					_ = os.RemoveAll(filepath.Join(m.tempDir, f.Name()))
				}
			}
		}
	}

	return nil
}

func (m *DefaultResourceManager) calculateDiskUsage(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
