package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/mcp"
	"github.com/webappsgo/wthr/src/server/handler"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
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

func TestPIDFileEnabled(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{name: "unset defaults enabled", want: true},
		{name: "boolean enabled", value: true, want: true},
		{name: "boolean disabled", value: false, want: false},
		{name: "string enabled", value: "yes", want: true},
		{name: "string disabled", value: "no", want: false},
		{name: "invalid string defaults enabled", value: "unknown", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pidFileEnabled(test.value); got != test.want {
				t.Errorf("pidFileEnabled(%v) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestServeLocaleFallbackAndETag(t *testing.T) {
	request := func(file, etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/locales/"+file, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("file", file)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		if etag != "" {
			r.Header.Set("If-None-Match", etag)
		}
		response := httptest.NewRecorder()
		serveLocale(response, r)
		return response
	}

	english := request("en.json", "")
	if english.Code != http.StatusOK || english.Header().Get("Content-Type") != "application/json; charset=utf-8" || english.Body.Len() == 0 {
		t.Fatalf("English locale response = %d %q %q", english.Code, english.Header().Get("Content-Type"), english.Body.String())
	}
	if english.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("locale Cache-Control = %q, want no-cache", english.Header().Get("Cache-Control"))
	}
	fallback := request("../invalid.json", "")
	if fallback.Code != http.StatusOK || fallback.Body.String() != english.Body.String() {
		t.Fatalf("invalid locale fallback = status %d, body differs from English", fallback.Code)
	}
	unchanged := request("en.json", english.Header().Get("ETag"))
	if unchanged.Code != http.StatusNotModified || unchanged.Body.Len() != 0 {
		t.Fatalf("conditional locale response = %d %q, want 304 with empty body", unchanged.Code, unchanged.Body.String())
	}
}

func TestMCPHelpSourceUsesPublicTools(t *testing.T) {
	if (mcpHelpSource{}).Enabled() {
		t.Fatal("MCP help source without a server should be disabled")
	}
	source := mcpHelpSource{server: mcp.NewServer(chi.NewRouter(), nil, nil, "test")}
	if !source.Enabled() {
		t.Fatal("MCP help source should be enabled when the server exists")
	}
	if tools := source.PublicToolList(); len(tools) != 0 {
		t.Fatalf("MCP help source with an empty router returned tools: %#v", tools)
	}
}

func TestStartupAndMetricsRouteHelpers(t *testing.T) {
	t.Run("startup language fallback and translation", func(t *testing.T) {
		if got := startupTranslate("missing.startup.key", "fallback message"); got != "fallback message" {
			t.Fatalf("startupTranslate(missing) = %q", got)
		}
		if got := startupTranslate("startup.smtp.not_detected", "fallback message"); got == "fallback message" || got == "" {
			t.Fatalf("startupTranslate(known key) = %q", got)
		}
	})

	t.Run("listen host environment precedence", func(t *testing.T) {
		t.Setenv("LISTEN", "[::]:8443")
		t.Setenv("SERVER_ADDRESS", "127.0.0.1:8080")
		if got := startupListenHost(); got != "::" {
			t.Fatalf("LISTEN host = %q, want ::", got)
		}
		t.Setenv("LISTEN", "")
		if got := startupListenHost(); got != "127.0.0.1" {
			t.Fatalf("SERVER_ADDRESS host = %q, want 127.0.0.1", got)
		}
		t.Setenv("SERVER_ADDRESS", "unix socket path")
		if got := startupListenHost(); got != "unix socket path" {
			t.Fatalf("unparsed listen address = %q", got)
		}
		t.Setenv("SERVER_ADDRESS", "")
		t.Setenv("LISTEN", "")
		t.Setenv("REVERSE_PROXY", "")
		if got := startupListenHost("127.0.0.2"); got != "127.0.0.2" {
			t.Fatalf("config listen host = %q, want 127.0.0.2", got)
		}
		t.Setenv("REVERSE_PROXY", "true")
		if got := startupListenHost(); got != "127.0.0.1" {
			t.Fatalf("reverse proxy listen host = %q", got)
		}
	})

	t.Run("daemonize config and environment precedence", func(t *testing.T) {
		t.Setenv("DAEMON", "")
		if !daemonizeRequested(true) || daemonizeRequested(false) {
			t.Fatal("daemonize config value was not used when DAEMON is unset")
		}
		t.Setenv("DAEMON", "false")
		if daemonizeRequested(true) {
			t.Fatal("DAEMON=false did not override the config value")
		}
		t.Setenv("DAEMON", "true")
		if !daemonizeRequested(false) {
			t.Fatal("DAEMON=true did not override the config value")
		}
	})

	t.Run("service directories include all data paths", func(t *testing.T) {
		paths := &util.DirectoryPaths{Config: "/config", Data: "/data", Cache: "/cache", Log: "/log"}
		got := serviceDirectories(paths)
		if len(got) != 7 || got[0] != "/config" || got[1] != "/data" || got[2] != "/cache" || got[3] != "/log" {
			t.Fatalf("serviceDirectories() = %#v", got)
		}
		for _, path := range got {
			if path == "" {
				t.Fatalf("serviceDirectories() contains an empty path: %#v", got)
			}
		}
	})

	t.Run("metrics aliases respect root setting", func(t *testing.T) {
		serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
		for _, test := range []struct {
			rootEnabled bool
			path        string
			want        int
		}{
			{path: "/server/metrics", want: http.StatusNoContent},
			{path: "/server/metrics/prometheus", want: http.StatusNoContent},
			{path: "/api/v1/server/metrics", want: http.StatusNoContent},
			{path: "/api/v1/server/metrics/grafana", want: http.StatusNoContent},
			{path: "/api/metrics", want: http.StatusNoContent},
			{path: "/api/metrics/loki", want: http.StatusNoContent},
			{path: "/metrics", want: http.StatusNotFound},
			{rootEnabled: true, path: "/metrics/prometheus", want: http.StatusNoContent},
		} {
			router := chi.NewRouter()
			registerMetricsRoutes(router, "/api/v1", test.rootEnabled, serve)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.want {
				t.Errorf("metrics route %q (root=%t) status = %d, want %d", test.path, test.rootEnabled, response.Code, test.want)
			}
		}
	})
}

func TestShowServerStatusReadsDatabasesAndEnvironment(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"PORT": "8123", "MODE": "debug", "ENVIRONMENT": "ignored", "LISTEN": "127.0.0.1", "SERVER_ADDRESS": "ignored", "REVERSE_PROXY": "true",
	} {
		t.Setenv(key, value)
	}
	dualDB, err := database.InitDualDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database.SetGlobalDualDB(dualDB)
	t.Cleanup(func() {
		database.SetGlobalDualDB(nil)
		if err := dualDB.Close(); err != nil {
			t.Errorf("close test databases: %v", err)
		}
	})

	if !showServerStatus(&database.DB{DB: dualDB.Users}, "test/server.db + users.db", true) {
		t.Fatal("showServerStatus reported healthy test databases as unhealthy")
	}
}

