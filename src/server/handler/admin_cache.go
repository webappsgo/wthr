package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
)

type cacheConfigRequest struct {
	Type          string `json:"type"`
	URL           string `json:"url"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	DB            int    `json:"db"`
	TLS           bool   `json:"tls"`
	TLSSkipVerify bool   `json:"tls_skip_verify"`
	PoolSize      int    `json:"pool_size"`
	MinIdle       int    `json:"min_idle"`
	Timeout       string `json:"timeout"`
	Prefix        string `json:"prefix"`
	TTL           string `json:"ttl"`
	Cluster       bool   `json:"cluster"`
	ClusterNodes  string `json:"cluster_nodes"`
	ClearURL      bool   `json:"clear_url"`
	ClearUsername bool   `json:"clear_username"`
	ClearPassword bool   `json:"clear_password"`
}

func cacheConfigView(c config.CacheConfig) map[string]interface{} {
	return map[string]interface{}{
		"type":                c.Type,
		"url_configured":      strings.TrimSpace(c.URL) != "",
		"host":                c.Host,
		"port":                c.Port,
		"username_configured": strings.TrimSpace(c.Username) != "",
		"password_configured": c.Password != "",
		"db":                  c.DB,
		"tls":                 c.TLS,
		"tls_skip_verify":     c.TLSSkipVerify,
		"pool_size":           c.PoolSize,
		"min_idle":            c.MinIdle,
		"timeout":             c.Timeout,
		"prefix":              c.Prefix,
		"ttl":                 c.TTL,
		"cluster":             c.Cluster,
		"cluster_nodes":       strings.Join(c.ClusterNodes, ", "),
	}
}

// UpdateCacheConfig validates and persists the PART 12 cache settings in
// server.yml. Credential fields are write-only; empty values preserve secrets.
func UpdateCacheConfig(w http.ResponseWriter, r *http.Request) {
	var req cacheConfigRequest
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			BadRequest(w, r, "Invalid cache configuration")
			return
		}
	} else if err := decodeCacheConfigForm(r, &req); err != nil {
		BadRequest(w, r, "Invalid cache configuration")
		return
	}

	cfg := config.GetGlobalConfig()
	if cfg == nil {
		var err error
		cfg, err = config.LoadConfig()
		if err != nil {
			InternalError(w, r, "Configuration is unavailable")
			return
		}
	}
	cache, err := applyCacheConfigRequest(cfg.Server.Cache, req)
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	clusterEnabled := false
	if serverDB := database.GetServerDB(); serverDB != nil {
		var enabled string
		err := database.QueryRowContext(r.Context(), serverDB, database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'cluster.enabled'").Scan(&enabled)
		if err == nil {
			clusterEnabled = strings.EqualFold(strings.TrimSpace(enabled), "true")
		} else if err != sql.ErrNoRows {
			InternalError(w, r, "Cannot verify cluster cache requirements")
			return
		}
	} else if strings.TrimSpace(cfg.Server.Database.Driver) != "" && !strings.EqualFold(cfg.Server.Database.Driver, "file") && !strings.EqualFold(cfg.Server.Database.Driver, "sqlite") {
		InternalError(w, r, "Cannot verify remote database cache requirements")
		return
	}
	if err := validateCacheBackendForMode(cfg, clusterEnabled, cache.Type); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	updatedConfig := *cfg
	updatedConfig.Server.Cache = cache
	if err := config.SaveConfig(&updatedConfig); err != nil {
		InternalError(w, r, "Failed to save cache configuration")
		return
	}
	config.SetGlobalConfig(&updatedConfig)
	if r.Method == http.MethodPost {
		settingsPath := adminPagePath(r, "/settings")
		http.Redirect(w, r, settingsPath, http.StatusSeeOther)
		return
	}
	RespondSuccess(w, r, "Cache settings saved. Restart the server to apply the backend change.", map[string]interface{}{
		"requires_restart": true,
		"cache":            cacheConfigView(cache),
	})
}

func validateCacheBackendForMode(cfg *config.AppConfig, clusterEnabled bool, backend string) error {
	if config.RequiresSharedCache(cfg, clusterEnabled) && backend != "redis" && backend != "valkey" {
		return errors.New("cluster and remote database modes require a Redis or Valkey cache backend")
	}
	return nil
}

func decodeCacheConfigForm(r *http.Request, req *cacheConfigRequest) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	form := r.Form
	intField := func(key string, target *int) error {
		value, err := strconv.Atoi(form.Get(key))
		if err != nil {
			return err
		}
		*target = value
		return nil
	}
	var err error
	for key, target := range map[string]*int{"port": &req.Port, "db": &req.DB, "pool_size": &req.PoolSize, "min_idle": &req.MinIdle} {
		if err = intField(key, target); err != nil {
			return err
		}
	}
	req.Type, req.URL, req.Host = form.Get("type"), form.Get("url"), form.Get("host")
	req.Username, req.Password = form.Get("username"), form.Get("password")
	req.Timeout, req.Prefix, req.TTL = form.Get("timeout"), form.Get("prefix"), form.Get("ttl")
	req.ClusterNodes = form.Get("cluster_nodes")
	req.TLS, req.TLSSkipVerify = form.Get("tls") == "on", form.Get("tls_skip_verify") == "on"
	req.Cluster = form.Get("cluster") == "on"
	req.ClearURL, req.ClearUsername, req.ClearPassword = form.Get("clear_url") == "on", form.Get("clear_username") == "on", form.Get("clear_password") == "on"
	return nil
}

func applyCacheConfigRequest(current config.CacheConfig, req cacheConfigRequest) (config.CacheConfig, error) {
	c := current
	c.Type = strings.ToLower(strings.TrimSpace(req.Type))
	switch c.Type {
	case "none", "memory", "valkey", "redis":
	default:
		return current, errors.New("cache type must be none, memory, valkey, or redis")
	}
	if req.Port < 1 || req.Port > 65535 || req.PoolSize < 1 || req.MinIdle < 0 || req.MinIdle > req.PoolSize || req.DB < 0 {
		return current, errors.New("cache port, pool, idle connection, or database setting is invalid")
	}
	timeout, err := time.ParseDuration(strings.TrimSpace(req.Timeout))
	if err != nil {
		return current, errors.New("cache timeout must be a valid duration")
	}
	if timeout <= 0 {
		return current, errors.New("cache timeout must be positive")
	}
	ttl, err := time.ParseDuration(strings.TrimSpace(req.TTL))
	if err != nil {
		return current, errors.New("cache TTL must be a valid duration")
	}
	if ttl <= 0 {
		return current, errors.New("cache TTL must be positive")
	}
	c.Host = strings.TrimSpace(req.Host)
	if c.Host == "" {
		c.Host = "localhost"
	}
	c.Port = req.Port
	c.DB = req.DB
	c.TLS = req.TLS
	c.TLSSkipVerify = req.TLSSkipVerify
	c.PoolSize = req.PoolSize
	c.MinIdle = req.MinIdle
	c.Timeout = strings.TrimSpace(req.Timeout)
	c.Prefix = strings.TrimSpace(req.Prefix)
	c.TTL = strings.TrimSpace(req.TTL)
	c.Cluster = req.Cluster
	c.ClusterNodes = make([]string, 0)
	for _, node := range strings.Split(req.ClusterNodes, ",") {
		node = strings.TrimSpace(node)
		if node == "" {
			continue
		}
		host, port, err := net.SplitHostPort(node)
		portNumber, portErr := strconv.Atoi(port)
		if err != nil || strings.TrimSpace(host) == "" || portErr != nil || portNumber < 1 || portNumber > 65535 {
			return current, errors.New("cache cluster nodes must be host:port addresses")
		}
		c.ClusterNodes = append(c.ClusterNodes, node)
	}
	if c.Cluster && ((c.Type != "valkey" && c.Type != "redis") || len(c.ClusterNodes) == 0) {
		return current, errors.New("cache cluster requires Redis or Valkey and at least one cluster node")
	}
	if req.ClearURL {
		c.URL = ""
	} else if strings.TrimSpace(req.URL) != "" {
		u, err := url.Parse(strings.TrimSpace(req.URL))
		if err != nil || u.Hostname() == "" || (u.Scheme != "redis" && u.Scheme != "rediss" && u.Scheme != "valkey") {
			return current, errors.New("cache URL must use redis, rediss, or valkey and include a host")
		}
		c.URL = strings.TrimSpace(req.URL)
	}
	if req.ClearUsername {
		c.Username = ""
	} else if req.Username != "" {
		c.Username = req.Username
	}
	if req.ClearPassword {
		c.Password = ""
	} else if req.Password != "" {
		c.Password = req.Password
	}
	return c, nil
}
