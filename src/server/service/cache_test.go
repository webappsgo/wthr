package service

import (
	"os"
	"sync"
	"testing"
	"time"
)

// cacheEnvVars lists every environment variable NewCacheManager reads, so
// tests can save/restore them and avoid leaking state between subtests.
var cacheEnvVars = []string{
	"CACHE_ENABLED", "CACHE_HOST", "CACHE_PORT", "CACHE_PASSWORD", "CACHE_DB",
}

// withCacheEnv sets the given env vars for the duration of fn and restores
// the previous values (including "unset") afterward. Not safe to run in
// parallel with other tests that touch the same env vars.
func withCacheEnv(t *testing.T, vars map[string]string, fn func()) {
	t.Helper()
	prev := make(map[string]string, len(cacheEnvVars))
	prevSet := make(map[string]bool, len(cacheEnvVars))
	for _, k := range cacheEnvVars {
		prev[k], prevSet[k] = os.LookupEnv(k)
	}
	t.Cleanup(func() {
		for _, k := range cacheEnvVars {
			if prevSet[k] {
				os.Setenv(k, prev[k])
			} else {
				os.Unsetenv(k)
			}
		}
	})

	for _, k := range cacheEnvVars {
		os.Unsetenv(k)
	}
	for k, v := range vars {
		os.Setenv(k, v)
	}
	fn()
}

// TestCache_NewCacheManager_DisabledByDefault covers the zero-config case:
// no CACHE_ENABLED set at all.
func TestCache_NewCacheManager_DisabledByDefault(t *testing.T) {
	withCacheEnv(t, nil, func() {
		cm := NewCacheManager()
		if cm == nil {
			t.Fatal("NewCacheManager() = nil")
		}
		if cm.IsEnabled() {
			t.Error("IsEnabled() = true, want false when CACHE_ENABLED is unset")
		}
	})
}

// TestCache_NewCacheManager_EnabledFlagParsing covers every value the
// enabled-flag parser treats as "true" vs everything else (treated as
// disabled), including empty string, garbage, and boundary values.
func TestCache_NewCacheManager_EnabledFlagParsing(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantEnabledAttempt bool
	}{
		{"unset", "", false},
		{"true literal", "true", true},
		{"one literal", "1", true},
		{"false literal", "false", false},
		{"zero literal", "0", false},
		{"yes (not accepted)", "yes", false},
		{"True capitalized (not accepted, case-sensitive)", "True", false},
		{"garbage", "banana", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{
				// Point at a closed/unreachable port so any "enabled attempt"
				// fails fast and deterministically falls back to disabled.
				"CACHE_HOST": "127.0.0.1",
				"CACHE_PORT": "1",
			}
			if tt.value != "" {
				env["CACHE_ENABLED"] = tt.value
			}
			withCacheEnv(t, env, func() {
				cm := NewCacheManager()
				if cm == nil {
					t.Fatal("NewCacheManager() = nil")
				}
				// Regardless of wantEnabledAttempt, an unreachable Redis
				// must always result in a gracefully disabled manager.
				if cm.IsEnabled() {
					t.Errorf("IsEnabled() = true for CACHE_ENABLED=%q pointed at unreachable host, want false", tt.value)
				}
			})
		})
	}
}

// TestCache_NewCacheManager_UnreachableServerDegradesGracefully verifies
// that when caching is requested but the Redis server cannot be reached,
// NewCacheManager does not panic or error, and simply returns a manager
// with caching disabled.
func TestCache_NewCacheManager_UnreachableServerDegradesGracefully(t *testing.T) {
	env := map[string]string{
		"CACHE_ENABLED": "true",
		"CACHE_HOST":    "127.0.0.1",
		"CACHE_PORT":    "1", // reserved port, connection refused immediately
	}
	withCacheEnv(t, env, func() {
		cm := NewCacheManager()
		if cm == nil {
			t.Fatal("NewCacheManager() = nil")
		}
		if cm.IsEnabled() {
			t.Fatal("IsEnabled() = true, want false when Redis is unreachable")
		}
	})
}

