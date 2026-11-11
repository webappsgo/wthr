package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

// NewOperatorToken returns a random 32-character token used when
// server.token is not configured.
func NewOperatorToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

// RequireOperatorToken gates every /debug/* endpoint per AI.md PART 6: the
// operator token (server.token) must be presented as a Bearer credential. The
// inbound token is hashed and compared in constant time against the hash
// computed once at startup. --debug enables the endpoints, it never replaces
// authentication.
func RequireOperatorToken(token string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if token == "" || !strings.HasPrefix(header, "Bearer ") {
				writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Operator token required")
				return
			}
			got := sha256.Sum256([]byte(strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Operator token required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
