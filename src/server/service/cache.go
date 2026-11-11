package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/webappsgo/wthr/src/config"
)

type memoryCacheEntry struct {
	value     string
	expiresAt time.Time
}

// CacheManager manages the configured memory or Redis/Valkey cache backend.
type CacheManager struct {
	client  redis.UniversalClient
	backend string
	ctx     context.Context
	prefix  string
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]memoryCacheEntry
}

// NewCacheManager builds a manager from server.cache configuration. Omitting
// the argument retains compatibility with callers and uses PART 12 defaults.
func NewCacheManager(configs ...config.CacheConfig) *CacheManager {
	cfg := config.DefaultCacheConfig()
	if len(configs) != 0 {
		cfg = mergeCacheConfig(configs[0])
	}
	backend := strings.ToLower(strings.TrimSpace(cfg.Type))
	if backend == "" {
		backend = "memory"
	}
	cfg = cacheURLFromEnvironment(cfg)
	backend = strings.ToLower(strings.TrimSpace(cfg.Type))
	ttl, err := time.ParseDuration(strings.TrimSpace(cfg.TTL))
	if err != nil || ttl <= 0 {
		ttl = time.Hour
	}
	cm := &CacheManager{backend: backend, ctx: context.Background(), prefix: cfg.Prefix, ttl: ttl}
	if backend == "memory" {
		cm.entries = make(map[string]memoryCacheEntry)
		return cm
	}
	if backend == "none" {
		return cm
	}
	if backend != "redis" && backend != "valkey" {
		cm.backend = "unavailable"
		return cm
	}

	opts, err := redisOptions(cfg)
	if err != nil {
		cm.backend = "unavailable"
		return cm
	}
	cm.client = redis.NewUniversalClient(opts)
	ctx, cancel := context.WithTimeout(cm.ctx, configuredTimeout(cfg))
	defer cancel()
	if err := cm.client.Ping(ctx).Err(); err != nil {
		_ = cm.client.Close()
		cm.client = nil
		cm.backend = "unavailable"
	}
	return cm
}

func mergeCacheConfig(cfg config.CacheConfig) config.CacheConfig {
	defaults := config.DefaultCacheConfig()
	if strings.TrimSpace(cfg.Type) == "" {
		cfg.Type = defaults.Type
	}
	if strings.TrimSpace(cfg.Host) == "" {
		cfg.Host = defaults.Host
	}
	if cfg.Port <= 0 {
		cfg.Port = defaults.Port
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = defaults.PoolSize
	}
	if cfg.MinIdle <= 0 {
		cfg.MinIdle = defaults.MinIdle
	}
	if strings.TrimSpace(cfg.Timeout) == "" {
		cfg.Timeout = defaults.Timeout
	}
	if strings.TrimSpace(cfg.Prefix) == "" {
		cfg.Prefix = defaults.Prefix
	}
	if strings.TrimSpace(cfg.TTL) == "" {
		cfg.TTL = defaults.TTL
	}
	if cfg.ClusterNodes == nil {
		cfg.ClusterNodes = defaults.ClusterNodes
	}
	return cfg
}

func cacheURLFromEnvironment(cfg config.CacheConfig) config.CacheConfig {
	if strings.TrimSpace(cfg.URL) != "" {
		return cfg
	}
	if envURL := strings.TrimSpace(os.Getenv("CACHE_URL")); envURL != "" {
		cfg.URL = envURL
		if strings.TrimSpace(cfg.Type) == "" || strings.EqualFold(strings.TrimSpace(cfg.Type), "memory") {
			cfg.Type = "valkey"
		}
	}
	return cfg
}

func configuredTimeout(cfg config.CacheConfig) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(cfg.Timeout))
	if err != nil || d <= 0 {
		return 5 * time.Second
	}
	return d
}

