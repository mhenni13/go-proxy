package discovery

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/mhenni13/go-proxy/internal/config"
)

// DNSProvider implements DNS-based service discovery
type DNSProvider struct {
	hostname    string
	port        int
	useSRV      bool   // Use DNS SRV records
	resolver    string // Custom DNS resolver (e.g., "8.8.8.8:53")
	useTLS      bool
	tlsInsecure bool
}

// DNSConfig holds configuration for DNS service discovery
type DNSConfig struct {
	Hostname    string `yaml:"hostname"`              // DNS hostname to resolve
	Port        int    `yaml:"port,omitempty"`        // Port (required if not using SRV)
	UseSRV      bool   `yaml:"use_srv,omitempty"`     // Use DNS SRV records
	Resolver    string `yaml:"resolver,omitempty"`    // Custom DNS resolver
	UseTLS      bool   `yaml:"use_tls,omitempty"`     // Use HTTPS for upstreams
	TLSInsecure bool   `yaml:"tls_insecure,omitempty"` // Skip TLS verification
}

// NewDNSProvider creates a new DNS service discovery provider
func NewDNSProvider(cfg DNSConfig) (*DNSProvider, error) {
	if cfg.Hostname == "" {
		return nil, fmt.Errorf("hostname is required for DNS discovery")
	}

	if !cfg.UseSRV && cfg.Port == 0 {
		return nil, fmt.Errorf("port is required when not using SRV records")
	}

	log.Printf("[DNS] Configured DNS discovery for %s (SRV: %v)", cfg.Hostname, cfg.UseSRV)

	return &DNSProvider{
		hostname:    cfg.Hostname,
		port:        cfg.Port,
		useSRV:      cfg.UseSRV,
		resolver:    cfg.Resolver,
		useTLS:      cfg.UseTLS,
		tlsInsecure: cfg.TLSInsecure,
	}, nil
}

// Discover returns upstreams from DNS resolution
func (p *DNSProvider) Discover(ctx context.Context) ([]config.Upstream, error) {
	if p.useSRV {
		return p.discoverFromSRV(ctx)
	}
	return p.discoverFromA(ctx)
}

// discoverFromA discovers upstreams using DNS A/AAAA records
func (p *DNSProvider) discoverFromA(ctx context.Context) ([]config.Upstream, error) {
	resolver := p.getResolver()

	ips, err := resolver.LookupIP(ctx, "ip", p.hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve hostname %s: %w", p.hostname, err)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found for hostname %s", p.hostname)
	}

	var upstreams []config.Upstream
	useTLS := p.useTLS
	tlsInsecure := p.tlsInsecure

	for _, ip := range ips {
		upstream := config.Upstream{
			Host:        ip.String(),
			Port:        p.port,
			TLS:         &useTLS,
			TLSInsecure: &tlsInsecure,
		}
		upstreams = append(upstreams, upstream)
	}

	return upstreams, nil
}

// discoverFromSRV discovers upstreams using DNS SRV records
func (p *DNSProvider) discoverFromSRV(ctx context.Context) ([]config.Upstream, error) {
	resolver := p.getResolver()

	_, srvRecords, err := resolver.LookupSRV(ctx, "", "", p.hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve SRV records for %s: %w", p.hostname, err)
	}

	if len(srvRecords) == 0 {
		return nil, fmt.Errorf("no SRV records found for %s", p.hostname)
	}

	var upstreams []config.Upstream
	useTLS := p.useTLS
	tlsInsecure := p.tlsInsecure

	for _, srv := range srvRecords {
		// Resolve target hostname to IP (SRV target might be another hostname)
		ips, err := resolver.LookupIP(ctx, "ip", srv.Target)
		if err != nil {
			log.Printf("[DNS] Warning: Failed to resolve SRV target %s: %v", srv.Target, err)
			continue
		}

		for _, ip := range ips {
			upstream := config.Upstream{
				Host:        ip.String(),
				Port:        int(srv.Port),
				TLS:         &useTLS,
				TLSInsecure: &tlsInsecure,
				Weight:      int(srv.Weight),
			}
			upstreams = append(upstreams, upstream)
		}
	}

	if len(upstreams) == 0 {
		return nil, fmt.Errorf("no valid upstreams discovered from SRV records for %s", p.hostname)
	}

	return upstreams, nil
}

// getResolver returns a custom resolver if configured, otherwise default
func (p *DNSProvider) getResolver() *net.Resolver {
	if p.resolver != "" {
		return &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, network, p.resolver)
			},
		}
	}
	return net.DefaultResolver
}

// Name returns the provider name
func (p *DNSProvider) Name() string {
	if p.useSRV {
		return fmt.Sprintf("dns-srv:%s", p.hostname)
	}
	return fmt.Sprintf("dns:%s:%d", p.hostname, p.port)
}

// Close releases resources
func (p *DNSProvider) Close() error {
	// DNS resolver doesn't require explicit cleanup
	return nil
}

// ParseSRVHostname parses a hostname and returns service, proto, and name
// Format: _service._proto.name (e.g., _http._tcp.example.com)
func ParseSRVHostname(hostname string) (service, proto, name string, err error) {
	// Simple SRV hostname validation
	if len(hostname) < 3 || hostname[0] != '_' {
		return "", "", "", fmt.Errorf("invalid SRV hostname format: %s (expected _service._proto.name)", hostname)
	}

	// For simplicity, we'll let the resolver handle the full format
	// Users should provide the full SRV hostname including _service._proto prefix
	return "", "", hostname, nil
}
