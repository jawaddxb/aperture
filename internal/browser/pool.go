package browser

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

const (
	// acquireTimeout is the maximum time Acquire will block when the pool is exhausted.
	acquireTimeout = 10 * time.Second
	// crashCheckInterval is how often the pool monitors for crashed instances.
	crashCheckInterval = 5 * time.Second
)

// Pool implements domain.BrowserPool.
// It manages N headless Chromium instances, pre-warming them on startup and
// detecting crashes via a background monitor goroutine.
type Pool struct {
	cfg         Config
	available   chan domain.BrowserInstance // buffered channel acts as the pool queue
	mu          sync.Mutex                  // guards allInstances
	allInstances []*instance
	counter     atomic.Int64 // instance ID counter
	closed      atomic.Bool
	stopMonitor chan struct{}
	wg          sync.WaitGroup
}

// Config holds pool configuration values.
type Config struct {
	// PoolSize is the number of pre-warmed Chromium instances to maintain.
	PoolSize int
	// ChromiumPath is the absolute path to the Chromium/Chrome executable.
	ChromiumPath string
	// SkipPreWarm disables instance pre-warming during NewPool.
	// Zero value (false) means pre-warm on startup — production default.
	// Set to true in tests to avoid requiring a real Chromium binary.
	SkipPreWarm bool
}

// NewPool creates and pre-warms a Pool with cfg.PoolSize Chromium instances.
// Returns an error if any instance fails to launch.
func NewPool(cfg Config) (*Pool, error) {
	if cfg.PoolSize <= 0 {
		return nil, fmt.Errorf("pool size must be > 0, got %d", cfg.PoolSize)
	}
	if cfg.ChromiumPath == "" {
		return nil, fmt.Errorf("chromium path must not be empty")
	}

	p := &Pool{
		cfg:         cfg,
		available:   make(chan domain.BrowserInstance, cfg.PoolSize),
		stopMonitor: make(chan struct{}),
	}

	if !cfg.SkipPreWarm {
		if err := p.prewarm(); err != nil {
			_ = p.Close()
			return nil, err
		}
	}

	p.wg.Add(1)
	go p.monitorCrashes()

	return p, nil
}

// Acquire returns an available BrowserInstance from the pool.
// Blocks up to 10 seconds if all instances are in use.
// Returns domain.ErrPoolExhausted if none become available in time.
func (p *Pool) Acquire(ctx context.Context) (domain.BrowserInstance, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("pool is closed")
	}

	deadline := time.Now().Add(acquireTimeout)
	tctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	start := time.Now()
	select {
	case inst := <-p.available:
		return inst, nil
	case <-tctx.Done():
		return nil, &domain.ErrPoolExhausted{
			PoolSize: p.cfg.PoolSize,
			Waited:   time.Since(start),
		}
	}
}

// Release returns an instance to the pool after resetting its state.
// If reset fails (e.g. the browser crashed), the instance is discarded and replaced.
func (p *Pool) Release(inst domain.BrowserInstance) {
	if p.closed.Load() {
		_ = inst.Close()
		return
	}

	conc, ok := inst.(*instance)
	if !ok {
		slog.Warn("pool.Release: unknown instance type, discarding")
		_ = inst.Close()
		return
	}

	if !conc.IsAlive() {
		slog.Warn("pool.Release: instance is dead, replacing", "id", conc.ID())
		p.replaceInstance(conc)
		return
	}

	if err := conc.reset(); err != nil {
		slog.Warn("pool.Release: reset failed, replacing", "id", conc.ID(), "error", err)
		p.replaceInstance(conc)
		return
	}

	p.available <- inst
}

// Size returns the configured maximum pool size.
func (p *Pool) Size() int {
	return p.cfg.PoolSize
}

// Available returns the number of instances currently waiting in the pool.
func (p *Pool) Available() int {
	return len(p.available)
}

// Close shuts down all instances and the crash monitor.
// Safe to call multiple times.
func (p *Pool) Close() error {
	if p.closed.Swap(true) {
		return nil
	}

	close(p.stopMonitor)
	p.wg.Wait()

	// Drain available channel and close instances.
	for {
		select {
		case inst := <-p.available:
			_ = inst.Close()
		default:
			goto drainDone
		}
	}
drainDone:

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, inst := range p.allInstances {
		_ = inst.Close()
	}
	p.allInstances = nil
	return nil
}

// prewarm launches cfg.PoolSize instances and places them in the available channel.
func (p *Pool) prewarm() error {
	for i := 0; i < p.cfg.PoolSize; i++ {
		inst, err := p.spawnInstance()
		if err != nil {
			return fmt.Errorf("prewarm: failed to launch instance %d: %w", i, err)
		}
		p.available <- inst
	}
	return nil
}

// spawnInstance creates a new Chromium instance and registers it with the pool.
func (p *Pool) spawnInstance() (*instance, error) {
	id := fmt.Sprintf("chrome-%d", p.counter.Add(1))
	opts := BuildAllocatorOptions(p.cfg.ChromiumPath)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	inst, err := newInstance(allocCtx, allocCancel, id)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.allInstances = append(p.allInstances, inst)
	p.mu.Unlock()

	slog.Debug("browser instance spawned", "id", id)
	return inst, nil
}

// replaceInstance closes the dead instance and spawns a fresh one into the pool.
func (p *Pool) replaceInstance(dead *instance) {
	_ = dead.Close()
	p.removeFromAll(dead)

	inst, err := p.spawnInstance()
	if err != nil {
		slog.Error("pool: failed to replace crashed instance", "id", dead.ID(), "error", err)
		return
	}

	slog.Info("pool: replaced crashed instance", "old", dead.ID(), "new", inst.ID())

	if !p.closed.Load() {
		p.available <- inst
	} else {
		_ = inst.Close()
	}
}

// removeFromAll removes an instance from the allInstances registry.
func (p *Pool) removeFromAll(target *instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	updated := p.allInstances[:0]
	for _, inst := range p.allInstances {
		if inst.ID() != target.ID() {
			updated = append(updated, inst)
		}
	}
	p.allInstances = updated
}

// monitorCrashes periodically checks the available queue for dead instances
// and replaces them. It only inspects instances currently in the available queue.
func (p *Pool) monitorCrashes() {
	defer p.wg.Done()
	ticker := time.NewTicker(crashCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopMonitor:
			return
		case <-ticker.C:
			p.sweepDeadInstances()
		}
	}
}

// sweepDeadInstances drains the available channel, discards dead instances,
// and re-queues healthy ones. Dead instances are replaced asynchronously.
func (p *Pool) sweepDeadInstances() {
	if p.closed.Load() {
		return
	}

	n := len(p.available)
	healthy := make([]domain.BrowserInstance, 0, n)
	dead := make([]*instance, 0)

	for i := 0; i < n; i++ {
		select {
		case inst := <-p.available:
			if inst.IsAlive() {
				healthy = append(healthy, inst)
			} else {
				if conc, ok := inst.(*instance); ok {
					dead = append(dead, conc)
				}
			}
		default:
		}
	}

	for _, inst := range healthy {
		p.available <- inst
	}

	for _, inst := range dead {
		slog.Warn("pool: detected crashed instance, replacing", "id", inst.ID())
		go p.replaceInstance(inst)
	}
}
