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
}

type FallbackResponse struct {
	Status     int    `yaml:"status"`
	Body       string `yaml:"body,omitempty"`
	BodyBase64 string `yaml:"body_base64,omitempty"`
}

type Route struct {
	Path      string     `yaml:"path"`
	Upstreams []Upstream `yaml:"upstreams"`
}

type Upstream struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	TLS         *bool  `yaml:"tls,omitempty"`
	TLSInsecure *bool  `yaml:"tls_insecure,omitempty"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
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
	for i, api := range cfg.APIs {
		for j, route := range api.Routes {
			for k, upstream := range route.Upstreams {
				// default tls to false if not set
				if upstream.TLS == nil {
					def := false
					cfg.APIs[i].Routes[j].Upstreams[k].TLS = &def
				}

				if err := cfg.ValidatePermissions(); err != nil {
					return nil, err
				}

				// Validate auth configs
				for _, auth := range cfg.Auth {
					if err := auth.Validate(); err != nil {
						return nil, fmt.Errorf("invalid auth config: %w", err)
					}
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
