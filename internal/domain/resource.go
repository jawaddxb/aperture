package domain

import (
	"context"
)

// ResourceStats holds metrics about browser resource consumption.
type ResourceStats struct {
	MemoryUsageBytes int64
	CPUUsagePercent  float64
	DiskUsageBytes   int64
	InstanceCount    int
}

// ResourceManager monitors and cleans up browser resources.
type ResourceManager interface {
	// GetStats returns current resource usage statistics.
	GetStats(ctx context.Context) (*ResourceStats, error)
	// Cleanup removes stale temporary data and expired resources.
	Cleanup(ctx context.Context) error
}
