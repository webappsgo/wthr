// Package middleware - canonical API error response writer per AI.md PART 14
package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/webappsgo/wthr/src/util"
)

// bearerChallenge builds the RFC 6750 WWW-Authenticate value AI.md PART 14
// requires on 401 and on token scope failures. invalid_token is only named
// for a token that was presented and rejected (invalid, expired, revoked).
func bearerChallenge(code string) string {
	base := fmt.Sprintf(`Bearer realm=%q`, util.ProjectName)
	switch code {
	case "TOKEN_INVALID", "TOKEN_EXPIRED", "TOKEN_REVOKED":
		return base + `, error="invalid_token"`
	case "FORBIDDEN":
		return base + `, error="insufficient_scope"`
	}
	return base
}

// writeAPIError writes the canonical error body from AI.md PART 14:
// {"ok":false,"error":"CODE","message":"..."}. The HTTP status carries the
// status, so it is never duplicated in the body, and operational metadata
// belongs in headers (Retry-After) rather than ad-hoc top-level fields.
func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", bearerChallenge(code))
	}
	w.WriteHeader(status)
	data, err := json.MarshalIndent(map[string]interface{}{
		"ok":      false,
		"error":   code,
		"message": message,
	}, "", "  ")
	if err != nil {
		return
	}
	_, _ = w.Write(append(data, '\n'))
}
