package browser_test

import (
	"context"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// newTestPool creates a pool for use in tests.
func newTestPool(t *testing.T, size int) *browser.Pool {
	t.Helper()
	p, err := browser.NewPool(browser.Config{
		PoolSize:     size,
		ChromiumPath: chromiumPath(t),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestPool_PreWarm verifies that a pool starts with N instances ready.
func TestPool_PreWarm(t *testing.T) {
	const size = 2
	p := newTestPool(t, size)

	if got := p.Size(); got != size {
		t.Errorf("Size() = %d, want %d", got, size)
	}
	if got := p.Available(); got != size {
		t.Errorf("Available() = %d, want %d (pool should be fully pre-warmed)", got, size)
	}
}

// TestPool_AcquireRelease verifies acquire → use → release → re-acquire cycle.
func TestPool_AcquireRelease(t *testing.T) {
	p := newTestPool(t, 2)

	ctx := context.Background()

	inst, err := p.Acquire(ctx)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if inst == nil {
		t.Fatal("Acquire returned nil instance")
	}
	if !inst.IsAlive() {
		t.Error("acquired instance reports IsAlive()=false")
	}
	if inst.ID() == "" {
		t.Error("instance ID must not be empty")
	}
	if inst.CreatedAt().IsZero() {
		t.Error("instance CreatedAt must not be zero")
	}

	// Pool should now have one fewer available instance.
	if got := p.Available(); got != 1 {
		t.Errorf("Available() after Acquire = %d, want 1", got)
	}

	// Release back to pool.
	p.Release(inst)

	// Give pool a moment to process the release.
	time.Sleep(50 * time.Millisecond)

	if got := p.Available(); got != 2 {
		t.Errorf("Available() after Release = %d, want 2", got)
	}

	// Should be acquirable again immediately.
	inst2, err := p.Acquire(ctx)
	if err != nil {
		t.Fatalf("second Acquire after Release: %v", err)
	}
	p.Release(inst2)
}

// TestPool_Exhaustion verifies that acquiring beyond pool size returns ErrPoolExhausted.
func TestPool_Exhaustion(t *testing.T) {
	const size = 2
	p := newTestPool(t, size)

	ctx := context.Background()

	// Acquire all instances.
	instances := make([]domain.BrowserInstance, size)
	for i := range instances {
		inst, err := p.Acquire(ctx)
		if err != nil {
			t.Fatalf("Acquire[%d]: %v", i, err)
		}
		instances[i] = inst
	}

	if got := p.Available(); got != 0 {
		t.Errorf("Available() after acquiring all = %d, want 0", got)
	}

	// Next acquire must time out.
	start := time.Now()
	_, err := p.Acquire(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when pool exhausted, got nil")
	}

	var poolErr *domain.ErrPoolExhausted
	switch typed := err.(type) {
	case *domain.ErrPoolExhausted:
		poolErr = typed
	default:
		t.Fatalf("expected *domain.ErrPoolExhausted, got %T: %v", err, err)
	}

	if poolErr.PoolSize != size {
		t.Errorf("ErrPoolExhausted.PoolSize = %d, want %d", poolErr.PoolSize, size)
	}

	// Should have waited approximately 10 seconds.
	const minWait = 9 * time.Second
	if elapsed < minWait {
		t.Errorf("Acquire returned after %v, expected to block for ~10s", elapsed)
	}

	// Clean up.
	for _, inst := range instances {
		p.Release(inst)
	}
}

// TestPool_AcquireSpeed verifies that Acquire from a pre-warmed pool is fast (<500ms).
func TestPool_AcquireSpeed(t *testing.T) {
	p := newTestPool(t, 1)

	ctx := context.Background()
	start := time.Now()
	inst, err := p.Acquire(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer p.Release(inst)

	const maxLatency = 500 * time.Millisecond
	if elapsed > maxLatency {
		t.Errorf("Acquire took %v, want <%v (pool should be pre-warmed)", elapsed, maxLatency)
	}
}
