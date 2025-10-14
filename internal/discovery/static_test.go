package discovery

import (
	"context"
	"testing"

	"github.com/mhenni13/go-proxy/internal/config"
)

func TestStaticProvider(t *testing.T) {
	t.Run("NewStaticProvider creates provider with upstreams", func(t *testing.T) {
		useTLS := false
		upstreams := []config.Upstream{
			{Host: "192.168.1.1", Port: 8080, TLS: &useTLS},
			{Host: "192.168.1.2", Port: 8080, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		if provider == nil {
			t.Fatal("expected provider to be created")
		}

		if len(provider.upstreams) != 2 {
			t.Errorf("expected 2 upstreams, got %d", len(provider.upstreams))
		}
	})

	t.Run("Discover returns configured upstreams", func(t *testing.T) {
		useTLS := false
		upstreams := []config.Upstream{
			{Host: "10.0.0.1", Port: 9000, TLS: &useTLS},
			{Host: "10.0.0.2", Port: 9000, TLS: &useTLS},
			{Host: "10.0.0.3", Port: 9000, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		ctx := context.Background()

		discovered, err := provider.Discover(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(discovered) != 3 {
			t.Errorf("expected 3 upstreams, got %d", len(discovered))
		}

		// Verify upstreams match
		for i, upstream := range discovered {
			if upstream.Host != upstreams[i].Host {
				t.Errorf("upstream[%d] host: expected %s, got %s", i, upstreams[i].Host, upstream.Host)
			}
			if upstream.Port != upstreams[i].Port {
				t.Errorf("upstream[%d] port: expected %d, got %d", i, upstreams[i].Port, upstream.Port)
			}
		}
	})

	t.Run("Discover returns empty list when no upstreams configured", func(t *testing.T) {
		provider := NewStaticProvider([]config.Upstream{})
		ctx := context.Background()

		discovered, err := provider.Discover(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(discovered) != 0 {
			t.Errorf("expected 0 upstreams, got %d", len(discovered))
		}
	})

	t.Run("Name returns 'static'", func(t *testing.T) {
		provider := NewStaticProvider([]config.Upstream{})
		if provider.Name() != "static" {
			t.Errorf("expected name 'static', got '%s'", provider.Name())
		}
	})

	t.Run("Close does not return error", func(t *testing.T) {
		provider := NewStaticProvider([]config.Upstream{})
		if err := provider.Close(); err != nil {
			t.Errorf("unexpected error on close: %v", err)
		}
	})

	t.Run("Multiple Discover calls return same upstreams", func(t *testing.T) {
		useTLS := true
		upstreams := []config.Upstream{
			{Host: "service.local", Port: 443, TLS: &useTLS},
		}

		provider := NewStaticProvider(upstreams)
		ctx := context.Background()

		// First discovery
		first, err := provider.Discover(ctx)
		if err != nil {
			t.Fatalf("first discover failed: %v", err)
		}

		// Second discovery
		second, err := provider.Discover(ctx)
		if err != nil {
			t.Fatalf("second discover failed: %v", err)
		}

		// Should return same data
		if len(first) != len(second) {
			t.Errorf("discover calls returned different lengths: %d vs %d", len(first), len(second))
		}

		if first[0].Host != second[0].Host {
			t.Errorf("discover calls returned different hosts: %s vs %s", first[0].Host, second[0].Host)
		}
	})
}
