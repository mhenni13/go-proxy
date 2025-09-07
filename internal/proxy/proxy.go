package proxy

import (
	"fmt"
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
			limiter := ratelimit.NewRateLimiter(api.RateLimit)

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
