package discovery

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/consul/api"
	"github.com/mhenni13/go-proxy/internal/config"
)

// ConsulProvider implements Consul-based service discovery
type ConsulProvider struct {
	client      *api.Client
	serviceName string
	tag         string
	datacenter  string
	useTLS      bool
	tlsInsecure bool
	onlyPassing bool // Only return services passing health checks
}

// ConsulConfig holds configuration for Consul service discovery
type ConsulConfig struct {
	Address     string `yaml:"address"`      // Consul agent address (e.g., "localhost:8500")
	ServiceName string `yaml:"service_name"` // Service name to discover
	Tag         string `yaml:"tag,omitempty"`         // Optional service tag filter
	Datacenter  string `yaml:"datacenter,omitempty"`  // Optional datacenter
	Token       string `yaml:"token,omitempty"`       // Optional ACL token
	UseTLS      bool   `yaml:"use_tls,omitempty"`     // Use HTTPS for upstreams
	TLSInsecure bool   `yaml:"tls_insecure,omitempty"` // Skip TLS verification
	OnlyPassing bool   `yaml:"only_passing,omitempty"` // Only return healthy services (default: true)
}

// NewConsulProvider creates a new Consul service discovery provider
func NewConsulProvider(cfg ConsulConfig) (*ConsulProvider, error) {
	consulConfig := api.DefaultConfig()

	if cfg.Address != "" {
		consulConfig.Address = cfg.Address
	}

	if cfg.Token != "" {
		consulConfig.Token = cfg.Token
	}

	client, err := api.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	// Default to only returning healthy services
	onlyPassing := true
	if cfg.OnlyPassing == false {
		onlyPassing = cfg.OnlyPassing
	}

	log.Printf("[Consul] Connected to Consul at %s", consulConfig.Address)

	return &ConsulProvider{
		client:      client,
		serviceName: cfg.ServiceName,
		tag:         cfg.Tag,
		datacenter:  cfg.Datacenter,
		useTLS:      cfg.UseTLS,
		tlsInsecure: cfg.TLSInsecure,
		onlyPassing: onlyPassing,
	}, nil
}

// Discover returns upstreams from Consul service catalog
func (p *ConsulProvider) Discover(ctx context.Context) ([]config.Upstream, error) {
	queryOpts := &api.QueryOptions{
		Datacenter: p.datacenter,
	}

	// Query Consul for service health
	var services []*api.ServiceEntry
	var err error

	if p.onlyPassing {
		// Get only passing services
		services, _, err = p.client.Health().Service(p.serviceName, p.tag, true, queryOpts)
	} else {
		// Get all services regardless of health
		services, _, err = p.client.Health().Service(p.serviceName, p.tag, false, queryOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query Consul for service %s: %w", p.serviceName, err)
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("no instances found for service %s", p.serviceName)
	}

	var upstreams []config.Upstream
	useTLS := p.useTLS
	tlsInsecure := p.tlsInsecure

	for _, entry := range services {
		// Use service address if available, otherwise use node address
		host := entry.Service.Address
		if host == "" {
			host = entry.Node.Address
		}

		port := entry.Service.Port

		upstream := config.Upstream{
			Host:        host,
			Port:        port,
			TLS:         &useTLS,
			TLSInsecure: &tlsInsecure,
		}

		// Extract version from service tags if present
		for _, tag := range entry.Service.Tags {
			if len(tag) > 8 && tag[:8] == "version:" {
				upstream.Version = tag[8:]
			}
			if len(tag) > 7 && tag[:7] == "weight:" {
				// Parse weight from tag (format: "weight:10")
				var weight int
				if _, err := fmt.Sscanf(tag[7:], "%d", &weight); err == nil {
					upstream.Weight = weight
				}
			}
		}

		upstreams = append(upstreams, upstream)
	}

	return upstreams, nil
}

// Name returns the provider name
func (p *ConsulProvider) Name() string {
	return fmt.Sprintf("consul:%s", p.serviceName)
}

// Close releases resources
func (p *ConsulProvider) Close() error {
	// Consul client doesn't require explicit cleanup
	return nil
}
