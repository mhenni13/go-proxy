package config

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Config struct {
		Port         int    `yaml:"port"`
		TLS          bool   `yaml:"tls"`
		TLSCertFile  string `yaml:"tls_cert_file,omitempty"`  // Path to TLS certificate file
		TLSKeyFile   string `yaml:"tls_key_file,omitempty"`   // Path to TLS private key file
		Auth         bool   `yaml:"auth"`
		ReadTimeout  string `yaml:"read_timeout"` // duration as string
		WriteTimeout string `yaml:"write_timeout"`
		IdleTimeout  string `yaml:"idle_timeout"`
	} `yaml:"config"`
	Auth []AuthConfig `yaml:"auth"`
	APIs []APIConfig  `yaml:"apis"`
}

type AuthConfig struct {
	Group      string      `yaml:"group"`
	Type       string      `yaml:"type"`             // "jwt" or "basic"
	JwtSecret  string      `yaml:"jwt_secret"`       // used only if type = jwt
	VerifyExpr string      `yaml:"verify,omitempty"` // expression for vulcand/predicate
	Users      []BasicUser `yaml:"users,omitempty"`  // used only if type = basic
}

type BasicUser struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"` // store bcrypt hash
}

type APIConfig struct {
	Name             string            `yaml:"name"`
	Protocol         string            `yaml:"protocol"`
	Public           bool              `yaml:"public"`
	Routes           []Route           `yaml:"routes"`
	CORS             CORSConfig        `yaml:"cors"`
	RateLimit        *int              `yaml:"rate_limit,omitempty"`
	LoadBalancing    string            `yaml:"load_balancing"`
	Sticky           bool              `yaml:"sticky_sessions"`
	Headers          map[string]string `yaml:"headers"`
	Cookies          map[string]string `yaml:"cookies"`
	MaxRetries       int               `yaml:"max_retries"`
	RetryDelayMs     int               `yaml:"retry_delay_ms"`
	FallbackResponse FallbackResponse  `yaml:"fallback_response"`
	Permissions      []string          `yaml:"permissions,omitempty"` // groups allowed
	Security         SecurityConfig    `yaml:"security,omitempty"`    // IP blocker and WAF settings
}

type FallbackResponse struct {
	Status     int    `yaml:"status"`
	Body       string `yaml:"body,omitempty"`
	BodyBase64 string `yaml:"body_base64,omitempty"`
}

type Route struct {
	Path               string            `yaml:"path"`
	Upstreams          []Upstream        `yaml:"upstreams,omitempty"`
	ServiceDiscovery   *DiscoveryConfig  `yaml:"service_discovery,omitempty"`
	DeploymentStrategy DeploymentConfig  `yaml:"deployment_strategy,omitempty"`
}

type DiscoveryConfig struct {
	Type          string               `yaml:"type"`           // "kubernetes", "consul", "dns", "file"
	RefreshPeriod string               `yaml:"refresh_period"` // e.g., "30s", "1m"
	Kubernetes    *KubernetesDiscovery `yaml:"kubernetes,omitempty"`
	Consul        *ConsulDiscovery     `yaml:"consul,omitempty"`
	DNS           *DNSDiscovery        `yaml:"dns,omitempty"`
	File          *FileDiscovery       `yaml:"file,omitempty"`
}

type DeploymentConfig struct {
	Type           string `yaml:"type,omitempty"`            // blue-green, canary, rolling, recreate
	ActiveVersion  string `yaml:"active_version,omitempty"`  // For blue-green: which version is active
	CanaryPercent  int    `yaml:"canary_percent,omitempty"`  // For canary: percentage to new version
	RollingPercent int    `yaml:"rolling_percent,omitempty"` // For rolling: percentage per step
}

type Upstream struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	TLS         *bool  `yaml:"tls,omitempty"`
	TLSInsecure *bool  `yaml:"tls_insecure,omitempty"`
	Version     string `yaml:"version,omitempty"` // Version label for deployment strategies
	Weight      int    `yaml:"weight,omitempty"`  // Weight for canary/rolling deployments
}

type KubernetesDiscovery struct {
	Namespace    string            `yaml:"namespace"`
	ServiceName  string            `yaml:"service_name"`
	Port         int32             `yaml:"port"`
	UseTLS       bool              `yaml:"use_tls,omitempty"`
	TLSInsecure  bool              `yaml:"tls_insecure,omitempty"`
	Labels       map[string]string `yaml:"labels,omitempty"`
	UseEndpoints bool              `yaml:"use_endpoints,omitempty"` // Discover individual pod IPs
	KubeConfig   string            `yaml:"kubeconfig,omitempty"`    // Path to kubeconfig (empty = in-cluster)
}