type serverRunExitPanic struct {
	code int
}

func (e serverRunExitPanic) Error() string { return "server run exited" }

func runServerUntilExit(t *testing.T, args ...string) (code int) {
	t.Helper()
	originalArgs := os.Args
	os.Args = append([]string{originalArgs[0]}, args...)
	previousRestart := restartPending.Swap(false)
	defer func() {
		os.Args = originalArgs
		restartPending.Store(previousRestart)
	}()

	defer func() {
		recovered := recover()
		exit, ok := recovered.(serverRunExitPanic)
		if !ok {
			t.Fatalf("runServerWithHooks() panic = %#v, want exit hook", recovered)
		}
		code = exit.code
	}()
	runServerWithHooks(serverRunHooks{
		exitProcess: func(code int) { panic(serverRunExitPanic{code: code}) },
		runRootPhase: func(*util.DirectoryPaths, util.StartupPorts, string) {
			t.Fatal("healthcheck path must exit before root phase")
		},
		serviceStop:    make(chan struct{}),
		installSignals: false,
	})
	t.Fatal("runServerWithHooks() returned without invoking the exit hook")
	return -1
}

func TestRunServerWithHooksRejectsUnsafeDirectoryBeforeStartup(t *testing.T) {
	t.Setenv("CLI_HEALTHCHECK_FLAG", "")
	if got := runServerUntilExit(t, "--config", "../invalid"); got != 64 {
		t.Fatalf("unsafe config directory exit code = %d, want 64", got)
	}
}

func TestRunServerWithHooksHealthcheckExitsWithResult(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   int
		wantExit int
	}{
		{name: "healthy listener", status: http.StatusOK, wantExit: 0},
		{name: "unhealthy listener", status: http.StatusServiceUnavailable, wantExit: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/server/healthz" {
					t.Errorf("healthcheck path = %q, want /server/healthz", r.URL.Path)
				}
				w.WriteHeader(test.status)
			}))
			defer listener.Close()

			t.Setenv("CLI_HEALTHCHECK_FLAG", "1")
			t.Setenv("PORT", strings.TrimPrefix(listener.URL, "http://127.0.0.1:"))
			if got := runServerUntilExit(t); got != test.wantExit {
				t.Fatalf("healthcheck exit code = %d, want %d", got, test.wantExit)
			}
		})
	}
}
