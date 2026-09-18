// Package middleware - canonical API error response writer per AI.md PART 14
package middleware

import (
	"encoding/json"
	"net/http"
)

// writeAPIError writes the canonical error body from AI.md PART 14:
// {"ok":false,"error":"CODE","message":"..."}. The HTTP status carries the
// status, so it is never duplicated in the body, and operational metadata
// belongs in headers (Retry-After) rather than ad-hoc top-level fields.
func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      false,
		"error":   code,
		"message": message,
	})
}
