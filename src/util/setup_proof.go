package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// setupProofTTL is how long a verified setup token proof stays acceptable.
// The setup wizard is a one-shot first-run flow; an hour matches the MaxAge
// previously set on the raw setup_token_verified cookie.
const setupProofTTL = 1 * time.Hour

// SignSetupProof builds the value of the setup_token_verified cookie for a
// successful setup-token verification. The value is "<issued-unix>.<hmac>",
// where the HMAC is SHA-256 over the timestamp keyed by the project-wide
// server.security.encryption_key. Binding the proof to the key means a value
// cannot be forged client-side the way a bare "true" cookie can, and the
// timestamp bounds how long a captured proof stays replayable.
func SignSetupProof(base64Key string, now time.Time) string {
	issued := strconv.FormatInt(now.Unix(), 10)
	return issued + "." + setupProofMAC(base64Key, issued)
}

// VerifySetupProof reports whether value is a proof minted by SignSetupProof
// under the same key and issued no earlier than setupProofTTL ago. It fails
// closed on an empty key, a malformed value, a bad signature, and a future or
// expired timestamp.
func VerifySetupProof(base64Key, value string, now time.Time) bool {
	keyBytes, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil || len(keyBytes) == 0 {
		return false
	}

	issued, signature, found := strings.Cut(value, ".")
	if !found || signature == "" {
		return false
	}

	if !hmac.Equal([]byte(signature), []byte(setupProofMAC(base64Key, issued))) {
		return false
	}

	issuedUnix, err := strconv.ParseInt(issued, 10, 64)
	if err != nil {
		return false
	}

	issuedAt := time.Unix(issuedUnix, 0)
	if issuedAt.After(now) {
		return false
	}

	return now.Sub(issuedAt) < setupProofTTL
}

// setupProofMAC returns the base64 HMAC-SHA256 of issued under base64Key.
func setupProofMAC(base64Key, issued string) string {
	keyBytes, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return ""
	}

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(issued))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
