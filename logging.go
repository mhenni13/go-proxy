package go_proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type Logger struct {
	encoder *json.Encoder
}

var logger = NewLogger()

func NewLogger() *Logger {
	return &Logger{encoder: json.NewEncoder(os.Stdout)}
}

func (l *Logger) Log(fields map[string]interface{}) {
	fields["timestamp"] = time.Now().Format(time.RFC3339Nano)
	_ = l.encoder.Encode(fields)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		r.Header.Set("X-Request-ID", requestID)
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		var bodyBuf bytes.Buffer
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)
			bodyBuf.Write(bodyBytes)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		logger.Log(map[string]interface{}{
			"type":        "http_request",
			"request_id":  requestID,
			"method":      r.Method,
			"path":        r.URL.Path,
			"client_ip":   r.RemoteAddr,
			"status":      lrw.statusCode,
			"duration_ms": duration.Milliseconds(),
			"body":        truncate(bodyBuf.String(), 200),
		})
	})
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
