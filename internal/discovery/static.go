package discovery

import (
	"context"

	"github.com/mhenni13/go-proxy/internal/config"
)

// StaticProvider implements static (configuration-based) service discovery
type StaticProvider struct {
	upstreams []config.Upstream
}

// NewStaticProvider creates a new static provider
func NewStaticProvider(upstreams []config.Upstream) *StaticProvider {
	return &StaticProvider{
		upstreams: upstreams,
	}
}

// Discover returns the configured static upstreams
func (p *StaticProvider) Discover(ctx context.Context) ([]config.Upstream, error) {
	return p.upstreams, nil
}

// Name returns the provider name
func (p *StaticProvider) Name() string {
	return "static"
}

// Close releases resources (no-op for static provider)
func (p *StaticProvider) Close() error {
	return nil
}
