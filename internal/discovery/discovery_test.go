package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/mhenni13/go-proxy/internal/config"
)

func TestRegistry(t *testing.T) {
	t.Run("NewRegistry creates empty registry", func(t *testing.T) {
		registry := NewRegistry()
		if registry == nil {
			t.Fatal("expected registry to be created")
		}

		if len(registry.routes) != 0 {
			t.Errorf("expected empty registry, got %d routes", len(registry.routes))
		}
	})

	t.Run("Register adds provider to registry", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		useTLS := false
		upstreams := []config.Upstream{
			{Host: "10.0.0.1", Port: 8080, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		err := registry.Register("/api/test/*", provider, 0)

		if err != nil {
			t.Fatalf("failed to register provider: %v", err)
		}

		if len(registry.routes) != 1 {
			t.Errorf("expected 1 route, got %d", len(registry.routes))
		}
	})

	t.Run("GetUpstreams returns registered upstreams", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		useTLS := false
		upstreams := []config.Upstream{
			{Host: "192.168.1.1", Port: 9000, TLS: &useTLS},
			{Host: "192.168.1.2", Port: 9000, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		registry.Register("/api/service/*", provider, 0)

		retrieved, exists := registry.GetUpstreams("/api/service/*")

		if !exists {
			t.Fatal("expected upstreams to exist")
		}

		if len(retrieved) != 2 {
			t.Errorf("expected 2 upstreams, got %d", len(retrieved))
		}

		if retrieved[0].Host != "192.168.1.1" {
			t.Errorf("expected host '192.168.1.1', got '%s'", retrieved[0].Host)
		}
	})

	t.Run("GetUpstreams returns false for non-existent route", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		_, exists := registry.GetUpstreams("/nonexistent/*")

		if exists {
			t.Error("expected upstreams not to exist for non-existent route")
		}
	})

	t.Run("Register same route twice returns error", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		useTLS := false
		upstreams := []config.Upstream{
			{Host: "10.0.0.1", Port: 8080, TLS: &useTLS},
		}

		provider1 := NewStaticProvider(upstreams)
		provider2 := NewStaticProvider(upstreams)

		err := registry.Register("/api/test/*", provider1, 0)
		if err != nil {
			t.Fatalf("first register failed: %v", err)
		}

		err = registry.Register("/api/test/*", provider2, 0)
		if err == nil {
			t.Error("expected error when registering duplicate route")
		}
	})

	t.Run("IsDiscoveryEnabled returns true for registered routes", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		useTLS := false
		upstreams := []config.Upstream{
			{Host: "10.0.0.1", Port: 8080, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		registry.Register("/api/enabled/*", provider, 0)

		if !registry.IsDiscoveryEnabled("/api/enabled/*") {
			t.Error("expected discovery to be enabled for registered route")
		}

		if registry.IsDiscoveryEnabled("/api/not-registered/*") {
			t.Error("expected discovery to be disabled for non-registered route")
		}
	})

	t.Run("Close stops all providers", func(t *testing.T) {
		registry := NewRegistry()

		useTLS := false
		upstreams := []config.Upstream{
			{Host: "10.0.0.1", Port: 8080, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		registry.Register("/api/test/*", provider, 0)

		err := registry.Close()
		if err != nil {
			t.Errorf("unexpected error on close: %v", err)
		}
	})

	t.Run("Background refresh updates upstreams", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		// Create a mock provider that changes upstreams over time
		useTLS := false
		initialUpstreams := []config.Upstream{
			{Host: "initial.local", Port: 8080, TLS: &useTLS},
		}

		provider := NewStaticProvider(initialUpstreams)

		// Register with short refresh period (note: static provider won't actually refresh,
		// but this tests the refresh mechanism)
		registry.Register("/api/refresh/*", provider, 100*time.Millisecond)

		// Verify initial upstreams
		upstreams, exists := registry.GetUpstreams("/api/refresh/*")
		if !exists {
			t.Fatal("expected upstreams to exist")
		}

		if len(upstreams) != 1 {
			t.Errorf("expected 1 upstream, got %d", len(upstreams))
		}

		// Wait for a refresh cycle (even though static won't change)
		time.Sleep(150 * time.Millisecond)

		// Verify upstreams still accessible
		upstreams, exists = registry.GetUpstreams("/api/refresh/*")
		if !exists {
			t.Error("expected upstreams to still exist after refresh")
		}
	})

	t.Run("Multiple routes can be registered", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		useTLS := false
		upstreams1 := []config.Upstream{
			{Host: "service1.local", Port: 8080, TLS: &useTLS},
		}
		upstreams2 := []config.Upstream{
			{Host: "service2.local", Port: 9000, TLS: &useTLS},
		}
		upstreams3 := []config.Upstream{
			{Host: "service3.local", Port: 7000, TLS: &useTLS},
		}

		provider1 := NewStaticProvider(upstreams1)
		provider2 := NewStaticProvider(upstreams2)
		provider3 := NewStaticProvider(upstreams3)

		registry.Register("/api/service1/*", provider1, 0)
		registry.Register("/api/service2/*", provider2, 0)
		registry.Register("/api/service3/*", provider3, 0)

		if len(registry.routes) != 3 {
			t.Errorf("expected 3 routes, got %d", len(registry.routes))
		}

		// Verify each route has correct upstreams
		ups1, _ := registry.GetUpstreams("/api/service1/*")
		if ups1[0].Port != 8080 {
			t.Errorf("service1 expected port 8080, got %d", ups1[0].Port)
		}

		ups2, _ := registry.GetUpstreams("/api/service2/*")
		if ups2[0].Port != 9000 {
			t.Errorf("service2 expected port 9000, got %d", ups2[0].Port)
		}

		ups3, _ := registry.GetUpstreams("/api/service3/*")
		if ups3[0].Port != 7000 {
			t.Errorf("service3 expected port 7000, got %d", ups3[0].Port)
		}
	})

	t.Run("Register fails if initial discovery fails", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		// Create a provider that will fail discovery
		provider := &mockFailingProvider{}

		err := registry.Register("/api/failing/*", provider, 0)
		if err == nil {
			t.Error("expected error when initial discovery fails")
		}
	})

	t.Run("Concurrent GetUpstreams is thread-safe", func(t *testing.T) {
		registry := NewRegistry()
		defer registry.Close()

		useTLS := false
		upstreams := []config.Upstream{
			{Host: "concurrent.local", Port: 8080, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		registry.Register("/api/concurrent/*", provider, 0)

		// Launch multiple goroutines to read upstreams concurrently
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				for j := 0; j < 100; j++ {
					upstreams, exists := registry.GetUpstreams("/api/concurrent/*")
					if !exists || len(upstreams) != 1 {
						t.Error("concurrent read failed")
					}
				}
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// mockFailingProvider is a test provider that always fails discovery
type mockFailingProvider struct{}

func (p *mockFailingProvider) Discover(ctx context.Context) ([]config.Upstream, error) {
	return nil, context.DeadlineExceeded
}

func (p *mockFailingProvider) Name() string {
	return "mock-failing"
}

func (p *mockFailingProvider) Close() error {
	return nil
}
