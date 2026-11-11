package service

import (
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/webappsgo/wthr/src/config"
)

func TestCacheManagerDefaultsToMemory(t *testing.T) {
	t.Setenv("CACHE_URL", "")
	cm := NewCacheManager()
	if !cm.IsEnabled() || cm.backend != "memory" {
		t.Fatalf("default manager = enabled %v, backend %q; want memory enabled", cm.IsEnabled(), cm.backend)
	}
	if cm.BackendType() != "memory" {
		t.Fatalf("BackendType() = %q, want memory", cm.BackendType())
	}
	if err := cm.SetCachedValue("weather", "sunny", 0); err != nil {
		t.Fatal(err)
	}
	got, err := cm.GetCachedValue("weather")
	if err != nil || got != "sunny" {
		t.Fatalf("GetCachedValue() = %q, %v; want sunny, nil", got, err)
	}
}

func TestCacheManagerMemoryCRUDAndPrefix(t *testing.T) {
	t.Setenv("CACHE_URL", "")
	cfg := config.DefaultCacheConfig()
	cfg.Prefix = "wthr:"
	cm := NewCacheManager(cfg)
	if err := cm.SetCachedValue("weather", "rain", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, ok := cm.entries["wthr:weather"]; !ok {
		t.Fatal("configured prefix was not applied")
	}
	if ok, err := cm.Exists("weather"); err != nil || !ok {
		t.Fatalf("Exists() = %v, %v; want true, nil", ok, err)
	}
	if err := cm.DeleteCachedValue("weather"); err != nil {
		t.Fatal(err)
	}
	if ok, err := cm.Exists("weather"); err != nil || ok {
		t.Fatalf("Exists() after delete = %v, %v; want false, nil", ok, err)
	}
}

func TestCacheManagerMemoryTTLAndExpiration(t *testing.T) {
	t.Setenv("CACHE_URL", "")
	cfg := config.DefaultCacheConfig()
	cfg.TTL = "40ms"
	cm := NewCacheManager(cfg)
	if err := cm.SetCachedValue("default", "value", 0); err != nil {
		t.Fatal(err)
	}
	ttl, err := cm.TTL("default")
	if err != nil || ttl <= 0 || ttl > 40*time.Millisecond {
		t.Fatalf("TTL() = %v, %v; want positive TTL no greater than 40ms", ttl, err)
	}
	time.Sleep(60 * time.Millisecond)
	if _, err := cm.GetCachedValue("default"); !errors.Is(err, redis.Nil) {
		t.Fatalf("GetCachedValue() after expiration error = %v, want redis.Nil", err)
	}
}

func TestCacheManagerMemoryIncrementExpirePatternAndFlush(t *testing.T) {
	t.Setenv("CACHE_URL", "")
	cm := NewCacheManager()
	if n, err := cm.Increment("counter"); err != nil || n != 1 {
		t.Fatalf("first Increment() = %d, %v; want 1, nil", n, err)
	}
	if n, err := cm.Increment("counter"); err != nil || n != 2 {
		t.Fatalf("second Increment() = %d, %v; want 2, nil", n, err)
	}
	if err := cm.SetCachedValue("group:a", "a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := cm.SetCachedValue("group:b", "b", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := cm.DeletePattern("group:*"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := cm.Exists("group:a"); ok {
		t.Fatal("DeletePattern left a matching key")
	}
	if err := cm.Expire("counter", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	if ok, err := cm.Exists("counter"); err != nil || ok {
		t.Fatalf("Exists() after Expire = %v, %v; want false, nil", ok, err)
	}
	if err := cm.Flush(); err != nil {
		t.Fatal(err)
	}
	stats, err := cm.GetStats()
	if err != nil || stats["entries"] != "0" {
		t.Fatalf("GetStats() = %v, %v; want zero entries", stats, err)
	}
}

func TestCacheManagerDisabledType(t *testing.T) {
	t.Setenv("CACHE_URL", "")
	cfg := config.DefaultCacheConfig()
	cfg.Type = "none"
	cm := NewCacheManager(cfg)
	if cm.IsEnabled() || !cm.IsCacheHealthy() {
		t.Fatalf("disabled manager enabled=%v healthy=%v; want false, true", cm.IsEnabled(), cm.IsCacheHealthy())
	}
	if _, err := cm.GetCachedValue("key"); err == nil {
		t.Fatal("GetCachedValue() on disabled backend should report an error")
	}
}

func TestCacheManagerUnavailableExternalBackendFailsHealthCheck(t *testing.T) {
	t.Setenv("CACHE_URL", "")
	cfg := config.DefaultCacheConfig()
	cfg.Type = "redis"
	cfg.Host = "127.0.0.1"
	cfg.Port = 1
	cfg.Timeout = "100ms"
	cm := NewCacheManager(cfg)
	if cm.IsEnabled() {
		t.Fatal("unreachable external backend should be disabled")
	}
	if cm.IsCacheHealthy() {
		t.Fatal("unreachable configured backend should fail the health check")
	}
}

func TestCacheConfigPartialDefaultsAndEnvironmentURL(t *testing.T) {
	t.Setenv("CACHE_URL", "valkey://env-user:env-pass@cache.example:6380/2")
	cfg := mergeCacheConfig(config.CacheConfig{Password: "configured-password"})
	if cfg.Type != "memory" || cfg.Prefix != "wthr:" || cfg.TTL != "1h" || cfg.Host != "localhost" || cfg.Port != 6379 || cfg.PoolSize != 10 || cfg.MinIdle != 2 {
		t.Fatalf("partial config defaults not preserved: %#v", cfg)
	}
	cfg = cacheURLFromEnvironment(cfg)
	if cfg.URL != "valkey://env-user:env-pass@cache.example:6380/2" || cfg.Type != "valkey" {
		t.Fatalf("environment URL should activate valkey: %#v", cfg)
	}
	opts, err := redisOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Addrs[0] != "cache.example:6380" || opts.Username != "env-user" || opts.Password != "env-pass" || opts.DB != 2 {
		t.Fatalf("environment URL options = %#v", opts)
	}
	cfg.URL = "redis://config-user:config-pass@configured.example:6379/4"
	cfg = cacheURLFromEnvironment(cfg)
	if cfg.URL != "redis://config-user:config-pass@configured.example:6379/4" {
		t.Fatalf("configured URL should take precedence: %q", cfg.URL)
	}
}

func TestCacheStatsRemovesExpiredMemoryEntries(t *testing.T) {
	t.Setenv("CACHE_URL", "")
	cm := NewCacheManager()
	cm.entries["expired"] = memoryCacheEntry{value: "stale", expiresAt: time.Now().Add(-time.Second)}
	stats, err := cm.GetStats()
	if err != nil || stats["entries"] != "0" {
		t.Fatalf("GetStats() = %#v, %v; want expired entry cleaned", stats, err)
	}
}

func TestRedisOptionsFromURLAndDiscreteConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.CacheConfig
		want redis.UniversalOptions
	}{
		{
			name: "URL takes precedence and supports valkey scheme",
			cfg:  config.CacheConfig{Type: "valkey", URL: "valkey://alice:secret@cache.example:6380/3", Host: "ignored", Port: 1234, PoolSize: 20, MinIdle: 4, Timeout: "2s"},
			want: redis.UniversalOptions{Addrs: []string{"cache.example:6380"}, Username: "alice", Password: "secret", DB: 3, PoolSize: 20, MinIdleConns: 4},
		},
		{
			name: "discrete fields",
			cfg:  config.CacheConfig{Type: "redis", Host: "cache.internal", Port: 6379, Username: "app", Password: "secret", DB: 5, PoolSize: 8, MinIdle: 2, Timeout: "3s"},
			want: redis.UniversalOptions{Addrs: []string{"cache.internal:6379"}, Username: "app", Password: "secret", DB: 5, PoolSize: 8, MinIdleConns: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := redisOptions(tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Addrs) != len(tt.want.Addrs) || got.Addrs[0] != tt.want.Addrs[0] || got.Username != tt.want.Username || got.Password != tt.want.Password || got.DB != tt.want.DB || got.PoolSize != tt.want.PoolSize || got.MinIdleConns != tt.want.MinIdleConns {
				t.Fatalf("redisOptions() = %#v, want fields %#v", got, tt.want)
			}
			if got.DialTimeout != 2*time.Second && tt.name == "URL takes precedence and supports valkey scheme" {
				t.Errorf("DialTimeout = %v, want 2s", got.DialTimeout)
			}
		})
	}
}

func TestRedisOptionsClusterAndTLS(t *testing.T) {
	cfg := config.DefaultCacheConfig()
	cfg.Type = "valkey"
	cfg.Cluster = true
	cfg.ClusterNodes = []string{"node1:6379", "node2:6379"}
	cfg.Password = "secret"
	cfg.TLS = true
	opts, err := redisOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Addrs) != 2 || opts.Password != "secret" || opts.TLSConfig == nil || opts.TLSConfig.MinVersion != 0x0303 {
		t.Fatalf("cluster/TLS options = %#v, want nodes, password, and TLS 1.2+", opts)
	}
	cfg.ClusterNodes = nil
	if _, err := redisOptions(cfg); err == nil {
		t.Fatal("cluster without nodes should be rejected")
	}
}

func TestRedisOptionsRejectInvalidURLAndScheme(t *testing.T) {
	for _, cacheURL := range []string{"not a URL", "http://cache.example"} {
		cfg := config.DefaultCacheConfig()
		cfg.Type = "redis"
		cfg.URL = cacheURL
		if _, err := redisOptions(cfg); err == nil {
			t.Errorf("redisOptions(%q) returned nil error", cacheURL)
		}
	}
}
