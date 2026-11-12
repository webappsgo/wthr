package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimitKeyForgedTokenFallsBackToIP(t *testing.T) {
	SetRateLimitTokenDBs(nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/x", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("Authorization", "Bearer usr_"+strings.Repeat("a", 32))

	got, err := rateLimitKey(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(got, "token:") {
		t.Fatalf("forged token minted its own bucket: %s", got)
	}
	if got != "203.0.113.9" {
		t.Fatalf("key = %q, want client IP", got)
	}
}

func TestRateLimitKeyValidTokenGetsOwnBucket(t *testing.T) {
	SetRateLimitTokenDBs(nil, nil)
	token := "usr_" + strings.Repeat("b", 32)
	sum := sha256.Sum256([]byte(token))
	tokenKeyMu.Lock()
	tokenKeyExpires[hex.EncodeToString(sum[:])] = time.Now().Add(time.Minute)
	tokenKeyMu.Unlock()

	keys := map[string]bool{}
	for _, addr := range []string{"203.0.113.1:1", "203.0.113.2:1"} {
		req := httptest.NewRequest("GET", "/api/v1/x", nil)
		req.RemoteAddr = addr
		req.Header.Set("Authorization", "Bearer "+token)
		key, err := rateLimitKey(req)
		if err != nil {
			t.Fatal(err)
		}
		keys[key] = true
	}
	if len(keys) != 1 {
		t.Fatalf("one token across two IPs must share a bucket, got %v", keys)
	}
	for key := range keys {
		if !strings.HasPrefix(key, "token:") || strings.Contains(key, token) {
			t.Fatalf("bad token key %q", key)
		}
	}
}

func TestRateLimitKeyForAuthClassesIsAlwaysIP(t *testing.T) {
	for _, name := range []rateLimiterName{rateLimitLogin, rateLimitPasswordReset, rateLimitRegistration, rateLimitFileUpload} {
		req := httptest.NewRequest("POST", "/x", nil)
		req.RemoteAddr = "203.0.113.5:1"
		req.Header.Set("Authorization", "Bearer usr_"+strings.Repeat("c", 32))
		key, _ := rateLimitKeyFor(name)(req)
		if strings.HasPrefix(key, "token:") {
			t.Fatalf("class %d keyed by token", name)
		}
	}
}
