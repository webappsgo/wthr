// Package middleware - Request body size limiter per AI.md PART 18
package middleware

import (
	"encoding/json"
	"net/http"
)

// BodySizeLimit constants per AI.md PART 18 line 15691
const (
	// DefaultMaxBodySize is 10MB per AI.md PART 18
	DefaultMaxBodySize = 10 << 20
)

// BodySizeLimitMiddleware limits request body size per AI.md PART 18 line 15691
// Default: 10MB (max_body_size: 10MB)
func BodySizeLimitMiddleware(maxSize int64) func(http.Handler) http.Handler {
	if maxSize <= 0 {
		maxSize = DefaultMaxBodySize
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip body size check for GET, HEAD, OPTIONS requests
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Check Content-Length header if present
			if r.ContentLength > maxSize {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":  "Request body too large",
					"code":   "BODY_TOO_LARGE",
					"status": http.StatusRequestEntityTooLarge,
				})
				return
			}

			// Wrap body with size limiter
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)

			next.ServeHTTP(w, r)
		})
	}
}
