package headers

import (
	"net/http"
)

func WithHeaders(next http.Handler, headers map[string]string, cookies map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set custom headers
		for k, v := range headers {
			w.Header().Set(k, v)
		}

		// Set custom cookies
		for k, v := range cookies {
			http.SetCookie(w, &http.Cookie{
				Name:  k,
				Value: v,
				Path:  "/",
			})
		}

		next.ServeHTTP(w, r)
	})
}