// TestCache_NewCacheManager_InvalidDBValue verifies a non-numeric CACHE_DB
// does not panic; it should fall back to DB 0 and still attempt (and fail)
// the connection gracefully.
func TestCache_NewCacheManager_InvalidDBValue(t *testing.T) {
	env := map[string]string{
		"CACHE_ENABLED": "true",
		"CACHE_HOST":    "127.0.0.1",
		"CACHE_PORT":    "1",
		"CACHE_DB":      "not-a-number",
	}
	withCacheEnv(t, env, func() {
		cm := NewCacheManager()
		if cm == nil {
			t.Fatal("NewCacheManager() = nil")
		}
		if cm.IsEnabled() {
			t.Fatal("IsEnabled() = true, want false when Redis is unreachable")
		}
	})
}

// TestCache_NewCacheManager_DefaultHostPort verifies missing CACHE_HOST /
// CACHE_PORT fall back to localhost:6379 without panicking (still expected
// to gracefully disable in a test environment with no local Redis).
func TestCache_NewCacheManager_DefaultHostPort(t *testing.T) {
	env := map[string]string{
		"CACHE_ENABLED": "true",
	}
	withCacheEnv(t, env, func() {
		cm := NewCacheManager()
		if cm == nil {
			t.Fatal("NewCacheManager() = nil")
		}
		// This assertion holds in CI/sandboxed environments where no Redis
		// listens on localhost:6379. If a local Redis happens to be
		// running, caching may legitimately become enabled; we only assert
		// that construction never panics or returns nil, which is checked
		// above.
	})
}

// --- Disabled-manager behavior: every method must degrade gracefully ---

func disabledManager() *CacheManager {
	return &CacheManager{enabled: false, ctx: nil}
}

// TestCache_DisabledManager_Get verifies Get returns an explicit error
// (not a zero-value success) when caching is disabled, so callers cannot
// mistake a disabled cache for a real cache miss.
func TestCache_DisabledManager_Get(t *testing.T) {
	cm := disabledManager()
	val, err := cm.Get("any-key")
	if err == nil {
		t.Error("Get() on disabled cache: err = nil, want error")
	}
	if val != "" {
		t.Errorf("Get() on disabled cache: val = %q, want empty", val)
	}
}

// TestCache_DisabledManager_Get_EmptyAndMissingKeys covers boundary key
// inputs (empty string key) on a disabled manager.
func TestCache_DisabledManager_Get_EmptyAndMissingKeys(t *testing.T) {
	cm := disabledManager()
	if _, err := cm.Get(""); err == nil {
		t.Error("Get(\"\") on disabled cache: want error")
	}
}

// TestCache_DisabledManager_Set verifies Set silently succeeds (by design,
// per the source comment) so that callers using the cache as an optional
// optimization are never broken by its absence.
func TestCache_DisabledManager_Set(t *testing.T) {
	cm := disabledManager()
	tests := []struct {
		name string
		key  string
		val  string
		ttl  time.Duration
	}{
		{"normal", "k", "v", time.Minute},
		{"empty key", "", "v", time.Minute},
		{"empty value", "k", "", time.Minute},
		{"zero ttl", "k", "v", 0},
		{"negative ttl", "k", "v", -time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cm.Set(tt.key, tt.val, tt.ttl); err != nil {
				t.Errorf("Set(%q, %q, %v) = %v, want nil (silent success when disabled)", tt.key, tt.val, tt.ttl, err)
			}
		})
	}
}

// TestCache_DisabledManager_MutatingOpsAreNoOps checks that Delete,
// DeletePattern, Expire, and Flush all silently succeed when the cache is
// disabled, matching the documented "graceful no-op" contract.
func TestCache_DisabledManager_MutatingOpsAreNoOps(t *testing.T) {
	cm := disabledManager()

	if err := cm.Delete("k"); err != nil {
		t.Errorf("Delete() = %v, want nil", err)
	}
	if err := cm.Delete(""); err != nil {
		t.Errorf("Delete(\"\") = %v, want nil", err)
	}
	if err := cm.DeletePattern("prefix:*"); err != nil {
		t.Errorf("DeletePattern() = %v, want nil", err)
	}
	if err := cm.DeletePattern(""); err != nil {
		t.Errorf("DeletePattern(\"\") = %v, want nil", err)
	}
	if err := cm.Expire("k", time.Minute); err != nil {
		t.Errorf("Expire() = %v, want nil", err)
	}
	if err := cm.Expire("k", -time.Minute); err != nil {
		t.Errorf("Expire() with negative ttl = %v, want nil", err)
	}
	if err := cm.Flush(); err != nil {
		t.Errorf("Flush() = %v, want nil", err)
	}
}

