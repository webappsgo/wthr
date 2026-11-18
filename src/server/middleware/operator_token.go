package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

const (
	// operatorTokenPrefix starts every operator token (AI.md PART 11 "Operator Token")
	operatorTokenPrefix = "tok_"
	// operatorTokenChars is the number of random base62 characters after the prefix
	operatorTokenChars = 32
	// base62Alphabet is the alphabet of the random part
	base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// operatorTokenVisibleTail is how many trailing characters the masked form shows
	operatorTokenVisibleTail = 4
)

// operatorTokenHash holds SHA-256(server.token), the only form kept in memory. Inbound
// tokens are hashed and compared in constant time, so the raw value is never retained here.
var operatorTokenHash atomic.Pointer[[sha256.Size]byte]

// NewOperatorToken returns a new operator token: the tok_ prefix and 32 URL-safe random
// base62 characters (AI.md PART 11 "Operator Token").
func NewOperatorToken() (string, error) {
	// A byte above 247 is discarded so every character is equally likely (248 = 4 * 62)
	const unbiasedLimit = 248

	token := make([]byte, 0, operatorTokenChars)
	buffer := make([]byte, operatorTokenChars*2)
	for len(token) < operatorTokenChars {
		if _, err := rand.Read(buffer); err != nil {
			return "", fmt.Errorf("generating operator token: %w", err)
		}
		for _, value := range buffer {
			if value >= unbiasedLimit {
				continue
			}
			token = append(token, base62Alphabet[int(value)%len(base62Alphabet)])
			if len(token) == operatorTokenChars {
				break
			}
		}
	}
	return operatorTokenPrefix + string(token), nil
}

// IsOperatorTokenFormat reports whether a value has the operator token shape.
func IsOperatorTokenFormat(token string) bool {
	if !strings.HasPrefix(token, operatorTokenPrefix) || len(token) != len(operatorTokenPrefix)+operatorTokenChars {
		return false
	}
	for _, r := range token[len(operatorTokenPrefix):] {
		if !strings.ContainsRune(base62Alphabet, r) {
			return false
		}
	}
	return true
}

// SetOperatorToken installs the current operator token. Only its SHA-256 is kept in memory;
// an empty value clears it, so no request can authenticate as the operator.
func SetOperatorToken(token string) {
	if token == "" {
		operatorTokenHash.Store(nil)
		return
	}
	sum := sha256.Sum256([]byte(token))
	operatorTokenHash.Store(&sum)
}

// IsOperatorToken reports whether the candidate is the current operator token, hashing it
// and comparing in constant time. The previous value stops working the moment a new one is set.
func IsOperatorToken(candidate string) bool {
	want := operatorTokenHash.Load()
	if want == nil || candidate == "" {
		return false
	}
	got := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// MaskOperatorToken returns the form shown in the admin panel: the prefix, a fixed mask and
// the last characters, never the full value.
func MaskOperatorToken(token string) string {
	if len(token) <= len(operatorTokenPrefix)+operatorTokenVisibleTail {
		return strings.Repeat("•", 8)
	}
	return operatorTokenPrefix + strings.Repeat("•", 8) + token[len(token)-operatorTokenVisibleTail:]
}

// RequireOperatorToken gates the operator-only endpoints per AI.md PART 6 and 11: the
// operator token (server.token) must be presented as a Bearer credential. It is read from
// the live state on every request, so a regenerated token applies at once. --debug enables
// the endpoints, it never replaces authentication.
func RequireOperatorToken() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				w.Header().Set("WWW-Authenticate", `Bearer realm="wthr"`)
				writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Operator token required")
				return
			}
			if !IsOperatorToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="wthr", error="invalid_token"`)
				writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Operator token required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
