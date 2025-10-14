package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/mhenni13/go-proxy/internal/auth"
	"github.com/mhenni13/go-proxy/internal/config"
	"github.com/mhenni13/go-proxy/internal/cors"
	"github.com/mhenni13/go-proxy/internal/deployment"
	"github.com/mhenni13/go-proxy/internal/discovery"
	"github.com/mhenni13/go-proxy/internal/headers"
	"github.com/mhenni13/go-proxy/internal/ipblocker"
	"github.com/mhenni13/go-proxy/internal/logging"
	"github.com/mhenni13/go-proxy/internal/ratelimit"
	"github.com/mhenni13/go-proxy/internal/waf"
)

var discoveryRegistry *discovery.Registry

// RegisterAPIs registers all API routes from config
func RegisterAPIs(mux *http.ServeMux, cfg *config.Config) {
	// Initialize discovery registry
	discoveryRegistry = discovery.NewRegistry()

	for _, api := range cfg.APIs {
		for _, route := range api.Routes {
			// Handle service discovery if configured
			upstreams := route.Upstreams
			if route.ServiceDiscovery != nil {
				provider, refreshPeriod, err := createDiscoveryProvider(route.ServiceDiscovery)
				if err != nil {
					log.Fatalf("Failed to create discovery provider for API %s, route %s: %v", api.Name, route.Path, err)
				}

				if err := discoveryRegistry.Register(route.Path, provider, refreshPeriod); err != nil {
					log.Fatalf("Failed to register discovery provider for API %s, route %s: %v", api.Name, route.Path, err)
				}

				// Get initial upstreams from discovery
				discoveredUpstreams, ok := discoveryRegistry.GetUpstreams(route.Path)
				if !ok || len(discoveredUpstreams) == 0 {
					log.Fatalf("No upstreams discovered for API %s, route %s", api.Name, route.Path)
				}
				upstreams = discoveredUpstreams
			}

			// Create deployment strategy
			strategy, err := deployment.NewStrategy(route.DeploymentStrategy, upstreams, api.LoadBalancing)
			if err != nil {
				log.Fatalf("Failed to create deployment strategy for API %s, route %s: %v", api.Name, route.Path, err)
			}

			limiter, err := ratelimit.NewRateLimiter(api.RateLimit)
			if err != nil {
				log.Fatalf("Invalid rate_limit for API %s: %v", api.Name, err)
			}

			// Initialize IP Blocker
			var ipBlocker *ipblocker.IPBlocker
			if api.Security.IPBlocker.Enabled {
				// Load from files if specified, otherwise use inline lists
				if api.Security.IPBlocker.AllowlistFile != "" || api.Security.IPBlocker.BlocklistFile != "" {
					ipBlocker, err = ipblocker.NewIPBlockerFromFiles(
						api.Security.IPBlocker.AllowlistFile,
						api.Security.IPBlocker.BlocklistFile,
						api.Security.IPBlocker.Mode,
					)
				} else {
					ipBlocker, err = ipblocker.NewIPBlocker(
						api.Security.IPBlocker.Allowlist,
						api.Security.IPBlocker.Blocklist,
						api.Security.IPBlocker.Mode,
					)
				}
				if err != nil {
					log.Fatalf("Failed to create IP blocker for API %s: %v", api.Name, err)
				}
			}

			// Initialize WAF
			var wafInstance *waf.WAF
			if api.Security.WAF.Enabled {
				// Convert config WAF rules to waf package format
				var wafRules []waf.RuleConfig
				for _, rule := range api.Security.WAF.CustomRules {
					wafRules = append(wafRules, waf.RuleConfig{
						ID:          rule.ID,
						Description: rule.Description,
						Pattern:     rule.Pattern,
						Target:      rule.Target,
						Action:      rule.Action,
					})
				}
				wafInstance, err = waf.NewWAFWithLog(true, api.Security.WAF.Mode, wafRules, api.Security.WAF.LogFile)
				if err != nil {
					log.Fatalf("Failed to create WAF for API %s: %v", api.Name, err)
				}
			}

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestID := r.Header.Get("X-Request-ID")

				// IP Blocker check
				if ipBlocker != nil {
					clientIP := ipblocker.ExtractIP(
						r.RemoteAddr,
						r.Header.Get("X-Forwarded-For"),
						r.Header.Get("X-Real-IP"),
					)

					if !ipBlocker.IsAllowed(clientIP) {
						logging.LoggerInstance.Log(map[string]interface{}{
							"type":       "ip_blocked",
							"request_id": requestID,
							"client_ip":  clientIP,
							"path":       r.URL.Path,
							"mode":       ipBlocker.GetMode(),
						})
						http.Error(w, "Forbidden", http.StatusForbidden)
						return
					}
				}

				// WAF check
				if wafInstance != nil && wafInstance.IsEnabled() {
					// Read body for WAF inspection
					var bodyBytes []byte
					if r.Body != nil {
						bodyBytes, _ = io.ReadAll(r.Body)
						r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body for downstream
					}

					blocked, ruleID, description := wafInstance.CheckRequest(r, string(bodyBytes))
					if blocked {
						event := map[string]interface{}{
							"type":        "waf_blocked",
							"request_id":  requestID,
							"client":      r.RemoteAddr,
							"path":        r.URL.Path,
							"rule_id":     ruleID,
							"description": description,
							"mode":        wafInstance.GetMode(),
						}
						logging.LoggerInstance.Log(event)
						wafInstance.LogEvent(event) // Log to WAF file if configured
						http.Error(w, "Forbidden - WAF Rule Triggered", http.StatusForbidden)
						return
					} else if ruleID != "" {
						// Log detection mode triggers
						event := map[string]interface{}{
							"type":        "waf_detected",
							"request_id":  requestID,
							"client":      r.RemoteAddr,
							"path":        r.URL.Path,
							"rule_id":     ruleID,
							"description": description,
							"mode":        wafInstance.GetMode(),
						}
						logging.LoggerInstance.Log(event)
						wafInstance.LogEvent(event) // Log to WAF file if configured
					}
				}

				if !api.Public && len(cfg.Auth) > 0 {
					var authorized bool
					var authErr error

					for _, groupName := range api.Permissions {
						authConfig, ok := cfg.GetAuthByGroup(groupName) // helper to find AuthConfig by group
						if !ok {
							continue
						}
						switch authConfig.Type {
						case "jwt":
							claims, ok := auth.ExtractJWTClaims(r, authConfig.JwtSecret)
							if !ok {
								continue // try next auth config
							}

							// If verify expression exists, evaluate it
							if authConfig.VerifyExpr != "" {
								ok, err := auth.EvalPredicate(authConfig.VerifyExpr, claims)
								if err != nil {
									authErr = err
									continue
								}
								if ok {
									authorized = true
									break
								}
							} else {
								// No verify expression → accept if JWT is valid
								authorized = true
								break
							}

						case "basic":
							username, password, ok := r.BasicAuth()
							if !ok {
								continue
							}

							for _, u := range authConfig.Users {
								if u.Username == username && auth.ComparePassword(u.PasswordHash, password) {
									authorized = true
									break
								}
							}
							if authorized {
								break
							}
						}
					}

					if !authorized {
						logging.LoggerInstance.Log(map[string]interface{}{
							"type":       "auth_error",
							"request_id": requestID,
							"client":     r.RemoteAddr,
							"path":       r.URL.Path,
							"error":      auth.AuthErrOrUnauthorized(authErr),
						})
						http.Error(w, "Unauthorized", http.StatusUnauthorized)
						return
					}
				}

				if !limiter.Allow() {
					http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
					return
				}

				// Update strategy with fresh upstreams if using service discovery
				if discoveryRegistry.IsDiscoveryEnabled(route.Path) {
					if upstreams, ok := discoveryRegistry.GetUpstreams(route.Path); ok && len(upstreams) > 0 {
						strategy.UpdateUpstreams(upstreams)
					}
				}

				var lastErr error
				for i := 0; i <= api.MaxRetries; i++ {
					target, err := strategy.SelectUpstream()
					if err != nil {
						logging.LoggerInstance.Log(map[string]interface{}{
							"type":       "upstream_selection_error",
							"request_id": requestID,
							"error":      err.Error(),
						})
						http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
						return
					}

					// Determine scheme based on TLS config
					scheme := "http"
					if target.TLS != nil && *target.TLS {
						scheme = "https"
					}

					proxy := httputil.NewSingleHostReverseProxy(&url.URL{
						Scheme: scheme,
						Host:   fmt.Sprintf("%s:%d", target.Host, target.Port),
					})

					// Configure TLS settings if https
					if scheme == "https" {
						proxy.Transport = &http.Transport{
							TLSClientConfig: &tls.Config{
								InsecureSkipVerify: target.TLSInsecure != nil && *target.TLSInsecure,
							},
						}
					}

					proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
						lastErr = err
					}

					proxy.ServeHTTP(w, r)
					if lastErr == nil {
						break
					}
					time.Sleep(time.Duration(api.RetryDelayMs) * time.Millisecond)
				}

				if lastErr != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(api.FallbackResponse.Status)
					_, _ = w.Write([]byte(api.FallbackResponse.Body))
				}
			})

			// Wrap with headers and CORS
			handlerWithHeaders := headers.WithHeaders(handler, api.Headers, api.Cookies)
			handlerWithCORS := cors.WithCORS(handlerWithHeaders, api.CORS)

			// Register route
			mux.Handle(route.Path, handlerWithCORS)
		}
	}
}