type ConsulDiscovery struct {
	Address     string `yaml:"address"`               // Consul agent address (e.g., "localhost:8500")
	ServiceName string `yaml:"service_name"`          // Service name to discover
	Tag         string `yaml:"tag,omitempty"`         // Optional service tag filter
	Datacenter  string `yaml:"datacenter,omitempty"`  // Optional datacenter
	Token       string `yaml:"token,omitempty"`       // Optional ACL token
	UseTLS      bool   `yaml:"use_tls,omitempty"`     // Use HTTPS for upstreams
	TLSInsecure bool   `yaml:"tls_insecure,omitempty"` // Skip TLS verification
	OnlyPassing bool   `yaml:"only_passing"`          // Only return healthy services (default: true)
}

type DNSDiscovery struct {
	Hostname    string `yaml:"hostname"`              // DNS hostname to resolve
	Port        int    `yaml:"port,omitempty"`        // Port (required if not using SRV)
	UseSRV      bool   `yaml:"use_srv,omitempty"`     // Use DNS SRV records
	Resolver    string `yaml:"resolver,omitempty"`    // Custom DNS resolver
	UseTLS      bool   `yaml:"use_tls,omitempty"`     // Use HTTPS for upstreams
	TLSInsecure bool   `yaml:"tls_insecure,omitempty"` // Skip TLS verification
}

type FileDiscovery struct {
	Path        string `yaml:"path"`                   // Path to file containing upstream list
	Format      string `yaml:"format,omitempty"`       // "json" or "yaml" (auto-detected if omitted)
	WatchPeriod string `yaml:"watch_period,omitempty"` // How often to check for file changes
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
}

type SecurityConfig struct {
	IPBlocker IPBlockerConfig `yaml:"ip_blocker,omitempty"`
	WAF       WAFConfig       `yaml:"waf,omitempty"`
}

type IPBlockerConfig struct {
	Enabled       bool     `yaml:"enabled"`
	Mode          string   `yaml:"mode"`                  // "allowlist", "blocklist", or "off"
	Allowlist     []string `yaml:"allowlist"`             // List of allowed IPs/CIDRs
	Blocklist     []string `yaml:"blocklist"`             // List of blocked IPs/CIDRs
	AllowlistFile string   `yaml:"allowlist_file,omitempty"` // Path to allowlist file
	BlocklistFile string   `yaml:"blocklist_file,omitempty"` // Path to blocklist file
}

type WAFConfig struct {
	Enabled     bool            `yaml:"enabled"`
	Mode        string          `yaml:"mode"` // "block" or "detect"
	LogFile     string          `yaml:"log_file,omitempty"` // Path to WAF log file
	CustomRules []WAFRuleConfig `yaml:"custom_rules,omitempty"`
}

type WAFRuleConfig struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Pattern     string `yaml:"pattern"`
	Target      string `yaml:"target"` // "uri", "body", "headers", "query", "all"
	Action      string `yaml:"action"` // "block", "log"
}

func (fr FallbackResponse) Validate() error {
	if fr.Body != "" && fr.BodyBase64 != "" {
		return fmt.Errorf("fallback_response cannot have both body and body_base64")
	}
	if fr.Body == "" && fr.BodyBase64 == "" {
		return fmt.Errorf("fallback_response must have either body or body_base64")
	}
	return nil
}

