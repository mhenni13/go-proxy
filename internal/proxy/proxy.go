package proxy

import (
	"encoding/base64"
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

				if cfg.Config.Auth && !api.Public {
					if !auth.ValidateJWT(r, cfg.Auth.JWTSecret) {
						logging.LoggerInstance.Log(map[string]interface{}{
							"type":       "auth_error",
							"request_id": requestID,
							"client":     r.RemoteAddr,
							"path":       r.URL.Path,
							"error":      "unauthorized",
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

					if api.Protocol == "websocket" {
						handleWebSocket(w, r, target)
						return
					}

					proxy := httputil.NewSingleHostReverseProxy(&url.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s:%d", target.Host, target.Port),
					})

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
					if api.FallbackResponse != nil {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(api.FallbackResponse.Status)
						var body []byte
						if api.FallbackResponse.BodyBase64 != "" {
							decoded, err := base64.StdEncoding.DecodeString(api.FallbackResponse.BodyBase64)
							if err != nil {
								body = []byte(`{"error":"invalid base64 fallback"}`)
							} else {
								body = decoded
							}
						} else {
							body = []byte(api.FallbackResponse.Body)
						}
						_, _ = w.Write(body)
					} else {
						// no fallback configured, return generic 502
						http.Error(w, "Bad Gateway", http.StatusBadGateway)
					}
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
