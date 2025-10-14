package discovery

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mhenni13/go-proxy/internal/config"
)

// Provider is the interface that all service discovery providers must implement
type Provider interface {
	// Discover returns the current list of upstreams
	Discover(ctx context.Context) ([]config.Upstream, error)
	// Name returns the provider name
	Name() string
	// Close releases any resources held by the provider
	Close() error
}

// Registry manages service discovery for routes
type Registry struct {
	routes   map[string]*RouteDiscovery // map[routePath]*RouteDiscovery
	mu       sync.RWMutex
	ctx      context.Context
	cancelFn context.CancelFunc
}

// RouteDiscovery holds discovery configuration for a single route
type RouteDiscovery struct {
	Provider      Provider
	RefreshPeriod time.Duration
	upstreams     []config.Upstream
	mu            sync.RWMutex
	stopChan      chan struct{}
}

// NewRegistry creates a new service discovery registry
func NewRegistry() *Registry {
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{
		routes:   make(map[string]*RouteDiscovery),
		ctx:      ctx,
		cancelFn: cancel,
	}
}

// Register registers a discovery provider for a specific route
func (r *Registry) Register(routePath string, provider Provider, refreshPeriod time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.routes[routePath]; exists {
		return fmt.Errorf("route %s already registered", routePath)
	}

	rd := &RouteDiscovery{
		Provider:      provider,
		RefreshPeriod: refreshPeriod,
		upstreams:     []config.Upstream{},
		stopChan:      make(chan struct{}),
	}

	// Initial discovery
	upstreams, err := provider.Discover(r.ctx)
	if err != nil {
		return fmt.Errorf("initial discovery failed for route %s: %w", routePath, err)
	}
	rd.upstreams = upstreams

	r.routes[routePath] = rd

	// Start background refresh if refresh period is set
	if refreshPeriod > 0 {
		go rd.startRefresh(r.ctx, routePath)
	}

	log.Printf("[ServiceDiscovery] Registered provider '%s' for route '%s' with %d upstreams (refresh: %v)",
		provider.Name(), routePath, len(upstreams), refreshPeriod)

	return nil
}

// GetUpstreams returns the current upstreams for a route
func (r *Registry) GetUpstreams(routePath string) ([]config.Upstream, bool) {
	r.mu.RLock()
	rd, exists := r.routes[routePath]
	r.mu.RUnlock()

	if !exists {
		return nil, false
	}

	rd.mu.RLock()
	defer rd.mu.RUnlock()
	return rd.upstreams, true
}

// Close stops all discovery providers and background refresh goroutines
func (r *Registry) Close() error {
	r.cancelFn()

	r.mu.Lock()
	defer r.mu.Unlock()

	for routePath, rd := range r.routes {
		close(rd.stopChan)
		if err := rd.Provider.Close(); err != nil {
			log.Printf("[ServiceDiscovery] Error closing provider for route %s: %v", routePath, err)
		}
	}

	return nil
}

// startRefresh runs the background refresh loop for a route
func (rd *RouteDiscovery) startRefresh(ctx context.Context, routePath string) {
	ticker := time.NewTicker(rd.RefreshPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rd.stopChan:
			return
		case <-ticker.C:
			upstreams, err := rd.Provider.Discover(ctx)
			if err != nil {
				log.Printf("[ServiceDiscovery] Failed to refresh upstreams for route %s: %v", routePath, err)
				continue
			}

			rd.mu.Lock()
			oldCount := len(rd.upstreams)
			rd.upstreams = upstreams
			rd.mu.Unlock()

			if len(upstreams) != oldCount {
				log.Printf("[ServiceDiscovery] Updated upstreams for route %s: %d -> %d",
					routePath, oldCount, len(upstreams))
			}
		}
	}
}

// IsDiscoveryEnabled checks if a route has service discovery enabled
func (r *Registry) IsDiscoveryEnabled(routePath string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.routes[routePath]
	return exists
}
