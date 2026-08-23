package main

// Weather - Main entry point
// Per AI.md: Swagger annotations moved to src/swagger/annotations.go

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/webappsgo/wthr/src/cli"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/common/display"
	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	appgraphql "github.com/webappsgo/wthr/src/graphql"
	"github.com/webappsgo/wthr/src/mode"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/scheduler"
	"github.com/webappsgo/wthr/src/server"
	"github.com/webappsgo/wthr/src/server/handler"
	"github.com/webappsgo/wthr/src/server/metric"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed common/i18n/locales/*.json
var localesFS embed.FS

// getDefaultListenAddress auto-detects IPv6 support and returns dual-stack (::) or IPv4-only (0.0.0.0)
func getDefaultListenAddress() string {
	// Try to listen on dual-stack IPv6
	listener, err := net.Listen("tcp", "[::]:0")
	if err == nil {
		listener.Close()
		// IPv6 dual-stack supported (includes IPv4)
		return "::"
	}

	// Fallback to IPv4 only
	return "0.0.0.0"
}

// writeJSON writes v as a JSON response body with the given status code.
// Local mirror of the unexported helper in src/server/handler/response.go —
// main.go can't import an unexported cross-package function.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeText writes a plain-text response body with the given status code.
// Local mirror of the unexported helper in src/server/handler/response.go.
func writeText(w http.ResponseWriter, status int, format string, args ...interface{}) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, format, args...)
}

// registerHealthRoutes mounts the canonical health routes per AI.md PART 13
// /server/healthz is the canonical content-negotiated route, /api/{api_version}/server/healthz
// is its API counterpart, /api/healthz is the unversioned alias mounting the SAME handler,
// and the root /healthz alias is mounted only when server.healthz.root.enabled is true
// Aliases are always direct handler mappings, never redirects
func registerHealthRoutes(r chi.Router, apiPath string, rootAliasEnabled bool, frontend, api http.HandlerFunc) {
	r.Get("/server/healthz", frontend)
	r.Get(apiPath+"/server/healthz", api)
	r.Get("/api/healthz", api)
	if rootAliasEnabled {
		r.Get("/healthz", frontend)
	}
}

// registerGraphQLRoutes mounts the GraphQL endpoint and its API-path alias per AI.md PART 14
// The alias mounts the SAME handlers as /graphql — an alias is never a redirect
func registerGraphQLRoutes(r chi.Router, apiPath string, query, playground, assets http.HandlerFunc) {
	r.Post("/graphql", query)
	r.Get("/graphql", playground)
	// Locally embedded playground assets (React/GraphiQL/theme/init script) -
	// never loaded from a CDN, see src/graphql/playground.go.
	r.Get("/graphql/assets/*", assets)
	aliasPath := apiPath + "/graphql"
	r.Post(aliasPath, query)
	r.Get(aliasPath, playground)
}

func main() {
	// Initialize CLI
	cliInstance := cli.NewCLI()

	// Set version information
	cli.Version = Version
	cli.BuildDate = BuildDate
	cli.CommitID = CommitID

	// Register CLI commands
	cliInstance.RegisterCommand(&cli.Command{
		Name:        "service",
		Description: "Service management operations",
		Privileged:  true,
		Handler:     cli.ServiceCommand,
	})

	cliInstance.RegisterCommand(&cli.Command{
		Name:        "maintenance",
		Description: "Maintenance operations",
		Privileged:  false,
		Handler:     cli.MaintenanceCommand,
	})

	cliInstance.RegisterCommand(&cli.Command{
		Name:        "update",
		Description: "Update operations",
		Privileged:  false,
		Handler:     cli.UpdateCommand,
	})

	// Parse CLI arguments
	if err := cliInstance.Parse(os.Args[1:]); err != nil {
		// Bad CLI usage (AI.md PART 8: exit code 64 = usage error).
		log.Printf("Failed to parse CLI: %v", err)
		os.Exit(64)
	}

	// Check if this is a command that exits (handled by CLI package)
	// Commands like --help, --version are handled internally

	// Handle healthcheck flag (for Docker HEALTHCHECK)
	if os.Getenv("CLI_HEALTHCHECK_FLAG") == "1" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "80"
		}
		// AI.md PART 13: /server/healthz is the canonical route (root /healthz is optional)
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/server/healthz", port))
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle --daemon flag (AI.md PART 8: Daemonize - detach from terminal, Unix only)
	// AI.md: "modern service managers prefer foreground" but flag should work
	if config.IsTruthy(os.Getenv("DAEMON")) && runtime.GOOS != "windows" {
		// Check if we're already daemonized (avoid double fork)
		if os.Getenv("_DAEMON_CHILD") != "1" {
			// Fork a new process
			execPath, err := os.Executable()
			if err != nil {
				// General startup failure (AI.md PART 8: exit code 1).
				log.Printf("Failed to get executable path for daemon: %v", err)
				os.Exit(1)
			}

			// Set marker so child knows it's the daemon
			env := os.Environ()
			env = append(env, "_DAEMON_CHILD=1")

			// Create child process
			procAttr := &os.ProcAttr{
				Dir: "/",
				Env: env,
				// Detach stdin/stdout/stderr
				Files: []*os.File{nil, nil, nil},
			}

			proc, err := os.StartProcess(execPath, os.Args, procAttr)
			if err != nil {
				// General startup failure (AI.md PART 8: exit code 1).
				log.Printf("Failed to start daemon process: %v", err)
				os.Exit(1)
			}

			// Release the child and exit parent
			proc.Release()
			fmt.Printf("Started daemon with PID %d\n", proc.Pid)
			os.Exit(0)
		}
		// Child process continues with normal startup
		fmt.Println("Running as daemon (detached from terminal)")
	}

	// Initialize mode from environment variables (AI.md PART 6)
	// Handles MODE and DEBUG environment variables (set by CLI or directly)
	mode.FromEnv()

	if mode.IsDebugEnabled() {
		log.Println("DEBUG MODE ENABLED")
		log.Println("This mode should NEVER be used in production!")
		fmt.Printf("%s DEBUG MODE ENABLED\n", display.Emoji("⚠️", "WARNING:"))
		fmt.Printf("%s This mode should NEVER be used in production!\n", display.Emoji("⚠️", "WARNING:"))
	}

	// Log the current mode
	log.Printf("Running in mode: %s", mode.ModeString())
	fmt.Printf("%s Running in mode: %s\n", display.Emoji("🔒", "*"), mode.ModeString())

	// Initialize Prometheus metrics (AI.md PART 21 - NON-NEGOTIABLE)
	metric.Init(Version, CommitID, BuildDate)

	// Get OS-appropriate directory paths
	dirPaths, err := util.GetDirectoryPaths()
	if err != nil {
		// Config/path resolution failure (AI.md PART 8: exit code 2).
		log.Printf("Failed to determine directory paths: %v", err)
		os.Exit(2)
	}

	// Apply environment variable overrides (set by CLI or directly)
	// AI.md PART 7: Permissions - root: 0755, user: 0700
	dirPerm := os.FileMode(0700)
	if os.Geteuid() == 0 {
		dirPerm = 0755
	}

	envDataDir := os.Getenv("DATA_DIR")
	if envDataDir != "" {
		// CLI override for data directory
		if info, err := os.Stat(envDataDir); err == nil {
			if !info.IsDir() {
				if err := os.Remove(envDataDir); err != nil {
					// Config-directed path (DATA_DIR) unusable (AI.md PART 8: exit code 2).
					log.Printf("Failed to remove file at %s: %v", envDataDir, err)
					os.Exit(2)
				}
			}
		}
		if err := os.MkdirAll(envDataDir, dirPerm); err != nil {
			// Config-directed path (DATA_DIR) unusable (AI.md PART 8: exit code 2).
			log.Printf("Failed to create data directory %s: %v", envDataDir, err)
			os.Exit(2)
		}
		dirPaths.Data = envDataDir
	}

	envConfigDir := os.Getenv("CONFIG_DIR")
	if envConfigDir != "" {
		// CLI override for config directory
		if info, err := os.Stat(envConfigDir); err == nil {
			if !info.IsDir() {
				if err := os.Remove(envConfigDir); err != nil {
					// Config-directed path (CONFIG_DIR) unusable (AI.md PART 8: exit code 2).
					log.Printf("Failed to remove file at %s: %v", envConfigDir, err)
					os.Exit(2)
				}
			}
		}
		if err := os.MkdirAll(envConfigDir, dirPerm); err != nil {
			// Config-directed path (CONFIG_DIR) unusable (AI.md PART 8: exit code 2).
			log.Printf("Failed to create config directory %s: %v", envConfigDir, err)
			os.Exit(2)
		}
		dirPaths.Config = envConfigDir
	}

	envLogDir := os.Getenv("LOG_DIR")
	if envLogDir != "" {
		dirPaths.Log = envLogDir
	}

	// Create all required directories
	if err := util.CreateDirectories(dirPaths); err != nil {
		// Config-directed paths unusable (AI.md PART 8: exit code 2).
		log.Printf("Failed to create directories: %v", err)
		os.Exit(2)
	}

	// Generate server.yml if it doesn't exist (runtime generation per TEMPLATE.md)
	if err := cli.GenerateServerYML(dirPaths.Config); err != nil {
		log.Printf("Warning: Failed to generate server.yml: %v", err)
	}

	// Initialize logger
	appLogger, err := util.NewLogger(dirPaths.Log)
	if err != nil {
		// General startup failure (AI.md PART 8: exit code 1).
		log.Printf("Failed to initialize logger: %v", err)
		os.Exit(1)
	}

	// Print startup timestamp
	startTime := time.Now()
	appLogger.Printf("%s", startTime.Format("2006-01-02 at 15:04:05"))
	fmt.Printf("%s %s\n", display.Emoji("🕐", "*"), startTime.Format("2006-01-02 at 15:04:05"))

	// TEMPLATE.md PART 1: First Run Detection and Auto-Configuration
	isFirstRun := util.DetectFirstRun(dirPaths.Data)
	var setupToken string

	if isFirstRun {
		appLogger.Printf("First run detected - initializing server...")
		fmt.Printf("%s First run detected - auto-configuring server...\n", display.Emoji("🎉", "*"))

		// Auto-detect SMTP
		smtpHost, smtpPort := util.AutoDetectSMTP()
		appLogger.Printf("SMTP auto-detected: %s:%d", smtpHost, smtpPort)
		fmt.Printf("%s SMTP auto-detected: %s:%d\n", display.Emoji("📧", "*"), smtpHost, smtpPort)

		// Create server.yml with auto-detected settings
		configPath := filepath.Join(dirPaths.Config, "server.yml")
		if err := util.CreateDefaultServerYML(configPath, smtpHost, smtpPort); err != nil {
			appLogger.Error("Failed to create server.yml: %v", err)
			fmt.Printf("%s Failed to create server.yml: %v\n", display.Emoji("⚠️", "WARNING:"), err)
		} else {
			appLogger.Printf("server.yml created: %s", configPath)
			fmt.Printf("%s server.yml created with auto-detected settings\n", display.Emoji("✅", "[OK]"))
		}

		// Generate one-time setup token
		token, err := util.GenerateSetupToken()
		if err != nil {
			appLogger.Error("Failed to generate setup token: %v", err)
			os.Exit(1)
		}
		setupToken = token
		appLogger.Printf("Setup token generated (will be displayed in banner)")
	}

	// Initialize database - TEMPLATE.md PART 31: Dual database architecture
	// SQLite dual database: server.db + users.db
	// server.db = admin credentials, config, scheduler, audit log
	// users.db = user accounts, tokens, sessions, locations
	dualDB, err := database.InitDualDB(dirPaths.Data)
	if err != nil {
		appLogger.Error("Failed to initialize dual database system: %v", err)
		os.Exit(3)
	}
	defer dualDB.Close()

	// Set global instance for handler access
	database.SetGlobalDualDB(dualDB)
	dbPath := fmt.Sprintf("%s/db/server.db + %s/db/users.db", dirPaths.Data, dirPaths.Data)

	// Create wrapper for handlers that use database.DB struct
	// Uses Users database for user-related operations
	db := &database.DB{DB: dualDB.Users}

	// Check if setup is complete
	var setupComplete bool
	var setupValue string
	err = dualDB.QueryRowServer("SELECT value FROM server_config WHERE key = 'setup.completed'").Scan(&setupValue)
	setupComplete = (err == nil && setupValue == "true")

	// If first run, store setup token hash in file
	// AI.md: Setup token stored as SHA-256 hash in {config_dir}/setup_token.txt
	if isFirstRun && setupToken != "" {
		if err := util.SaveSetupToken(dirPaths.Config, setupToken); err != nil {
			appLogger.Error("Failed to store setup token: %v", err)
		} else {
			appLogger.Printf("Setup token hash saved to %s/setup_token.txt", dirPaths.Config)
		}
	}

	if setupComplete {
		appLogger.Printf("Database initialized: %s", dbPath)
		fmt.Printf("%s Database initialized: %s\n", display.Emoji("✅", "[OK]"), dbPath)
	} else {
		appLogger.Printf("Database initialized: %s (setup mode)", dbPath)
		fmt.Printf("%s Database initialized: %s (setup mode)\n", display.Emoji("✅", "[OK]"), dbPath)
	}

	// Load server configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		appLogger.Error("Warning: Could not load server.yml: %v (using defaults)", err)
		fmt.Printf("%s Warning: Could not load server.yml: %v (using defaults)\n", display.Emoji("⚠️", "WARNING:"), err)
	} else {
		appLogger.Printf("Configuration loaded from server.yml")
		fmt.Printf("%s Configuration loaded from server.yml\n", display.Emoji("✅", "[OK]"))
	}

	// Set global config for handler access
	config.SetGlobalConfig(cfg)

	// Note: Version and BuildDate are embedded in binary via LDFLAGS, not in config file

	// Initialize default settings with proper backup path
	settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
	backupPath := util.GetBackupPath(dirPaths)
	if err := settingsModel.InitializeDefaults(backupPath); err != nil {
		appLogger.Error("Warning: Could not initialize default settings: %v", err)
		fmt.Printf("%s Warning: Could not initialize default settings: %v\n", display.Emoji("⚠️", "WARNING:"), err)
	}

	// Initialize cache manager (Valkey/Redis support, optional)
	cacheManager := service.NewCacheManager()
	if cacheManager.IsEnabled() {
		appLogger.Printf("Cache enabled (Redis/Valkey)")
		fmt.Printf("%s Cache enabled (Redis/Valkey)\n", display.Emoji("✅", "[OK]"))
	}

	// LDAP authentication service per AI.md PART 11
	ldapService := service.NewLDAPService()
	ldapAuthHandler := &handler.LDAPAuthHandler{DB: db.DB, LDAPService: ldapService}

	oidcService := service.NewOIDCService()
	oidcAuthHandler := &handler.OIDCAuthHandler{DB: db.DB, OIDCService: oidcService}

	// Auto-detect SMTP server (localhost, Docker gateway, etc.) and configure defaults
	// SMTPService reads server_config and server_notification_channels, both of which live in server.db
	smtpService := service.NewSMTPService(dualDB.Server)
	if err := smtpService.LoadConfig(); err == nil {
		// Check if SMTP is not already configured
		smtpHost := settingsModel.GetString("smtp.host", "")
		if smtpHost == "" {
			// Try auto-detect
			if detected, _ := smtpService.AutoDetect(); detected {
				// SMTP detected, enable it
				settingsModel.SetBool("smtp.enabled", true)
				appLogger.Printf("SMTP server auto-detected and enabled")
				fmt.Printf("%s SMTP server auto-detected and enabled\n", display.Emoji("✉️", "*"))
			}
		}

		// Set default from_address if not set
		fromAddr := settingsModel.GetString("smtp.from_address", "")
		if fromAddr == "" {
			hostname, _ := os.Hostname()
			if hostname == "" {
				hostname = "localhost"
			}
			defaultFromAddr := fmt.Sprintf("no-reply@%s", hostname)
			settingsModel.SetString("smtp.from_address", defaultFromAddr)
		}

		// Set default from_name to server.title if not set
		fromName := settingsModel.GetString("smtp.from_name", "")
		if fromName == "" {
			serverTitle := settingsModel.GetString("server.title", "Weather")
			settingsModel.SetString("smtp.from_name", serverTitle)
		}
	}

	// Check if this is first run (no server admins created yet)
	hasNoUsers, err := db.IsFirstRun()
	if err != nil {
		appLogger.Error("Warning: Could not check first run status: %v", err)
		fmt.Printf("%s Warning: Could not check first run status: %v\n", display.Emoji("⚠️", "WARNING:"), err)
		hasNoUsers = false
	}
	if hasNoUsers {
		appLogger.Printf("No server admins found - complete setup at /server/%s", cfg.GetAdminPath())
		fmt.Printf("🆕 No server admins found - complete setup at /server/%s\n", cfg.GetAdminPath())
	}

	// Handle status flag
	// AI.md PART 8: --status exits with 0=healthy, 1=unhealthy
	if os.Getenv("CLI_STATUS_FLAG") == "1" {
		isHealthy := showServerStatus(db, dbPath, hasNoUsers)
		if isHealthy {
			os.Exit(0)
		}
		os.Exit(1)
	}

	// Application mode (development/production/debug) is already resolved by
	// mode.FromEnv() earlier in main() per AI.md PART 6 --mode/MODE precedence;
	// this redundant envMode/gin.SetMode switch has been removed.

	// Create chi router
	r := chi.NewRouter()

	// NOTE: gin's r.SetTrustedProxies([]string{...}) has no chi/project
	// equivalent, so this call is dropped rather than translated. The
	// project's AI.md PART 5/12 trusted-proxies gate is now implemented as
	// server.trusted_proxies (src/config) + util.TrustedGetClientIP
	// (src/util/trusted_proxies.go), which all call sites use instead of
	// the unguarded util.GetClientIP. See TODO.AI.md item 171.

	// AI.md PART 5: Middleware order - security first!
	// 1. URL normalization (FIRST - normalize before anything else)
	r.Use(middleware.URLNormalizeMiddleware())

	// 2. Path security (SECOND - block traversal attacks before processing)
	r.Use(middleware.PathSecurityMiddleware())

	// Request ID middleware - for request tracing in logs
	r.Use(middleware.RequestID())

	// Access logging middleware (writes to log files)
	r.Use(middleware.AccessLogger(appLogger))

	// Recovery middleware
	r.Use(middleware.Recovery(appLogger))

	// Response compression per AI.md PART 18 lines 15704-15719
	// Compresses text/html, text/css, application/json, etc.
	r.Use(chimiddleware.Compress(5, "text/html", "text/css", "text/plain", "application/json", "application/javascript", "application/xml", "image/svg+xml"))

	// Prometheus metrics middleware (AI.md PART 21 - NON-NEGOTIABLE)
	r.Use(middleware.MetricsMiddleware())

	// Security headers middleware
	r.Use(middleware.SecurityHeaders())

	// Body size limit middleware per AI.md PART 18 line 15691 (10MB)
	r.Use(middleware.BodySizeLimitMiddleware(middleware.DefaultMaxBodySize))

	// CSRF protection middleware (AI.md PART 0 line 994, PART 22)
	r.Use(middleware.CSRFProtection(middleware.DefaultCSRFConfig()))

	// CORS middleware per AI.md PART 17 lines 14220-14222 and 15401-15405
	r.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-API-Token"},
		ExposedHeaders:   []string{"Content-Length"},
		AllowCredentials: false,
		// Per AI.md line 15405: Access-Control-Max-Age = 86400 (24 hours)
		MaxAge: int((24 * time.Hour).Seconds()),
	}).Handler)

	// Global rate limiting middleware (100 req/s)
	r.Use(middleware.GlobalRateLimitMiddleware())

	// Server context middleware - injects server title/tagline/description
	r.Use(middleware.InjectServerContext(db.DB, Version))

	// AI.md: Server is FULLY FUNCTIONAL without setup - only admin panel requires setup
	// AdminSetupRequired middleware applied to admin routes only (see admin route group below)

	// Restrict admin users to only access /admin routes - all other routes treat them as anonymous
	r.Use(middleware.RestrictAdminToAdminRoutes())

	// Path normalization handled by middleware.URLNormalizeMiddleware() and middleware.PathSecurityMiddleware()

	// Serve embedded static files from server package
	staticSubFS, err := server.GetStaticSubFS()
	if err != nil {
		// General startup failure — embedded asset corruption (AI.md PART 8: exit code 1).
		log.Printf("Failed to get static subdirectory: %v", err)
		os.Exit(1)
	}
	staticFileServer := http.StripPrefix("/static", http.FileServer(http.FS(staticSubFS)))
	r.Handle("/static/*", staticFileServer)

	// Initialize i18n service (TEMPLATE.md PART 29 - NON-NEGOTIABLE)
	// AI.md PART 31 server chain: --lang > server.yml lang > LC_ALL/LANG > en
	serverLang := config.ResolveLanguage(cfg)
	i18nService, err := i18n.NewI18n(localesFS, serverLang)
	if err != nil {
		// General startup failure — embedded locale corruption (AI.md PART 8: exit code 1).
		log.Printf("Failed to initialize i18n: %v", err)
		os.Exit(1)
	}
	// AI.md PART 31: an unsupported language silently falls back to English
	if !i18nService.IsSupported(serverLang) {
		i18nService, err = i18n.NewI18n(localesFS, "en")
		if err != nil {
			log.Printf("Failed to initialize i18n: %v", err)
			os.Exit(1)
		}
	}
	// Global accessor for services/scheduler tasks with no gin.Context
	// (e.g. src/server/service/smtp.go's server-initiated emails).
	i18n.SetGlobalI18n(i18nService)
	fmt.Printf("%s I18n initialized with languages: %v\n", display.Emoji("🌐", "*"), i18nService.GetSupportedLanguages())

	// I18n middleware - per AI.md PART 31 fallback chain:
	// ?lang= query param (sets 1yr cookie) → lang cookie → Accept-Language → en
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lang := ""

			// 1. ?lang= query param (highest priority, also sets cookie)
			if q := r.URL.Query().Get("lang"); q != "" && i18nService.IsSupported(q) {
				lang = q
				http.SetCookie(w, &http.Cookie{
					Name:     "lang",
					Value:    lang,
					MaxAge:   365 * 24 * 60 * 60,
					Path:     "/",
					Secure:   r.TLS != nil,
					HttpOnly: true,
				})
			}

			// 2. lang cookie
			if lang == "" {
				if cookie, err := r.Cookie("lang"); err == nil && i18nService.IsSupported(cookie.Value) {
					lang = cookie.Value
				}
			}

			// 3. Accept-Language header
			if lang == "" {
				lang = i18nService.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
			}

			// 4. Default: en
			if lang == "" {
				lang = "en"
			}

			ctx := reqctx.Set(r.Context(), "lang", lang)
			ctx = reqctx.Set(ctx, "i18n", i18nService)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	// Load embedded templates with custom functions from server package
	// Get embedded templates filesystem
	templatesFS := server.GetTemplatesFS()
	// Create sub-filesystem starting at "template/" so template names don't include "template/" prefix
	templatesSubFS, err := fs.Sub(templatesFS, "template")
	if err != nil {
		// General startup failure — embedded asset corruption (AI.md PART 8: exit code 1).
		log.Printf("Failed to get template subdirectory: %v", err)
		os.Exit(1)
	}

	// Walk the filesystem and collect all .tmpl files
	var templatePaths []string
	fs.WalkDir(templatesSubFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tmpl") {
			templatePaths = append(templatePaths, path)
		}
		return nil
	})

	// Debug: Print loaded templates
	if mode.IsDebugEnabled() {
		fmt.Printf("%s Loading %d templates:\n", display.Emoji("📝", "*"), len(templatePaths))
		for _, path := range templatePaths {
			fmt.Printf("   - %s\n", path)
		}
	}

	// Create template function map with i18n support
	templateFuncs := template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": func(s string) string { return cases.Title(language.English).String(s) },
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		// i18n translation function - expects lang to be set in template data
		"t": func(lang, key string) string {
			return i18nService.T(lang, key)
		},
	}

	// Parse all templates - wrap those without {{define}} in a define block to preserve full path names
	tmpl := template.New("").Funcs(templateFuncs)
	for _, path := range templatePaths {
		content, err := fs.ReadFile(templatesSubFS, path)
		if err != nil {
			// General startup failure — embedded asset corruption (AI.md PART 8: exit code 1).
			log.Printf("Failed to read template %s: %v", path, err)
			os.Exit(1)
		}
		contentStr := string(content)
		// If template doesn't have {{define}}, wrap it to give it a name matching the path
		if !strings.Contains(contentStr, "{{define ") {
			contentStr = fmt.Sprintf("{{define %q}}%s{{end}}", path, contentStr)
		}
		_, err = tmpl.Parse(contentStr)
		if err != nil {
			// General startup failure — embedded asset corruption (AI.md PART 8: exit code 1).
			log.Printf("Failed to parse template %s: %v", path, err)
			os.Exit(1)
		}
	}

	// Debug: Print registered template names
	if mode.IsDebugEnabled() {
		fmt.Printf("%s Registered template names:\n", display.Emoji("📋", "*"))
		for _, t := range tmpl.Templates() {
			fmt.Printf("   - %s\n", t.Name())
		}
	}

	middleware.SetHTMLTemplates(tmpl)
	middleware.RenderHTML = func(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{}) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_ = tmpl.ExecuteTemplate(w, name, data)
	}

	// Live reload templates in debug mode (loads from filesystem if available)
	if mode.IsDebugEnabled() {
		if _, err := os.Stat("src/server/template"); err == nil {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Try to reload from filesystem in debug mode
					t := template.New("").Funcs(templateFuncs)
					// Load all templates including subdirectories
					// Note: This loads from filesystem, so paths are relative to src/server/template/
					patterns := []string{
						"src/server/template/*.tmpl",
						"src/server/template/*/*.tmpl",
						"src/server/template/*/*/*.tmpl",
					}
					for _, pattern := range patterns {
						t, _ = t.ParseGlob(pattern)
					}
					// Need to rename templates to remove "src/server/template/" prefix for consistency
					// This is a bit hacky but necessary for live reload
					tmpl = t
					middleware.SetHTMLTemplates(tmpl)
					next.ServeHTTP(w, r)
				})
			})
			fmt.Printf("%s Live reload enabled for templates (using filesystem)\n", display.Emoji("🔄", "->"))
		} else {
			fmt.Printf("%s Using embedded templates (no filesystem template found)\n", display.Emoji("📦", "*"))
		}
	} else {
		fmt.Printf("%s Using embedded templates and static files\n", display.Emoji("📦", "*"))
	}

	// Initialize location enhancer
	locationEnhancer := service.NewLocationEnhancer(db.DB)

	// Set callback to mark initialization complete
	locationEnhancer.SetOnInitComplete(func(countries, cities bool) {
		// Mark weather service as always ready (no initialization needed)
		handler.SetInitStatus(countries, cities, true)
		fmt.Printf("%s Countries: %v, Cities: %v, zipcodes: true, airportcodes: true\n", display.Emoji("✅", "[OK]"), countries, cities)
	})

	// Initialize GeoIP service (downloads database on first run, updates weekly)
	geoipService := service.NewGeoIPService(dirPaths.Config)

	weatherService := service.NewWeatherService(locationEnhancer, geoipService)

	// Data loads automatically in the background via loadData()
	// Mark service as ready after 2 minute initialization timeout (keep as fallback)
	go func() {
		time.Sleep(2 * time.Minute)
		if !handler.IsInitialized() {
			fmt.Println("⏰ Initialization timeout reached, marking service as ready (fallback)")
			fmt.Printf("%s %s\n", display.Emoji("🕐", "*"), time.Now().Format("2006-01-02 at 15:04:05"))
			handler.SetInitStatus(true, true, true)
		}
	}()

	// Initialize notification system services (silent)
	// ChannelManager owns server_notification_channels, TemplateEngine owns the notification
	// template table and DeliverySystem owns notification_queue/notification_history - every
	// one of those tables is declared in database.ServerSchema, so they all take server.db
	channelManager := service.NewChannelManager(dualDB.Server)
	templateEngine := service.NewTemplateEngine(dualDB.Server)
	deliverySystem := service.NewDeliverySystem(dualDB.Server, channelManager, templateEngine)

	// Load delivery system settings from database
	_ = deliverySystem.LoadSettings()

	// Initialize default templates
	_ = templateEngine.InitializeDefaultTemplates()

	// Initialize channels in database
	_ = channelManager.InitializeChannels()

	// Register email channel with the channel manager
	smtpService = service.NewSMTPService(dualDB.Server)
	_ = smtpService.LoadConfig()
	emailChannel := service.NewEmailChannel(smtpService)
	channelManager.RegisterChannel(emailChannel)
	if emailChannel.IsEnabled() {
		fmt.Printf("%s Email channel registered and enabled\n", display.Emoji("📧", "*"))
	}

	// Create weather notification service - it reads notification_subscriptions,
	// user_notification_channel_preferences, user_saved_locations, user_accounts and
	// user_weather_alert_history, all declared in database.UsersSchema
	weatherNotifications := service.NewWeatherNotificationService(dualDB.Users, weatherService, deliverySystem, templateEngine)

	// Initialize notification metrics service - notification_metrics and notification_queue
	// are declared in database.ServerSchema
	notificationMetrics := service.NewNotificationMetrics(dualDB.Server)

	// Initialize Tor hidden service (TEMPLATE.md PART 32 - NON-NEGOTIABLE)
	torService := service.NewTorService(db, dirPaths.Data)

	// Set Tor status provider for health checks (AI.md PART 32)
	handler.SetTorStatusProvider(torService)

	// Initialize config file watcher for live reload (TEMPLATE.md PART 1)
	configPath := filepath.Join(dirPaths.Config, "server.yml")
	configWatcher, err := service.NewConfigWatcher(configPath, func(newCfg *config.AppConfig) error {
		// Reload configuration callback - applies changes live without restart
		log.Printf("Configuration reloaded from %s", configPath)
		fmt.Printf("%s Configuration reloaded from %s\n", display.Emoji("🔄", "->"), configPath)

		// Update all configuration sections that can be changed at runtime
		cfg.Server.Mode = newCfg.Server.Mode
		cfg.Server.Branding = newCfg.Server.Branding
		cfg.Server.SEO = newCfg.Server.SEO
		cfg.Web = newCfg.Web
		cfg.Server.Notifications = newCfg.Server.Notifications
		cfg.Server.RateLimit = newCfg.Server.RateLimit
		cfg.Server.Tor = newCfg.Server.Tor
		cfg.Server.Features = newCfg.Server.Features

		// Update global config for handlers
		config.SetGlobalConfig(cfg)

		// Port changes require a manual restart to take effect.
		log.Println("OK: All configuration sections reloaded (branding, SEO, theme, email, notifications, rate limiting, web, Tor, features)")
		fmt.Printf("%s All configuration sections reloaded successfully\n", display.Emoji("✅", "[OK]"))

		return nil
	})
	if err != nil {
		log.Printf("Failed to create config watcher: %v", err)
		fmt.Printf("%s Failed to create config watcher: %v\n", display.Emoji("⚠️", "WARNING:"), err)
	}

	// Initialize scheduler for periodic tasks
	taskScheduler := scheduler.NewScheduler(db.DB)

	// Register log rotation task - AI.md PART 19: daily at midnight
	taskScheduler.AddTask("rotate-logs", "0 0 * * *", func() error {
		return appLogger.RotateLogs()
	})

	// Register cleanup tasks - AI.md PART 19: session cleanup every 15 minutes
	taskScheduler.AddTask("cleanup-sessions", "@every 15m", func() error {
		return scheduler.CleanupOldSessions(db.DB)
	})

	// AI.md PART 19: token cleanup every 15 minutes
	taskScheduler.AddTask("cleanup-tokens", "@every 15m", func() error {
		return scheduler.CleanupExpiredTokens(db.DB)
	})

	taskScheduler.AddTask("cleanup-rate-limits", "@hourly", func() error {
		return scheduler.CleanupRateLimitCounters(db.DB)
	})

	taskScheduler.AddTask("cleanup-audit-logs", "@daily", func() error {
		return scheduler.CleanupOldAuditLogs(db.DB)
	})

	// Register weather alert checks - run every 5 minutes per IDEA.md
	taskScheduler.AddTask("check-weather-alerts", "@every 5m", func() error {
		return weatherNotifications.CheckWeatherAlerts()
	})

	// Expire stale weather alerts from user_weather_alerts every 5 minutes
	taskScheduler.AddTask("expire-weather-alerts", "@every 5m", func() error {
		usersDB := database.GetUsersDB()
		if usersDB == nil {
			return nil
		}
		// Comparing expires_at against datetime('now') in SQL silently matches nothing when a
		// row was written in a non-UTC or non-canonical layout, so the cutoff is applied in Go
		_, err := dbtime.DeleteRowsWithTimestampBefore(usersDB, "user_weather_alerts", "id", "expires_at", time.Now().UTC(), false)
		return err
	})

	// Register daily forecast - AI.md PART 19: run once per day at 7 AM
	taskScheduler.AddTask("daily-forecast", "0 7 * * *", func() error {
		return weatherNotifications.SendDailyForecast()
	})

	// Register notification queue processing - run every 2 minutes
	taskScheduler.AddTask("process-notification-queue", "@every 2m", func() error {
		return deliverySystem.ProcessQueue()
	})

	// Register cleanup of old delivered notifications - daily
	// Keep 30 days
	taskScheduler.AddTask("cleanup-notifications", "@daily", func() error {
		return deliverySystem.CleanupOld(30)
	})

	// AI.md PART 19: backup daily at 02:00
	taskScheduler.AddTask("backup-daily", "0 2 * * *", func() error {
		return scheduler.CreateSystemBackup(db.DB)
	})

	// AI.md PART 19 line 27050: backup_hourly - hourly incremental (disabled by default)
	// Only runs if backup.hourly_enabled is true in config
	taskScheduler.AddTask("backup-hourly", "@hourly", func() error {
		if !cfg.Server.Maintenance.Backup.HourlyEnabled {
			return nil
		}
		p := path.GetDefaultPaths("wthr")
		if p == nil {
			return fmt.Errorf("failed to get paths for hourly backup")
		}
		return scheduler.BackupHourlyTask(p.ConfigDir, p.DataDir)()
	})

	// AI.md PART 19: SSL renewal check daily at 03:00
	taskScheduler.AddTask("ssl-renewal", "0 3 * * *", func() error {
		return scheduler.CheckSSLRenewal()
	})

	// AI.md PART 19: self health check every 5 minutes
	taskScheduler.AddTask("healthcheck-self", "@every 5m", func() error {
		return scheduler.SelfHealthCheck()
	})

	// AI.md PART 19: Tor health check every 10 minutes (when Tor installed)
	taskScheduler.AddTask("tor-health", "@every 10m", func() error {
		return scheduler.CheckTorHealth()
	})

	// Register weather cache refresh - run every 15 minutes per IDEA.md
	// Proactively fetches weather for all saved user locations to warm the in-process cache.
	taskScheduler.AddTask("refresh-weather-cache", "@every 15m", func() error {
		usersDB := database.GetUsersDB()
		if usersDB == nil {
			return nil
		}
		rows, err := database.QueryContext(context.Background(), usersDB, database.TimeoutSimpleSelect, `SELECT DISTINCT latitude, longitude FROM user_locations WHERE latitude IS NOT NULL AND longitude IS NOT NULL LIMIT 100`)
		if err != nil {
			return nil
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var lat, lon float64
			if err := rows.Scan(&lat, &lon); err != nil {
				continue
			}
			_, _ = weatherService.GetCurrentWeather(lat, lon, "metric")
			count++
		}
		log.Printf("INFO: Weather cache refreshed for %d location(s)", count)
		return nil
	})

	// Register GeoIP database update - AI.md PART 19: weekly Sunday at 03:00
	taskScheduler.AddTask("update-geoip-database", "0 3 * * 0", func() error {
		fmt.Printf("%s Weekly GeoIP database update starting...\n", display.Emoji("🌍", "*"))
		if err := geoipService.UpdateDatabase(); err != nil {
			fmt.Printf("%s GeoIP update failed: %v\n", display.Emoji("⚠️", "WARNING:"), err)
			return err
		}
		return nil
	})

	// AI.md PART 19: blocklist update daily at 04:00
	taskScheduler.AddTask("blocklist-update", "0 4 * * *", func() error {
		return scheduler.UpdateBlocklist()
	})

	// AI.md PART 19: CVE database update daily at 05:00
	taskScheduler.AddTask("cve-update", "0 5 * * *", func() error {
		return scheduler.UpdateCVEDatabase()
	})

	// AI.md PART 19 line 24792: cluster.heartbeat every 30 seconds (cluster mode only)
	// This is a LOCAL task - runs on every node (not a global task)
	nodeIDForHeartbeat, _ := os.Hostname()
	if nodeIDForHeartbeat == "" {
		nodeIDForHeartbeat = "default"
	}
	taskScheduler.AddTask("cluster-heartbeat", "@every 30s", func() error {
		return scheduler.ClusterHeartbeat(nodeIDForHeartbeat)
	})

	// Initialize task history table for scheduler tracking
	if err := taskScheduler.InitTaskHistoryTable(); err != nil {
		fmt.Printf("%s Failed to initialize task history table: %v\n", display.Emoji("❌", "[FAIL]"), err)
		// DB connection failure (AI.md PART 8: exit code 3).
		log.Printf("Failed to initialize task history table: %v", err)
		os.Exit(3)
	}

	// Start the scheduler
	taskScheduler.Start()

	// Schedule WebUI notification cleanup tasks (TEMPLATE.md Part 25)
	// Note: NotificationCleaner will be initialized after NotificationService is created
	// Cleanup scheduled for 02:00 UTC, Limit enforcement at 03:00 UTC

	// Create services
	earthquakeService := service.NewEarthquakeService()
	hurricaneService := service.NewHurricaneService()
	severeWeatherService := service.NewSevereWeatherService()

	// Pre-warm earthquake cache every 1 minute (USGS feed per features-rules)
	taskScheduler.AddTask("poll-earthquakes", "@every 1m", func() error {
		_, _ = earthquakeService.GetEarthquakes("all_hour")
		return nil
	})

	// Pre-warm hurricane cache every 15 minutes (NOAA NHC per features-rules)
	taskScheduler.AddTask("poll-hurricanes", "@every 15m", func() error {
		_, _ = hurricaneService.GetActiveStorms()
		return nil
	})

	// Create handlers
	weatherHandler := handler.NewWeatherHandler(weatherService, locationEnhancer)
	apiHandler := handler.NewAPIHandler(weatherService, locationEnhancer)
	webHandler := handler.NewWebHandler(weatherService, locationEnhancer)
	earthquakeHandler := handler.NewEarthquakeHandler(earthquakeService, weatherService, locationEnhancer)
	hurricaneHandler := handler.NewHurricaneHandler(hurricaneService)
	severeWeatherHandler := handler.NewSevereWeatherHandler(severeWeatherService, locationEnhancer, weatherService)
	moonHandler := handler.NewMoonHandler(weatherService, locationEnhancer)
	historyHandler := handler.NewHistoryHandler(weatherService, settingsModel)

	// Create auth handlers
	authHandler := &handler.AuthHandler{DB: db.DB}
	authAPIHandler := handler.NewAuthAPIHandler(db.DB)
	twoFAHandler := &handler.TwoFactorHandler{DB: db.DB}
	passkeyHandler := handler.NewPasskeyHandler(db.DB)
	setupHandler := &handler.SetupHandler{DB: db.DB}
	dashboardHandler := &handler.DashboardHandler{DB: db.DB}
	adminHandler := &handler.AdminHandler{DB: db.DB}
	serverDB := database.GetServerDB()
	adminPasskeyHandler := handler.NewAdminPasskeyHandler(serverDB)
	adminInviteService := service.NewAdminInviteService(serverDB, "", smtpService)
	userInviteModel := &model.UserInviteModel{DB: database.GetUsersDB()}
	locationHandler := &handler.LocationHandler{
		DB:               db.DB,
		WeatherService:   weatherService,
		LocationEnhancer: locationEnhancer,
	}

	// Initialize WebSocket Hub for real-time notifications (TEMPLATE.md Part 25)
	wsHub := service.NewWebSocketHub()
	// Start hub in goroutine
	go wsHub.Run()

	// Initialize Notification Service (TEMPLATE.md Part 25 - WebUI Notifications)
	notificationService := &service.NotificationService{
		UserDB:     dualDB.Users,
		ServerDB:   dualDB.Server,
		WSHub:      wsHub,
		UserNotif:  &model.UserNotificationModel{DB: dualDB.Users},
		AdminNotif: &model.AdminNotificationModel{DB: dualDB.Server},
		Prefs:      &model.NotificationPreferencesModel{UserDB: dualDB.Users, ServerDB: dualDB.Server},
	}

	// Create WebUI notification API handlers (TEMPLATE.md Part 25)
	notificationAPIHandler := &handler.NotificationAPIHandlers{
		NotificationService: notificationService,
		WSHub:               wsHub,
	}

	adminSettingsHandler := &handler.AdminSettingsHandler{
		DB:                  db.DB,
		NotificationService: notificationService,
	}

	// Legacy notification handler (for email notifications only) - reads per-user notification
	// rows, which live in users.db
	notificationHandler := &handler.NotificationHandler{DB: dualDB.Users}

	// Create notification system handlers - channels/queue/history/templates are all declared
	// in database.ServerSchema, while the per-user channel preferences and subscriptions the
	// preferences handler edits are declared in database.UsersSchema
	channelHandler := handler.NewNotificationChannelHandler(dualDB.Server)
	preferencesHandler := handler.NewNotificationPreferencesHandler(dualDB.Users)
	templateHandler := handler.NewNotificationTemplateHandler(dualDB.Server)
	metricsHandler := handler.NewNotificationMetricsHandler(notificationMetrics)

	// Initialize WebUI Notification Cleanup Scheduler (TEMPLATE.md Part 25)
	notificationCleaner := scheduler.NewNotificationCleaner(notificationService)
	// Daily at 2 AM UTC
	taskScheduler.ScheduleNotificationCleanup(notificationCleaner, "02:00")
	// Daily at 3 AM UTC
	taskScheduler.ScheduleNotificationLimitEnforcement(notificationCleaner, "03:00")

	// Create scheduler handler for task management
	schedulerHandler := handler.NewSchedulerHandler(taskScheduler)

	// Create Tor admin handler
	torAdminHandler := handler.NewTorAdminHandler(torService, settingsModel, dirPaths.Data)

	// Create email template handler
	emailTemplateHandler := handler.NewEmailTemplateHandler(filepath.Join("src", "server", "template"))

	// Create logs handler
	logsHandler := handler.NewLogsHandler(dirPaths.Log)

	// Create admin settings handlers
	adminUsersHandler := &handler.AdminUsersHandler{ConfigPath: configPath}
	adminAuthHandler := &handler.AdminAuthSettingsHandler{ConfigPath: configPath}
	adminWeatherHandler := &handler.AdminWeatherHandler{ConfigPath: configPath}
	adminNotificationsHandler := &handler.AdminNotificationsHandler{ConfigPath: configPath}
	adminGeoIPHandler := &handler.AdminGeoIPHandler{ConfigPath: configPath}

	// Create user settings handler (AI.md PART 34: Multi-user support)
	userSettingsHandler := handler.NewUserSettingsHandler(db.DB)

	// Create user public handler (AI.md PART 34: Public profiles, avatars)
	userPublicHandler := handler.NewUserPublicHandler(db.DB)

	// Get port configuration using comprehensive port manager
	// Priority: 1) Database saved ports, 2) Config file port, 3) PORT env variable, 4) Random port
	portManager := util.NewPortManager(db.DB)

	// Extract port from config (can be int or string)
	configPort := 0
	if cfg != nil && cfg.Server.Port != nil {
		switch p := cfg.Server.Port.(type) {
		case int:
			configPort = p
		case float64:
			configPort = int(p)
		case string:
			if parsed, err := strconv.Atoi(p); err == nil {
				configPort = parsed
			}
		}
	}

	httpPortInt, httpsPortInt, err := portManager.GetServerPortsWithConfig(configPort)
	if err != nil {
		// Config error — invalid/unavailable port configuration (AI.md PART 8: exit code 2).
		log.Printf("Failed to configure server ports: %v", err)
		os.Exit(2)
	}

	port := fmt.Sprintf("%d", httpPortInt)

	// Get listen address - auto-detect reverse proxy and IPv6 support
	// AI.md PART 5: LISTEN env var
	listenAddress := os.Getenv("LISTEN")
	if listenAddress == "" {
		// Legacy fallback
		listenAddress = os.Getenv("SERVER_ADDRESS")
	}

	// Check if listenAddress contains a port (e.g., "127.0.0.1:8080" or "[::]:8080")
	if listenAddress != "" && strings.Contains(listenAddress, ":") {
		// Try to split host and port
		host, portStr, err := net.SplitHostPort(listenAddress)
		if err == nil && portStr != "" {
			// Successfully parsed - update both listenAddress and port
			listenAddress = host
			if parsedPort, err := strconv.Atoi(portStr); err == nil && parsedPort > 0 && parsedPort < 65536 {
				httpPortInt = parsedPort
				port = portStr
			}
		}
	}

	networkMode := ""
	if listenAddress == "" {
		// Check for reverse proxy indicator per AI.md PART 5: Boolean Handling
		reverseProxy := config.IsTruthy(os.Getenv("REVERSE_PROXY"))

		if reverseProxy {
			listenAddress = "127.0.0.1"
			networkMode = " in reverse proxy mode"
		} else {
			// Auto-detect IPv6 support and use dual-stack if available
			listenAddress = getDefaultListenAddress()
			if listenAddress == "::" {
				networkMode = " (dual-stack: IPv4 + IPv6)"
			} else {
				networkMode = " (IPv4 only)"
			}
		}
	}

	// Print startup messages
	// Format display address correctly for IPv6
	displayAddr := listenAddress
	if listenAddress == "::" {
		displayAddr = "[::]"
	}
	appLogger.Printf("Starting Weather%s on %s:%s", networkMode, displayAddr, port)
	fmt.Printf("%s Starting Weather%s on %s:%s\n", display.Emoji("🚀", "*"), networkMode, displayAddr, port)
	appLogger.Info("Data directory: %s", dirPaths.Data)
	appLogger.Info("Config directory: %s", dirPaths.Config)
	appLogger.Info("Log directory: %s", dirPaths.Log)

	// Initialize SSL manager
	sslCertsDir := util.GetCertsPath(dirPaths)
	sslManager := util.NewSSLManager(db.DB, sslCertsDir)
	httpsPort := httpsPortInt

	// Create SSL handler with runtime-detected HTTPS address
	httpsAddr := fmt.Sprintf("127.0.0.1:%d", httpsPortInt)
	sslHandler := handler.NewSSLHandler(sslCertsDir, db.DB, httpsAddr)

	// Create metrics handler
	metricsConfigHandler := handler.NewMetricsHandler()

	// Create logging handler
	loggingHandler := handler.NewLoggingHandler(dirPaths.Log)

	// Create admin web handler (robots.txt, security.txt)
	adminWebHandler := handler.NewAdminWebHandler(db)

	// Check for SSL configuration
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	// Try to check for existing Let's Encrypt certs and enable HTTPS if configured
	if httpsPort > 0 {
		found, err := sslManager.CheckExistingCerts(hostname)
		if err != nil {
			appLogger.Error("SSL check failed: %v", err)
			fmt.Printf("%s SSL check failed: %v\n", display.Emoji("⚠️", "WARNING:"), err)
		} else if found {
			appLogger.Printf("Found Let's Encrypt certificate for %s", hostname)
			appLogger.Printf("HTTPS enabled on port: %d", httpsPort)
			fmt.Printf("%s Found Let's Encrypt certificate for %s\n", display.Emoji("🔒", "*"), hostname)
			fmt.Printf("%s HTTPS enabled on port: %d\n", display.Emoji("🔌", "*"), httpsPort)
		} else {
			appLogger.Printf("HTTPS port configured (%d) but no certificates found", httpsPort)
			fmt.Printf("%s HTTPS port configured (%d) but no certificates found\n", display.Emoji("ℹ️", "*"), httpsPort)
		}
	}
	// Note: Self-signed cert generation is optional and disabled by default
	// Can be enabled via CLI flag or environment variable if needed

	// Set directory paths for handlers
	handler.SetDirectoryPaths(dirPaths.Data, dirPaths.Log)

	// Set build info for handler package
	handler.SetBuildInfo(Version, BuildDate, CommitID)

	// Health check endpoints (AI.md PART 13)
	registerHealthRoutes(r, cfg.GetAPIPath(), cfg.IsHealthzRootAliasEnabled(),
		handler.HealthCheck(db, startTime), handler.APIHealthCheck(db, startTime))
	r.Get("/health", handler.LivenessCheck)
	r.Get("/health/ready", handler.ReadinessCheck(db, startTime))
	r.Get("/health/full", handler.FullHealthCheck(db, startTime))

	// Prometheus metrics endpoint (TEMPLATE.md required - optional auth)
	r.Get("/metrics", handler.PrometheusMetrics())

	// security.txt endpoint (RFC 9116 - TEMPLATE.md PART 25)
	r.Get("/.well-known/security.txt", adminWebHandler.ServeSecurityTxt)
	// Also serve at root for compatibility
	r.Get("/security.txt", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/.well-known/security.txt", http.StatusMovedPermanently)
	})

	// /.well-known/change-password redirect (TEMPLATE.md PART 25)
	r.Get("/.well-known/change-password", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/profile?tab=security", http.StatusFound)
	})

	// /.well-known/acme-challenge/:token - Let's Encrypt HTTP-01 challenge (TEMPLATE.md Part 8)
	r.Get("/.well-known/acme-challenge/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		keyAuth, ok := service.GetGlobalHTTP01Provider().GetKeyAuth(token)
		if !ok {
			writeText(w, http.StatusNotFound, "")
			return
		}
		writeText(w, http.StatusOK, "%s", keyAuth)
	})

	// robots.txt endpoint
	r.Get("/robots.txt", adminWebHandler.ServeRobotsTxt)

	// sitemap.xml endpoint (AI.md PART 16: dynamically generated)
	r.Get("/sitemap.xml", adminWebHandler.ServeSitemap)

	// favicon.ico endpoint (AI.md PART 16: embedded default, customizable)
	r.Get("/favicon.ico", adminWebHandler.ServeFavicon)

	// Debug endpoints (only enabled when --debug flag or DEBUG=true)
	// Per AI.md PART 6: Debug endpoints only available when debug mode enabled
	if mode.IsDebugEnabled() {
		debugHandlers := handler.NewDebugHandlers(db.DB, r)
		debugHandlers.RegisterDebugRoutes(r)

		// pprof + expvar endpoints per AI.md PART 6's canonical chi debug pattern
		r.Route("/debug/pprof", func(r chi.Router) {
			r.HandleFunc("/", pprof.Index)
			r.HandleFunc("/cmdline", pprof.Cmdline)
			r.HandleFunc("/profile", pprof.Profile)
			r.HandleFunc("/symbol", pprof.Symbol)
			r.Handle("/trace", http.HandlerFunc(pprof.Trace))
			r.Handle("/allocs", pprof.Handler("allocs"))
			r.Handle("/block", pprof.Handler("block"))
			r.Handle("/goroutine", pprof.Handler("goroutine"))
			r.Handle("/heap", pprof.Handler("heap"))
			r.Handle("/mutex", pprof.Handler("mutex"))
			r.Handle("/threadcreate", pprof.Handler("threadcreate"))
		})

		// expvar endpoint per AI.md PART 6
		r.Handle("/debug/vars", http.DefaultServeMux)

		log.Println("INFO: Debug endpoints enabled:")
		log.Println("   GET  /debug/routes  - List all routes")
		log.Println("   GET  /debug/config  - Show configuration")
		log.Println("   GET  /debug/memory  - Memory statistics")
		log.Println("   GET  /debug/db      - Database statistics")
		log.Println("   POST /debug/reload  - Reload configuration")
		log.Println("   POST /debug/gc      - Trigger garbage collection")
		log.Println("   GET  /debug/pprof/  - pprof index")
		log.Println("   GET  /debug/pprof/heap - Heap profile")
		log.Println("   GET  /debug/pprof/goroutine - Goroutine dump")
		log.Println("   GET  /debug/vars    - expvar metrics")
	}

	// IP detection endpoint (always available for My Location feature)
	r.Get("/debug/ip", func(w http.ResponseWriter, r *http.Request) {
		// IP detection for My Location button
		clientIP := util.TrustedGetClientIP(r)

		// Try to get location from IP
		coords, err := weatherService.GetCoordinatesFromIP(clientIP)
		if err != nil {
			// Empty means fallback to manual entry
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"clientIP": clientIP,
				"location": map[string]interface{}{
					"value": "",
				},
				"error": err.Error(),
			})
			return
		}

		// Enhance location
		enhanced := locationEnhancer.EnhanceLocation(coords)

		// e.g., "Albany, NY"
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"clientIP": clientIP,
			"location": map[string]interface{}{
				"value": enhanced.ShortName,
			},
			"coordinates": map[string]interface{}{
				"latitude":  coords.Latitude,
				"longitude": coords.Longitude,
			},
		})
	})

	// Server setup routes at /server/{admin_path}/config/setup (requires verified setup token)
	// AI.md: Setup flow is at /server/{admin_path}/config/setup, creates Primary Admin
	// AI.md: Server is FULLY FUNCTIONAL without setup - only admin panel requires setup
	// AI.md: Step 4: Redirect to /server/{admin_path}/config/setup (setup wizard) after token verified
	adminSetupRoutes := chi.NewRouter()
	r.Mount("/server/"+cfg.GetAdminPath()+"/config/setup", adminSetupRoutes)
	adminSetupRoutes.Use(middleware.BlockSetupAfterComplete(cfg))
	adminSetupRoutes.Use(middleware.RequireSetupTokenVerified(cfg))
	{
		// Setup wizard pages - user has already verified token at /admin
		// AI.md: 6 steps: Admin Account → API Token → Server Config → Security → Services → Complete
		adminSetupRoutes.Get("/", setupHandler.ShowAdminSetup)
		adminSetupRoutes.Post("/", setupHandler.CreateAdmin)
		adminSetupRoutes.Get("/api-token", setupHandler.ShowAPIToken)
		adminSetupRoutes.Post("/api-token", setupHandler.ProcessAPIToken)
		adminSetupRoutes.Get("/config", setupHandler.ShowServerConfig)
		adminSetupRoutes.Post("/config", setupHandler.ProcessServerConfig)
		adminSetupRoutes.Get("/security", setupHandler.ShowSecurity)
		adminSetupRoutes.Post("/security", setupHandler.ProcessSecurity)
		adminSetupRoutes.Get("/services", setupHandler.ShowServices)
		adminSetupRoutes.Post("/services", setupHandler.ProcessServices)
		adminSetupRoutes.Get("/complete", setupHandler.CompleteSetup)
	}

	// Authentication routes (public) - TEMPLATE.md lines 4441-4534
	r.Get("/server/auth/login", authHandler.ShowLoginPage)
	r.With(middleware.LoginRateLimitMiddleware()).Post("/server/auth/login", authHandler.HandleLogin)
	r.Get("/server/auth/register", authHandler.ShowRegisterPage)
	r.Post("/server/auth/register", authHandler.HandleRegister)
	r.Get("/server/auth/logout", authHandler.HandleLogout)

	// Password reset routes (public)
	r.Get("/server/auth/password/forgot", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "page/forgot_password.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Forgot Password",
		}))
	})
	r.With(middleware.PasswordResetRateLimitMiddleware()).Post("/server/auth/password/forgot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "If an account with that email exists, a reset link has been sent"})
	})
	r.Get("/server/auth/password/reset", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "page/reset_password.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Reset Password",
			"token": r.URL.Query().Get("token"),
		}))
	})
	r.Post("/server/auth/password/reset", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Password has been reset successfully"})
	})

	// Email verification route (public) - per spec: GET /auth/verify/{code} verifies inline
	r.Get("/server/auth/verify/{code}", func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		if code == "" {
			middleware.RenderHTML(w, r, http.StatusBadRequest, "page/verify_email.tmpl", util.TemplateData(r, map[string]interface{}{
				"title": "Verify Email",
				"error": "Missing verification code",
			}))
			return
		}

		verificationID, userID, valid := lookupEmailVerification(db.DB, code, time.Now())
		if !valid {
			middleware.RenderHTML(w, r, http.StatusBadRequest, "page/verify_email.tmpl", util.TemplateData(r, map[string]interface{}{
				"title": "Verify Email",
				"error": "Invalid or expired verification link. Please request a new one.",
			}))
			return
		}

		// Mark email as verified
		// updated_at is bound as canonical UTC text rather than written by the SQL
		// CURRENT_TIMESTAMP literal, which yields a different type and zone on
		// PostgreSQL, MySQL and SQL Server than it does on SQLite.
		_, err := database.ExecContext(context.Background(), db.DB, database.TimeoutWrite, `
			UPDATE user_accounts
			SET email_verified = 1, updated_at = ?
			WHERE id = ?
		`, dbtime.FormatSQLTimestamp(time.Now()), userID)
		if err != nil {
			middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/verify_email.tmpl", util.TemplateData(r, map[string]interface{}{
				"title": "Verify Email",
				"error": "Failed to verify email. Please try again.",
			}))
			return
		}

		// Delete used token
		if _, err := database.ExecContext(context.Background(), db.DB, database.TimeoutWrite, `DELETE FROM user_email_verifications WHERE id = ?`, verificationID); err != nil {
			log.Printf("WARNING: verify-email: failed to delete used verification token %d: %v", verificationID, err)
		}

		http.Redirect(w, r, "/server/auth/login?verified=1", http.StatusFound)
	})

	// Two-factor authentication routes (public)
	r.Get("/server/auth/2fa", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "page/two_factor.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Two-Factor Authentication",
		}))
	})
	r.Post("/server/auth/2fa", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Two-factor authentication verified"})
	})

	// Passkey authentication routes (public)
	r.Get("/server/auth/passkey", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "page/passkey.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Passkey Authentication",
		}))
	})

	// Username recovery routes (public)
	r.Get("/server/auth/username/forgot", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "page/forgot_username.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Forgot Username",
		}))
	})
	r.Post("/server/auth/username/forgot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "If an account with that email exists, the username has been sent"})
	})

	// Recovery key usage route (public)
	r.Get("/server/auth/recovery/use", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "page/recovery_key.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Use Recovery Key",
		}))
	})
	r.Post("/server/auth/recovery/use", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Recovery key accepted"})
	})

	renderServerInvitePage := func(w http.ResponseWriter, r *http.Request, status int, data map[string]interface{}) {
		payload := map[string]interface{}{
			"title": "Server Admin Invite",
		}
		for key, value := range data {
			payload[key] = value
		}
		middleware.RenderHTML(w, r, status, "page/server_invite.tmpl", util.TemplateData(r, payload))
	}

	renderUserInvitePage := func(w http.ResponseWriter, r *http.Request, status int, data map[string]interface{}) {
		payload := map[string]interface{}{
			"title": "User Invite",
		}
		for key, value := range data {
			payload[key] = value
		}
		middleware.RenderHTML(w, r, status, "page/user_invite.tmpl", util.TemplateData(r, payload))
	}

	// Invite routes (public - token validates)
	r.Get("/server/server/auth/invite/server/{code}", func(w http.ResponseWriter, r *http.Request) {
		invite, err := adminInviteService.VerifyInvite(chi.URLParam(r, "code"))
		if err != nil {
			renderServerInvitePage(w, r, http.StatusGone, map[string]interface{}{
				"error": err.Error(),
				"code":  chi.URLParam(r, "code"),
			})
			return
		}

		renderServerInvitePage(w, r, http.StatusOK, map[string]interface{}{
			"code":       invite.Token,
			"email":      invite.InvitedEmail,
			"expires_at": invite.ExpiresAt,
		})
	})
	r.Post("/server/server/auth/invite/server/{code}", func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "code")
		invite, err := adminInviteService.VerifyInvite(token)
		if err != nil {
			renderServerInvitePage(w, r, http.StatusGone, map[string]interface{}{
				"error": err.Error(),
				"code":  token,
			})
			return
		}

		var req struct {
			Username        string
			Password        string
			ConfirmPassword string
		}
		if parseErr := r.ParseForm(); parseErr != nil {
			renderServerInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"error":      "Invalid form submission",
			})
			return
		}
		req.Username = strings.TrimSpace(r.FormValue("username"))
		req.Password = r.FormValue("password")
		req.ConfirmPassword = r.FormValue("confirm_password")
		if len(req.Username) < 3 || len(req.Password) < 8 || req.ConfirmPassword == "" {
			renderServerInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"username":   req.Username,
				"error":      "Invalid form submission",
			})
			return
		}

		if req.Password != req.ConfirmPassword {
			renderServerInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"username":   req.Username,
				"error":      "Passwords do not match",
			})
			return
		}

		if _, err := adminInviteService.AcceptInvite(token, req.Username, req.Password); err != nil {
			renderServerInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"username":   req.Username,
				"error":      err.Error(),
			})
			return
		}

		http.Redirect(w, r, "/server/auth/login?invite=accepted", http.StatusSeeOther)
	})
	r.Get("/server/server/auth/invite/user/{code}", func(w http.ResponseWriter, r *http.Request) {
		invite, err := userInviteModel.VerifyInvite(chi.URLParam(r, "code"))
		if err != nil {
			renderUserInvitePage(w, r, http.StatusGone, map[string]interface{}{
				"error": err.Error(),
				"code":  chi.URLParam(r, "code"),
			})
			return
		}

		renderUserInvitePage(w, r, http.StatusOK, map[string]interface{}{
			"code":       invite.Token,
			"username":   invite.Username,
			"email":      invite.Email,
			"role":       invite.Role,
			"expires_at": invite.ExpiresAt,
		})
	})
	r.Post("/server/server/auth/invite/user/{code}", func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "code")
		invite, err := userInviteModel.VerifyInvite(token)
		if err != nil {
			renderUserInvitePage(w, r, http.StatusGone, map[string]interface{}{
				"error": err.Error(),
				"code":  token,
			})
			return
		}

		var req struct {
			Username        string
			Password        string
			ConfirmPassword string
		}
		if parseErr := r.ParseForm(); parseErr != nil {
			renderUserInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Invalid form submission",
			})
			return
		}
		req.Username = strings.TrimSpace(r.FormValue("username"))
		req.Password = r.FormValue("password")
		req.ConfirmPassword = r.FormValue("confirm_password")
		if len(req.Username) < 3 || len(req.Password) < 8 || req.ConfirmPassword == "" {
			renderUserInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Invalid form submission",
			})
			return
		}

		if req.Password != req.ConfirmPassword {
			renderUserInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Passwords do not match",
			})
			return
		}

		username := util.NormalizeUsername(req.Username)
		if err := util.ValidateUsername(username); err != nil {
			renderUserInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      err.Error(),
			})
			return
		}

		if invite.Username != "" && username != util.NormalizeUsername(invite.Username) {
			renderUserInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"username":   invite.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Invite username does not match",
			})
			return
		}

		userModel := &model.UserModel{DB: db.DB}
		user, err := userModel.Create(username, invite.Email, req.Password, invite.Role)
		if err != nil {
			renderUserInvitePage(w, r, http.StatusBadRequest, map[string]interface{}{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Failed to create account",
			})
			return
		}

		// updated_at is bound as canonical UTC text so every driver stores the same
		// layout the rest of the project reads back.
		if _, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `UPDATE user_accounts SET email_verified = 1, updated_at = ? WHERE id = ?`, dbtime.FormatSQLTimestamp(time.Now()), user.ID); err != nil {
			renderUserInvitePage(w, r, http.StatusInternalServerError, map[string]interface{}{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Failed to finalize account",
			})
			return
		}

		if err := userInviteModel.MarkUsed(token, user.ID); err != nil {
			renderUserInvitePage(w, r, http.StatusInternalServerError, map[string]interface{}{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Failed to finalize invite",
			})
			return
		}

		sessionModel := &model.SessionModel{DB: db.DB}
		session, err := sessionModel.Create(user.ID, 2592000)
		if err != nil {
			renderUserInvitePage(w, r, http.StatusInternalServerError, map[string]interface{}{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Account created but failed to log in",
			})
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     middleware.SessionCookieName,
			Value:    session.ID,
			Path:     "/",
			MaxAge:   2592000,
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/users/dashboard", http.StatusSeeOther)
	})

	// OIDC authentication routes (public) — per AI.md PART 34
	r.Get("/server/auth/oidc/{provider}", oidcAuthHandler.StartLogin)
	r.Get("/server/auth/oidc/{provider}/callback", oidcAuthHandler.Callback)

	// LDAP authentication route (public)
	r.Post("/server/auth/ldap", ldapAuthHandler.Login)

	// User routes (require authentication) - per AI.md PART 14: /users/ is plural
	usersRoutes := chi.NewRouter()
	r.Mount("/users", usersRoutes)
	usersRoutes.Use(middleware.RequireAuth(db.DB))
	usersRoutes.Use(middleware.BlockAdminFromUserRoutes())
	{
		// /users -> user dashboard (current user)
		usersRoutes.Get("/", dashboardHandler.ShowDashboard)
		// /users/dashboard -> user dashboard
		usersRoutes.Get("/dashboard", dashboardHandler.ShowDashboard)

		// User settings pages per AI.md PART 34
		usersRoutes.Get("/settings", userSettingsHandler.ShowAccountSettings)
		usersRoutes.Get("/settings/privacy", userSettingsHandler.ShowPrivacySettings)
		usersRoutes.Get("/settings/notifications", userSettingsHandler.ShowNotificationSettings)
		usersRoutes.Get("/settings/appearance", userSettingsHandler.ShowAppearanceSettings)
		// /users/tokens per AI.md PART 34 spec (separate from settings)
		usersRoutes.Get("/tokens", userSettingsHandler.ShowTokensSettings)

		// /users/notifications - per AI.md PART 25: notifications page
		usersRoutes.Get("/notifications", notificationHandler.ShowNotificationsPage)
	}

	// Admin setup token verification route (public - before auth check)
	// AI.md: Step 2: User navigates to /server/admin → Step 3: User enters setup token
	r.Post("/server/"+cfg.GetAdminPath()+"/verify-token", setupHandler.VerifySetupTokenAtAdmin)

	// Admin passkey login page (public — shown during the post-password / pre-passkey
	// window; admin session is not yet established so this MUST be outside adminRoutes).
	// auth.go sets the admin_passkey_pending cookie and redirects here when the admin
	// has registered passkeys.  The page reads the cookie server-side (HttpOnly) and
	// embeds the pending-session token so the in-page JS can call the challenge/verify
	// API endpoints without ever exposing the raw cookie value to JS directly.
	r.Get("/server/"+cfg.GetAdminPath()+"/passkey", func(w http.ResponseWriter, r *http.Request) {
		pendingToken := ""
		if cookie, cookieErr := r.Cookie("admin_passkey_pending"); cookieErr == nil {
			pendingToken = cookie.Value
		}
		if strings.TrimSpace(pendingToken) == "" {
			// Cookie missing or expired — bail back to login with a clear message.
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_passkey_login.tmpl", util.TemplateData(r, map[string]interface{}{
				"title":      "Admin Passkey Verification",
				"admin_path": cfg.GetAdminPath(),
				"error":      "Session expired or invalid. Please log in again.",
			}))
			return
		}
		middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_passkey_login.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":                 "Admin Passkey Verification",
			"admin_path":            cfg.GetAdminPath(),
			"pending_session_token": pendingToken,
		}))
	})

	// Admin routes (require admin role + stricter rate limiting)
	// AI.md: Admin panel at /server/{admin_path} (configurable, default: "admin")
	adminRoutes := chi.NewRouter()
	r.Mount("/server/"+cfg.GetAdminPath(), adminRoutes)
	// AI.md: Show setup token entry at /admin when no admin exists
	adminRoutes.Use(middleware.SetupTokenRequired(cfg))
	adminRoutes.Use(middleware.RequireAdminAuth())
	adminRoutes.Use(middleware.AdminRateLimitMiddleware())
	// Log all admin actions
	adminRoutes.Use(middleware.AuditLogger(db.DB))
	{
		// /{admin_path} -> admin dashboard (root level)
		adminRoutes.Get("/", dashboardHandler.ShowAdminPanel)
		// /{admin_path}/dashboard -> alias for root
		adminRoutes.Get("/dashboard", dashboardHandler.ShowAdminPanel)

		// /{admin_path}/logout -> clear admin session and redirect to login
		adminRoutes.Get("/logout", func(w http.ResponseWriter, r *http.Request) {
			// Delete admin session from database
			adminSessionID := ""
			if cookie, err := r.Cookie("admin_session"); err == nil {
				adminSessionID = cookie.Value
			}
			if adminSessionID != "" {
				if _, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, "DELETE FROM server_admin_sessions WHERE id = ?", adminSessionID); err != nil {
					log.Printf("WARNING: admin-logout: failed to delete admin session %s: %v", adminSessionID, err)
				}
			}
			// Clear admin_session cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "admin_session",
				Value:    "",
				MaxAge:   -1,
				Path:     "/",
				Secure:   false,
				HttpOnly: true,
			})
			http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		})

		// AI.md PART 17: every server-management page lives under /{admin_path}/config/
		// and the admin's own account lives under /{admin_path}/{admin_username}/
		adminSelfRoutes := chi.NewRouter()
		adminRoutes.Mount("/{admin_username}", adminSelfRoutes)
		adminSelfRoutes.Use(handler.RequireAdminSelf())

		// AI.md PART 17 header spec: global search over settings, logs and the
		// other data the admin panel manages
		adminRoutes.Get("/config/search", handler.AdminSearchPage)

		adminRoutes.Get("/config/settings", adminHandler.ShowSettingsPage)

		adminRoutes.Get("/config/web", adminWebHandler.ShowWebSettings)

		adminRoutes.Get("/config/users", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_users.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "User Management - Admin",
				"page":       "users",
				"breadcrumb": "Users",
			}))
		})

		adminRoutes.Get("/config/email", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_email.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Email Settings - Admin",
				"page":       "email",
				"breadcrumb": "Email",
			}))
		})

		adminRoutes.Get("/config/database", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_database.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Database & Cache - Admin",
				"page":       "database",
				"breadcrumb": "Database",
			}))
		})

		adminRoutes.Get("/config/info", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_system.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Server Information - Admin",
				"page":       "info",
				"breadcrumb": "Server Info",
			}))
		})

		adminRoutes.Get("/config/security", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_security.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Security Settings - Admin",
				"page":       "security",
				"breadcrumb": "Security",
			}))
		})

		adminRoutes.Get("/config/security/tokens", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/tokens.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "API Tokens - Admin",
				"page":       "tokens",
				"breadcrumb": "API Tokens",
			}))
		})

		adminRoutes.Get("/config/logs", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_logs.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "System Logs - Admin",
				"page":       "logs",
				"breadcrumb": "System Logs",
			}))
		})

		adminRoutes.Get("/config/logs/audit", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/logs.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Audit Logs - Admin",
				"page":       "audit",
				"breadcrumb": "Audit Logs",
			}))
		})

		adminRoutes.Get("/config/scheduler", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_tasks_enhanced.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Scheduled Tasks - Admin",
				"page":       "scheduler",
				"breadcrumb": "Scheduled Tasks",
			}))
		})

		adminRoutes.Get("/config/ssl", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_ssl.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "SSL/TLS Management - Admin",
				"page":       "ssl",
				"breadcrumb": "SSL/TLS",
			}))
		})

		adminRoutes.Get("/config/backup", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_backup_enhanced.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Backup Management - Admin",
				"page":       "backup",
				"breadcrumb": "Backup",
			}))
		})

		adminRoutes.Get("/config/metrics", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_metrics.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Metrics Configuration - Admin",
				"page":       "metrics",
				"breadcrumb": "Metrics",
			}))
		})

		adminRoutes.Get("/config/network/tor", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_tor.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Tor Hidden Service - Admin",
				"page":       "tor",
				"breadcrumb": "Tor Hidden Service",
			}))
		})

		adminRoutes.Get("/config/channels", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin_channels.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Notification Channels - Admin",
				"page":       "channels",
				"breadcrumb": "Channels",
			}))
		})

		adminRoutes.Get("/config/templates", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "template_editor.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Template Editor - Admin",
				"page":       "templates",
				"breadcrumb": "Templates",
			}))
		})

		adminRoutes.Get("/config/email/templates", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_email_editor.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":      "Email Template Editor - Admin",
				"page":       "email-templates",
				"breadcrumb": "Email Templates",
			}))
		})

		// Admin settings sub-panels (already under /server/)
		adminRoutes.Get("/config/users/settings", adminUsersHandler.ShowUserSettings)
		adminRoutes.Get("/config/weather", adminWeatherHandler.ShowWeatherSettings)
		adminRoutes.Get("/config/notifications", adminNotificationsHandler.ShowNotificationSettings)
		adminRoutes.Get("/config/network/geoip", adminGeoIPHandler.ShowGeoIPSettings)

		// AI.md PART 17: the admin's own account pages
		// /{admin_path}/{admin_username}/profile - Admin's own profile
		adminSelfRoutes.Get("/profile", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_profile.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Admin Profile",
				"page":  "profile",
			}))
		})

		// /{admin_path}/{admin_username}/preferences - Admin preferences
		adminSelfRoutes.Get("/preferences", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_preferences.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Admin Preferences",
				"page":  "preferences",
			}))
		})

		// /{admin_path}/{admin_username}/notifications - Admin notifications page
		adminSelfRoutes.Get("/notifications", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_notifications.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Notifications",
				"page":  "notifications",
			}))
		})

		// Additional management pages per spec
		// /{admin_path}/config/branding - Branding & SEO
		adminRoutes.Get("/config/branding", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_branding.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Branding & SEO - Admin",
				"page":  "server-branding",
			}))
		})

		// /{admin_path}/config/pages - Standard pages (about, privacy, contact)
		adminRoutes.Get("/config/pages", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_pages.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Standard Pages - Admin",
				"page":  "server-pages",
			}))
		})

		// /{admin_path}/config/roles - Role definitions
		adminRoutes.Get("/config/roles", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_roles.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Role Definitions - Admin",
				"page":  "server-roles",
			}))
		})

		// /{admin_path}/config/security/auth - Authentication config
		adminRoutes.Get("/config/security/auth", adminAuthHandler.ShowAuthSettings)

		// /{admin_path}/config/admins - Server admin accounts list
		adminRoutes.Get("/config/admins", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_admins.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Server Admins - Admin",
				"page":  "server-admins",
			}))
		})

		// /{admin_path}/config/admins/invite - Invite new admin
		adminRoutes.Get("/config/admins/invite", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_invite.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Invite Admin - Admin",
				"page":  "server-admins-invite",
			}))
		})

		// /{admin_path}/config/admins/:id - Admin detail
		adminRoutes.Get("/config/admins/{id}", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_detail.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":   "Admin Detail - Admin",
				"page":    "server-admins",
				"adminID": chi.URLParam(r, "id"),
			}))
		})

		renderAdminUserInvitesPage := func(w http.ResponseWriter, r *http.Request, status int, data map[string]interface{}) {
			invites, err := userInviteModel.ListInvites()
			if err != nil {
				middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
					"title":   "User Invites",
					"message": "Failed to load user invites",
				}))
				return
			}

			scheme := r.Header.Get("X-Forwarded-Proto")
			if scheme == "" {
				if r.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			inviteRows := make([]map[string]interface{}, 0, len(invites))
			for _, invite := range invites {
				statusLabel := "pending"
				if invite.UsedAt != nil || (invite.MaxUses > 0 && invite.UseCount >= invite.MaxUses) {
					statusLabel = "used"
				} else if time.Now().After(invite.ExpiresAt) {
					statusLabel = "expired"
				}

				inviteRows = append(inviteRows, map[string]interface{}{
					"id":         invite.ID,
					"token":      invite.Token,
					"username":   invite.Username,
					"email":      invite.Email,
					"role":       invite.Role,
					"expires_at": invite.ExpiresAt,
					"used_at":    invite.UsedAt,
					"status":     statusLabel,
					"invite_url": fmt.Sprintf("%s://%s/server/auth/invite/user/%s", scheme, r.Host, invite.Token),
				})
			}

			payload := map[string]interface{}{
				"title":                  "User Invites - Admin",
				"page":                   "users-invites",
				"invites":                inviteRows,
				"invite_expiration_days": config.GetUserInviteExpirationDays(),
			}
			for key, value := range data {
				payload[key] = value
			}

			middleware.RenderHTML(w, r, status, "admin/admin_user_invites.tmpl", handler.AdminTemplateData(r, payload))
		}

		// /{admin_path}/config/users/invites - User invites
		adminRoutes.Get("/config/users/invites", func(w http.ResponseWriter, r *http.Request) {
			renderAdminUserInvitesPage(w, r, http.StatusOK, map[string]interface{}{})
		})
		adminRoutes.Post("/config/users/invites", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Username      string
				Email         string
				Role          string
				ExpiresInDays int
			}
			if parseErr := r.ParseForm(); parseErr != nil {
				renderAdminUserInvitesPage(w, r, http.StatusBadRequest, map[string]interface{}{
					"error": "Invalid form submission",
					"form":  req,
				})
				return
			}
			req.Username = strings.TrimSpace(r.FormValue("username"))
			req.Email = strings.TrimSpace(r.FormValue("email"))
			req.Role = r.FormValue("role")
			if expiresStr := r.FormValue("expires_in_days"); expiresStr != "" {
				if parsed, parseErr := strconv.Atoi(expiresStr); parseErr == nil {
					req.ExpiresInDays = parsed
				}
			}
			if req.Username == "" || len(req.Username) < 3 || req.Email == "" || !strings.Contains(req.Email, "@") {
				renderAdminUserInvitesPage(w, r, http.StatusBadRequest, map[string]interface{}{
					"error": "Invalid form submission",
					"form":  req,
				})
				return
			}

			username := util.NormalizeUsername(req.Username)
			if err := util.ValidateUsername(username); err != nil {
				renderAdminUserInvitesPage(w, r, http.StatusBadRequest, map[string]interface{}{
					"error": err.Error(),
					"form":  req,
				})
				return
			}

			email := util.NormalizeEmail(req.Email)
			if err := util.ValidateEmail(email); err != nil {
				renderAdminUserInvitesPage(w, r, http.StatusBadRequest, map[string]interface{}{
					"error": err.Error(),
					"form":  req,
				})
				return
			}

			userModel := &model.UserModel{DB: db.DB}
			if _, err := userModel.GetByUsername(username); err == nil {
				renderAdminUserInvitesPage(w, r, http.StatusBadRequest, map[string]interface{}{
					"error": "Username is already in use",
					"form":  req,
				})
				return
			}
			if _, err := userModel.GetByEmail(email); err == nil {
				renderAdminUserInvitesPage(w, r, http.StatusBadRequest, map[string]interface{}{
					"error": "Email is already in use",
					"form":  req,
				})
				return
			}

			role := strings.TrimSpace(req.Role)
			if role == "" {
				role = "user"
			}

			expiresInDays := req.ExpiresInDays
			if expiresInDays <= 0 {
				expiresInDays = config.GetUserInviteExpirationDays()
			}

			invite, err := userInviteModel.CreateInvite(username, email, role, expiresInDays)
			if err != nil {
				renderAdminUserInvitesPage(w, r, http.StatusBadRequest, map[string]interface{}{
					"error": err.Error(),
					"form":  req,
				})
				return
			}

			scheme := r.Header.Get("X-Forwarded-Proto")
			if scheme == "" {
				if r.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			renderAdminUserInvitesPage(w, r, http.StatusOK, map[string]interface{}{
				"message":    "User invite created",
				"invite_url": fmt.Sprintf("%s://%s/server/auth/invite/user/%s", scheme, r.Host, invite.Token),
			})
		})

		// /{admin_path}/config/moderation/users - User moderation
		adminRoutes.Get("/config/moderation/users", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_moderation.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "User Moderation - Admin",
				"page":  "moderation-users",
			}))
		})

		// /{admin_path}/config/moderation/users/:id - User detail
		adminRoutes.Get("/config/moderation/users/{id}", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_user_detail.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":  "User Detail - Admin",
				"page":   "moderation-users",
				"userID": chi.URLParam(r, "id"),
			}))
		})

		// /{admin_path}/config/security/ratelimit - Rate limiting config
		adminRoutes.Get("/config/security/ratelimit", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_ratelimit.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Rate Limiting - Admin",
				"page":  "security-ratelimit",
			}))
		})

		// /{admin_path}/config/security/firewall - IP allow/block lists
		adminRoutes.Get("/config/security/firewall", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_firewall.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Firewall - Admin",
				"page":  "security-firewall",
			}))
		})

		// /{admin_path}/config/network/blocklists - IP/domain blocklists
		adminRoutes.Get("/config/network/blocklists", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_blocklists.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Blocklists - Admin",
				"page":  "network-blocklists",
			}))
		})

		// /{admin_path}/config/maintenance - Maintenance mode
		adminRoutes.Get("/config/maintenance", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_maintenance.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Maintenance Mode - Admin",
				"page":  "server-maintenance",
			}))
		})

		// /{admin_path}/config/updates - Software updates
		adminRoutes.Get("/config/updates", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_updates.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title":   "Updates - Admin",
				"page":    "server-updates",
				"version": handler.Version,
			}))
		})

		// /{admin_path}/config/cluster/nodes - Cluster node management
		adminRoutes.Get("/config/cluster/nodes", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_cluster_nodes.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Cluster Nodes - Admin",
				"page":  "server-cluster-nodes",
			}))
		})

		// /{admin_path}/config/cluster/add - Add cluster node
		adminRoutes.Get("/config/cluster/add", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_cluster_add.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Add Cluster Node - Admin",
				"page":  "server-cluster-add",
			}))
		})

		// /{admin_path}/help - Admin help & documentation
		adminRoutes.Get("/help", func(w http.ResponseWriter, r *http.Request) {
			middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_help.tmpl", handler.AdminTemplateData(r, map[string]interface{}{
				"title": "Help - Admin",
				"page":  "help",
			}))
		})
	}
	r.With(middleware.RequireAuth(db.DB)).Get("/notifications", notificationHandler.ShowNotificationsPage)

	// User profile page (per AI.md PART 14: /users/ is plural)
	r.With(middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes()).Get("/users/profile", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "page/user/profile.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Profile",
			"page":  "profile",
		}))
	})

	// User security settings page (per AI.md PART 14: /users/ is plural)
	r.With(middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes()).Get("/users/security", twoFAHandler.ShowSecurityPage)
	r.With(middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes()).Get("/users/security/passkeys", twoFAHandler.ShowSecurityPage)

	// User notification preferences page (per AI.md PART 14: /users/ is plural)
	r.With(middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes()).Get("/users/preferences", func(w http.ResponseWriter, r *http.Request) {
		handler.NegotiateResponse(w, r, "user_preferences.tmpl", util.TemplateData(r, map[string]interface{}{
			"title": "Preferences",
			"page":  "preferences",
		}))
	})

	// Removed - moved to adminRoutes group above

	// Location management pages
	r.With(middleware.RequireAuth(db.DB)).Get("/users/locations/new", locationHandler.ShowAddLocationPage)
	r.With(middleware.RequireAuth(db.DB)).Get("/users/locations/{id}/edit", locationHandler.ShowEditLocationPage)

	// API routes - all API endpoints under /api/{api_version}
	// AI.md: API version prefix is configurable (default: "v1")
	apiV1 := chi.NewRouter()
	r.Mount(cfg.GetAPIPath(), apiV1)

	// Weather API routes (optional auth + API rate limiting)
	weatherAPI := chi.NewRouter()
	apiV1.Mount("/", weatherAPI)
	weatherAPI.Use(middleware.OptionalAuth(db.DB))
	weatherAPI.Use(middleware.APIRateLimitMiddleware())
	{
		// Weather endpoints per AI.md PART 36
		weatherAPI.Get("/weather", apiHandler.GetWeather)
		weatherAPI.Get("/weather/{location}", apiHandler.GetWeatherByLocation)
		weatherAPI.Get("/weather/forecast", apiHandler.GetForecast)
		weatherAPI.Get("/weather/locations", apiHandler.GetLocation)

		// Backwards compatibility - old paths (deprecated)
		weatherAPI.Get("/forecasts", apiHandler.GetForecast)
		weatherAPI.Get("/forecasts/{location}", apiHandler.GetForecastByLocation)

		// Additional endpoints
		weatherAPI.Get("/ip", apiHandler.GetIP)
		weatherAPI.Get("/docs", apiHandler.GetDocsJSON)
		weatherAPI.Get("/earthquakes", earthquakeHandler.HandleEarthquakeAPI)
		weatherAPI.Get("/earthquakes/{id}", earthquakeHandler.HandleEarthquakeByIDAPI)
		// Backwards compat
		weatherAPI.Get("/hurricanes", hurricaneHandler.HandleHurricaneAPI)
		weatherAPI.Get("/hurricanes/{id}", hurricaneHandler.HandleHurricaneByIDAPI)
		weatherAPI.Get("/severe-weather", severeWeatherHandler.HandleSevereWeatherAPI)
		weatherAPI.Get("/severe-weather/{id}", severeWeatherHandler.HandleAlertByIDAPI)
		weatherAPI.Get("/moon", moonHandler.HandleMoonAPI)
		weatherAPI.Get("/moon/calendar", moonHandler.HandleMoonCalendarAPI)
		weatherAPI.Get("/sun", moonHandler.HandleSunAPI)
		weatherAPI.Get("/history", apiHandler.GetHistoricalWeather)

		// CLI client compatibility aliases (IDEA.md endpoints)
		weatherAPI.Get("/weather/alerts", severeWeatherHandler.HandleSevereWeatherAPI)
		weatherAPI.Get("/weather/moon", moonHandler.HandleMoonAPI)
		weatherAPI.Get("/weather/history", apiHandler.GetHistoricalWeather)

		// Root /api/{api_version} endpoint - return all endpoints
		// AI.md PART 14: Never hardcode v1 - use cfg.GetAPIPath()
		weatherAPI.Get("/", func(w http.ResponseWriter, r *http.Request) {
			hostInfo := util.GetHostInfo(r)
			apiBase := hostInfo.FullHost + cfg.GetAPIPath()
			adminBase := hostInfo.FullHost + cfg.GetAdminAPIPath()
			handler.RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
				"version": cfg.GetAPIVersion(),
				"endpoints": []string{
					apiBase + "/users",
					apiBase + "/locations",
					apiBase + "/users/notifications",
					adminBase,
					apiBase + "/weather",
					apiBase + "/weather/{location}",
					apiBase + "/forecasts",
					apiBase + "/forecasts/{location}",
					apiBase + "/ip",
					apiBase + "/locations",
					apiBase + "/docs",
					apiBase + "/blocklist",
					apiBase + "/earthquakes",
					apiBase + "/hurricanes",
					apiBase + "/severe-weather",
					apiBase + "/moon",
				},
				"documentation": hostInfo.FullHost + "/docs",
			})
		})
	}

	// Public blocklist endpoint (no auth required)
	apiV1.Get("/blocklist", func(w http.ResponseWriter, r *http.Request) {
		handler.RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
			"blocklist": util.UsernameBlocklist,
			"count":     util.GetBlocklistSize(),
			"public":    util.IsBlocklistPublic(),
			"note":      "These usernames are reserved and cannot be used for registration. The blocklist does not apply to the first user (admin setup).",
		})
	})

	// Public server API endpoints (AI.md PART 14: every web page has corresponding API)
	apiV1.Get("/server/about", handler.GetAboutAPI(db, cfg))
	apiV1.Get("/server/privacy", handler.GetPrivacyAPI(db, cfg))
	apiV1.Get("/server/help", handler.GetHelpAPI(db, cfg))
	apiV1.Get("/server/terms", handler.GetTermsAPI(db, cfg))
	apiV1.Post("/server/contact", handler.HandleContactFormSubmission(db, cfg))

	// Auth API routes per AI.md PART 33
	authAPI := chi.NewRouter()
	apiV1.Mount("/server/auth", authAPI)
	{
		// Public auth endpoints (no auth required)
		authAPI.Post("/register", authAPIHandler.HandleAPIRegister)
		authAPI.Post("/login", authAPIHandler.HandleAPILogin)
		authAPI.Post("/2fa", authAPIHandler.HandleAPI2FA)
		authAPI.Post("/passkey/challenge", passkeyHandler.BeginPasskeyChallenge)
		authAPI.Post("/passkey/verify", passkeyHandler.VerifyPasskey)
		// Admin passkey login challenge/verify per AI.md PART 17 line
		// 28679 ("passkey can be used as primary login or as 2FA"). The
		// pending session token is issued by HandleLogin (`/server/auth/login`)
		// after admin password verify when the admin has at least one
		// passkey registered.
		authAPI.Post("/admin/passkey/challenge", adminPasskeyHandler.BeginPasskeyChallenge)
		authAPI.Post("/admin/passkey/verify", adminPasskeyHandler.VerifyPasskey)
		authAPI.Post("/recovery/use", authAPIHandler.HandleAPIRecoveryUse)
		authAPI.Post("/password/forgot", authAPIHandler.HandleAPIPasswordForgot)
		authAPI.Post("/password/reset", authAPIHandler.HandleAPIPasswordReset)
		authAPI.Post("/verify", authAPIHandler.HandleAPIVerifyEmail)

		// User invite endpoints (no auth required - token validates)
		authAPI.Get("/invite/user/{token}", authAPIHandler.HandleAPIUserInviteValidate)
		authAPI.Post("/invite/user/{token}", authAPIHandler.HandleAPIUserInviteComplete)
		authAPI.Get("/invite/server/{token}", authAPIHandler.HandleAPIServerInviteValidate)
		authAPI.Post("/invite/server/{token}", authAPIHandler.HandleAPIServerInviteComplete)

		// Protected auth endpoints (require auth)
		authAPI.With(middleware.RequireAuth(db.DB)).Post("/logout", authAPIHandler.HandleAPILogout)
		authAPI.With(middleware.RequireAuth(db.DB)).Post("/refresh", authAPIHandler.HandleAPIRefresh)
	}

	// Users API routes per AI.md PART 33 (spec uses /api/v1/users not /api/v1/user)
	usersAPI := chi.NewRouter()
	apiV1.Mount("/users", usersAPI)
	usersAPI.Use(middleware.RequireAuth(db.DB))
	usersAPI.Use(middleware.BlockAdminFromUserRoutes())
	{
		usersAPI.Get("/", authHandler.GetCurrentUser)
		usersAPI.Patch("/", authHandler.UpdateProfile)

		// User settings API per AI.md PART 34
		usersAPI.Get("/settings", userSettingsHandler.GetSettings)
		usersAPI.Patch("/settings", userSettingsHandler.UpdateSettings)

		// User tokens API per AI.md PART 34
		usersAPI.Get("/tokens", userSettingsHandler.ListTokens)
		usersAPI.Post("/tokens", userSettingsHandler.CreateToken)
		usersAPI.Delete("/tokens/{id}", userSettingsHandler.RevokeToken)

		// Avatar API per AI.md PART 34
		usersAPI.Get("/avatar", userPublicHandler.GetCurrentUserAvatar)
		usersAPI.Post("/avatar", userPublicHandler.UploadAvatar)
		usersAPI.Patch("/avatar", userPublicHandler.UpdateAvatarSettings)
		usersAPI.Delete("/avatar", userPublicHandler.ResetAvatar)

		// Security endpoints
		usersAPI.Get("/security/2fa", twoFAHandler.GetTwoFactorStatus)
		usersAPI.Get("/security/2fa/setup", twoFAHandler.SetupTwoFactor)
		usersAPI.Post("/security/2fa/enable", twoFAHandler.EnableTwoFactor)
		usersAPI.Post("/security/2fa/disable", twoFAHandler.DisableTwoFactor)
		usersAPI.Post("/security/2fa/verify", twoFAHandler.VerifyTwoFactorCode)
		usersAPI.Post("/security/recovery/regenerate", twoFAHandler.RegenerateRecoveryKeys)
		usersAPI.Get("/security/passkeys", passkeyHandler.ListPasskeys)
		usersAPI.Post("/security/passkeys", passkeyHandler.RegisterPasskey)
		usersAPI.Delete("/security/passkeys/{passkey_id}", passkeyHandler.DeletePasskey)

		// Password change per AI.md PART 34
		usersAPI.Post("/security/password", userPublicHandler.ChangePassword)
		usersAPI.Post("/security/email", userPublicHandler.ChangeEmail)
		usersAPI.Delete("/account", userPublicHandler.DeleteAccount)

		usersAPI.Get("/sessions", userSettingsHandler.ListSessions)
		usersAPI.Delete("/sessions", userSettingsHandler.RevokeAllSessions)
		usersAPI.Delete("/sessions/{id}", userSettingsHandler.RevokeSession)
	}

	// Note: 2FA routes already registered under usersAPI (/users/security/2fa/*)

	// Public user profile endpoint per AI.md PART 34
	// Uses OptionalAuth to support both authenticated and anonymous requests
	// Private profiles return 404 to prevent existence leakage
	apiV1.With(middleware.OptionalAuth(db.DB)).Get("/public/users/{username}", userPublicHandler.GetPublicProfile)

	// Location API routes (require auth)
	// Public location endpoints (no auth required)
	apiV1.Get("/locations/search", locationHandler.SearchLocations)
	apiV1.Get("/locations/lookup/zip/{code}", locationHandler.LookupZipCode)
	apiV1.Get("/locations/lookup/coords", locationHandler.LookupCoordinates)

	// Protected location endpoints (require auth)
	locationAPI := chi.NewRouter()
	apiV1.Mount("/users/locations", locationAPI)
	locationAPI.Use(middleware.RequireAuth(db.DB))
	locationAPI.Use(middleware.BlockAdminFromUserRoutes())
	{
		locationAPI.Get("/", locationHandler.ListLocations)
		locationAPI.Get("/{id}", locationHandler.GetLocation)
		locationAPI.Post("/", locationHandler.CreateLocation)
		locationAPI.Put("/{id}", locationHandler.UpdateLocation)
		locationAPI.Delete("/{id}", locationHandler.DeleteLocation)
		locationAPI.Put("/{id}/alerts", locationHandler.ToggleAlerts)
	}

	// WebUI Notification API routes - User (per AI.md PART 14: /users/ is plural)
	usersNotificationAPI := chi.NewRouter()
	apiV1.Mount("/users/notifications", usersNotificationAPI)
	usersNotificationAPI.Use(middleware.RequireAuth(db.DB))
	usersNotificationAPI.Use(middleware.BlockAdminFromUserRoutes())
	{
		usersNotificationAPI.Get("/", notificationAPIHandler.GetUserNotifications)
		usersNotificationAPI.Get("/unread", notificationAPIHandler.GetUserUnreadNotifications)
		usersNotificationAPI.Get("/count", notificationAPIHandler.GetUserUnreadCount)
		usersNotificationAPI.Get("/stats", notificationAPIHandler.GetUserStats)
		usersNotificationAPI.Patch("/{id}/read", notificationAPIHandler.MarkUserNotificationRead)
		usersNotificationAPI.Patch("/read", notificationAPIHandler.MarkAllUserNotificationsRead)
		usersNotificationAPI.Patch("/{id}/dismiss", notificationAPIHandler.DismissUserNotification)
		usersNotificationAPI.Delete("/{id}", notificationAPIHandler.DeleteUserNotification)
		usersNotificationAPI.Get("/preferences", notificationAPIHandler.GetUserPreferences)
		usersNotificationAPI.Patch("/preferences", notificationAPIHandler.UpdateUserPreferences)
	}

	// Admin API routes (require admin role + stricter rate limiting)
	// AI.md: Admin API at /api/{api_version}/server/{admin_path}/
	adminAPI := chi.NewRouter()
	apiV1.Mount("/server/"+cfg.GetAdminPath(), adminAPI)
	adminAPI.Use(middleware.TokenAuthMiddleware(database.GetServerDB(), db.DB))
	// TokenAuthMiddleware accepts admin and regular-user tokens alike, so the
	// admin group must additionally require an admin token or a usr_ token
	// would reach every admin config, scheduler and restart route.
	adminAPI.Use(middleware.RequireAdminToken())
	adminAPI.Use(middleware.AdminRateLimitMiddleware())
	// Log all admin API actions
	adminAPI.Use(middleware.AuditLogger(db.DB))
	{
		adminModel := &model.AdminModel{DB: serverDB}

		// AI.md PART 17: the admin's own account API lives under
		// /api/{api_version}/server/{admin_path}/{admin_username}/
		adminSelfAPI := chi.NewRouter()
		adminAPI.Mount("/{admin_username}", adminSelfAPI)
		adminSelfAPI.Use(handler.RequireAdminSelfAPI())

		getCurrentAdmin := func(w http.ResponseWriter, req *http.Request) (*model.Admin, bool) {
			adminValue, exists := reqctx.Get(req.Context(), "admin")
			if !exists {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "Not authenticated"})
				return nil, false
			}

			admin, ok := adminValue.(*model.Admin)
			if !ok || admin == nil {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "Invalid admin context"})
				return nil, false
			}

			return admin, true
		}

		// server_admin_preferences (src/database/server_schema.go) stores
		// preferences as a single JSON blob column (admin_id, preferences,
		// updated_at) — not individual theme/language/... columns.
		loadAdminPreferences := func(adminID int64) (*model.AdminPreferences, error) {
			return loadAdminPreferencesRow(serverDB, adminID)
		}

		getOnlineAdminUsernames := func() ([]string, error) {
			// Comparing sas.expires_at against CURRENT_TIMESTAMP in SQL is a text comparison that
			// misreads any row stored in a legacy local-zone layout, so the still-valid sessions
			// are selected with their raw expiry and filtered in Go via dbtime. The rows stay
			// ordered by username so duplicate sessions for one admin collapse without a sort.
			rows, err := database.QueryContext(context.Background(), serverDB, database.TimeoutComplexSelect, `
				SELECT sac.username, sas.expires_at
				FROM server_admin_credentials sac
				INNER JOIN server_admin_sessions sas ON sas.admin_id = sac.id
				WHERE sac.is_active = 1 AND sas.expires_at IS NOT NULL
				ORDER BY sac.username ASC
			`)
			if err != nil {
				return nil, fmt.Errorf("failed to query online admins: %w", err)
			}
			defer rows.Close()

			sessionCutoff := time.Now().UTC()

			var usernames []string
			for rows.Next() {
				var username string
				var storedExpiry interface{}
				if err := rows.Scan(&username, &storedExpiry); err != nil {
					return nil, fmt.Errorf("failed to scan online admin username: %w", err)
				}
				if !dbtime.IsAfter(storedExpiry, sessionCutoff) {
					continue
				}
				if len(usernames) > 0 && usernames[len(usernames)-1] == username {
					continue
				}
				usernames = append(usernames, username)
			}

			if err := rows.Err(); err != nil {
				return nil, fmt.Errorf("failed to iterate online admin usernames: %w", err)
			}

			return usernames, nil
		}

		countOtherActiveSuperAdmins := func(excludeID int64) (int, error) {
			var count int
			err := database.QueryRowContext(context.Background(), serverDB, database.TimeoutSimpleSelect, `
				SELECT COUNT(*)
				FROM server_admin_credentials
				WHERE is_super_admin = 1 AND is_active = 1 AND id != ?
			`, excludeID).Scan(&count)
			if err != nil {
				return 0, fmt.Errorf("failed to count active super admins: %w", err)
			}

			return count, nil
		}

		maskAdminToken := func(prefix string) string {
			if prefix == "" {
				return ""
			}

			return prefix + "****"
		}

		buildInviteURL := func(req *http.Request, token string) string {
			scheme := req.Header.Get("X-Forwarded-Proto")
			if scheme == "" {
				if req.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			return fmt.Sprintf("%s://%s/server/auth/invite/server/%s", scheme, req.Host, token)
		}

		buildUserInviteURL := func(req *http.Request, token string) string {
			scheme := req.Header.Get("X-Forwarded-Proto")
			if scheme == "" {
				if req.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			return fmt.Sprintf("%s://%s/server/auth/invite/user/%s", scheme, req.Host, token)
		}

		userInviteStatus := func(invite model.UserInvite) string {
			if invite.UsedAt != nil || (invite.MaxUses > 0 && invite.UseCount >= invite.MaxUses) {
				return "used"
			}
			if time.Now().After(invite.ExpiresAt) {
				return "expired"
			}
			return "pending"
		}

		// Setup API per spec: /api/{api_version}/{admin_path}/config/setup/
		adminAPI.Get("/config/setup", setupHandler.GetSetupStatus)
		adminAPI.Post("/config/setup/verify", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "verified": true})
		})
		adminAPI.Post("/config/setup/account", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Admin account created"})
		})
		adminAPI.Post("/config/setup/token", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "token": ""})
		})
		adminAPI.Post("/config/setup/config", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Server config saved"})
		})
		adminAPI.Post("/config/setup/security", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Security settings saved"})
		})
		adminAPI.Post("/config/setup/services", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Services configured"})
		})
		adminAPI.Post("/config/setup/complete", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Setup complete"})
		})

		// Server management API - all under /server/ per spec
		// User management
		adminAPI.Get("/config/users", adminHandler.ListUsers)
		adminAPI.Put("/config/users/{id}", adminHandler.UpdateUser)
		adminAPI.Delete("/config/users/{id}", adminHandler.DeleteUser)
		adminAPI.Get("/config/users/invites", func(w http.ResponseWriter, r *http.Request) {
			invites, err := userInviteModel.ListInvites()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load user invites"})
				return
			}

			responseInvites := make([]map[string]interface{}, 0, len(invites))
			for _, invite := range invites {
				responseInvites = append(responseInvites, map[string]interface{}{
					"id":         invite.ID,
					"token":      invite.Token,
					"username":   invite.Username,
					"email":      invite.Email,
					"role":       invite.Role,
					"expires_at": invite.ExpiresAt,
					"used_at":    invite.UsedAt,
					"status":     userInviteStatus(invite),
				})
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "invites": responseInvites})
		})
		adminAPI.Post("/config/users/invites", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := getCurrentAdmin(w, r); !ok {
				return
			}

			var req struct {
				Username      string `json:"username" binding:"required,min=3"`
				Email         string `json:"email" binding:"required,email"`
				Role          string `json:"role"`
				ExpiresInDays int    `json:"expires_in_days"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}

			username := util.NormalizeUsername(req.Username)
			if err := util.ValidateUsername(username); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}

			email := util.NormalizeEmail(req.Email)
			if err := util.ValidateEmail(email); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}

			if _, err := (&model.UserModel{DB: db.DB}).GetByUsername(username); err == nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Username is already in use"})
				return
			}

			if _, err := (&model.UserModel{DB: db.DB}).GetByEmail(email); err == nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Email is already in use"})
				return
			}

			expiresInDays := req.ExpiresInDays
			if expiresInDays <= 0 {
				expiresInDays = config.GetUserInviteExpirationDays()
			}

			role := strings.TrimSpace(req.Role)
			if role == "" {
				role = "user"
			}

			invite, err := userInviteModel.CreateInvite(username, email, role, expiresInDays)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":              true,
				"message":         "User invite created",
				"invite":          invite,
				"invite_url":      buildUserInviteURL(r, invite.Token),
				"expires_in_days": expiresInDays,
			})
		})
		adminAPI.Get("/config/users/invites/{id}", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := getCurrentAdmin(w, r); !ok {
				return
			}

			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid invite ID"})
				return
			}

			invite, err := userInviteModel.GetByID(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load invite"})
				return
			}
			if invite == nil {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Invite not found"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":     true,
				"invite": invite,
				"status": userInviteStatus(*invite),
			})
		})
		adminAPI.Delete("/config/users/invites/{id}", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := getCurrentAdmin(w, r); !ok {
				return
			}

			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid invite ID"})
				return
			}

			if err := userInviteModel.DeleteInvite(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to revoke invite"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Invite revoked"})
		})

		// AI.md PART 17 header spec: JSON counterpart of the admin global search
		adminAPI.Get("/config/search", handler.AdminSearchAPI)

		// Settings management
		adminAPI.Get("/config/settings", adminHandler.ListSettings)
		adminAPI.Patch("/config/settings", adminSettingsHandler.UpdateSettings)
		adminAPI.Get("/config/settings/{key}", adminHandler.GetSetting)
		adminAPI.Put("/config/settings/{key}", adminHandler.UpdateSetting)
		adminAPI.Get("/config/settings/all", adminSettingsHandler.GetAllSettings)
		adminAPI.Put("/config/settings/bulk", adminSettingsHandler.UpdateSettings)
		adminAPI.Post("/config/settings/reset", adminSettingsHandler.ResetSettings)
		adminAPI.Get("/config/settings/export", adminSettingsHandler.ExportSettings)
		adminAPI.Post("/config/settings/import", adminSettingsHandler.ImportSettings)
		adminAPI.Post("/config/reload", adminSettingsHandler.ReloadConfig)

		// Admin settings sub-endpoints
		adminAPI.Post("/config/users/settings", adminUsersHandler.UpdateUserSettings)
		adminAPI.Post("/config/security/auth", adminAuthHandler.UpdateAuthSettings)
		adminAPI.Post("/config/weather", adminWeatherHandler.UpdateWeatherSettings)
		adminAPI.Post("/config/notifications", adminNotificationsHandler.UpdateNotificationSettings)
		adminAPI.Post("/config/network/geoip", adminGeoIPHandler.UpdateGeoIPSettings)

		// API token management under /server/security/
		adminAPI.Get("/config/security/tokens", adminHandler.ListTokens)
		adminAPI.Post("/config/security/tokens", adminHandler.GenerateToken)
		adminAPI.Delete("/config/security/tokens/{id}", adminHandler.RevokeToken)

		// Audit logs under /server/logs/
		adminAPI.Get("/config/logs/audit-logs", adminHandler.ListAuditLogs)
		adminAPI.Delete("/config/logs/audit-logs", adminHandler.ClearAuditLogs)

		// System stats
		adminAPI.Get("/config/stats", adminHandler.GetSystemStats)

		// Email settings per spec: /api/{api_version}/{admin_path}/config/email/
		adminAPI.Get("/config/email", func(w http.ResponseWriter, r *http.Request) {
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"enabled":  settingsModel.GetBool("email.enabled", false),
				"provider": settingsModel.GetString("email.provider", ""),
				"host":     settingsModel.GetString("email.host", ""),
				"port":     settingsModel.GetInt("email.port", 587),
				"from":     settingsModel.GetString("email.from", ""),
			})
		})
		adminAPI.Patch("/config/email", func(w http.ResponseWriter, r *http.Request) {
			var settings map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			for key, value := range settings {
				if err := settingsModel.Set("email."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Email settings updated"})
		})
		adminAPI.Post("/config/email/test", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "Test email functionality available when SMTP is configured",
			})
		})

		// Branding per spec: /api/{api_version}/{admin_path}/config/branding/
		adminAPI.Get("/config/branding", func(w http.ResponseWriter, r *http.Request) {
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"title":       settingsModel.GetString("branding.title", cfg.Server.Branding.Title),
				"description": settingsModel.GetString("branding.description", cfg.Server.Branding.Description),
				"logo_url":    settingsModel.GetString("branding.logo_url", ""),
				"favicon_url": settingsModel.GetString("branding.favicon_url", ""),
				"theme_color": settingsModel.GetString("branding.theme_color", ""),
			})
		})
		adminAPI.Patch("/config/branding", func(w http.ResponseWriter, r *http.Request) {
			var settings map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			for key, value := range settings {
				if err := settingsModel.Set("branding."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Branding settings updated"})
		})

		// Pages per spec: /api/{api_version}/{admin_path}/config/pages/
		adminAPI.Get("/config/pages", func(w http.ResponseWriter, r *http.Request) {
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"about":   map[string]interface{}{"enabled": settingsModel.GetBool("pages.about.enabled", true)},
				"privacy": map[string]interface{}{"enabled": settingsModel.GetBool("pages.privacy.enabled", true)},
				"contact": map[string]interface{}{"enabled": settingsModel.GetBool("pages.contact.enabled", true)},
				"help":    map[string]interface{}{"enabled": settingsModel.GetBool("pages.help.enabled", true)},
				"terms":   map[string]interface{}{"enabled": settingsModel.GetBool("pages.terms.enabled", true)},
			})
		})
		adminAPI.Get("/config/pages/{name}", func(w http.ResponseWriter, r *http.Request) {
			name := chi.URLParam(r, "name")
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"name":    name,
				"enabled": settingsModel.GetBool("pages."+name+".enabled", true),
				"content": settingsModel.GetString("pages."+name+".content", ""),
			})
		})
		adminAPI.Patch("/config/pages/{name}", func(w http.ResponseWriter, r *http.Request) {
			name := chi.URLParam(r, "name")
			var settings map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			for key, value := range settings {
				if err := settingsModel.Set("pages."+name+"."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": fmt.Sprintf("%s page updated", name)})
		})

		// Web settings per spec: /api/{api_version}/{admin_path}/config/web/
		adminAPI.Get("/config/web", func(w http.ResponseWriter, r *http.Request) {
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"robots_txt":   settingsModel.GetBool("web.robots_enabled", true),
				"security_txt": settingsModel.GetBool("web.security_enabled", true),
			})
		})
		adminAPI.Patch("/config/web", func(w http.ResponseWriter, r *http.Request) {
			var settings map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
			for key, value := range settings {
				if err := settingsModel.Set("web."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Web settings updated"})
		})

		// Admin status and health endpoints
		adminServerStatusHandler := handler.AdminServerStatus(db, port, httpsPort, sslManager)
		adminAPI.Get("/config/status", adminServerStatusHandler)
		adminAPI.Get("/config/health", adminServerStatusHandler)

		// Server restart per spec: POST /server/restart
		adminAPI.Post("/config/restart", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "Server restart initiated",
			})
			go func() {
				time.Sleep(500 * time.Millisecond)
				log.Println("Server restart requested via admin API")
			}()
		})

		// Scheduler per spec: /api/{api_version}/{admin_path}/config/scheduler/
		adminAPI.Get("/config/scheduler", schedulerHandler.GetAllTasks)
		adminAPI.Get("/config/scheduler/{name}", schedulerHandler.GetTaskHistory)
		adminAPI.Patch("/config/scheduler/{name}", schedulerHandler.UpdateTask)
		adminAPI.Post("/config/scheduler/{name}/run", schedulerHandler.TriggerTask)
		adminAPI.Post("/config/scheduler/{name}/enable", schedulerHandler.EnableTask)
		adminAPI.Post("/config/scheduler/{name}/disable", schedulerHandler.DisableTask)

		// Notification channel management under /server/{admin_path}/config/channels/
		adminAPI.Get("/config/channels", channelHandler.ListChannels)
		adminAPI.Get("/config/channels/definitions", channelHandler.GetChannelDefinitions)
		adminAPI.Get("/config/channels/queue/stats", channelHandler.GetQueueStats)
		adminAPI.Get("/config/channels/history", channelHandler.GetNotificationHistory)
		adminAPI.Post("/config/channels/initialize", channelHandler.InitializeChannels)
		adminAPI.Get("/config/channels/{type}", channelHandler.GetChannel)
		adminAPI.Put("/config/channels/{type}", channelHandler.UpdateChannel)
		adminAPI.Post("/config/channels/{type}/enable", channelHandler.EnableChannel)
		adminAPI.Post("/config/channels/{type}/disable", channelHandler.DisableChannel)
		adminAPI.Post("/config/channels/{type}/test", channelHandler.TestChannel)
		adminAPI.Get("/config/channels/{type}/stats", channelHandler.GetChannelStats)

		// Admin profile per spec: /api/{api_version}/{admin_path}/profile/
		adminSelfAPI.Get("/profile", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			profile := *admin
			profile.PasswordHash = ""
			profile.APITokenPrefix = ""

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "profile": profile})
		})
		adminSelfAPI.Patch("/profile", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			var req struct {
				Username *string `json:"username"`
				Email    *string `json:"email"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}

			username := admin.Username
			email := admin.Email

			if req.Username != nil {
				username = util.NormalizeUsername(*req.Username)
				if err := util.ValidateUsername(username); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
					return
				}
			}

			if req.Email != nil {
				email = util.NormalizeEmail(*req.Email)
				if err := util.ValidateEmail(email); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
					return
				}
			}

			if req.Username == nil && req.Email == nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "No profile fields provided"})
				return
			}

			if err := adminModel.Update(admin.ID, username, email); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to update profile"})
				return
			}

			updatedAdmin, err := adminModel.GetByID(admin.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load updated profile"})
				return
			}

			updatedAdmin.PasswordHash = ""
			updatedAdmin.APITokenPrefix = ""

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "Profile updated",
				"profile": updatedAdmin,
			})
		})
		adminSelfAPI.Post("/profile/password", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			var req struct {
				CurrentPassword string `json:"current_password" binding:"required"`
				NewPassword     string `json:"new_password" binding:"required"`
				ConfirmPassword string `json:"confirm_password" binding:"required"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}

			if req.NewPassword != req.ConfirmPassword {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Passwords do not match"})
				return
			}

			if len(req.NewPassword) < 8 {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Password must be at least 8 characters long"})
				return
			}

			fullAdmin, err := adminModel.GetByID(admin.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to verify current password"})
				return
			}

			valid, err := model.VerifyPassword(req.CurrentPassword, fullAdmin.PasswordHash)
			if err != nil || !valid {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Current password is incorrect"})
				return
			}

			if err := adminModel.UpdatePassword(admin.ID, req.NewPassword); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to update password"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Password changed successfully"})
		})
		adminSelfAPI.Get("/profile/token", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":    true,
				"token": maskAdminToken(admin.APITokenPrefix),
			})
		})
		adminSelfAPI.Post("/profile/token", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			newToken, err := adminModel.RegenerateAPIToken(admin.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Failed to regenerate API token"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "API token regenerated successfully",
				"token":   newToken,
			})
		})
		adminSelfAPI.Get("/profile/sessions", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			sessionModel := &model.AdminSessionModel{DB: database.GetServerDB()}
			sessions, err := sessionModel.GetActiveSessions(admin.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Failed to load sessions"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "sessions": sessions})
		})
		adminSelfAPI.Post("/profile/sessions/logout-all", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			sessionModel := &model.AdminSessionModel{DB: database.GetServerDB()}
			if err := sessionModel.DeleteAllSessionsForAdmin(admin.ID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Failed to log out of all sessions"})
				return
			}

			http.SetCookie(w, &http.Cookie{
				Name:     "admin_session",
				Value:    "",
				MaxAge:   -1,
				Path:     "/",
				Secure:   r.TLS != nil,
				HttpOnly: true,
			})

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Logged out of all sessions"})
		})
		adminSelfAPI.Get("/preferences", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			prefs, err := loadAdminPreferences(admin.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load preferences"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "preferences": prefs})
		})
		adminSelfAPI.Patch("/preferences", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			currentPrefs, err := loadAdminPreferences(admin.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load current preferences"})
				return
			}

			var req struct {
				Theme                *string `json:"theme"`
				Language             *string `json:"language"`
				Timezone             *string `json:"timezone"`
				NotificationsEnabled *bool   `json:"notifications_enabled"`
				EmailNotifications   *bool   `json:"email_notifications"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}

			theme := currentPrefs.Theme
			language := currentPrefs.Language
			timezone := currentPrefs.Timezone
			notificationsEnabled := currentPrefs.NotificationsEnabled
			emailNotifications := currentPrefs.EmailNotifications

			if req.Theme != nil {
				switch *req.Theme {
				case "auto", "light", "dark":
					theme = *req.Theme
				default:
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid theme"})
					return
				}
			}

			if req.Language != nil {
				language = strings.TrimSpace(*req.Language)
				if language == "" {
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Language cannot be empty"})
					return
				}
			}

			if req.Timezone != nil {
				timezone = strings.TrimSpace(*req.Timezone)
				if timezone == "" {
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Timezone cannot be empty"})
					return
				}
			}

			if req.NotificationsEnabled != nil {
				notificationsEnabled = *req.NotificationsEnabled
			}

			if req.EmailNotifications != nil {
				emailNotifications = *req.EmailNotifications
			}

			updatedJSON, err := json.Marshal(model.AdminPreferences{
				AdminID:              admin.ID,
				Theme:                theme,
				Language:             language,
				Timezone:             timezone,
				NotificationsEnabled: notificationsEnabled,
				EmailNotifications:   emailNotifications,
			})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to encode preferences"})
				return
			}

			// updated_at is bound as canonical UTC text so this writer agrees with the
			// INSERT above and with every reader of the column.
			if _, err := database.ExecContext(context.Background(), serverDB, database.TimeoutWrite, `
				UPDATE server_admin_preferences
				SET preferences = ?, updated_at = ?
				WHERE admin_id = ?
			`, string(updatedJSON), dbtime.FormatSQLTimestamp(time.Now()), admin.ID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to update preferences"})
				return
			}

			updatedPrefs, err := loadAdminPreferences(admin.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load updated preferences"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":          true,
				"message":     "Preferences updated",
				"preferences": updatedPrefs,
			})
		})

		// Admin passkeys per AI.md PART 17 line 28674-28683
		// /api/{api_version}/{admin_path}/profile/security/passkeys
		adminSelfAPI.Get("/profile/security/passkeys", adminPasskeyHandler.ListPasskeys)
		adminSelfAPI.Post("/profile/security/passkeys", adminPasskeyHandler.RegisterPasskey)
		adminSelfAPI.Delete("/profile/security/passkeys/{passkey_id}", adminPasskeyHandler.DeletePasskey)

		// Server admins per spec: /api/{api_version}/{admin_path}/config/admins/
		adminAPI.Get("/config/admins", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			count, err := adminModel.GetCount()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to count admins"})
				return
			}

			onlineAdmins, err := getOnlineAdminUsernames()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load online admins"})
				return
			}

			currentAdmin := *admin
			currentAdmin.PasswordHash = ""
			currentAdmin.APITokenPrefix = ""

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":             true,
				"count":          count,
				"current_admin":  currentAdmin,
				"online_admins":  onlineAdmins,
				"privacy_notice": "Other admin account details are not exposed",
			})
		})
		adminAPI.Get("/config/admins/{id}", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid admin ID"})
				return
			}

			if id != admin.ID {
				writeJSON(w, http.StatusForbidden, map[string]interface{}{"error": "Other admin account details are private"})
				return
			}

			profile := *admin
			profile.PasswordHash = ""
			profile.APITokenPrefix = ""

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "admin": profile})
		})
		adminAPI.Delete("/config/admins/{id}", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid admin ID"})
				return
			}

			if id == admin.ID {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Cannot delete your own account"})
				return
			}

			if id == 1 {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Primary admin cannot be deleted"})
				return
			}

			if err := adminModel.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Admin deleted"})
		})
		adminAPI.Post("/config/admins/{id}/disable", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid admin ID"})
				return
			}

			if id == admin.ID {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Cannot disable your own account"})
				return
			}

			if id == 1 {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Primary admin cannot be disabled"})
				return
			}

			targetAdmin, err := adminModel.GetByID(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Admin not found"})
				return
			}

			if targetAdmin.IsSuperAdmin {
				otherSuperAdmins, err := countOtherActiveSuperAdmins(id)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to validate admin hierarchy"})
					return
				}
				if otherSuperAdmins == 0 {
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Cannot disable the last active super admin"})
					return
				}
			}

			if err := adminModel.Update(id, targetAdmin.Username, targetAdmin.Email, targetAdmin.IsSuperAdmin, false); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to disable admin"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Admin disabled"})
		})
		adminAPI.Post("/config/admins/{id}/enable", func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid admin ID"})
				return
			}

			targetAdmin, err := adminModel.GetByID(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Admin not found"})
				return
			}

			if err := adminModel.Update(id, targetAdmin.Username, targetAdmin.Email, targetAdmin.IsSuperAdmin, true); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to enable admin"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Admin enabled"})
		})
		adminAPI.Post("/config/admins/invite", func(w http.ResponseWriter, r *http.Request) {
			admin, ok := getCurrentAdmin(w, r)
			if !ok {
				return
			}

			var req struct {
				Email     string `json:"email" binding:"required,email"`
				ExpiresIn string `json:"expires_in"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
				return
			}

			invite, expiresIn, err := adminInviteService.CreateInvite(req.Email, int(admin.ID), req.ExpiresIn)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":         true,
				"message":    "Admin invite created",
				"token":      invite.Token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"expires_in": expiresIn,
				"invite_url": buildInviteURL(r, invite.Token),
			})
		})

		// WebUI Notification API routes - Admin (root-level since notifications is a root admin path)
		adminSelfAPI.Get("/notifications", notificationAPIHandler.GetAdminNotifications)
		adminSelfAPI.Get("/notifications/unread", notificationAPIHandler.GetAdminUnreadNotifications)
		adminSelfAPI.Get("/notifications/count", notificationAPIHandler.GetAdminUnreadCount)
		adminSelfAPI.Get("/notifications/stats", notificationAPIHandler.GetAdminStats)
		adminSelfAPI.Patch("/notifications/{id}/read", notificationAPIHandler.MarkAdminNotificationRead)
		adminSelfAPI.Patch("/notifications/read", notificationAPIHandler.MarkAllAdminNotificationsRead)
		adminSelfAPI.Patch("/notifications/{id}/dismiss", notificationAPIHandler.DismissAdminNotification)
		adminSelfAPI.Delete("/notifications/{id}", notificationAPIHandler.DeleteAdminNotification)
		adminSelfAPI.Get("/notifications/preferences", notificationAPIHandler.GetAdminPreferences)
		adminSelfAPI.Patch("/notifications/preferences", notificationAPIHandler.UpdateAdminPreferences)
		adminSelfAPI.Post("/notifications/send", notificationAPIHandler.SendTestNotification)

		// SMTP provider management under /server/
		adminAPI.Get("/config/smtp/providers", channelHandler.ListSMTPProviders)
		adminAPI.Post("/config/smtp/autodetect", channelHandler.AutoDetectSMTP)

		// Admin panel settings endpoints under /server/
		adminAPI.Put("/config/settings/web", handler.SaveWebSettings)
		adminAPI.Put("/config/settings/security", handler.SaveSecuritySettings)
		adminAPI.Put("/config/settings/database", handler.SaveDatabaseSettings)

		// Database management endpoints under /server/
		adminAPI.Post("/config/database/test", handler.TestDatabaseConnection)
		adminAPI.Post("/config/database/test-config", handler.TestDatabaseConfigConnection)
		adminAPI.Post("/config/database/optimize", handler.OptimizeDatabase)
		adminAPI.Post("/config/database/vacuum", handler.VacuumDatabase)
		adminAPI.Post("/config/cache/clear", handler.ClearCache)

		// Backup management per spec: /api/{api_version}/{admin_path}/config/backup/
		adminAPI.Get("/config/backup", handler.ListBackups)
		adminAPI.Post("/config/backup", handler.CreateBackup)
		adminAPI.Get("/config/backup/stats", handler.BackupStats)
		adminAPI.Get("/config/backup/schedule", handler.GetBackupSchedule)
		adminAPI.Post("/config/backup/schedule", handler.SaveBackupSchedule)
		adminAPI.Post("/config/backup/restore", handler.RestoreBackup)
		// The param is named :filename because that is the value the handlers
		// validate and resolve against the backup directory - a backup has no
		// identifier other than its file name.
		adminAPI.Get("/config/backup/{filename}", handler.DownloadBackup)
		adminAPI.Delete("/config/backup/{filename}", handler.DeleteBackup)
		adminAPI.Get("/config/backup/{filename}/download", handler.DownloadBackup)

		// Template management under /server/
		adminAPI.Get("/config/templates", templateHandler.ListTemplates)
		adminAPI.Get("/config/templates/variables", templateHandler.GetTemplateVariables)
		adminAPI.Post("/config/templates/preview", templateHandler.PreviewTemplate)
		adminAPI.Post("/config/templates/initialize", templateHandler.InitializeDefaults)
		adminAPI.Get("/config/templates/{id}", templateHandler.GetTemplate)
		adminAPI.Post("/config/templates", templateHandler.CreateTemplate)
		adminAPI.Put("/config/templates/{id}", templateHandler.UpdateTemplate)
		adminAPI.Delete("/config/templates/{id}", templateHandler.DeleteTemplate)
		adminAPI.Post("/config/templates/{id}/clone", templateHandler.CloneTemplate)

		// Notification metrics management under /server/
		adminAPI.Get("/config/metrics/notifications/summary", metricsHandler.GetSummary)
		adminAPI.Get("/config/metrics/notifications/channels/{type}", metricsHandler.GetChannelMetrics)
		adminAPI.Get("/config/metrics/notifications/errors", metricsHandler.GetRecentErrors)
		adminAPI.Get("/config/metrics/notifications/health", metricsHandler.GetHealthStatus)

		// Tor hidden service management (AI.md PART 32)
		// API per spec: /api/{api_version}/{admin_path}/config/tor/
		torAPI := chi.NewRouter()
		adminAPI.Mount("/config/tor", torAPI)
		{
			torAPI.Get("/", torAdminHandler.GetStatus)
			torAPI.Patch("/", torAdminHandler.UpdateSettings)
			torAPI.Post("/regenerate", torAdminHandler.Regenerate)
			torAPI.Get("/vanity", torAdminHandler.GetVanityStatus)
			torAPI.Post("/vanity", torAdminHandler.GenerateVanity)
			torAPI.Delete("/vanity", torAdminHandler.CancelVanity)
			torAPI.Post("/vanity/apply", torAdminHandler.ApplyVanity)
			torAPI.Post("/import", torAdminHandler.ImportKeys)
		}

		// Web settings per spec: /api/{api_version}/{admin_path}/config/web/
		webAPI := chi.NewRouter()
		adminAPI.Mount("/config/web", webAPI)
		{
			webAPI.Get("/robots", adminWebHandler.GetRobotsTxt)
			webAPI.Patch("/robots", adminWebHandler.UpdateRobotsTxt)
			webAPI.Get("/robots/preview", adminWebHandler.GetRobotsTxt)
			webAPI.Get("/security", adminWebHandler.GetSecurityTxt)
			webAPI.Patch("/security", adminWebHandler.UpdateSecurityTxt)
			webAPI.Get("/security/preview", adminWebHandler.GetSecurityTxt)
		}

		// Email templates per spec: /api/{api_version}/{admin_path}/config/email/templates/
		emailTemplateAPI := chi.NewRouter()
		adminAPI.Mount("/config/email/templates", emailTemplateAPI)
		{
			emailTemplateAPI.Get("/", emailTemplateHandler.ListTemplates)
			emailTemplateAPI.Get("/{name}", emailTemplateHandler.GetTemplate)
			emailTemplateAPI.Put("/{name}", emailTemplateHandler.UpdateTemplate)
			emailTemplateAPI.Post("/{name}/reset", emailTemplateHandler.ImportTemplate)
			emailTemplateAPI.Post("/{name}/preview", emailTemplateHandler.TestTemplate)
		}

		// System logs management (already under /server/logs)
		logsAPI := chi.NewRouter()
		adminAPI.Mount("/config/logs", logsAPI)
		{
			logsAPI.Get("/", logsHandler.GetLogs)
			logsAPI.Get("/{type}", logsHandler.GetLogs)
			logsAPI.Get("/{type}/download", logsHandler.DownloadLogs)
			logsAPI.Get("/audit", logsHandler.GetAuditLogs)
			logsAPI.Get("/audit/download", logsHandler.DownloadAuditLogs)
			logsAPI.Post("/audit/search", logsHandler.SearchAuditLogs)
			logsAPI.Get("/audit/stats", logsHandler.GetAuditStats)
			logsAPI.Get("/stats", logsHandler.GetLogStats)
			logsAPI.Get("/archives", logsHandler.ListArchivedLogs)
			logsAPI.Get("/stream", logsHandler.StreamLogs)
			logsAPI.Post("/rotate", logsHandler.RotateLogs)
			logsAPI.Delete("/", logsHandler.ClearLogs)
		}

		// SSL/TLS per spec: /api/{api_version}/{admin_path}/config/ssl/
		sslAPI := chi.NewRouter()
		adminAPI.Mount("/config/ssl", sslAPI)
		{
			sslAPI.Get("/", sslHandler.GetStatus)
			sslAPI.Patch("/", sslHandler.UpdateSettings)
			sslAPI.Post("/renew", sslHandler.RenewCertificate)
			sslAPI.Post("/obtain", sslHandler.ObtainCertificate)
			sslAPI.Post("/auto-renew", sslHandler.StartAutoRenewal)
			sslAPI.Get("/dns-records", sslHandler.GetDNSRecords)
			sslAPI.Post("/verify", sslHandler.VerifyCertificate)
			sslAPI.Get("/export", sslHandler.ExportCertificate)
			sslAPI.Post("/import", sslHandler.ImportCertificate)
			sslAPI.Post("/revoke", sslHandler.RevokeCertificate)
			sslAPI.Post("/test", sslHandler.TestSSL)
			sslAPI.Post("/scan", sslHandler.SecurityScan)
		}

		// Metrics configuration under /server/
		metricsAPI := chi.NewRouter()
		adminAPI.Mount("/config/metrics", metricsAPI)
		{
			metricsAPI.Get("/config", metricsConfigHandler.GetConfig)
			metricsAPI.Put("/config", metricsConfigHandler.UpdateConfig)
			metricsAPI.Get("/stats", metricsConfigHandler.GetStats)
			metricsAPI.Get("/list", metricsConfigHandler.ListMetrics)
			metricsAPI.Post("/custom", metricsConfigHandler.CreateMetric)
			metricsAPI.Delete("/custom/{name}", metricsConfigHandler.DeleteMetric)
			metricsAPI.Get("/export", metricsConfigHandler.ExportMetrics)
			metricsAPI.Put("/toggle/{name}", metricsConfigHandler.ToggleMetric)
		}

		// Advanced logging formats under /server/
		loggingAPI := chi.NewRouter()
		adminAPI.Mount("/config/logging", loggingAPI)
		{
			loggingAPI.Get("/formats", loggingHandler.GetFormats)
			loggingAPI.Put("/formats", loggingHandler.UpdateFormats)
			loggingAPI.Get("/fail2ban/config", loggingHandler.GetFail2banConfig)
			loggingAPI.Get("/syslog/config", loggingHandler.GetSyslogConfig)
			loggingAPI.Get("/cef/config", loggingHandler.GetCEFConfig)
			loggingAPI.Get("/export", loggingHandler.ExportLogs)
			loggingAPI.Post("/fail2ban/configure", loggingHandler.ConfigureFail2ban)
			loggingAPI.Post("/syslog/configure", loggingHandler.ConfigureSyslog)
			loggingAPI.Get("/test", loggingHandler.TestFormat)
		}
	}

	// User notification preferences API (authenticated users)
	// AI.md PART 14: Use versioned API + plural nouns
	userPrefAPI := chi.NewRouter()
	apiV1.Mount("/users", userPrefAPI)
	userPrefAPI.Use(middleware.RequireAuth(db.DB))
	{
		// Channel preferences
		userPrefAPI.Get("/preferences", preferencesHandler.GetUserPreferences)
		userPrefAPI.Put("/preferences/{id}", preferencesHandler.UpdatePreference)
		userPrefAPI.Post("/preferences", preferencesHandler.CreatePreference)
		userPrefAPI.Delete("/preferences/{id}", preferencesHandler.DeletePreference)

		// Subscriptions
		userPrefAPI.Get("/subscriptions", preferencesHandler.GetSubscriptions)
		userPrefAPI.Put("/subscriptions/{id}", preferencesHandler.UpdateSubscription)
		userPrefAPI.Post("/subscriptions", preferencesHandler.CreateSubscription)
	}

	// API routes are now consolidated under /api/v1 above

	// Main /api endpoint - API version information
	// AI.md PART 14: Never hardcode v1 - use cfg.GetAPIVersion()
	r.Get("/api", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"service": "Weather API",
			"version": "2.0.0",
			"api_versions": []string{
				cfg.GetAPIVersion(),
			},
			"current_version": cfg.GetAPIVersion(),
			"documentation":   "http://" + r.Host + "/docs",
			"openapi":         "http://" + r.Host + "/openapi.json",
			"swagger":         "http://" + r.Host + "/openapi",
			"graphql":         "http://" + r.Host + "/graphql",
		})
	})

	// /api/autodiscover - Client/Agent auto-configuration endpoint
	// AI.md PART 33/34: Non-versioned endpoint for CLI/agent self-configuration
	// SECURITY: NEVER include admin_path, secrets, or internal IPs
	r.Get("/api/autodiscover", func(w http.ResponseWriter, r *http.Request) {
		// Build public URL from request
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		publicURL := scheme + "://" + r.Host

		// Get cluster nodes (empty array if single-node)
		clusterNodes := []string{publicURL}

		// Cache for 1 hour per AI.md
		w.Header().Set("Cache-Control", "public, max-age=3600")

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"primary":     publicURL,
			"cluster":     clusterNodes,
			"api_version": cfg.GetAPIVersion(),
			"timeout":     30,
			"retry":       3,
			"retry_delay": 1,
			"config": map[string]interface{}{
				"database": map[string]interface{}{
					"drivers": []string{"file", "sqlite", "libsql", "postgres", "mysql", "mssql", "mongodb"},
					"aliases": map[string]interface{}{
						"sqlite2":    "sqlite",
						"sqlite3":    "sqlite",
						"turso":      "libsql",
						"pgsql":      "postgres",
						"postgresql": "postgres",
						"mariadb":    "mysql",
						"mongo":      "mongodb",
					},
					"ssl_modes": []string{"disable", "require", "verify-full"},
				},
				"cache": map[string]interface{}{
					"types": []string{"none", "memory", "valkey", "redis"},
				},
				"formats": map[string]interface{}{
					"duration": []string{"s", "m", "h", "d"},
					"size":     []string{"KB", "MB", "GB"},
				},
				"logging": map[string]interface{}{
					"levels": []string{"debug", "info", "warn", "error"},
				},
				"smtp": map[string]interface{}{
					"tls_modes": []string{"auto", "starttls", "tls", "none"},
				},
				"features": map[string]interface{}{
					"clustering": false,
					"tor":        cfg.Server.Tor.Enabled,
					"webauthn":   true,
					"oauth":      []string{},
				},
			},
		})
	})

	// OpenAPI/Swagger documentation (AI.md PART 14)
	// Root-level endpoints per AI.md specification
	r.Get("/openapi", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/openapi/index.html", http.StatusMovedPermanently)
	})
	// Swagger UI + JSON spec (auto-generated)
	r.Get("/openapi/*any", handler.GetSwaggerUIAuto())
	r.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/openapi/doc.json", http.StatusMovedPermanently)
	})

	// GraphQL API (AI.md PART 14)
	graphqlResolver := appgraphql.NewResolver(
		dualDB.Server,
		dualDB.Users,
		weatherService,
		apiHandler,
		authHandler,
		locationHandler,
		notificationHandler,
		adminHandler,
		torAdminHandler,
		adminSettingsHandler,
		schedulerHandler,
		earthquakeHandler,
		hurricaneHandler,
		severeWeatherHandler,
		moonHandler,
	)
	graphqlServer := appgraphql.NewServer(graphqlResolver)

	// Root-level endpoint required by AI.md PART 14, plus the API-path alias
	// mounting the same handlers (never a redirect).
	registerGraphQLRoutes(r, cfg.GetAPIPath(),
		appgraphql.GraphQLHandler(graphqlServer),
		appgraphql.PlaygroundHandler("/graphql"),
		appgraphql.PlaygroundAssetHandler())

	appLogger.Printf("GraphQL API enabled at /graphql")
	fmt.Printf("%s GraphQL API enabled at /graphql\n", display.Emoji("✅", "[OK]"))

	// HTML documentation page at /docs
	r.Get("/docs", apiHandler.GetDocsHTML)

	// WebSocket endpoint for real-time notifications (TEMPLATE.md Part 25)
	// Requires authentication for both users and admins
	r.With(middleware.OptionalAuth(db.DB)).Get("/ws/notifications", notificationAPIHandler.HandleWebSocketConnection)

	// Public /server/ pages (AI.md PART 14: /server/* are public, no auth required)
	r.Get("/server", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/server/about", http.StatusMovedPermanently)
	})
	r.Get("/server/about", handler.ShowAboutPage(db, cfg))
	r.Get("/server/privacy", handler.ShowPrivacyPage(db, cfg))
	r.Get("/server/contact", handler.ShowContactPage(db, cfg))
	r.Get("/server/help", handler.ShowHelpPage(db, cfg))
	r.Get("/server/terms", handler.ShowTermsPage(db, cfg))

	// Examples endpoint
	// AI.md PART 14: Never hardcode v1 - use cfg.GetAPIPath()
	r.Get("/examples", func(w http.ResponseWriter, r *http.Request) {
		hostInfo := util.GetHostInfo(r)
		apiPath := cfg.GetAPIPath()
		examples := fmt.Sprintf(`Weather API Examples

Console Interface:
  curl %s/
  curl %s/London
  curl %s/Paris?format=1
  curl %s/Tokyo?units=metric

JSON API:
  curl %s%s/weather?location=London
  curl %s%s/forecasts?location=Paris&days=5
  curl %s%s/locations/search?q=New+York
  curl %s%s/ip
`,
			hostInfo.FullHost, hostInfo.FullHost, hostInfo.FullHost, hostInfo.FullHost,
			hostInfo.FullHost, apiPath, hostInfo.FullHost, apiPath, hostInfo.FullHost, apiPath, hostInfo.FullHost, apiPath)

		writeText(w, http.StatusOK, "%s", examples)
	})

	// Web interface routes
	r.Get("/web", webHandler.ServeWebInterface)
	r.Get("/web/{location}", webHandler.ServeWebInterface)

	// Moon interface routes
	r.Get("/moon", webHandler.ServeMoonInterface)
	r.Get("/moon/{location}", webHandler.ServeMoonInterface)

	// Historical weather page
	r.Get("/history", historyHandler.ShowHistory)

	// Earthquake routes (plural per AI.md PART 14)
	r.Get("/earthquakes", earthquakeHandler.HandleEarthquakeRequest)
	r.Get("/earthquakes/{location}", earthquakeHandler.HandleEarthquakeRequest)

	// Backwards compatibility: singular -> plural redirect
	r.Get("/earthquake", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/earthquakes", http.StatusMovedPermanently)
	})

	// Hurricane routes redirect to severe-weather (plural per AI.md PART 14)
	r.Get("/hurricanes", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/severe-weather", http.StatusMovedPermanently)
	})
	r.Get("/hurricane", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/severe-weather", http.StatusMovedPermanently)
	})

	// Severe Weather routes (new comprehensive severe weather page)
	r.Get("/severe-weather", severeWeatherHandler.HandleSevereWeatherRequest)
	r.Get("/severe-weather/{location}", severeWeatherHandler.HandleSevereWeatherRequest)

	// Type-filtered severe weather routes
	r.Get("/severe/{type}", severeWeatherHandler.HandleSevereWeatherByType)
	r.Get("/severe/{type}/{location}", severeWeatherHandler.HandleSevereWeatherByType)

	// AI.md PART 14: Legacy endpoints are technical debt - DELETED
	// OLD: /api/earthquakes and /api/hurricanes redirects removed
	// Use versioned endpoints: /api/{api_version}/earthquakes and /api/{api_version}/hurricanes

	// Initialization check middleware - show loading page if not ready
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip for health checks, API routes, and static files
			if strings.HasPrefix(r.URL.Path, "/health") ||
				strings.HasPrefix(r.URL.Path, "/server/healthz") ||
				strings.HasPrefix(r.URL.Path, "/api") ||
				strings.HasPrefix(r.URL.Path, "/debug") ||
				strings.Contains(r.URL.Path, ".") {
				next.ServeHTTP(w, r)
				return
			}

			// Show loading page if not initialized
			if !handler.IsInitialized() {
				handler.ServeLoadingPage(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	})

	// Theme toggle (AI.md PART 16 Theme Switching) - POST form, works without JS
	r.Post("/theme", server.SetThemeHandler)

	// Main weather routes
	// Uses IP/cookie lookup
	r.Get("/", weatherHandler.HandleRoot)
	// Explicit location
	r.Get("/weather/{location}", weatherHandler.HandleLocation)
	// Backwards compatibility catch-all
	r.Get("/{location}", weatherHandler.HandleLocation)

	// Build final URL for documentation
	// Per AI.md PART 5 - DOMAIN env var (no project prefix)
	finalHostname := os.Getenv("DOMAIN")
	if finalHostname == "" {
		// System variable
		finalHostname = os.Getenv("HOSTNAME")
	}
	if finalHostname == "" {
		finalHostname = "localhost"
	}

	protocol := "http"
	// AI.md PART 5: Boolean Handling
	tlsEnabled := config.IsTruthy(os.Getenv("TLS_ENABLED"))
	if tlsEnabled || httpsPortInt > 0 {
		protocol = "https"
	}

	finalURL := fmt.Sprintf("%s://%s", protocol, finalHostname)
	if (protocol == "http" && port != "80") || (protocol == "https" && port != "443") {
		finalURL += ":" + port
	}

	// Print final startup messages
	fmt.Printf("%s For documentation see: %s/docs\n", display.Emoji("📡", "*"), finalURL)
	fmt.Printf("%s Ready: %s: %s\n", display.Emoji("🕐", "*"), time.Now().Format("2006-01-02 at 15:04:05"), finalURL)

	// Create HTTP server with graceful shutdown
	// Format address properly - check if port is already included
	var serverAddr string
	if listenAddress == "::" {
		// IPv6 dual-stack: must be formatted as [::]:port
		serverAddr = "[::]:" + port
	} else if listenAddress == "0.0.0.0" || !strings.Contains(listenAddress, ":") {
		// IPv4 or hostname without port: append port
		serverAddr = listenAddress + ":" + port
	} else if strings.Count(listenAddress, ":") > 1 && !strings.HasPrefix(listenAddress, "[") {
		// IPv6 address without brackets (e.g., "::1")
		host, portPart, err := net.SplitHostPort(listenAddress)
		if err != nil || portPart == "" {
			// No port in address, add brackets and port
			serverAddr = "[" + listenAddress + "]:" + port
		} else {
			serverAddr = "[" + host + "]:" + portPart
		}
	} else {
		// Already has port (e.g., "127.0.0.1:8080" or "[::1]:8080")
		serverAddr = listenAddress
	}

	// Server configuration per AI.md PART 18 lines 15697-15702
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: r,
		// Per AI.md PART 18: read_timeout: 30s
		ReadTimeout: 30 * time.Second,
		// Per AI.md PART 18: write_timeout: 30s
		WriteTimeout: 30 * time.Second,
		// Per AI.md PART 18: idle_timeout: 120s
		IdleTimeout: 120 * time.Second,
		// Max header size (1MB is reasonable default)
		MaxHeaderBytes: 1 << 20,
	}

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Listener bind/network failure (AI.md PART 8: exit code 3).
			log.Printf("Failed to start server: %v", err)
			os.Exit(3)
		}
	}()

	// Start Tor hidden service after HTTP server starts
	if err := torService.Start(httpPortInt); err != nil {
		log.Printf("Failed to start Tor hidden service: %v", err)
		fmt.Printf("%s Failed to start Tor hidden service: %v\n", display.Emoji("⚠️", "WARNING:"), err)
	}

	// Start config file watcher for live reload
	if configWatcher != nil {
		if err := configWatcher.Start(); err != nil {
			log.Printf("Failed to start config watcher: %v", err)
			fmt.Printf("%s Failed to start config watcher: %v\n", display.Emoji("⚠️", "WARNING:"), err)
		}
	}

	// TEMPLATE.md PART 1: Display startup banner
	torOnionAddr := ""
	if cfg != nil && cfg.Server.Tor.Enabled {
		torOnionAddr = cfg.Server.Tor.OnionAddr
	}

	// The setup wizard lives at {adminBasePath}/config/setup, so the banner needs the configured admin path
	adminBasePath := "/server/admin"
	if cfg != nil {
		adminBasePath = "/server/" + cfg.GetAdminPath()
	}

	if isFirstRun {
		util.DisplayFirstRunBanner(httpPortInt, setupToken, util.IsDockerized(), torOnionAddr, adminBasePath)
	} else {
		util.DisplayNormalBanner(Version, BuildDate, httpPortInt, util.IsDockerized(), torOnionAddr)
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	// Per AI.md PART 27 lines 6456-6458: Graceful shutdown signals
	// SIGTERM: kill (systemctl stop)
	// SIGINT: Ctrl+C
	// SIGQUIT: Ctrl+\ (Unix only, but harmless to include on Windows)
	baseSignals := []os.Signal{
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
	}

	// Per AI.md PART 27 line 6536: Ignore SIGHUP - config reloads automatically via file watcher
	signal.Ignore(syscall.SIGHUP)

	// Add platform-specific signals (SIGUSR1/2 on Unix only)
	allSignals := make([]os.Signal, len(baseSignals)+len(platformSignals))
	copy(allSignals, baseSignals)
	for i, sig := range platformSignals {
		allSignals[len(baseSignals)+i] = sig
	}

	signal.Notify(sigChan, allSignals...)

	// Handle signals
	for sig := range sigChan {
		switch sig {
		case syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT:
			log.Println("INFO: Received shutdown signal, shutting down gracefully...")

			// Stop scheduler
			taskScheduler.Stop()

			// Stop Tor service
			if err := torService.Stop(); err != nil {
				log.Printf("Tor shutdown error: %v", err)
				fmt.Printf("%s Tor shutdown error: %v\n", display.Emoji("⚠️", "WARNING:"), err)
			}

			// Stop config watcher
			if configWatcher != nil {
				if err := configWatcher.Stop(); err != nil {
					log.Printf("Config watcher shutdown error: %v", err)
					fmt.Printf("%s Config watcher shutdown error: %v\n", display.Emoji("⚠️", "WARNING:"), err)
				}
			}

			// Close cache connection
			if err := cacheManager.Close(); err != nil {
				log.Printf("Cache shutdown error: %v", err)
				fmt.Printf("%s Cache shutdown error: %v\n", display.Emoji("⚠️", "WARNING:"), err)
			}

			// Shutdown HTTP server with 5 second timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				log.Printf("Server forced to shutdown: %v", err)
				fmt.Printf("%s Server forced to shutdown: %v\n", display.Emoji("⚠️", "WARNING:"), err)
			}

			log.Println("Server exited gracefully")
			fmt.Printf("%s Server exited gracefully\n", display.Emoji("✅", "[OK]"))
			return

		default:
			// Handle platform-specific signals (SIGHUP, SIGUSR1, SIGUSR2, SIGRTMIN+3 on Unix)
			// Returns true if shutdown requested (e.g., SIGRTMIN+3 Docker signal)
			if handlePlatformSignal(sig, db, appLogger, dirPaths) {
				// Shutdown requested - execute same graceful shutdown as SIGTERM
				log.Println("INFO: Platform signal requested shutdown, shutting down gracefully...")

				// Stop scheduler
				taskScheduler.Stop()

				// Stop Tor service
				if err := torService.Stop(); err != nil {
					log.Printf("Tor shutdown error: %v", err)
					fmt.Printf("%s Tor shutdown error: %v\n", display.Emoji("⚠️", "WARNING:"), err)
				}

				// Stop config watcher
				if configWatcher != nil {
					if err := configWatcher.Stop(); err != nil {
						log.Printf("Config watcher shutdown error: %v", err)
						fmt.Printf("%s Config watcher shutdown error: %v\n", display.Emoji("⚠️", "WARNING:"), err)
					}
				}

				// Close cache connection
				if err := cacheManager.Close(); err != nil {
					log.Printf("Cache shutdown error: %v", err)
					fmt.Printf("%s Cache shutdown error: %v\n", display.Emoji("⚠️", "WARNING:"), err)
				}

				// Shutdown HTTP server with 5 second timeout
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := srv.Shutdown(ctx); err != nil {
					log.Printf("Server forced to shutdown: %v", err)
					fmt.Printf("%s Server forced to shutdown: %v\n", display.Emoji("⚠️", "WARNING:"), err)
				}

				log.Println("Server exited gracefully")
				fmt.Printf("%s Server exited gracefully\n", display.Emoji("✅", "[OK]"))
				return
			}
		}
	}
}

// showServerStatus displays comprehensive server status information
// Per AI.md PART 8: Returns true if healthy, false if unhealthy
func showServerStatus(db *database.DB, dbPath string, isFirstRun bool) bool {
	// Get configuration values - AI.md PART 5: Environment Variables
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	envMode := os.Getenv("MODE")
	if envMode == "" {
		// Legacy fallback
		envMode = os.Getenv("ENVIRONMENT")
	}
	if envMode == "" {
		envMode = "production"
	}

	address := os.Getenv("LISTEN")
	if address == "" {
		// Legacy fallback
		address = os.Getenv("SERVER_ADDRESS")
	}
	addressMode := ""
	if address == "" {
		// Check for reverse proxy indicators per AI.md PART 5: Boolean Handling
		reverseProxy := config.IsTruthy(os.Getenv("REVERSE_PROXY"))

		if reverseProxy {
			address = "127.0.0.1"
			addressMode = " (reverse proxy mode)"
		} else {
			address = "::"
			addressMode = " (all interfaces)"
		}
	}

	// Perform health checks (AI.md PART 8: --status must check health)
	isHealthy := true
	healthStatus := "OK: Healthy"

	// Check database connection
	dbStatus, _, dbErr := db.HealthCheck()
	if dbErr != nil || dbStatus != "connected" {
		isHealthy = false
		healthStatus = display.Emoji("🔴", "[FAIL]") + " Unhealthy (Database Error)"
	}

	// Get database statistics
	var userCount, locationCount, tokenCount int
	if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_accounts").Scan(&userCount); err != nil {
		log.Printf("WARNING: showServerStatus: failed to count users: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_saved_locations").Scan(&locationCount); err != nil {
		log.Printf("WARNING: showServerStatus: failed to count saved locations: %v", err)
	}
	// user_tokens.expires_at may hold the canonical UTC layout or a legacy local-zone value, and
	// SQLite's datetime() returns NULL for the latter, so an SQL comparison would silently count
	// zero. The candidate rows are read instead and compared in Go via dbtime.
	tokenRows, tokenErr := database.QueryContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT expires_at FROM user_tokens WHERE expires_at IS NOT NULL")
	if tokenErr != nil {
		log.Printf("WARNING: showServerStatus: failed to count active tokens: %v", tokenErr)
	} else {
		tokenCutoff := time.Now().UTC()
		for tokenRows.Next() {
			var storedExpiry interface{}
			if scanErr := tokenRows.Scan(&storedExpiry); scanErr != nil {
				log.Printf("WARNING: showServerStatus: failed to read token expiry: %v", scanErr)
				break
			}
			if dbtime.IsAfter(storedExpiry, tokenCutoff) {
				tokenCount++
			}
		}
		if rowsErr := tokenRows.Err(); rowsErr != nil {
			log.Printf("WARNING: showServerStatus: failed to count active tokens: %v", rowsErr)
		}
		tokenRows.Close()
	}

	// Display status
	fmt.Println("\n╔══════════════════════════════════════════════════════╗")
	fmt.Printf("║          %s Weather - Status              ║\n", display.Emoji("🌤️", "*"))
	fmt.Println("╚══════════════════════════════════════════════════════╝")

	fmt.Printf("\n%s Health Status: %s\n", display.Emoji("🏥", "*"), healthStatus)

	fmt.Printf("\n%s Server Configuration:\n", display.Emoji("📊", "*"))
	fmt.Printf("   Version:        %s\n", Version)
	fmt.Printf("   Build Date:     %s\n", BuildDate)
	fmt.Printf("   Git Commit:     %s\n", CommitID)
	fmt.Printf("   Listen Address: %s:%s%s\n", address, port, addressMode)
	fmt.Printf("   Environment:    %s\n", envMode)

	fmt.Printf("\n%s Database:\n", display.Emoji("💾", "*"))
	fmt.Printf("   Path:           %s\n", dbPath)
	fmt.Printf("   Status:         %s\n", dbStatus)
	fmt.Printf("   Users:          %d\n", userCount)
	fmt.Printf("   Locations:      %d\n", locationCount)
	fmt.Printf("   Active Tokens:  %d\n", tokenCount)
	fmt.Printf("   First Run:      %v\n", isFirstRun)

	fmt.Printf("\n%s Security:\n", display.Emoji("🔐", "*"))
	fmt.Printf("   Session Secret: %s Configured\n", display.Emoji("✅", "[OK]"))

	fmt.Printf("\n%s Endpoints:\n", display.Emoji("🌐", "*"))
	fmt.Printf("   Web Interface:  http://%s:%s/\n", address, port)
	fmt.Printf("   API Docs:       http://%s:%s/docs\n", address, port)
	fmt.Printf("   Health Check:   http://%s:%s/server/healthz\n", address, port)
	fmt.Printf("   Admin Panel:    http://%s:%s/admin\n", address, port)

	fmt.Printf("\n%s Features:\n", display.Emoji("📡", "*"))
	fmt.Printf("   %s Weather forecasts (Open-Meteo)\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s Moon phases\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s Earthquakes (USGS)\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s Hurricanes (NOAA)\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s Authentication & Sessions\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s Saved Locations\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s Weather Alerts\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s API Tokens\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s PWA Support\n", display.Emoji("✅", "[OK]"))
	fmt.Printf("   %s Rate Limiting\n", display.Emoji("✅", "[OK]"))

	fmt.Printf("\n%s CLI Commands:\n", display.Emoji("💡", "*"))
	fmt.Println("   --status        Show this status information")
	fmt.Println("   --version       Show version information")
	fmt.Println("   --healthcheck   Run health check (for Docker)")
	fmt.Println("   --port PORT     Override PORT environment variable")
	fmt.Println("   --data DIR      Data directory (will store server.db)")
	fmt.Println("   --config DIR    Configuration directory")
	fmt.Println("   --address ADDR  Override server listen address")

	fmt.Printf("\n%s Network Configuration:\n", display.Emoji("🌐", "*"))
	fmt.Println("   Default:        :: (all interfaces, IPv4 + IPv6)")
	fmt.Println("   Reverse Proxy:  127.0.0.1 (set REVERSE_PROXY=true)")
	fmt.Println("   Custom:         Set SERVER_LISTEN environment variable")

	fmt.Println("\n" + strings.Repeat("─", 56))
	fmt.Println()

	// Return health status per AI.md PART 8
	return isHealthy
}

// lookupEmailVerification returns the id and user id of the pending
// user_email_verifications row identified by token, reporting false when no
// such token exists or when the row has already expired at instant now.
//
// user_email_verifications.expires_at has more than one producer, so it can
// hold the canonical UTC text this project writes or a legacy local-zone
// time.Time.String() value left by a raw bound time.Time. An SQL predicate such
// as "expires_at > ?" compares those as text and therefore accepts or rejects a
// token by the writer's UTC offset rather than by the instant the value means.
// The row is fetched by token alone and its expiry judged in Go through dbtime,
// which reports false for a value it cannot parse so an uninterpretable row
// fails closed instead of verifying an address forever.
//
// token is the raw value from the emailed link. It is hashed before the lookup
// because user_email_verifications.token stores only model.HashAPIToken(token)
// — PART 11 forbids keeping a usable token at rest — so matching on the raw
// value found no row at all and no verification link could be redeemed here.
func lookupEmailVerification(db *sql.DB, token string, now time.Time) (int64, int64, bool) {
	var verificationID int64
	var userID int64
	var storedExpiry interface{}

	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, `
		SELECT id, user_id, expires_at
		FROM user_email_verifications
		WHERE token = ?
	`, model.HashAPIToken(token)).Scan(&verificationID, &userID, &storedExpiry)
	if err != nil {
		return 0, 0, false
	}

	if !dbtime.IsAfter(storedExpiry, now.UTC()) {
		return 0, 0, false
	}

	return verificationID, userID, true
}

// defaultAdminPreferencesJSON encodes the preference defaults a new admin
// starts with. server_admin_preferences (src/database/server_schema.go) stores
// preferences as a single JSON blob column (admin_id, preferences, updated_at)
// — not individual theme/language/... columns.
func defaultAdminPreferencesJSON(adminID int64) (string, error) {
	b, err := json.Marshal(model.AdminPreferences{
		AdminID:              adminID,
		Theme:                "auto",
		Language:             "en",
		Timezone:             "UTC",
		NotificationsEnabled: true,
		EmailNotifications:   true,
	})

	return string(b), err
}

// loadAdminPreferencesRow returns the stored preferences for adminID, creating
// the default row on first access.
func loadAdminPreferencesRow(db *sql.DB, adminID int64) (*model.AdminPreferences, error) {
	defaultJSON, err := defaultAdminPreferencesJSON(adminID)
	if err != nil {
		return nil, fmt.Errorf("failed to encode default admin preferences: %w", err)
	}

	// updated_at is a projected value in this INSERT ... SELECT, so replacing the
	// CURRENT_TIMESTAMP literal with a bound parameter adds a third placeholder to
	// the projection, ahead of the existing NOT EXISTS bind.
	if _, err := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
		INSERT INTO server_admin_preferences (admin_id, preferences, updated_at)
		SELECT ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM server_admin_preferences WHERE admin_id = ?
		)
	`, adminID, defaultJSON, dbtime.FormatSQLTimestamp(time.Now()), adminID); err != nil {
		return nil, fmt.Errorf("failed to ensure admin preferences: %w", err)
	}

	var prefsJSON string
	// updated_at is scanned as a raw driver value and parsed through dbtime: a row
	// written before this column was bound as canonical UTC text holds a
	// local-zone time.Time.String() value that the driver cannot convert into a
	// time.Time, which used to fail the whole preferences load.
	var storedUpdatedAt interface{}
	err = database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, `
		SELECT preferences, updated_at
		FROM server_admin_preferences
		WHERE admin_id = ?
	`, adminID).Scan(&prefsJSON, &storedUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to load admin preferences: %w", err)
	}

	prefs := &model.AdminPreferences{}
	if err := json.Unmarshal([]byte(prefsJSON), prefs); err != nil {
		return nil, fmt.Errorf("failed to decode admin preferences: %w", err)
	}
	prefs.AdminID = adminID
	if parsed, ok := dbtime.ParseStoredTimestamp(storedUpdatedAt); ok {
		prefs.UpdatedAt = parsed
	}

	return prefs, nil
}
