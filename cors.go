package go_proxy

import (
	"net/http"
	"strings"
)

func withCORS(next http.Handler, corsCfg CORSConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Allowed origins
		if len(corsCfg.AllowedOrigins) > 0 {
			if contains(corsCfg.AllowedOrigins, "*") || contains(corsCfg.AllowedOrigins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}

		// Allowed methods
		if len(corsCfg.AllowedMethods) > 0 {
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(corsCfg.AllowedMethods, ", "))
		}

		// Allowed headers
		if len(corsCfg.AllowedHeaders) > 0 {
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(corsCfg.AllowedHeaders, ", "))
		}

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func contains(arr []string, s string) bool {
	for _, a := range arr {
		if a == s {
			return true
		}
	}
	return false
}