func redisOptions(cfg config.CacheConfig) (*redis.UniversalOptions, error) {
	var opts *redis.Options
	if strings.TrimSpace(cfg.URL) != "" {
		u, err := url.Parse(strings.TrimSpace(cfg.URL))
		if err != nil {
			return nil, fmt.Errorf("parse cache URL: %w", err)
		}
		if u.Scheme != "redis" && u.Scheme != "rediss" && u.Scheme != "valkey" {
			return nil, fmt.Errorf("unsupported cache URL scheme %q", u.Scheme)
		}
		normalizedURL := strings.TrimSpace(cfg.URL)
		if u.Scheme == "valkey" {
			normalizedURL = "redis:" + strings.TrimPrefix(normalizedURL, "valkey:")
		}
		parsed, err := redis.ParseURL(normalizedURL)
		if err != nil {
			return nil, fmt.Errorf("parse cache URL: %w", err)
		}
		opts = parsed
	} else {
		host := strings.TrimSpace(cfg.Host)
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port <= 0 || port > 65535 {
			port = 6379
		}
		opts = &redis.Options{Addr: net.JoinHostPort(host, strconv.Itoa(port)), Username: cfg.Username, Password: cfg.Password, DB: max(cfg.DB, 0)}
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.MinIdle > 0 {
		opts.MinIdleConns = cfg.MinIdle
	}
	connectionTimeout := configuredTimeout(cfg)
	opts.DialTimeout = connectionTimeout
	opts.ReadTimeout = connectionTimeout
	opts.WriteTimeout = connectionTimeout
	if cfg.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.TLSSkipVerify} //nolint:gosec // Explicit PART 12 option.
	}
	if opts.TLSConfig != nil {
		opts.TLSConfig.InsecureSkipVerify = cfg.TLSSkipVerify //nolint:gosec // Explicit PART 12 option.
	}
	if cfg.Cluster {
		if len(cfg.ClusterNodes) == 0 {
			return nil, errors.New("cache cluster requires at least one cluster node")
		}
		return &redis.UniversalOptions{Addrs: cfg.ClusterNodes, Username: opts.Username, Password: opts.Password, DB: opts.DB, PoolSize: opts.PoolSize, MinIdleConns: opts.MinIdleConns, DialTimeout: opts.DialTimeout, ReadTimeout: opts.ReadTimeout, WriteTimeout: opts.WriteTimeout, TLSConfig: opts.TLSConfig}, nil
	}
	return &redis.UniversalOptions{Addrs: []string{opts.Addr}, Username: opts.Username, Password: opts.Password, DB: opts.DB, PoolSize: opts.PoolSize, MinIdleConns: opts.MinIdleConns, DialTimeout: opts.DialTimeout, ReadTimeout: opts.ReadTimeout, WriteTimeout: opts.WriteTimeout, TLSConfig: opts.TLSConfig}, nil
}

func (cm *CacheManager) IsEnabled() bool {
	return cm != nil && (cm.backend == "memory" || ((cm.backend == "redis" || cm.backend == "valkey") && cm.client != nil))
}

// BackendType reports the active backend, including "none" or "unavailable".
func (cm *CacheManager) BackendType() string {
	if cm == nil {
		return "unavailable"
	}
	return cm.backend
}

func (cm *CacheManager) cacheKey(key string) string { return cm.prefix + key }

func (cm *CacheManager) GetCachedValue(key string) (string, error) {
	if !cm.IsEnabled() {
		return "", errors.New("cache not enabled")
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, time.Second)
		defer cancel()
		return cm.client.Get(ctx, cm.cacheKey(key)).Result()
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	entry, ok := cm.entries[cm.cacheKey(key)]
	if !ok {
		return "", redis.Nil
	}
	if !entry.expiresAt.IsZero() && !time.Now().Before(entry.expiresAt) {
		delete(cm.entries, cm.cacheKey(key))
		return "", redis.Nil
	}
	return entry.value, nil
}

func (cm *CacheManager) SetCachedValue(key, value string, ttl time.Duration) error {
	if !cm.IsEnabled() {
		return nil
	}
	if ttl <= 0 {
		ttl = cm.ttl
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, time.Second)
		defer cancel()
		return cm.client.Set(ctx, cm.cacheKey(key), value, ttl).Err()
	}
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	cm.mu.Lock()
	cm.entries[cm.cacheKey(key)] = memoryCacheEntry{value: value, expiresAt: expiresAt}
	cm.mu.Unlock()
	return nil
}

func (cm *CacheManager) DeleteCachedValue(key string) error {
	if !cm.IsEnabled() {
		return nil
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, time.Second)
		defer cancel()
		return cm.client.Del(ctx, cm.cacheKey(key)).Err()
	}
	cm.mu.Lock()
	delete(cm.entries, cm.cacheKey(key))
	cm.mu.Unlock()
	return nil
}

func (cm *CacheManager) DeletePattern(pattern string) error {
	if !cm.IsEnabled() {
		return nil
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, 5*time.Second)
		defer cancel()
		iter := cm.client.Scan(ctx, 0, cm.cacheKey(pattern), 0).Iterator()
		for iter.Next(ctx) {
			if err := cm.client.Del(ctx, iter.Val()).Err(); err != nil {
				return err
			}
		}
		return iter.Err()
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for key := range cm.entries {
		match, err := path.Match(cm.cacheKey(pattern), key)
		if err != nil {
			return err
		}
		if match {
			delete(cm.entries, key)
		}
	}
	return nil
}

