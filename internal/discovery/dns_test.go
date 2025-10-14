package discovery

import (
	"context"
	"testing"
)

func TestDNSProvider(t *testing.T) {
	t.Run("NewDNSProvider with hostname and port", func(t *testing.T) {
		provider, err := NewDNSProvider(DNSConfig{
			Hostname: "example.com",
			Port:     8080,
			UseSRV:   false,
		})

		if err != nil {
			t.Fatalf("failed to create DNS provider: %v", err)
		}

		if provider == nil {
			t.Fatal("expected provider to be created")
		}

		if provider.hostname != "example.com" {
			t.Errorf("expected hostname 'example.com', got '%s'", provider.hostname)
		}

		if provider.port != 8080 {
			t.Errorf("expected port 8080, got %d", provider.port)
		}
	})

	t.Run("NewDNSProvider with SRV", func(t *testing.T) {
		provider, err := NewDNSProvider(DNSConfig{
			Hostname: "_http._tcp.example.com",
			UseSRV:   true,
		})

		if err != nil {
			t.Fatalf("failed to create DNS provider: %v", err)
		}

		if !provider.useSRV {
			t.Error("expected useSRV to be true")
		}
	})

	t.Run("Error on missing hostname", func(t *testing.T) {
		_, err := NewDNSProvider(DNSConfig{
			Port: 8080,
		})

		if err == nil {
			t.Error("expected error for missing hostname, got nil")
		}
	})

	t.Run("Error on missing port without SRV", func(t *testing.T) {
		_, err := NewDNSProvider(DNSConfig{
			Hostname: "example.com",
			UseSRV:   false,
		})

		if err == nil {
			t.Error("expected error for missing port without SRV, got nil")
		}
	})

	t.Run("Name returns correct format", func(t *testing.T) {
		provider, _ := NewDNSProvider(DNSConfig{
			Hostname: "example.com",
			Port:     9000,
		})

		expected := "dns:example.com:9000"
		if provider.Name() != expected {
			t.Errorf("expected name '%s', got '%s'", expected, provider.Name())
		}
	})

	t.Run("Name returns correct format for SRV", func(t *testing.T) {
		provider, _ := NewDNSProvider(DNSConfig{
			Hostname: "_service._tcp.example.com",
			UseSRV:   true,
		})

		expected := "dns-srv:_service._tcp.example.com"
		if provider.Name() != expected {
			t.Errorf("expected name '%s', got '%s'", expected, provider.Name())
		}
	})

	t.Run("Close does not return error", func(t *testing.T) {
		provider, _ := NewDNSProvider(DNSConfig{
			Hostname: "example.com",
			Port:     8080,
		})

		if err := provider.Close(); err != nil {
			t.Errorf("unexpected error on close: %v", err)
		}
	})

	t.Run("Discover localhost resolves to IP", func(t *testing.T) {
		provider, err := NewDNSProvider(DNSConfig{
			Hostname: "localhost",
			Port:     8080,
			UseSRV:   false,
		})

		if err != nil {
			t.Fatalf("failed to create DNS provider: %v", err)
		}

		ctx := context.Background()
		upstreams, err := provider.Discover(ctx)

		if err != nil {
			t.Fatalf("discover failed: %v", err)
		}

		if len(upstreams) == 0 {
			t.Error("expected at least one upstream for localhost")
		}

		// localhost should resolve to 127.0.0.1 or ::1
		foundLocalhost := false
		for _, upstream := range upstreams {
			if upstream.Host == "127.0.0.1" || upstream.Host == "::1" {
				foundLocalhost = true
				break
			}
		}

		if !foundLocalhost {
			t.Error("expected localhost to resolve to 127.0.0.1 or ::1")
		}

		// Check port is set
		if upstreams[0].Port != 8080 {
			t.Errorf("expected port 8080, got %d", upstreams[0].Port)
		}
	})

	t.Run("TLS settings are applied to upstreams", func(t *testing.T) {
		provider, err := NewDNSProvider(DNSConfig{
			Hostname:    "localhost",
			Port:        443,
			UseTLS:      true,
			TLSInsecure: true,
		})

		if err != nil {
			t.Fatalf("failed to create DNS provider: %v", err)
		}

		ctx := context.Background()
		upstreams, err := provider.Discover(ctx)

		if err != nil {
			t.Fatalf("discover failed: %v", err)
		}

		if len(upstreams) == 0 {
			t.Fatal("expected at least one upstream")
		}

		upstream := upstreams[0]
		if upstream.TLS == nil || !*upstream.TLS {
			t.Error("expected TLS to be enabled")
		}

		if upstream.TLSInsecure == nil || !*upstream.TLSInsecure {
			t.Error("expected TLSInsecure to be enabled")
		}
	})

	t.Run("Custom DNS resolver configuration", func(t *testing.T) {
		provider, err := NewDNSProvider(DNSConfig{
			Hostname: "example.com",
			Port:     8080,
			Resolver: "8.8.8.8:53",
		})

		if err != nil {
			t.Fatalf("failed to create DNS provider: %v", err)
		}

		if provider.resolver != "8.8.8.8:53" {
			t.Errorf("expected resolver '8.8.8.8:53', got '%s'", provider.resolver)
		}
	})

	t.Run("ParseSRVHostname validates format", func(t *testing.T) {
		// Valid SRV hostname
		_, _, _, err := ParseSRVHostname("_http._tcp.example.com")
		if err != nil {
			t.Errorf("expected valid SRV hostname to succeed, got error: %v", err)
		}

		// Invalid SRV hostname (missing underscore)
		_, _, _, err = ParseSRVHostname("http.tcp.example.com")
		if err == nil {
			t.Error("expected error for invalid SRV hostname without underscore")
		}

		// Invalid SRV hostname (too short)
		_, _, _, err = ParseSRVHostname("_h")
		if err == nil {
			t.Error("expected error for too short hostname")
		}
	})

	t.Run("Error on non-existent hostname", func(t *testing.T) {
		provider, err := NewDNSProvider(DNSConfig{
			Hostname: "this-hostname-definitely-does-not-exist-12345.invalid",
			Port:     8080,
		})

		if err != nil {
			t.Fatalf("failed to create DNS provider: %v", err)
		}

		ctx := context.Background()
		_, err = provider.Discover(ctx)

		if err == nil {
			t.Error("expected error for non-existent hostname, got nil")
		}
	})
}
