package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/handler"
	"github.com/webappsgo/wthr/src/server/service"
)

func TestRuntimeCacheConfigForModeRequiresSharedBackend(t *testing.T) {
	t.Run("cluster rejects memory backend at runtime", func(t *testing.T) {
		cfg := &config.AppConfig{Server: config.ServerConfig{Cache: config.DefaultCacheConfig()}}
		got, misconfigured := runtimeCacheConfigForMode(cfg, true)
		if !misconfigured || got.Type != "unavailable" {
			t.Fatalf("runtime cache = %#v, misconfigured = %t; want unavailable, true", got, misconfigured)
		}
		if cfg.Server.Cache.Type != "memory" {
			t.Fatalf("runtime validation mutated persisted config: %q", cfg.Server.Cache.Type)
		}
	})
	t.Run("cluster accepts redis backend", func(t *testing.T) {
		cfg := &config.AppConfig{Server: config.ServerConfig{Cache: config.CacheConfig{Type: "redis"}}}
		got, misconfigured := runtimeCacheConfigForMode(cfg, true)
		if misconfigured || got.Type != "redis" {
			t.Fatalf("runtime cache = %#v, misconfigured = %t; want redis, false", got, misconfigured)
		}
	})
}

// TestGetDefaultListenAddress covers the IPv6-detection / fallback logic in
// getDefaultListenAddress. It only ever binds to an ephemeral local port
// ("[::]:0") and closes it immediately - no external network access, no
// fixed port binding - so it is safe to exercise in a unit test.
func TestGetDefaultListenAddress(t *testing.T) {
	t.Run("returns a valid listen address", func(t *testing.T) {
		got := getDefaultListenAddress()

		switch got {
		case "::", "0.0.0.0":
			// both are valid depending on whether the test environment
			// supports IPv6 dual-stack listening
		default:
			t.Fatalf("getDefaultListenAddress() = %q, want %q or %q", got, "::", "0.0.0.0")
		}
	})

	t.Run("does not leak the probe listener", func(t *testing.T) {
		// Calling it twice in a row must not fail or hang - if the first
		// call leaked its ephemeral listener, a second dual-stack bind on
		// "[::]:0" would still succeed (different ephemeral port), so this
		// mainly guards against a panic/deadlock regression rather than a
		// specific leaked-socket assertion.
		first := getDefaultListenAddress()
		second := getDefaultListenAddress()

		if first != second {
			t.Fatalf("getDefaultListenAddress() not stable across calls: first=%q second=%q", first, second)
		}
	})
}

func TestCLIBinaryCatalogAndNameValidation(t *testing.T) {
	for _, name := range []string{"wthr-cli-linux-amd64", "wthr-cli-windows-arm64.exe"} {
		if !validCLIBinaryName(name, "wthr") {
			t.Errorf("validCLIBinaryName(%q) = false", name)
		}
	}
	for _, name := range []string{"../wthr-cli-linux-amd64", "wthr-cli-linux-amd64.exe", "wthr-cli-plan9-amd64"} {
		if validCLIBinaryName(name, "wthr") {
			t.Errorf("validCLIBinaryName(%q) = true, want false", name)
		}
	}

	dir := t.TempDir()
	binary := []byte("test cli binary")
	if err := os.WriteFile(filepath.Join(dir, "wthr-cli-linux-amd64"), binary, 0600); err != nil {
		t.Fatal(err)
	}
	catalog := discoverCLIVersions(dir, "wthr", "1.2.3", map[string]config.UpdateVersion{"linux-amd64": {Version: "1.2.4"}})
	entry, ok := catalog["linux-amd64"]
	if !ok || entry.Version != "1.2.4" {
		t.Fatalf("catalog entry = %#v, want version 1.2.4", entry)
	}
	hash := sha256.Sum256(binary)
	if entry.SHA256 != hex.EncodeToString(hash[:]) {
		t.Errorf("catalog checksum = %q, want %q", entry.SHA256, hex.EncodeToString(hash[:]))
	}
	if len(catalog) != 1 || strings.Contains(entry.SHA256, " ") {
		t.Fatalf("catalog includes unavailable binaries or malformed checksum: %#v", catalog)
	}
}

func TestCacheContextMiddlewareConnectsAdminCacheClear(t *testing.T) {
	cache := service.NewCacheManager()
	if err := cache.SetCachedValue("weather", "cached", 0); err != nil {
		t.Fatal(err)
	}
	clear := cacheContextMiddleware(cache)(http.HandlerFunc(handler.ClearCache))
	req := httptest.NewRequest(http.MethodPost, "/admin/cache/clear", nil)
	response := httptest.NewRecorder()
	clear.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("cache clear status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if _, err := cache.GetCachedValue("weather"); err == nil {
		t.Fatal("cache clear left cached entry in manager")
	}
}

// TestWriteJSON covers the local writeJSON mirror of the response helper -
// status code, content type, and body encoding.
func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusTeapot, map[string]string{"hello": "world"})

	if rec.Code != http.StatusTeapot {
		t.Fatalf("writeJSON() status = %d, want %d", rec.Code, http.StatusTeapot)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("writeJSON() Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("writeJSON() body did not decode as JSON: %v", err)
	}

	if body["hello"] != "world" {
		t.Fatalf("writeJSON() body = %v, want hello=world", body)
	}
}

// TestWriteText covers the local writeText mirror of the response helper -
// status code, content type, and formatted body.
func TestWriteText(t *testing.T) {
	rec := httptest.NewRecorder()

	writeText(rec, http.StatusAccepted, "count=%d", 3)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("writeText() status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("writeText() Content-Type = %q, want %q", ct, "text/plain; charset=utf-8")
	}

	if got := rec.Body.String(); got != "count=3" {
		t.Fatalf("writeText() body = %q, want %q", got, "count=3")
	}
}
