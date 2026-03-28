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
	cfg          Config
	available    chan domain.BrowserInstance // buffered channel acts as the pool queue
	mu           sync.Mutex                  // guards allInstances
	allInstances []*instance
	counter      atomic.Int64 // instance ID counter
	closed       atomic.Bool
	stopMonitor  chan struct{}
	wg           sync.WaitGroup
}

// Acquire returns an available BrowserInstance from the pool.
// Blocks up to 10 seconds if all instances are in use.
// If profileID is provided, the instance will be configured with that profile's data.
func (p *Pool) Acquire(ctx context.Context, profileIDs ...string) (domain.BrowserInstance, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("pool is closed")
	}

	profileID := ""
	if len(profileIDs) > 0 {
		profileID = profileIDs[0]
	}

	deadline := time.Now().Add(acquireTimeout)
	tctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	start := time.Now()
	select {
	case inst := <-p.available:
		return p.prepareInstance(ctx, inst, profileID)
	case <-tctx.Done():
		return nil, &domain.ErrPoolExhausted{
			PoolSize: p.cfg.PoolSize,
			Waited:   time.Since(start),
		}
	}
}

// prepareInstance ensures the acquired instance matches the requested profile and has proxy configured.
func (p *Pool) prepareInstance(ctx context.Context, inst domain.BrowserInstance, profileID string) (domain.BrowserInstance, error) {
	conc, _ := inst.(*instance)
	if conc.profileID != profileID {
		newInst, err := p.recreateInstanceWithProfile(ctx, conc, profileID)
		if err != nil {
			p.available <- inst // put it back
			return nil, err
		}
		inst = newInst
	}

	if p.cfg.ProxyProvider != nil {
		if err := p.configureProxy(ctx, inst, profileID); err != nil {
			p.Release(inst)
			return nil, err
		}
	}
	return inst, nil
}

func (p *Pool) recreateInstanceWithProfile(ctx context.Context, old *instance, profileID string) (domain.BrowserInstance, error) {
	_ = old.Close()
	p.removeFromAll(old)

	inst, err := p.spawnInstance(profileID)
	if err != nil {
		return nil, fmt.Errorf("failed to recreate instance with profile %s: %w", profileID, err)
	}

	return inst, nil
}

// configureProxy resolves and applies proxy settings to an instance.
func (p *Pool) configureProxy(ctx context.Context, inst domain.BrowserInstance, sessionID string) error {
	proxy, err := p.cfg.ProxyProvider.GetProxy(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("proxy provider: %w", err)
	}
	if proxy == nil {
		return nil
	}

	if proxy.Username != "" || proxy.Password != "" {
		if conc, ok := inst.(*instance); ok {
			conc.setProxyAuth(proxy.Username, proxy.Password)
		}
	}

	return nil
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

// prewarm launches cfg.PoolSize instances and places them in the available channel.
func (p *Pool) prewarm() error {
	for i := 0; i < p.cfg.PoolSize; i++ {
		inst, err := p.spawnInstance("")
		if err != nil {
			return fmt.Errorf("prewarm: failed to launch instance %d: %w", i, err)
		}
		p.available <- inst
	}
	return nil
}

// spawnInstance creates a new Chromium instance and registers it with the pool.
func (p *Pool) spawnInstance(profileID string) (*instance, error) {
	id := fmt.Sprintf("chrome-%d", p.counter.Add(1))

	var extra []chromedp.ExecAllocatorOption
	if p.cfg.ProxyURL != "" {
		extra = append(extra, chromedp.ProxyServer(p.cfg.ProxyURL))
	}

	if profileID != "" && p.cfg.ProfileManager != nil {
		profile, err := p.cfg.ProfileManager.CreateProfile(context.Background(), profileID)
		if err != nil {
			return nil, fmt.Errorf("failed to create profile for instance: %w", err)
		}
		extra = append(extra, chromedp.UserDataDir(profile.Path))
	}

	opts := BuildAllocatorOptions(p.cfg.ChromiumPath, p.cfg.Stealth, extra...)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	inst, err := newInstance(allocCtx, allocCancel, id, p.cfg.Stealth, profileID)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.allInstances = append(p.allInstances, inst)
	p.mu.Unlock()

	slog.Debug("browser instance spawned", "id", id, "profile", profileID)
	return inst, nil
}

// replaceInstance closes the dead instance and spawns a fresh one into the pool.
func (p *Pool) replaceInstance(dead *instance) {
	_ = dead.Close()
	p.removeFromAll(dead)

	inst, err := p.spawnInstance(dead.profileID)
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
