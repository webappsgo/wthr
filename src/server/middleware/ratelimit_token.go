package middleware

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/server/model"
)

const (
	// tokenKeyTTL is how long a validated token keeps its own rate-limit
	// bucket without another database lookup.
	tokenKeyTTL = 30 * time.Second
	// tokenKeyCacheMax bounds the validated-token cache.
	tokenKeyCacheMax = 4096
)

var (
	tokenKeyMu      sync.Mutex
	tokenKeyServer  *sql.DB
	tokenKeyUsers   *sql.DB
	tokenKeyExpires = map[string]time.Time{}
)

// SetRateLimitTokenDBs gives the rate limiter the databases it needs to tell a
// valid Bearer token from a forged one. Until it is called every request is
// limited per IP.
func SetRateLimitTokenDBs(serverDB, usersDB *sql.DB) {
	tokenKeyMu.Lock()
	defer tokenKeyMu.Unlock()
	tokenKeyServer = serverDB
	tokenKeyUsers = usersDB
	tokenKeyExpires = map[string]time.Time{}
}

// bearerToken returns the Bearer credential of a request, or "".
func bearerToken(r *http.Request) string {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

// rateLimitKey keys a request by its valid Bearer token (AI.md PART 12: clients
// that share an egress IP never throttle one another) and by client IP
// otherwise. A missing, malformed, unknown, expired or revoked token falls back
// to the IP, so failed authentication always counts per IP and a forged token
// can never mint a fresh bucket.
func rateLimitKey(r *http.Request) (string, error) {
	if token := bearerToken(r); token != "" && validTokenForRateLimit(token) {
		sum := sha256.Sum256([]byte(token))

		return "token:" + hex.EncodeToString(sum[:16]), nil
	}

	return rateLimitClientIP(r)
}

// validTokenForRateLimit reports whether token is a currently valid admin or
// user API token, caching a positive answer for tokenKeyTTL.
func validTokenForRateLimit(token string) bool {
	if ValidateTokenPrefix(token) != nil {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	id := hex.EncodeToString(sum[:])

	tokenKeyMu.Lock()
	serverDB, usersDB := tokenKeyServer, tokenKeyUsers
	if expires, ok := tokenKeyExpires[id]; ok && time.Now().Before(expires) {
		tokenKeyMu.Unlock()
		return true
	}
	tokenKeyMu.Unlock()

	valid := false
	switch DetectTokenType(token) {
	case TokenTypeAdmin:
		if serverDB != nil {
			_, err := (&model.AdminModel{DB: serverDB}).GetByAPIToken(token)
			valid = err == nil
		}
	case TokenTypeUser:
		if usersDB != nil {
			_, err := (&model.TokenModelV2{DB: usersDB}).ValidateToken(token)
			valid = err == nil
		}
	}
	if !valid {
		return false
	}

	tokenKeyMu.Lock()
	defer tokenKeyMu.Unlock()
	if len(tokenKeyExpires) >= tokenKeyCacheMax {
		now := time.Now()
		for key, expires := range tokenKeyExpires {
			if now.After(expires) {
				delete(tokenKeyExpires, key)
			}
		}
		if len(tokenKeyExpires) >= tokenKeyCacheMax {
			tokenKeyExpires = map[string]time.Time{}
		}
	}
	tokenKeyExpires[id] = time.Now().Add(tokenKeyTTL)

	return true
}
