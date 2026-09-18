package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheManager handles optional Redis/Valkey caching
type CacheManager struct {
	client  *redis.Client
	enabled bool
	ctx     context.Context
}

// NewCacheManager creates a new cache manager
// Cache is optional - if connection fails, it gracefully disables caching
func NewCacheManager() *CacheManager {
	ctx := context.Background()

	// Check if caching is enabled
	cacheEnabled := os.Getenv("CACHE_ENABLED")
	if cacheEnabled != "true" && cacheEnabled != "1" {
		return &CacheManager{
			enabled: false,
			ctx:     ctx,
		}
	}

	// Get cache configuration
	host := os.Getenv("CACHE_HOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("CACHE_PORT")
	if port == "" {
		port = "6379"
	}

	password := os.Getenv("CACHE_PASSWORD")

	dbNum := 0
	if dbStr := os.Getenv("CACHE_DB"); dbStr != "" {
		if num, err := strconv.Atoi(dbStr); err == nil {
			dbNum = num
		}
	}

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", host, port),
		Password:     password,
		DB:           dbNum,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	// Test connection (non-blocking)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// Cache unavailable - disable it gracefully
		client.Close()
		return &CacheManager{
			enabled: false,
			ctx:     context.Background(),
		}
	}

	return &CacheManager{
		client:  client,
		enabled: true,
		ctx:     context.Background(),
	}
}

// IsEnabled returns whether caching is active
func (cm *CacheManager) IsEnabled() bool {
	return cm.enabled
}

// GetCachedValue retrieves a value from cache
func (cm *CacheManager) GetCachedValue(key string) (string, error) {
	if !cm.enabled {
		return "", fmt.Errorf("cache not enabled")
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 1*time.Second)
	defer cancel()

	return cm.client.Get(ctx, key).Result()
}

// SetCachedValue stores a value in cache with TTL
func (cm *CacheManager) SetCachedValue(key string, value string, ttl time.Duration) error {
	if !cm.enabled {
		// Silently succeed if cache disabled
		return nil
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 1*time.Second)
	defer cancel()

	return cm.client.Set(ctx, key, value, ttl).Err()
}

// DeleteCachedValue removes a key from cache
func (cm *CacheManager) DeleteCachedValue(key string) error {
	if !cm.enabled {
		// Silently succeed if cache disabled
		return nil
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 1*time.Second)
	defer cancel()

	return cm.client.Del(ctx, key).Err()
}

// DeletePattern removes all keys matching a pattern
func (cm *CacheManager) DeletePattern(pattern string) error {
	if !cm.enabled {
		// Silently succeed if cache disabled
		return nil
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 5*time.Second)
	defer cancel()

	// Scan and delete matching keys
	iter := cm.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := cm.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Exists checks if a key exists in cache
func (cm *CacheManager) Exists(key string) (bool, error) {
	if !cm.enabled {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 1*time.Second)
	defer cancel()

	result, err := cm.client.Exists(ctx, key).Result()
	return result > 0, err
}

// TTL returns the remaining time to live of a key
func (cm *CacheManager) TTL(key string) (time.Duration, error) {
	if !cm.enabled {
		return 0, fmt.Errorf("cache not enabled")
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 1*time.Second)
	defer cancel()

	return cm.client.TTL(ctx, key).Result()
}

// Increment increments a numeric value
func (cm *CacheManager) Increment(key string) (int64, error) {
	if !cm.enabled {
		return 0, fmt.Errorf("cache not enabled")
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 1*time.Second)
	defer cancel()

	return cm.client.Incr(ctx, key).Result()
}

// Expire sets a timeout on a key
func (cm *CacheManager) Expire(key string, ttl time.Duration) error {
	if !cm.enabled {
		return nil
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 1*time.Second)
	defer cancel()

	return cm.client.Expire(ctx, key, ttl).Err()
}

// Flush clears all cache entries (use with caution!)
func (cm *CacheManager) Flush() error {
	if !cm.enabled {
		return nil
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 5*time.Second)
	defer cancel()

	return cm.client.FlushDB(ctx).Err()
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats() (map[string]string, error) {
	if !cm.enabled {
		return map[string]string{
			"status": "disabled",
		}, nil
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 2*time.Second)
	defer cancel()

	info, err := cm.client.Info(ctx, "stats").Result()
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"status": "enabled",
		"info":   info,
	}, nil
}

// Close closes the cache connection
func (cm *CacheManager) Close() error {
	if cm.enabled && cm.client != nil {
		return cm.client.Close()
	}
	return nil
}

// IsCacheHealthy reports whether the cache backend is reachable. A disabled
// cache means the built-in memory cache is in use, which is always available.
func (cm *CacheManager) IsCacheHealthy() bool {
	if !cm.enabled {
		return true
	}
	return cm.Ping() == nil
}

// Ping tests the cache connection
func (cm *CacheManager) Ping() error {
	if !cm.enabled {
		return fmt.Errorf("cache not enabled")
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 2*time.Second)
	defer cancel()

	return cm.client.Ping(ctx).Err()
}
