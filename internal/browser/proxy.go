package browser

import (
	"context"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// StaticProxyProvider returns a fixed proxy for all requests.
type StaticProxyProvider struct {
	proxy *domain.Proxy
}

// NewStaticProxyProvider constructs a StaticProxyProvider.
func NewStaticProxyProvider(proxy *domain.Proxy) *StaticProxyProvider {
	return &StaticProxyProvider{proxy: proxy}
}

// GetProxy returns the static proxy.
func (p *StaticProxyProvider) GetProxy(_ context.Context, _ string) (*domain.Proxy, error) {
	return p.proxy, nil
}
