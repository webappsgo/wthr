package handler

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/cluster"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// ShowClusterAddPage renders the one-time join token form for an existing node.
func ShowClusterAddPage(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.CurrentConfig()
	remoteDatabase := database.RemoteDatabaseConfigured()
	sharedCache := cfg != nil && (strings.EqualFold(cfg.Server.Cache.Type, "valkey") || strings.EqualFold(cfg.Server.Cache.Type, "redis"))
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_cluster_add.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title":            "Add Cluster Node - Admin",
		"page":             "server-cluster-add",
		"cluster_enabled":  cfg != nil && cfg.Server.Cluster.Enabled,
		"remote_database":  remoteDatabase,
		"shared_cache":     sharedCache,
		"needs_conversion": !remoteDatabase || !sharedCache,
	}))
}

// EnableClusterMode validates shared endpoints, migrates the local databases,
// prepares cluster metadata and issues the first one-time join token.
func EnableClusterMode(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if err := r.ParseForm(); err != nil {
		renderClusterAddError(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}
	databaseURL := strings.TrimSpace(r.FormValue("database_url"))
	cacheURL := strings.TrimSpace(r.FormValue("cache_url"))
	existingRemote := database.RemoteDatabaseConfigured()
	dbParts, err := database.ParseConnectionString(databaseURL)
	if databaseURL != "" || !existingRemote {
		if err != nil || (dbParts.Type != "postgres" && dbParts.Type != "mysql") {
			renderClusterAddError(w, r, http.StatusBadRequest, "Enter a valid PostgreSQL or MySQL connection URL.")
			return
		}
	}
	cacheParsed, err := url.Parse(cacheURL)
	if err != nil || cacheParsed.Hostname() == "" || (cacheParsed.Scheme != "redis" && cacheParsed.Scheme != "rediss" && cacheParsed.Scheme != "valkey") {
		renderClusterAddError(w, r, http.StatusBadRequest, "Enter a valid Valkey or Redis connection URL.")
		return
	}
	keys := []string{"server.database.driver", "server.database.url", "server.cache.type", "server.cache.url", "server.cluster.enabled", "server.cluster.id"}
	saved := make(map[string]settingSnapshot, len(keys))
	for _, key := range keys {
		value, exists, readErr := config.ReadSetting(key)
		if readErr != nil {
			renderClusterAddError(w, r, http.StatusInternalServerError, "Could not read current configuration.")
			return
		}
		saved[key] = settingSnapshot{value: value, exists: exists}
	}
	restore := func() {
		for _, key := range keys {
			_ = config.DeleteSyncedSetting(key)
			snapshot := saved[key]
			if snapshot.exists {
				_ = config.WriteSetting(key, snapshot.value)
			}
		}
	}
	settings := []struct {
		key   string
		value any
	}{{"server.cache.type", cacheTypeForURL(cacheParsed.Scheme)}, {"server.cache.url", cacheURL}, {"server.cluster.enabled", true}}
	if databaseURL != "" {
		settings = append(settings, struct {
			key   string
			value any
		}{"server.database.driver", dbParts.Type}, struct {
			key   string
			value any
		}{"server.database.url", databaseURL})
	}
	for _, setting := range settings {
		if err := config.WriteSetting(setting.key, setting.value); err != nil {
			restore()
			renderClusterAddError(w, r, http.StatusInternalServerError, "Could not save cluster configuration.")
			return
		}
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		restore()
		renderClusterAddError(w, r, http.StatusBadRequest, "Cluster configuration is invalid.")
		return
	}
	cache := service.NewCacheManager(cfg.Server.Cache)
	if !cache.IsEnabled() {
		_ = cache.Close()
		restore()
		renderClusterAddError(w, r, http.StatusServiceUnavailable, "Could not connect to the shared cache.")
		return
	}
	_ = cache.Close()
	paths, err := util.GetDirectoryPaths()
	if err != nil {
		restore()
		renderClusterAddError(w, r, http.StatusInternalServerError, "Could not resolve application directories.")
		return
	}
	remoteDB := database.GetServerDB()
	var temporaryRemote *database.DualDB
	if !existingRemote {
		temporaryRemote, err = database.InitDualDB(paths.Data)
		if err != nil {
			restore()
			renderClusterAddError(w, r, http.StatusServiceUnavailable, "Could not connect to the shared database or migrate local data.")
			return
		}
		remoteDB = temporaryRemote.Server
	}
	nodeID, _ := os.Hostname()
	if err := cluster.PrepareClusterDatabase(remoteDB, nodeID, ""); err != nil {
		if temporaryRemote != nil {
			_ = temporaryRemote.Close()
		}
		restore()
		renderClusterAddError(w, r, http.StatusInternalServerError, "Could not prepare cluster metadata.")
		return
	}
	token, expires, err := cluster.CreateJoinToken(r.Context(), remoteDB, AdminUsername(r))
	if temporaryRemote != nil {
		_ = temporaryRemote.Close()
	}
	if err != nil {
		restore()
		renderClusterAddError(w, r, http.StatusInternalServerError, "Could not create the join token.")
		return
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_cluster_add.tmpl", AdminTemplateData(r, map[string]interface{}{"title": "Add Cluster Node - Admin", "page": "server-cluster-add", "cluster_enabled": true, "remote_database": true, "join_token": token, "expires_at": expires.UTC().Format(time.RFC3339), "restart_required": true}))
}

type settingSnapshot struct {
	value  any
	exists bool
}

func cacheTypeForURL(scheme string) string {
	if scheme == "valkey" {
		return "valkey"
	}
	return "redis"
}

func renderClusterAddError(w http.ResponseWriter, r *http.Request, status int, message string) {
	cfg, _ := config.CurrentConfig()
	remoteDatabase := database.RemoteDatabaseConfigured()
	sharedCache := cfg != nil && (strings.EqualFold(cfg.Server.Cache.Type, "valkey") || strings.EqualFold(cfg.Server.Cache.Type, "redis"))
	middleware.RenderHTML(w, r, status, "admin/admin_cluster_add.tmpl", AdminTemplateData(r, map[string]interface{}{"title": "Add Cluster Node - Admin", "page": "server-cluster-add", "error": message, "cluster_enabled": cfg != nil && cfg.Server.Cluster.Enabled, "remote_database": remoteDatabase, "shared_cache": sharedCache, "needs_conversion": !remoteDatabase || !sharedCache}))
}

// ShowClusterRemovePage confirms removal of this node only.
func ShowClusterRemovePage(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_cluster_remove.tmpl", AdminTemplateData(r, map[string]interface{}{"title": "Remove This Node - Admin", "page": "server-cluster-remove"}))
}

// LeaveClusterForm removes this node from shared membership and configures its next startup as standalone.
func LeaveClusterForm(w http.ResponseWriter, r *http.Request) {
	paths, err := util.GetDirectoryPaths()
	if err == nil {
		err = cluster.LeaveCluster(r.Context(), database.GetServerDB(), paths.Data, paths.Config, hostnameOrEmpty())
	}
	if err != nil {
		renderClusterAddError(w, r, http.StatusInternalServerError, "Could not remove this node from the cluster.")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func LeaveClusterAPI(w http.ResponseWriter, r *http.Request) {
	paths, err := util.GetDirectoryPaths()
	if err == nil {
		err = cluster.LeaveCluster(r.Context(), database.GetServerDB(), paths.Data, paths.Config, hostnameOrEmpty())
	}
	if err != nil {
		RespondError(w, r, http.StatusConflict, "CLUSTER_LEAVE_FAILED", "This node could not leave the cluster")
		return
	}
	RespondSuccess(w, r, "This node was removed from the cluster; restart to create fresh local databases", map[string]any{"restart_required": true})
}

func hostnameOrEmpty() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// GenerateClusterJoinTokenForm creates a one-time token and displays it once.
func GenerateClusterJoinTokenForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "admin/admin_cluster_add.tmpl", AdminTemplateData(r, map[string]interface{}{"title": "Add Cluster Node - Admin", "page": "server-cluster-add", "error": "Invalid form data"}))
		return
	}
	createClusterJoinTokenPage(w, r)
}

func createClusterJoinTokenPage(w http.ResponseWriter, r *http.Request) {
	db := database.GetServerDB()
	token, expires, err := cluster.CreateJoinToken(r.Context(), db, AdminUsername(r))
	data := map[string]interface{}{"title": "Add Cluster Node - Admin", "page": "server-cluster-add"}
	if err != nil {
		data["error"] = "A shared PostgreSQL or MySQL database and Valkey or Redis cache are required before a node can join."
		middleware.RenderHTML(w, r, http.StatusServiceUnavailable, "admin/admin_cluster_add.tmpl", AdminTemplateData(r, data))
		return
	}
	data["join_token"] = token
	data["expires_at"] = expires.UTC().Format(time.RFC3339)
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_cluster_add.tmpl", AdminTemplateData(r, data))
}

// GenerateClusterJoinTokenAPI issues a one-time token to an authenticated admin.
func GenerateClusterJoinTokenAPI(w http.ResponseWriter, r *http.Request) {
	token, expires, err := cluster.CreateJoinToken(r.Context(), database.GetServerDB(), AdminUsername(r))
	if err != nil {
		RespondError(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Cluster joining is unavailable")
		return
	}
	RespondSuccess(w, r, "Cluster join token created", map[string]interface{}{"token": token, "expires_at": expires.UTC().Format(time.RFC3339)})
}
