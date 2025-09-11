package proxy

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/mhenni13/go-proxy/internal/auth"
	"github.com/mhenni13/go-proxy/internal/config"
	"github.com/mhenni13/go-proxy/internal/cors"
	"github.com/mhenni13/go-proxy/internal/headers"
	"github.com/mhenni13/go-proxy/internal/logging"
	"github.com/mhenni13/go-proxy/internal/ratelimit"
)

// RegisterAPIs registers all API routes from config
func RegisterAPIs(mux *http.ServeMux, cfg *config.Config) {
	for _, api := range cfg.APIs {
		for _, route := range api.Routes {
			lb := NewLoadBalancer(api.LoadBalancing, route.Upstreams)
			limiter, err := ratelimit.NewRateLimiter(api.RateLimit)
			if err != nil {
				log.Fatalf("Invalid rate_limit for API %s: %v", api.Name, err)
			}

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestID := r.Header.Get("X-Request-ID")

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

				var lastErr error
				for i := 0; i <= api.MaxRetries; i++ {
					target := lb.Next()

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
