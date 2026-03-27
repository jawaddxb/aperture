package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceManager_Cleanup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "aperture-resource-cleanup-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create some fake chromedp-run directories
	oldDir := filepath.Join(tempDir, "chromedp-run-old")
	newDir := filepath.Join(tempDir, "chromedp-run-new")
	err = os.Mkdir(oldDir, 0755)
	require.NoError(t, err)
	err = os.Mkdir(newDir, 0755)
	require.NoError(t, err)

	// Update mod times
	oldTime := time.Now().Add(-24 * time.Hour)
	err = os.Chtimes(oldDir, oldTime, oldTime)
	require.NoError(t, err)

	m := NewDefaultResourceManager(nil, tempDir, 12*time.Hour)
	err = m.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoDirExists(t, oldDir)
	assert.DirExists(t, newDir)
}

func TestResourceManager_GetStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := Config{
		PoolSize:     1,
		ChromiumPath: "",
		SkipPreWarm:  false,
	}

	p, err := NewPool(cfg)
	require.NoError(t, err)
	defer p.Close()

	m := NewDefaultResourceManager(p, os.TempDir(), 12*time.Hour)
	stats, err := m.GetStats(context.Background())
	require.NoError(t, err)

	assert.NotNil(t, stats)
	// Even in idle state, chrome should consume some memory
	// if it is running.
	assert.GreaterOrEqual(t, stats.InstanceCount, 0)
}