func (disc DiscoveryConfig) Validate() error {
	if disc.Type == "" {
		return fmt.Errorf("service_discovery type is required")
	}

	validTypes := []string{"kubernetes", "consul", "dns", "file"}
	isValid := false
	for _, t := range validTypes {
		if disc.Type == t {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("service_discovery type must be one of: %v, got: %s", validTypes, disc.Type)
	}

	// Validate type-specific configuration
	switch disc.Type {
	case "kubernetes":
		if disc.Kubernetes == nil {
			return fmt.Errorf("kubernetes configuration is required when type is 'kubernetes'")
		}
		if disc.Kubernetes.Namespace == "" {
			return fmt.Errorf("kubernetes.namespace is required")
		}
		if disc.Kubernetes.ServiceName == "" {
			return fmt.Errorf("kubernetes.service_name is required")
		}

	case "consul":
		if disc.Consul == nil {
			return fmt.Errorf("consul configuration is required when type is 'consul'")
		}
		if disc.Consul.Address == "" {
			return fmt.Errorf("consul.address is required")
		}
		if disc.Consul.ServiceName == "" {
			return fmt.Errorf("consul.service_name is required")
		}

	case "dns":
		if disc.DNS == nil {
			return fmt.Errorf("dns configuration is required when type is 'dns'")
		}
		if disc.DNS.Hostname == "" {
			return fmt.Errorf("dns.hostname is required")
		}
		if !disc.DNS.UseSRV && disc.DNS.Port == 0 {
			return fmt.Errorf("dns.port is required when not using SRV records")
		}

	case "file":
		if disc.File == nil {
			return fmt.Errorf("file configuration is required when type is 'file'")
		}
		if disc.File.Path == "" {
			return fmt.Errorf("file.path is required")
		}
	}

	return nil
}

func (dc DeploymentConfig) Validate(upstreams []Upstream) error {
	if dc.Type == "" {
		return nil // No deployment strategy is valid
	}

	switch dc.Type {
	case "blue-green":
		if dc.ActiveVersion == "" {
			return fmt.Errorf("active_version is required for blue-green deployment")
		}
		// Check that all upstreams have version labels
		for _, up := range upstreams {
			if up.Version == "" {
				return fmt.Errorf("all upstreams must have a version label for blue-green deployment")
			}
		}
		// Check that active version exists
		found := false
		for _, up := range upstreams {
			if up.Version == dc.ActiveVersion {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("active_version %s not found in upstreams", dc.ActiveVersion)
		}

	case "canary":
		if dc.CanaryPercent < 0 || dc.CanaryPercent > 100 {
			return fmt.Errorf("canary_percent must be between 0 and 100")
		}
		// Check that all upstreams have version labels
		versions := make(map[string]bool)
		for _, up := range upstreams {
			if up.Version == "" {
				return fmt.Errorf("all upstreams must have a version label for canary deployment")
			}
			versions[up.Version] = true
		}
		if len(versions) != 2 {
			return fmt.Errorf("canary deployment requires exactly 2 versions, found %d", len(versions))
		}

	case "rolling":
		// Check that all upstreams have weights
		for _, up := range upstreams {
			if up.Weight <= 0 {
				return fmt.Errorf("all upstreams must have a positive weight for rolling deployment")
			}
		}

	case "recreate":
		if dc.ActiveVersion == "" {
			return fmt.Errorf("active_version is required for recreate deployment")
		}
		// Check that active version exists
		found := false
		for _, up := range upstreams {
			if up.Version == dc.ActiveVersion {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("active_version %s not found in upstreams", dc.ActiveVersion)
		}

	default:
		return fmt.Errorf("unknown deployment type: %s (must be blue-green, canary, rolling, or recreate)", dc.Type)
	}

	return nil
}

func (sc SecurityConfig) Validate() error {
	// Validate IP Blocker
	if sc.IPBlocker.Enabled {
		if sc.IPBlocker.Mode != "allowlist" && sc.IPBlocker.Mode != "blocklist" && sc.IPBlocker.Mode != "off" {
			return fmt.Errorf("ip_blocker mode must be 'allowlist', 'blocklist', or 'off', got: %s", sc.IPBlocker.Mode)
		}

		if sc.IPBlocker.Mode == "allowlist" && len(sc.IPBlocker.Allowlist) == 0 {
			return fmt.Errorf("ip_blocker allowlist mode requires at least one IP/CIDR in allowlist")
		}

		if sc.IPBlocker.Mode == "blocklist" && len(sc.IPBlocker.Blocklist) == 0 {
			return fmt.Errorf("ip_blocker blocklist mode requires at least one IP/CIDR in blocklist")
		}
	}

	// Validate WAF
	if sc.WAF.Enabled {
		if sc.WAF.Mode != "block" && sc.WAF.Mode != "detect" {
			return fmt.Errorf("waf mode must be 'block' or 'detect', got: %s", sc.WAF.Mode)
		}

		// Validate custom rules
		for i, rule := range sc.WAF.CustomRules {
			if rule.ID == "" {
				return fmt.Errorf("waf custom rule %d: id is required", i)
			}
			if rule.Pattern == "" {
				return fmt.Errorf("waf custom rule %s: pattern is required", rule.ID)
			}
			if rule.Target != "uri" && rule.Target != "body" && rule.Target != "headers" && rule.Target != "query" && rule.Target != "all" {
				return fmt.Errorf("waf custom rule %s: target must be 'uri', 'body', 'headers', 'query', or 'all'", rule.ID)
			}
			if rule.Action != "block" && rule.Action != "log" {
				return fmt.Errorf("waf custom rule %s: action must be 'block' or 'log'", rule.ID)
			}
		}
	}

	return nil
}

func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Ensure required fields are initialized
	if cfg.Config.Port == 0 {
		cfg.Config.Port = 8080 // default port
	}
	if cfg.APIs == nil {
		cfg.APIs = []APIConfig{}
	}

	// Validate TLS configuration
	if cfg.Config.TLS {
		if cfg.Config.TLSCertFile == "" {
			return nil, fmt.Errorf("tls_cert_file is required when tls is enabled")
		}
		if cfg.Config.TLSKeyFile == "" {
			return nil, fmt.Errorf("tls_key_file is required when tls is enabled")
		}
	}
	// Validate permissions
	if err := cfg.ValidatePermissions(); err != nil {
		return nil, err
	}

	// Validate auth configs
	for _, auth := range cfg.Auth {
		if err := auth.Validate(); err != nil {
			return nil, fmt.Errorf("invalid auth config: %w", err)
		}
	}

	// Validate and set defaults for each API's routes and upstreams
	for i, api := range cfg.APIs {
		// Validate security config
		if err := api.Security.Validate(); err != nil {
			return nil, fmt.Errorf("invalid security config for API %s: %w", api.Name, err)
		}

		for j, route := range api.Routes {
			// Validate service discovery or upstreams
			if route.ServiceDiscovery != nil && len(route.Upstreams) > 0 {
				return nil, fmt.Errorf("route %s cannot have both upstreams and service_discovery", route.Path)
			}
			if route.ServiceDiscovery == nil && len(route.Upstreams) == 0 {
				return nil, fmt.Errorf("route %s must have either upstreams or service_discovery", route.Path)
			}
			if route.ServiceDiscovery != nil {
				if err := route.ServiceDiscovery.Validate(); err != nil {
					return nil, fmt.Errorf("invalid service_discovery for API %s, route %s: %w", api.Name, route.Path, err)
				}
			}

			// Validate deployment strategy
			if err := route.DeploymentStrategy.Validate(route.Upstreams); err != nil {
				return nil, fmt.Errorf("invalid deployment strategy for API %s, route %s: %w", api.Name, route.Path, err)
			}

			for k, upstream := range route.Upstreams {
				// default tls to false if not set
				if upstream.TLS == nil {
					def := false
					cfg.APIs[i].Routes[j].Upstreams[k].TLS = &def
				}

				// if tls=true but tls_insecure missing -> default to false
				if *cfg.APIs[i].Routes[j].Upstreams[k].TLS && upstream.TLSInsecure == nil {
					def := false
					cfg.APIs[i].Routes[j].Upstreams[k].TLSInsecure = &def
				}

				// if tls_insecure is set but tls=false -> error
				if (upstream.TLS == nil || !*upstream.TLS) && upstream.TLSInsecure != nil {
					return nil, fmt.Errorf("invalid config: tls_insecure cannot be set without tls=true (API %s, route %s)", api.Name, route.Path)
				}
			}
		}
	}
	return &cfg, nil
}

func (a *AuthConfig) Validate() error {
	switch a.Type {
	case "jwt":
		if a.JwtSecret == "" {
			return fmt.Errorf("jwt_secret is required for group %s (type=jwt)", a.Group)
		}
	case "basic":
		if len(a.Users) == 0 {
			return fmt.Errorf("at least one user is required for group %s (type=basic)", a.Group)
		}
		for _, u := range a.Users {
			if u.Username == "" || u.PasswordHash == "" {
				return fmt.Errorf("username and password are required for basic auth user in group %s", a.Group)
			}
		}
	default:
		return fmt.Errorf("invalid auth type '%s' for group %s (must be jwt or basic)", a.Type, a.Group)
	}
	return nil
}

func (cfg *Config) GetAuthByGroup(group string) (*AuthConfig, bool) {
	for _, a := range cfg.Auth {
		if a.Group == group {
			return &a, true
		}
	}
	return nil, false
}

func (cfg *Config) ValidatePermissions() error {
	// Build a map of all auth groups
	authGroups := make(map[string]struct{})
	for _, a := range cfg.Auth {
		authGroups[a.Group] = struct{}{}
	}

	// Iterate over all APIs and check their permissions
	for _, api := range cfg.APIs {
		for _, perm := range api.Permissions {
			if _, ok := authGroups[perm]; !ok {
				return fmt.Errorf("API '%s' has unknown permission group '%s'", api.Name, perm)
			}
		}
	}

	return nil
}
