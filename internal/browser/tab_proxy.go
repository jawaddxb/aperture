// Package browser provides Chromium lifecycle management for Aperture.
// This file implements PoolTabProxy, which adapts BrowserPool to TabManager
// by acquiring an instance, delegating tab operations, and releasing it.
package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// PoolTabProxy implements domain.TabManager by checking out a browser
// instance from the pool for each operation. This allows tab executors
// to be wired at startup without holding a permanent browser reference.
type PoolTabProxy struct {
	pool domain.BrowserPool
}

// NewPoolTabProxy constructs a PoolTabProxy backed by pool.
func NewPoolTabProxy(pool domain.BrowserPool) *PoolTabProxy {
	return &PoolTabProxy{pool: pool}
}

// ListTabs acquires a browser from the pool and lists its open tabs.
func (p *PoolTabProxy) ListTabs(ctx context.Context) ([]domain.Tab, error) {
	inst, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("PoolTabProxy.ListTabs: %w", err)
	}
	defer p.pool.Release(inst)

	mgr := NewChromeTabManager(inst.Context())
	return mgr.ListTabs(ctx)
}

// NewTab acquires a browser from the pool and opens a new tab.
func (p *PoolTabProxy) NewTab(ctx context.Context, url string) (*domain.Tab, error) {
	inst, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("PoolTabProxy.NewTab: %w", err)
	}
	defer p.pool.Release(inst)

	mgr := NewChromeTabManager(inst.Context())
	return mgr.NewTab(ctx, url)
}

// SwitchTab acquires a browser from the pool and switches the active tab.
func (p *PoolTabProxy) SwitchTab(ctx context.Context, tabID string) error {
	inst, err := p.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("PoolTabProxy.SwitchTab: %w", err)
	}
	defer p.pool.Release(inst)

	mgr := NewChromeTabManager(inst.Context())
	return mgr.SwitchTab(ctx, tabID)
}

// CloseTab acquires a browser from the pool and closes the tab.
func (p *PoolTabProxy) CloseTab(ctx context.Context, tabID string) error {
	inst, err := p.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("PoolTabProxy.CloseTab: %w", err)
	}
	defer p.pool.Release(inst)

	mgr := NewChromeTabManager(inst.Context())
	return mgr.CloseTab(ctx, tabID)
}

// WaitForNewTab acquires a browser from the pool and waits for a new tab.
func (p *PoolTabProxy) WaitForNewTab(ctx context.Context, timeout time.Duration) (*domain.Tab, error) {
	inst, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("PoolTabProxy.WaitForNewTab: %w", err)
	}
	defer p.pool.Release(inst)

	mgr := NewChromeTabManager(inst.Context())
	return mgr.WaitForNewTab(ctx, timeout)
}

// compile-time interface assertion.
var _ domain.TabManager = (*PoolTabProxy)(nil)