func (cm *CacheManager) Exists(key string) (bool, error) {
	if !cm.IsEnabled() {
		return false, nil
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, time.Second)
		defer cancel()
		n, err := cm.client.Exists(ctx, cm.cacheKey(key)).Result()
		return n > 0, err
	}
	_, err := cm.GetCachedValue(key)
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	return err == nil, err
}

func (cm *CacheManager) TTL(key string) (time.Duration, error) {
	if !cm.IsEnabled() {
		return 0, errors.New("cache not enabled")
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, time.Second)
		defer cancel()
		return cm.client.TTL(ctx, cm.cacheKey(key)).Result()
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	entry, ok := cm.entries[cm.cacheKey(key)]
	if !ok {
		return -2 * time.Second, nil
	}
	if entry.expiresAt.IsZero() {
		return -time.Second, nil
	}
	remaining := time.Until(entry.expiresAt)
	if remaining <= 0 {
		delete(cm.entries, cm.cacheKey(key))
		return -2 * time.Second, nil
	}
	return remaining, nil
}

func (cm *CacheManager) Increment(key string) (int64, error) {
	if !cm.IsEnabled() {
		return 0, errors.New("cache not enabled")
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, time.Second)
		defer cancel()
		value, err := cm.client.Incr(ctx, cm.cacheKey(key)).Result()
		if err == nil && value == 1 {
			err = cm.client.Expire(ctx, cm.cacheKey(key), cm.ttl).Err()
		}
		return value, err
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cacheKey := cm.cacheKey(key)
	entry := cm.entries[cacheKey]
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		entry = memoryCacheEntry{}
	}
	var value int64
	if entry.value != "" {
		parsed, err := strconv.ParseInt(entry.value, 10, 64)
		if err != nil {
			return 0, errors.New("cache value is not an integer")
		}
		value = parsed
	}
	value++
	if entry.expiresAt.IsZero() {
		entry.expiresAt = time.Now().Add(cm.ttl)
	}
	entry.value = strconv.FormatInt(value, 10)
	cm.entries[cacheKey] = entry
	return value, nil
}

func (cm *CacheManager) Expire(key string, ttl time.Duration) error {
	if !cm.IsEnabled() {
		return nil
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, time.Second)
		defer cancel()
		return cm.client.Expire(ctx, cm.cacheKey(key), ttl).Err()
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cacheKey := cm.cacheKey(key)
	entry, ok := cm.entries[cacheKey]
	if !ok {
		return nil
	}
	if ttl <= 0 {
		delete(cm.entries, cacheKey)
		return nil
	}
	entry.expiresAt = time.Now().Add(ttl)
	cm.entries[cacheKey] = entry
	return nil
}

func (cm *CacheManager) Flush() error {
	if !cm.IsEnabled() {
		return nil
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, 5*time.Second)
		defer cancel()
		if cm.prefix != "" {
			return cm.DeletePattern("*")
		}
		return cm.client.FlushDB(ctx).Err()
	}
	cm.mu.Lock()
	clear(cm.entries)
	cm.mu.Unlock()
	return nil
}

func (cm *CacheManager) GetStats() (map[string]string, error) {
	if !cm.IsEnabled() {
		return map[string]string{"status": "disabled"}, nil
	}
	if cm.client != nil {
		ctx, cancel := context.WithTimeout(cm.ctx, 2*time.Second)
		defer cancel()
		info, err := cm.client.Info(ctx, "stats").Result()
		if err != nil {
			return nil, err
		}
		return map[string]string{"status": "enabled", "type": cm.backend, "info": info}, nil
	}
	cm.mu.Lock()
	cm.deleteExpiredLocked(time.Now())
	size := len(cm.entries)
	cm.mu.Unlock()
	return map[string]string{"status": "enabled", "type": "memory", "entries": strconv.Itoa(size)}, nil
}

func (cm *CacheManager) deleteExpiredLocked(now time.Time) {
	for key, entry := range cm.entries {
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
			delete(cm.entries, key)
		}
	}
}

func (cm *CacheManager) Close() error {
	if cm == nil {
		return nil
	}
	if cm.client != nil {
		return cm.client.Close()
	}
	return nil
}

func (cm *CacheManager) IsCacheHealthy() bool {
	if cm == nil {
		return false
	}
	if cm.backend == "memory" || cm.backend == "none" {
		return true
	}
	return cm.Ping() == nil
}

func (cm *CacheManager) Ping() error {
	if !cm.IsEnabled() {
		return errors.New("cache not enabled")
	}
	if cm.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(cm.ctx, 2*time.Second)
	defer cancel()
	return cm.client.Ping(ctx).Err()
}
