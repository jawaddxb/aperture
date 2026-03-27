// Package observe provides structured logging and metrics for Aperture.
// This file implements InMemoryMetrics, a thread-safe in-memory MetricsCollector.
package observe

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// actionStats holds per-action counters.
type actionStats struct {
	count       int64 // total invocations
	errCount    int64 // failed invocations
	totalNs     int64 // total duration in nanoseconds
}

// InMemoryMetrics collects runtime statistics using atomic counters.
// Safe for concurrent use; Snapshot does not hold a lock during read.
type InMemoryMetrics struct {
	totalActions  atomic.Int64
	totalSessions atomic.Int64
	totalErrors   atomic.Int64

	mu      sync.RWMutex
	actions map[string]*actionStats
}

// NewInMemoryMetrics constructs a ready-to-use InMemoryMetrics.
func NewInMemoryMetrics() *InMemoryMetrics {
	return &InMemoryMetrics{
		actions: make(map[string]*actionStats),
	}
}

// RecordAction records a single action execution.
func (m *InMemoryMetrics) RecordAction(action string, duration time.Duration, success bool) {
	m.totalActions.Add(1)

	stats := m.getOrCreate(action)
	atomic.AddInt64(&stats.count, 1)
	atomic.AddInt64(&stats.totalNs, int64(duration))
	if !success {
		atomic.AddInt64(&stats.errCount, 1)
		m.totalErrors.Add(1)
	}
}

// RecordSession records session-level aggregates.
func (m *InMemoryMetrics) RecordSession(_ time.Duration, stepsTotal int, stepsFailed int) {
	m.totalSessions.Add(1)
	// Session-level step counts contribute to global action/error tallies
	// via individual RecordAction calls; we track sessions separately here.
	_ = stepsTotal
	_ = stepsFailed
}

// Snapshot returns a point-in-time copy of all metrics.
// It holds the read lock only to copy the actions map keys; atomic reads
// of counters happen without holding the lock.
func (m *InMemoryMetrics) Snapshot() *domain.MetricsSnapshot {
	totalActions := m.totalActions.Load()
	totalErrors := m.totalErrors.Load()

	m.mu.RLock()
	actionKeys := make([]string, 0, len(m.actions))
	for k := range m.actions {
		actionKeys = append(actionKeys, k)
	}
	m.mu.RUnlock()

	counts := make(map[string]int64, len(actionKeys))
	avgDurations := make(map[string]float64, len(actionKeys))

	for _, key := range actionKeys {
		m.mu.RLock()
		stats := m.actions[key]
		m.mu.RUnlock()

		cnt := atomic.LoadInt64(&stats.count)
		totalNs := atomic.LoadInt64(&stats.totalNs)
		counts[key] = cnt
		if cnt > 0 {
			avgDurations[key] = float64(totalNs) / float64(cnt) / 1e6 // → ms
		}
	}

	var errorRate float64
	if totalActions > 0 {
		errorRate = float64(totalErrors) / float64(totalActions)
	}

	return &domain.MetricsSnapshot{
		TotalActions:  totalActions,
		TotalSessions: m.totalSessions.Load(),
		ActionCounts:  counts,
		AvgDurationMs: avgDurations,
		ErrorRate:     errorRate,
	}
}

// getOrCreate returns the actionStats for key, creating it if absent.
func (m *InMemoryMetrics) getOrCreate(key string) *actionStats {
	m.mu.RLock()
	s, ok := m.actions[key]
	m.mu.RUnlock()
	if ok {
		return s
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock.
	if s, ok = m.actions[key]; ok {
		return s
	}
	s = &actionStats{}
	m.actions[key] = s
	return s
}

// compile-time interface check.
var _ domain.MetricsCollector = (*InMemoryMetrics)(nil)
