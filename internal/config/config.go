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
	Auth AuthConfig  `yaml:"auth"`
	APIs []APIConfig `yaml:"apis"`
}

type AuthConfig struct {
	Type      string `yaml:"type"`
	JWTSecret string `yaml:"jwt_secret"`
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
	FallbackResponse *FallbackResponse `yaml:"fallback_response,omitempty"`
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
