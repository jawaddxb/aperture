package browser

import (
	"fmt"

	"github.com/ApertureHQ/aperture/internal/domain"
)

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
	// ProxyURL is an optional static proxy server address, e.g. "http://proxy.example.com:8080".
	ProxyURL string
	// ProxyProvider is an optional provider to resolve a proxy per session.
	// If both ProxyURL and ProxyProvider are set, ProxyURL is used as a fallback.
	ProxyProvider domain.ProxyProvider
	// ProfileManager is an optional manager to handle browser user data directories.
	ProfileManager domain.ProfileManager
	// Stealth defines settings to avoid bot detection.
	Stealth domain.StealthConfig
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
