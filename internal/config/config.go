package config


import (
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Config struct {
		Port int  `yaml:"port"`
		TLS  bool `yaml:"tls"`
		Auth bool `yaml:"auth"`
		ReadTimeout  string        `yaml:"read_timeout"`  // duration as string
		WriteTimeout string        `yaml:"write_timeout"`
		IdleTimeout  string        `yaml:"idle_timeout"`
	} `yaml:"config"`
	Auth AuthConfig `yaml:"auth"`
	APIs []APIConfig `yaml:"apis"`
}

type AuthConfig struct {
	Type      string `yaml:"type"`
	JWTSecret string `yaml:"jwt_secret"`
}

type APIConfig struct {
	Name            string            `yaml:"name"`
	Protocol        string            `yaml:"protocol"`
	Public          bool              `yaml:"public"`
	Routes          []Route           `yaml:"routes"`
	CORS            CORSConfig        `yaml:"cors"`
	RateLimit       int               `yaml:"rate_limit"`
	LoadBalancing   string            `yaml:"load_balancing"`
	Sticky          bool              `yaml:"sticky_sessions"`
	Headers         map[string]string `yaml:"headers"`
	Cookies         map[string]string `yaml:"cookies"`
	MaxRetries      int               `yaml:"max_retries"`
	RetryDelayMs    int               `yaml:"retry_delay_ms"`
	FallbackResponse FallbackResponse `yaml:"fallback_response"`
}

type FallbackResponse struct {
	Status int    `yaml:"status"`
	Body   string `yaml:"body"`
}

type Route struct {
	Path      string     `yaml:"path"`
	Upstreams []Upstream `yaml:"upstreams"`
}

type Upstream struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
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
	return &cfg, nil
}