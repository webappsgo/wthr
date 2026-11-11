package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestDecodeCacheConfigForm(t *testing.T) {
	r := httptest.NewRequest("POST", "/server/administration/config/settings/cache", strings.NewReader("type=valkey&port=6379&db=0&pool_size=10&min_idle=2&timeout=5s&prefix=wthr%3A&ttl=1h&cluster=on&cluster_nodes=node1%3A6379&clear_password=on"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var got cacheConfigRequest
	if err := decodeCacheConfigForm(r, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "valkey" || got.Port != 6379 || got.PoolSize != 10 || !got.Cluster || got.ClusterNodes != "node1:6379" || !got.ClearPassword {
		t.Fatalf("decoded form = %+v", got)
	}
}

func TestValidateCacheBackendForMode(t *testing.T) {
	tests := []struct {
		name    string
		driver  string
		cluster bool
		backend string
		wantErr bool
	}{
		{name: "single instance memory", driver: "file", backend: "memory"},
		{name: "cluster rejects memory", driver: "file", cluster: true, backend: "memory", wantErr: true},
		{name: "cluster accepts valkey", driver: "file", cluster: true, backend: "valkey"},
		{name: "remote database rejects none", driver: "postgres", backend: "none", wantErr: true},
		{name: "remote database accepts redis", driver: "mysql", backend: "redis"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &config.AppConfig{}
			cfg.Server.Database.Driver = test.driver
			err := validateCacheBackendForMode(cfg, test.cluster, test.backend)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCacheBackendForMode() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestApplyCacheConfigRequestPreservesSecretsAndValidates(t *testing.T) {
	current := config.DefaultCacheConfig()
	current.URL = "valkey://old-user:old-password@cache.example:6379/0"
	current.Username = "old-user"
	current.Password = "old-password"
	req := cacheConfigRequest{
		Type:          "VALKEY",
		Host:          "cache.internal",
		Port:          6379,
		DB:            2,
		PoolSize:      10,
		MinIdle:       2,
		Timeout:       "5s",
		TTL:           "1h",
		Prefix:        "wthr:",
		ClusterNodes:  "node1:6379, node2:6380",
		Cluster:       true,
		TLS:           true,
		TLSSkipVerify: false,
	}
	got, err := applyCacheConfigRequest(current, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "valkey" || got.URL != current.URL || got.Username != current.Username || got.Password != current.Password {
		t.Fatalf("backend or credentials were not preserved: %#v", got)
	}
	if len(got.ClusterNodes) != 2 || got.ClusterNodes[1] != "node2:6380" || !got.Cluster || !got.TLS {
		t.Fatalf("cache connection settings not applied: %#v", got)
	}

	req.ClearURL = true
	req.ClearUsername = true
	req.ClearPassword = true
	got, err = applyCacheConfigRequest(current, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "" || got.Username != "" || got.Password != "" {
		t.Fatalf("explicit secret clearing failed: %#v", got)
	}

	invalid := []cacheConfigRequest{
		{Type: "memory", Port: 6379, PoolSize: 10, MinIdle: 2, Timeout: "5s", TTL: "1h", URL: "http://cache.example"},
		{Type: "memory", Port: 6379, PoolSize: 10, MinIdle: 2, Timeout: "0s", TTL: "1h"},
		{Type: "memory", Port: 6379, PoolSize: 10, MinIdle: 2, Timeout: "5s", TTL: "1h", Cluster: true},
	}
	for i, test := range invalid {
		if _, err := applyCacheConfigRequest(current, test); err == nil {
			t.Errorf("invalid config case %d returned no error", i)
		}
	}
}

func TestCacheConfigViewNeverReturnsCredentials(t *testing.T) {
	cfg := config.DefaultCacheConfig()
	cfg.URL = "valkey://alice:sensitive-password@cache.example:6379/1"
	cfg.Username = "alice"
	cfg.Password = "sensitive-password"
	view := cacheConfigView(cfg)
	for key := range view {
		if strings.Contains(strings.ToLower(key), "password") || strings.Contains(strings.ToLower(key), "username") || strings.Contains(strings.ToLower(key), "url") {
			if key == "url_configured" || key == "username_configured" || key == "password_configured" {
				continue
			}
			t.Fatalf("cache settings view exposes sensitive field %q", key)
		}
	}
	if view["url_configured"] != true || view["username_configured"] != true || view["password_configured"] != true {
		t.Fatalf("cache settings view does not report configured secrets: %#v", view)
	}
}