// createDiscoveryProvider creates a service discovery provider from config
func createDiscoveryProvider(cfg *config.DiscoveryConfig) (discovery.Provider, time.Duration, error) {
	// Parse refresh period
	var refreshPeriod time.Duration
	if cfg.RefreshPeriod != "" {
		var err error
		refreshPeriod, err = time.ParseDuration(cfg.RefreshPeriod)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid refresh_period: %w", err)
		}
	}

	switch cfg.Type {
	case "kubernetes":
		if cfg.Kubernetes == nil {
			return nil, 0, fmt.Errorf("kubernetes configuration is required when type is 'kubernetes'")
		}

		k8sCfg := discovery.KubernetesConfig{
			Namespace:    cfg.Kubernetes.Namespace,
			ServiceName:  cfg.Kubernetes.ServiceName,
			Port:         cfg.Kubernetes.Port,
			UseTLS:       cfg.Kubernetes.UseTLS,
			TLSInsecure:  cfg.Kubernetes.TLSInsecure,
			Labels:       cfg.Kubernetes.Labels,
			UseEndpoints: cfg.Kubernetes.UseEndpoints,
			KubeConfig:   cfg.Kubernetes.KubeConfig,
		}

		provider, err := discovery.NewKubernetesProvider(k8sCfg)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create kubernetes provider: %w", err)
		}

		return provider, refreshPeriod, nil

	case "consul":
		if cfg.Consul == nil {
			return nil, 0, fmt.Errorf("consul configuration is required when type is 'consul'")
		}

		consulCfg := discovery.ConsulConfig{
			Address:     cfg.Consul.Address,
			ServiceName: cfg.Consul.ServiceName,
			Tag:         cfg.Consul.Tag,
			Datacenter:  cfg.Consul.Datacenter,
			Token:       cfg.Consul.Token,
			UseTLS:      cfg.Consul.UseTLS,
			TLSInsecure: cfg.Consul.TLSInsecure,
			OnlyPassing: cfg.Consul.OnlyPassing,
		}

		provider, err := discovery.NewConsulProvider(consulCfg)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create consul provider: %w", err)
		}

		return provider, refreshPeriod, nil

	case "dns":
		if cfg.DNS == nil {
			return nil, 0, fmt.Errorf("dns configuration is required when type is 'dns'")
		}

		dnsCfg := discovery.DNSConfig{
			Hostname:    cfg.DNS.Hostname,
			Port:        cfg.DNS.Port,
			UseSRV:      cfg.DNS.UseSRV,
			Resolver:    cfg.DNS.Resolver,
			UseTLS:      cfg.DNS.UseTLS,
			TLSInsecure: cfg.DNS.TLSInsecure,
		}

		provider, err := discovery.NewDNSProvider(dnsCfg)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create dns provider: %w", err)
		}

		return provider, refreshPeriod, nil

	case "file":
		if cfg.File == nil {
			return nil, 0, fmt.Errorf("file configuration is required when type is 'file'")
		}

		fileCfg := discovery.FileConfig{
			Path:        cfg.File.Path,
			Format:      cfg.File.Format,
			WatchPeriod: cfg.File.WatchPeriod,
		}

		provider, err := discovery.NewFileProvider(fileCfg)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create file provider: %w", err)
		}

		return provider, refreshPeriod, nil

	default:
		return nil, 0, fmt.Errorf("unsupported discovery type: %s", cfg.Type)
	}
}

// GetDiscoveryRegistry returns the discovery registry for graceful shutdown
func GetDiscoveryRegistry() *discovery.Registry {
	return discoveryRegistry
}