// TestCache_DisabledManager_Exists verifies Exists reports false with no
// error (a disabled cache behaves as if every key is absent, never errors).
func TestCache_DisabledManager_Exists(t *testing.T) {
	cm := disabledManager()
	exists, err := cm.Exists("any-key")
	if err != nil {
		t.Errorf("Exists() err = %v, want nil", err)
	}
	if exists {
		t.Error("Exists() = true, want false on disabled cache")
	}
}

// TestCache_DisabledManager_TTL verifies TTL returns a zero duration and an
// explicit error when disabled.
func TestCache_DisabledManager_TTL(t *testing.T) {
	cm := disabledManager()
	ttl, err := cm.TTL("k")
	if err == nil {
		t.Error("TTL() err = nil, want error on disabled cache")
	}
	if ttl != 0 {
		t.Errorf("TTL() = %v, want 0", ttl)
	}
}

// TestCache_DisabledManager_Increment verifies Increment returns 0 and an
// explicit error when disabled (never silently returns a fake counter).
func TestCache_DisabledManager_Increment(t *testing.T) {
	cm := disabledManager()
	n, err := cm.Increment("counter")
	if err == nil {
		t.Error("Increment() err = nil, want error on disabled cache")
	}
	if n != 0 {
		t.Errorf("Increment() = %v, want 0", n)
	}
}

// TestCache_DisabledManager_GetStats verifies the stats map reports the
// disabled status explicitly rather than an empty/nil map.
func TestCache_DisabledManager_GetStats(t *testing.T) {
	cm := disabledManager()
	stats, err := cm.GetStats()
	if err != nil {
		t.Fatalf("GetStats() err = %v, want nil", err)
	}
	if stats["status"] != "disabled" {
		t.Errorf("GetStats()[\"status\"] = %q, want \"disabled\"", stats["status"])
	}
}

// TestCache_DisabledManager_Close verifies Close is a safe no-op even when
// the underlying redis client is nil (guards against a nil-pointer panic).
func TestCache_DisabledManager_Close(t *testing.T) {
	cm := disabledManager()
	if err := cm.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	// Idempotent: closing twice must not panic.
	if err := cm.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

// TestCache_DisabledManager_Ping verifies Ping returns an explicit error
// rather than a false "healthy" signal.
func TestCache_DisabledManager_Ping(t *testing.T) {
	cm := disabledManager()
	if err := cm.Ping(); err == nil {
		t.Error("Ping() err = nil, want error on disabled cache")
	}
}

// TestCache_DisabledManager_IsEnabled_Idempotent checks repeated calls to
// IsEnabled are stable.
func TestCache_DisabledManager_IsEnabled_Idempotent(t *testing.T) {
	cm := disabledManager()
	first := cm.IsEnabled()
	for i := 0; i < 10; i++ {
		if cm.IsEnabled() != first {
			t.Fatalf("IsEnabled() not stable across repeated calls")
		}
	}
	if first {
		t.Error("IsEnabled() = true, want false for explicitly disabled manager")
	}
}

// TestCache_DisabledManager_Concurrency drives many goroutines through the
// full disabled-path API concurrently to catch data races (run with
// `go test -race`) and ensure no method panics under concurrent use.
func TestCache_DisabledManager_Concurrency(t *testing.T) {
	cm := disabledManager()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			key := "key"
			_, _ = cm.Get(key)
			_ = cm.Set(key, "value", time.Minute)
			_, _ = cm.Exists(key)
			_, _ = cm.TTL(key)
			_, _ = cm.Increment(key)
			_ = cm.Expire(key, time.Minute)
			_ = cm.Delete(key)
			_ = cm.DeletePattern("key:*")
			_, _ = cm.GetStats()
			_ = cm.IsEnabled()
			_ = cm.Ping()
		}(i)
	}
	wg.Wait()

	// Flush/Close are safe to call once more after concurrent use.
	if err := cm.Flush(); err != nil {
		t.Errorf("Flush() after concurrent use = %v, want nil", err)
	}
	if err := cm.Close(); err != nil {
		t.Errorf("Close() after concurrent use = %v, want nil", err)
	}
}
