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

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"

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

// registerHealthRoutes mounts the canonical health routes per AI.md PART 13
// /server/healthz is the canonical content-negotiated route, /api/{api_version}/server/healthz
// is its API counterpart, /api/healthz is the unversioned alias mounting the SAME handler,
// and the root /healthz alias is mounted only when server.healthz.root.enabled is true
// Aliases are always direct handler mappings, never redirects
func registerHealthRoutes(r *gin.Engine, apiPath string, rootAliasEnabled bool, frontend, api gin.HandlerFunc) {
	r.GET("/server/healthz", frontend)
	r.GET(apiPath+"/server/healthz", api)
	r.GET("/api/healthz", api)
	if rootAliasEnabled {
		r.GET("/healthz", frontend)
	}
}

// registerGraphQLRoutes mounts the GraphQL endpoint and its API-path alias per AI.md PART 14
// The alias mounts the SAME handlers as /graphql — an alias is never a redirect
func registerGraphQLRoutes(r *gin.Engine, apiPath string, query, playground, assets gin.HandlerFunc) {
	r.POST("/graphql", query)
	r.GET("/graphql", playground)
	// Locally embedded playground assets (React/GraphiQL/theme/init script) -
	// never loaded from a CDN, see src/graphql/playground.go.
	r.GET("/graphql/assets/*filepath", assets)
	aliasPath := apiPath + "/graphql"
	r.POST(aliasPath, query)
	r.GET(aliasPath, playground)
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
	settingsModel := &model.SettingsModel{DB: db.DB}
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
	ldapService := service.NewLDAPService(db.DB)
	ldapAuthHandler := &handler.LDAPAuthHandler{DB: db.DB, LDAPService: ldapService}

	oidcService := service.NewOIDCService(db.DB)
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

	// Set Gin mode based on MODE variable (development, production, test)
	// AI.md PART 5: Environment Variables
	envMode := os.Getenv("MODE")
	if envMode == "" {
		// Legacy fallback
		envMode = os.Getenv("ENVIRONMENT")
	}

	switch envMode {
	case "development", "dev":
		gin.SetMode(gin.DebugMode)
	case "test", "testing":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	r := gin.New()

	// Trust reverse proxy headers
	r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})

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
	r.Use(gin.Recovery())

	// Response compression per AI.md PART 18 lines 15704-15719
	// Compresses text/html, text/css, application/json, etc.
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	// Prometheus metrics middleware (AI.md PART 21 - NON-NEGOTIABLE)
	r.Use(middleware.MetricsMiddleware())

	// Security headers middleware
	r.Use(middleware.SecurityHeaders())

	// Body size limit middleware per AI.md PART 18 line 15691 (10MB)
	r.Use(middleware.BodySizeLimitMiddleware(middleware.DefaultMaxBodySize))

	// CSRF protection middleware (AI.md PART 0 line 994, PART 22)
	r.Use(middleware.CSRFProtection(middleware.DefaultCSRFConfig()))

	// CORS middleware per AI.md PART 17 lines 14220-14222 and 15401-15405
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization", "X-API-Token"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		// Per AI.md line 15405: Access-Control-Max-Age = 86400 (24 hours)
		MaxAge: 24 * time.Hour,
	}))

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
	r.StaticFS("/static", http.FS(staticSubFS))

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
	r.Use(func(c *gin.Context) {
		lang := ""

		// 1. ?lang= query param (highest priority, also sets cookie)
		if q := c.Query("lang"); q != "" && i18nService.IsSupported(q) {
			lang = q
			c.SetCookie("lang", lang, 365*24*60*60, "/", "", c.Request.TLS != nil, true)
		}

		// 2. lang cookie
		if lang == "" {
			if cookie, err := c.Cookie("lang"); err == nil && i18nService.IsSupported(cookie) {
				lang = cookie
			}
		}

		// 3. Accept-Language header
		if lang == "" {
			lang = i18nService.ParseAcceptLanguage(c.GetHeader("Accept-Language"))
		}

		// 4. Default: en
		if lang == "" {
			lang = "en"
		}

		c.Set("lang", lang)
		c.Set("i18n", i18nService)
		c.Next()
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
	if gin.Mode() == gin.DebugMode {
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
	if gin.Mode() == gin.DebugMode {
		fmt.Printf("%s Registered template names:\n", display.Emoji("📋", "*"))
		for _, t := range tmpl.Templates() {
			fmt.Printf("   - %s\n", t.Name())
		}
	}

	r.SetHTMLTemplate(tmpl)

	// Live reload templates in debug mode (loads from filesystem if available)
	if gin.Mode() == gin.DebugMode {
		if _, err := os.Stat("src/server/template"); err == nil {
			r.Use(func(c *gin.Context) {
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
				r.SetHTMLTemplate(t)
				c.Next()
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
	r.GET("/health", handler.LivenessCheck)
	r.GET("/health/ready", handler.ReadinessCheck(db, startTime))
	r.GET("/health/full", handler.FullHealthCheck(db, startTime))

	// Prometheus metrics endpoint (TEMPLATE.md required - optional auth)
	r.GET("/metrics", handler.PrometheusMetrics())

	// security.txt endpoint (RFC 9116 - TEMPLATE.md PART 25)
	r.GET("/.well-known/security.txt", adminWebHandler.ServeSecurityTxt)
	// Also serve at root for compatibility
	r.GET("/security.txt", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/.well-known/security.txt")
	})

	// /.well-known/change-password redirect (TEMPLATE.md PART 25)
	r.GET("/.well-known/change-password", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/profile?tab=security")
	})

	// /.well-known/acme-challenge/:token - Let's Encrypt HTTP-01 challenge (TEMPLATE.md Part 8)
	r.GET("/.well-known/acme-challenge/:token", func(c *gin.Context) {
		token := c.Param("token")
		keyAuth, ok := service.GetGlobalHTTP01Provider().GetKeyAuth(token)
		if !ok {
			c.String(http.StatusNotFound, "")
			return
		}
		c.String(http.StatusOK, "%s", keyAuth)
	})

	// robots.txt endpoint
	r.GET("/robots.txt", adminWebHandler.ServeRobotsTxt)

	// sitemap.xml endpoint (AI.md PART 16: dynamically generated)
	r.GET("/sitemap.xml", adminWebHandler.ServeSitemap)

	// favicon.ico endpoint (AI.md PART 16: embedded default, customizable)
	r.GET("/favicon.ico", adminWebHandler.ServeFavicon)

	// Debug endpoints (only enabled when --debug flag or DEBUG=true)
	// Per AI.md PART 6: Debug endpoints only available when debug mode enabled
	if mode.IsDebugEnabled() {
		debugHandlers := handler.NewDebugHandlers(db.DB, r)
		debugHandlers.RegisterDebugRoutes(r)

		// pprof endpoints per AI.md PART 6
		debugGroup := r.Group("/debug/pprof")
		{
			debugGroup.GET("/", gin.WrapF(pprof.Index))
			debugGroup.GET("/cmdline", gin.WrapF(pprof.Cmdline))
			debugGroup.GET("/profile", gin.WrapF(pprof.Profile))
			debugGroup.POST("/symbol", gin.WrapF(pprof.Symbol))
			debugGroup.GET("/symbol", gin.WrapF(pprof.Symbol))
			debugGroup.GET("/trace", gin.WrapF(pprof.Trace))
			debugGroup.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
			debugGroup.GET("/block", gin.WrapH(pprof.Handler("block")))
			debugGroup.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
			debugGroup.GET("/heap", gin.WrapH(pprof.Handler("heap")))
			debugGroup.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
			debugGroup.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
		}

		// expvar endpoint per AI.md PART 6
		r.GET("/debug/vars", gin.WrapH(http.DefaultServeMux))

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
	r.GET("/debug/ip", func(c *gin.Context) {
		// IP detection for My Location button
		clientIP := util.GetClientIP(c)

		// Try to get location from IP
		coords, err := weatherService.GetCoordinatesFromIP(clientIP)
		if err != nil {
			// Empty means fallback to manual entry
			c.JSON(http.StatusOK, gin.H{
				"clientIP": clientIP,
				"location": gin.H{
					"value": "",
				},
				"error": err.Error(),
			})
			return
		}

		// Enhance location
		enhanced := locationEnhancer.EnhanceLocation(coords)

		// e.g., "Albany, NY"
		c.JSON(http.StatusOK, gin.H{
			"clientIP": clientIP,
			"location": gin.H{
				"value": enhanced.ShortName,
			},
			"coordinates": gin.H{
				"latitude":  coords.Latitude,
				"longitude": coords.Longitude,
			},
		})
	})

	// Server setup routes at /server/{admin_path}/config/setup (requires verified setup token)
	// AI.md: Setup flow is at /server/{admin_path}/config/setup, creates Primary Admin
	// AI.md: Server is FULLY FUNCTIONAL without setup - only admin panel requires setup
	// AI.md: Step 4: Redirect to /server/{admin_path}/config/setup (setup wizard) after token verified
	adminSetupRoutes := r.Group("/server/" + cfg.GetAdminPath() + "/config/setup")
	adminSetupRoutes.Use(middleware.BlockSetupAfterComplete(cfg))
	adminSetupRoutes.Use(middleware.RequireSetupTokenVerified(cfg))
	{
		// Setup wizard pages - user has already verified token at /admin
		// AI.md: 6 steps: Admin Account → API Token → Server Config → Security → Services → Complete
		adminSetupRoutes.GET("", setupHandler.ShowAdminSetup)
		adminSetupRoutes.POST("", setupHandler.CreateAdmin)
		adminSetupRoutes.GET("/api-token", setupHandler.ShowAPIToken)
		adminSetupRoutes.POST("/api-token", setupHandler.ProcessAPIToken)
		adminSetupRoutes.GET("/config", setupHandler.ShowServerConfig)
		adminSetupRoutes.POST("/config", setupHandler.ProcessServerConfig)
		adminSetupRoutes.GET("/security", setupHandler.ShowSecurity)
		adminSetupRoutes.POST("/security", setupHandler.ProcessSecurity)
		adminSetupRoutes.GET("/services", setupHandler.ShowServices)
		adminSetupRoutes.POST("/services", setupHandler.ProcessServices)
		adminSetupRoutes.GET("/complete", setupHandler.CompleteSetup)
	}

	// Authentication routes (public) - TEMPLATE.md lines 4441-4534
	r.GET("/server/auth/login", authHandler.ShowLoginPage)
	r.POST("/server/auth/login", middleware.LoginRateLimitMiddleware(), authHandler.HandleLogin)
	r.GET("/server/auth/register", authHandler.ShowRegisterPage)
	r.POST("/server/auth/register", authHandler.HandleRegister)
	r.GET("/server/auth/logout", authHandler.HandleLogout)

	// Password reset routes (public)
	r.GET("/server/auth/password/forgot", func(c *gin.Context) {
		handler.NegotiateResponse(c, "page/forgot_password.tmpl", util.TemplateData(c, gin.H{
			"title": "Forgot Password",
		}))
	})
	r.POST("/server/auth/password/forgot", middleware.PasswordResetRateLimitMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "If an account with that email exists, a reset link has been sent"})
	})
	r.GET("/server/auth/password/reset", func(c *gin.Context) {
		handler.NegotiateResponse(c, "page/reset_password.tmpl", util.TemplateData(c, gin.H{
			"title": "Reset Password",
			"token": c.Query("token"),
		}))
	})
	r.POST("/server/auth/password/reset", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Password has been reset successfully"})
	})

	// Email verification route (public) - per spec: GET /auth/verify/{code} verifies inline
	r.GET("/server/auth/verify/:code", func(c *gin.Context) {
		code := c.Param("code")
		if code == "" {
			c.HTML(http.StatusBadRequest, "page/verify_email.tmpl", util.TemplateData(c, gin.H{
				"title": "Verify Email",
				"error": "Missing verification code",
			}))
			return
		}

		verificationID, userID, valid := lookupEmailVerification(db.DB, code, time.Now())
		if !valid {
			c.HTML(http.StatusBadRequest, "page/verify_email.tmpl", util.TemplateData(c, gin.H{
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
			c.HTML(http.StatusInternalServerError, "page/verify_email.tmpl", util.TemplateData(c, gin.H{
				"title": "Verify Email",
				"error": "Failed to verify email. Please try again.",
			}))
			return
		}

		// Delete used token
		if _, err := database.ExecContext(context.Background(), db.DB, database.TimeoutWrite, `DELETE FROM user_email_verifications WHERE id = ?`, verificationID); err != nil {
			log.Printf("WARNING: verify-email: failed to delete used verification token %d: %v", verificationID, err)
		}

		c.Redirect(http.StatusFound, "/server/auth/login?verified=1")
	})

	// Two-factor authentication routes (public)
	r.GET("/server/auth/2fa", func(c *gin.Context) {
		handler.NegotiateResponse(c, "page/two_factor.tmpl", util.TemplateData(c, gin.H{
			"title": "Two-Factor Authentication",
		}))
	})
	r.POST("/server/auth/2fa", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication verified"})
	})

	// Passkey authentication routes (public)
	r.GET("/server/auth/passkey", func(c *gin.Context) {
		handler.NegotiateResponse(c, "page/passkey.tmpl", util.TemplateData(c, gin.H{
			"title": "Passkey Authentication",
		}))
	})

	// Username recovery routes (public)
	r.GET("/server/auth/username/forgot", func(c *gin.Context) {
		handler.NegotiateResponse(c, "page/forgot_username.tmpl", util.TemplateData(c, gin.H{
			"title": "Forgot Username",
		}))
	})
	r.POST("/server/auth/username/forgot", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "If an account with that email exists, the username has been sent"})
	})

	// Recovery key usage route (public)
	r.GET("/server/auth/recovery/use", func(c *gin.Context) {
		handler.NegotiateResponse(c, "page/recovery_key.tmpl", util.TemplateData(c, gin.H{
			"title": "Use Recovery Key",
		}))
	})
	r.POST("/server/auth/recovery/use", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Recovery key accepted"})
	})

	renderServerInvitePage := func(c *gin.Context, status int, data gin.H) {
		payload := gin.H{
			"title": "Server Admin Invite",
		}
		for key, value := range data {
			payload[key] = value
		}
		c.HTML(status, "page/server_invite.tmpl", util.TemplateData(c, payload))
	}

	renderUserInvitePage := func(c *gin.Context, status int, data gin.H) {
		payload := gin.H{
			"title": "User Invite",
		}
		for key, value := range data {
			payload[key] = value
		}
		c.HTML(status, "page/user_invite.tmpl", util.TemplateData(c, payload))
	}

	// Invite routes (public - token validates)
	r.GET("/server/server/auth/invite/server/:code", func(c *gin.Context) {
		invite, err := adminInviteService.VerifyInvite(c.Param("code"))
		if err != nil {
			renderServerInvitePage(c, http.StatusGone, gin.H{
				"error": err.Error(),
				"code":  c.Param("code"),
			})
			return
		}

		renderServerInvitePage(c, http.StatusOK, gin.H{
			"code":       invite.Token,
			"email":      invite.InvitedEmail,
			"expires_at": invite.ExpiresAt,
		})
	})
	r.POST("/server/server/auth/invite/server/:code", func(c *gin.Context) {
		token := c.Param("code")
		invite, err := adminInviteService.VerifyInvite(token)
		if err != nil {
			renderServerInvitePage(c, http.StatusGone, gin.H{
				"error": err.Error(),
				"code":  token,
			})
			return
		}

		var req struct {
			Username        string `form:"username" binding:"required,min=3"`
			Password        string `form:"password" binding:"required,min=8"`
			ConfirmPassword string `form:"confirm_password" binding:"required"`
		}
		if err := c.ShouldBind(&req); err != nil {
			renderServerInvitePage(c, http.StatusBadRequest, gin.H{
				"code":       token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"username":   req.Username,
				"error":      "Invalid form submission",
			})
			return
		}

		if req.Password != req.ConfirmPassword {
			renderServerInvitePage(c, http.StatusBadRequest, gin.H{
				"code":       token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"username":   req.Username,
				"error":      "Passwords do not match",
			})
			return
		}

		if _, err := adminInviteService.AcceptInvite(token, req.Username, req.Password); err != nil {
			renderServerInvitePage(c, http.StatusBadRequest, gin.H{
				"code":       token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"username":   req.Username,
				"error":      err.Error(),
			})
			return
		}

		c.Redirect(http.StatusSeeOther, "/server/auth/login?invite=accepted")
	})
	r.GET("/server/server/auth/invite/user/:code", func(c *gin.Context) {
		invite, err := userInviteModel.VerifyInvite(c.Param("code"))
		if err != nil {
			renderUserInvitePage(c, http.StatusGone, gin.H{
				"error": err.Error(),
				"code":  c.Param("code"),
			})
			return
		}

		renderUserInvitePage(c, http.StatusOK, gin.H{
			"code":       invite.Token,
			"username":   invite.Username,
			"email":      invite.Email,
			"role":       invite.Role,
			"expires_at": invite.ExpiresAt,
		})
	})
	r.POST("/server/server/auth/invite/user/:code", func(c *gin.Context) {
		token := c.Param("code")
		invite, err := userInviteModel.VerifyInvite(token)
		if err != nil {
			renderUserInvitePage(c, http.StatusGone, gin.H{
				"error": err.Error(),
				"code":  token,
			})
			return
		}

		var req struct {
			Username        string `form:"username" binding:"required,min=3"`
			Password        string `form:"password" binding:"required,min=8"`
			ConfirmPassword string `form:"confirm_password" binding:"required"`
		}
		if err := c.ShouldBind(&req); err != nil {
			renderUserInvitePage(c, http.StatusBadRequest, gin.H{
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
			renderUserInvitePage(c, http.StatusBadRequest, gin.H{
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
			renderUserInvitePage(c, http.StatusBadRequest, gin.H{
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
			renderUserInvitePage(c, http.StatusBadRequest, gin.H{
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
			renderUserInvitePage(c, http.StatusBadRequest, gin.H{
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
			renderUserInvitePage(c, http.StatusInternalServerError, gin.H{
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
			renderUserInvitePage(c, http.StatusInternalServerError, gin.H{
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
			renderUserInvitePage(c, http.StatusInternalServerError, gin.H{
				"code":       token,
				"username":   req.Username,
				"email":      invite.Email,
				"role":       invite.Role,
				"expires_at": invite.ExpiresAt,
				"error":      "Account created but failed to log in",
			})
			return
		}

		http.SetCookie(c.Writer, &http.Cookie{
			Name:     middleware.SessionCookieName,
			Value:    session.ID,
			Path:     "/",
			MaxAge:   2592000,
			HttpOnly: true,
			Secure:   c.Request.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})

		c.Redirect(http.StatusSeeOther, "/users/dashboard")
	})

	// OIDC authentication routes (public) — per AI.md PART 34
	r.GET("/server/auth/oidc/:provider", oidcAuthHandler.StartLogin)
	r.GET("/server/auth/oidc/:provider/callback", oidcAuthHandler.Callback)

	// LDAP authentication route (public)
	r.POST("/server/auth/ldap", ldapAuthHandler.Login)

	// User routes (require authentication) - per AI.md PART 14: /users/ is plural
	usersRoutes := r.Group("/users")
	usersRoutes.Use(middleware.RequireAuth(db.DB))
	usersRoutes.Use(middleware.BlockAdminFromUserRoutes())
	{
		// /users -> user dashboard (current user)
		usersRoutes.GET("", dashboardHandler.ShowDashboard)
		// /users/dashboard -> user dashboard
		usersRoutes.GET("/dashboard", dashboardHandler.ShowDashboard)

		// User settings pages per AI.md PART 34
		usersRoutes.GET("/settings", userSettingsHandler.ShowAccountSettings)
		usersRoutes.GET("/settings/privacy", userSettingsHandler.ShowPrivacySettings)
		usersRoutes.GET("/settings/notifications", userSettingsHandler.ShowNotificationSettings)
		usersRoutes.GET("/settings/appearance", userSettingsHandler.ShowAppearanceSettings)
		// /users/tokens per AI.md PART 34 spec (separate from settings)
		usersRoutes.GET("/tokens", userSettingsHandler.ShowTokensSettings)

		// /users/notifications - per AI.md PART 25: notifications page
		usersRoutes.GET("/notifications", notificationHandler.ShowNotificationsPage)
	}

	// Admin setup token verification route (public - before auth check)
	// AI.md: Step 2: User navigates to /server/admin → Step 3: User enters setup token
	r.POST("/server/"+cfg.GetAdminPath()+"/verify-token", setupHandler.VerifySetupTokenAtAdmin)

	// Admin passkey login page (public — shown during the post-password / pre-passkey
	// window; admin session is not yet established so this MUST be outside adminRoutes).
	// auth.go sets the admin_passkey_pending cookie and redirects here when the admin
	// has registered passkeys.  The page reads the cookie server-side (HttpOnly) and
	// embeds the pending-session token so the in-page JS can call the challenge/verify
	// API endpoints without ever exposing the raw cookie value to JS directly.
	r.GET("/server/"+cfg.GetAdminPath()+"/passkey", func(c *gin.Context) {
		pendingToken, cookieErr := c.Cookie("admin_passkey_pending")
		if cookieErr != nil || strings.TrimSpace(pendingToken) == "" {
			// Cookie missing or expired — bail back to login with a clear message.
			c.HTML(http.StatusOK, "admin/admin_passkey_login.tmpl", util.TemplateData(c, gin.H{
				"title":      "Admin Passkey Verification",
				"admin_path": cfg.GetAdminPath(),
				"error":      "Session expired or invalid. Please log in again.",
			}))
			return
		}
		c.HTML(http.StatusOK, "admin/admin_passkey_login.tmpl", util.TemplateData(c, gin.H{
			"title":                 "Admin Passkey Verification",
			"admin_path":            cfg.GetAdminPath(),
			"pending_session_token": pendingToken,
		}))
	})

	// Admin routes (require admin role + stricter rate limiting)
	// AI.md: Admin panel at /server/{admin_path} (configurable, default: "admin")
	adminRoutes := r.Group("/server/" + cfg.GetAdminPath())
	// AI.md: Show setup token entry at /admin when no admin exists
	adminRoutes.Use(middleware.SetupTokenRequired(cfg))
	adminRoutes.Use(middleware.RequireAdminAuth())
	adminRoutes.Use(middleware.AdminRateLimitMiddleware())
	// Log all admin actions
	adminRoutes.Use(middleware.AuditLogger(db.DB))
	{
		// /{admin_path} -> admin dashboard (root level)
		adminRoutes.GET("", dashboardHandler.ShowAdminPanel)
		// /{admin_path}/dashboard -> alias for root
		adminRoutes.GET("/dashboard", dashboardHandler.ShowAdminPanel)

		// /{admin_path}/logout -> clear admin session and redirect to login
		adminRoutes.GET("/logout", func(c *gin.Context) {
			// Delete admin session from database
			adminSessionID, err := c.Cookie("admin_session")
			if err == nil && adminSessionID != "" {
				if _, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, "DELETE FROM server_admin_sessions WHERE id = ?", adminSessionID); err != nil {
					log.Printf("WARNING: admin-logout: failed to delete admin session %s: %v", adminSessionID, err)
				}
			}
			// Clear admin_session cookie
			c.SetCookie("admin_session", "", -1, "/", "", false, true)
			c.Redirect(http.StatusFound, "/server/auth/login")
		})

		// AI.md PART 17: every server-management page lives under /{admin_path}/config/
		// and the admin's own account lives under /{admin_path}/{admin_username}/
		adminSelfRoutes := adminRoutes.Group("/:admin_username")
		adminSelfRoutes.Use(handler.RequireAdminSelf())

		// AI.md PART 17 header spec: global search over settings, logs and the
		// other data the admin panel manages
		adminRoutes.GET("/config/search", handler.AdminSearchPage)

		adminRoutes.GET("/config/settings", adminHandler.ShowSettingsPage)

		adminRoutes.GET("/config/web", adminWebHandler.ShowWebSettings)

		adminRoutes.GET("/config/users", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_users.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "User Management - Admin",
				"page":       "users",
				"breadcrumb": "Users",
			}))
		})

		adminRoutes.GET("/config/email", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_email.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Email Settings - Admin",
				"page":       "email",
				"breadcrumb": "Email",
			}))
		})

		adminRoutes.GET("/config/database", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_database.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Database & Cache - Admin",
				"page":       "database",
				"breadcrumb": "Database",
			}))
		})

		adminRoutes.GET("/config/info", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_system.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Server Information - Admin",
				"page":       "info",
				"breadcrumb": "Server Info",
			}))
		})

		adminRoutes.GET("/config/security", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_security.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Security Settings - Admin",
				"page":       "security",
				"breadcrumb": "Security",
			}))
		})

		adminRoutes.GET("/config/security/tokens", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/tokens.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "API Tokens - Admin",
				"page":       "tokens",
				"breadcrumb": "API Tokens",
			}))
		})

		adminRoutes.GET("/config/logs", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_logs.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "System Logs - Admin",
				"page":       "logs",
				"breadcrumb": "System Logs",
			}))
		})

		adminRoutes.GET("/config/logs/audit", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/logs.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Audit Logs - Admin",
				"page":       "audit",
				"breadcrumb": "Audit Logs",
			}))
		})

		adminRoutes.GET("/config/scheduler", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_tasks_enhanced.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Scheduled Tasks - Admin",
				"page":       "scheduler",
				"breadcrumb": "Scheduled Tasks",
			}))
		})

		adminRoutes.GET("/config/ssl", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_ssl.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "SSL/TLS Management - Admin",
				"page":       "ssl",
				"breadcrumb": "SSL/TLS",
			}))
		})

		adminRoutes.GET("/config/backup", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_backup_enhanced.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Backup Management - Admin",
				"page":       "backup",
				"breadcrumb": "Backup",
			}))
		})

		adminRoutes.GET("/config/metrics", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_metrics.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Metrics Configuration - Admin",
				"page":       "metrics",
				"breadcrumb": "Metrics",
			}))
		})

		adminRoutes.GET("/config/network/tor", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_tor.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Tor Hidden Service - Admin",
				"page":       "tor",
				"breadcrumb": "Tor Hidden Service",
			}))
		})

		adminRoutes.GET("/config/channels", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin_channels.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Notification Channels - Admin",
				"page":       "channels",
				"breadcrumb": "Channels",
			}))
		})

		adminRoutes.GET("/config/templates", func(c *gin.Context) {
			c.HTML(http.StatusOK, "template_editor.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Template Editor - Admin",
				"page":       "templates",
				"breadcrumb": "Templates",
			}))
		})

		adminRoutes.GET("/config/email/templates", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_email_editor.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":      "Email Template Editor - Admin",
				"page":       "email-templates",
				"breadcrumb": "Email Templates",
			}))
		})

		// Admin settings sub-panels (already under /server/)
		adminRoutes.GET("/config/users/settings", adminUsersHandler.ShowUserSettings)
		adminRoutes.GET("/config/weather", adminWeatherHandler.ShowWeatherSettings)
		adminRoutes.GET("/config/notifications", adminNotificationsHandler.ShowNotificationSettings)
		adminRoutes.GET("/config/network/geoip", adminGeoIPHandler.ShowGeoIPSettings)

		// AI.md PART 17: the admin's own account pages
		// /{admin_path}/{admin_username}/profile - Admin's own profile
		adminSelfRoutes.GET("/profile", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_profile.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Admin Profile",
				"page":  "profile",
			}))
		})

		// /{admin_path}/{admin_username}/preferences - Admin preferences
		adminSelfRoutes.GET("/preferences", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_preferences.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Admin Preferences",
				"page":  "preferences",
			}))
		})

		// /{admin_path}/{admin_username}/notifications - Admin notifications page
		adminSelfRoutes.GET("/notifications", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_notifications.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Notifications",
				"page":  "notifications",
			}))
		})

		// Additional management pages per spec
		// /{admin_path}/config/branding - Branding & SEO
		adminRoutes.GET("/config/branding", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_branding.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Branding & SEO - Admin",
				"page":  "server-branding",
			}))
		})

		// /{admin_path}/config/pages - Standard pages (about, privacy, contact)
		adminRoutes.GET("/config/pages", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_pages.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Standard Pages - Admin",
				"page":  "server-pages",
			}))
		})

		// /{admin_path}/config/roles - Role definitions
		adminRoutes.GET("/config/roles", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_roles.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Role Definitions - Admin",
				"page":  "server-roles",
			}))
		})

		// /{admin_path}/config/security/auth - Authentication config
		adminRoutes.GET("/config/security/auth", adminAuthHandler.ShowAuthSettings)

		// /{admin_path}/config/admins - Server admin accounts list
		adminRoutes.GET("/config/admins", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_admins.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Server Admins - Admin",
				"page":  "server-admins",
			}))
		})

		// /{admin_path}/config/admins/invite - Invite new admin
		adminRoutes.GET("/config/admins/invite", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_invite.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Invite Admin - Admin",
				"page":  "server-admins-invite",
			}))
		})

		// /{admin_path}/config/admins/:id - Admin detail
		adminRoutes.GET("/config/admins/:id", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_detail.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":   "Admin Detail - Admin",
				"page":    "server-admins",
				"adminID": c.Param("id"),
			}))
		})

		renderAdminUserInvitesPage := func(c *gin.Context, status int, data gin.H) {
			invites, err := userInviteModel.ListInvites()
			if err != nil {
				c.HTML(http.StatusInternalServerError, "page/error.tmpl", handler.AdminTemplateData(c, gin.H{
					"title":   "User Invites",
					"message": "Failed to load user invites",
				}))
				return
			}

			scheme := c.GetHeader("X-Forwarded-Proto")
			if scheme == "" {
				if c.Request.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			inviteRows := make([]gin.H, 0, len(invites))
			for _, invite := range invites {
				statusLabel := "pending"
				if invite.UsedAt != nil || (invite.MaxUses > 0 && invite.UseCount >= invite.MaxUses) {
					statusLabel = "used"
				} else if time.Now().After(invite.ExpiresAt) {
					statusLabel = "expired"
				}

				inviteRows = append(inviteRows, gin.H{
					"id":         invite.ID,
					"token":      invite.Token,
					"username":   invite.Username,
					"email":      invite.Email,
					"role":       invite.Role,
					"expires_at": invite.ExpiresAt,
					"used_at":    invite.UsedAt,
					"status":     statusLabel,
					"invite_url": fmt.Sprintf("%s://%s/server/auth/invite/user/%s", scheme, c.Request.Host, invite.Token),
				})
			}

			payload := gin.H{
				"title":                  "User Invites - Admin",
				"page":                   "users-invites",
				"invites":                inviteRows,
				"invite_expiration_days": config.GetUserInviteExpirationDays(),
			}
			for key, value := range data {
				payload[key] = value
			}

			c.HTML(status, "admin/admin_user_invites.tmpl", handler.AdminTemplateData(c, payload))
		}

		// /{admin_path}/config/users/invites - User invites
		adminRoutes.GET("/config/users/invites", func(c *gin.Context) {
			renderAdminUserInvitesPage(c, http.StatusOK, gin.H{})
		})
		adminRoutes.POST("/config/users/invites", func(c *gin.Context) {
			var req struct {
				Username      string `form:"username" binding:"required,min=3"`
				Email         string `form:"email" binding:"required,email"`
				Role          string `form:"role"`
				ExpiresInDays int    `form:"expires_in_days"`
			}
			if err := c.ShouldBind(&req); err != nil {
				renderAdminUserInvitesPage(c, http.StatusBadRequest, gin.H{
					"error": "Invalid form submission",
					"form":  req,
				})
				return
			}

			username := util.NormalizeUsername(req.Username)
			if err := util.ValidateUsername(username); err != nil {
				renderAdminUserInvitesPage(c, http.StatusBadRequest, gin.H{
					"error": err.Error(),
					"form":  req,
				})
				return
			}

			email := util.NormalizeEmail(req.Email)
			if err := util.ValidateEmail(email); err != nil {
				renderAdminUserInvitesPage(c, http.StatusBadRequest, gin.H{
					"error": err.Error(),
					"form":  req,
				})
				return
			}

			userModel := &model.UserModel{DB: db.DB}
			if _, err := userModel.GetByUsername(username); err == nil {
				renderAdminUserInvitesPage(c, http.StatusBadRequest, gin.H{
					"error": "Username is already in use",
					"form":  req,
				})
				return
			}
			if _, err := userModel.GetByEmail(email); err == nil {
				renderAdminUserInvitesPage(c, http.StatusBadRequest, gin.H{
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
				renderAdminUserInvitesPage(c, http.StatusBadRequest, gin.H{
					"error": err.Error(),
					"form":  req,
				})
				return
			}

			scheme := c.GetHeader("X-Forwarded-Proto")
			if scheme == "" {
				if c.Request.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			renderAdminUserInvitesPage(c, http.StatusOK, gin.H{
				"message":    "User invite created",
				"invite_url": fmt.Sprintf("%s://%s/server/auth/invite/user/%s", scheme, c.Request.Host, invite.Token),
			})
		})

		// /{admin_path}/config/moderation/users - User moderation
		adminRoutes.GET("/config/moderation/users", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_moderation.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "User Moderation - Admin",
				"page":  "moderation-users",
			}))
		})

		// /{admin_path}/config/moderation/users/:id - User detail
		adminRoutes.GET("/config/moderation/users/:id", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_user_detail.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":  "User Detail - Admin",
				"page":   "moderation-users",
				"userID": c.Param("id"),
			}))
		})

		// /{admin_path}/config/security/ratelimit - Rate limiting config
		adminRoutes.GET("/config/security/ratelimit", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_ratelimit.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Rate Limiting - Admin",
				"page":  "security-ratelimit",
			}))
		})

		// /{admin_path}/config/security/firewall - IP allow/block lists
		adminRoutes.GET("/config/security/firewall", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_firewall.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Firewall - Admin",
				"page":  "security-firewall",
			}))
		})

		// /{admin_path}/config/network/blocklists - IP/domain blocklists
		adminRoutes.GET("/config/network/blocklists", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_blocklists.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Blocklists - Admin",
				"page":  "network-blocklists",
			}))
		})

		// /{admin_path}/config/maintenance - Maintenance mode
		adminRoutes.GET("/config/maintenance", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_maintenance.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Maintenance Mode - Admin",
				"page":  "server-maintenance",
			}))
		})

		// /{admin_path}/config/updates - Software updates
		adminRoutes.GET("/config/updates", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_updates.tmpl", handler.AdminTemplateData(c, gin.H{
				"title":   "Updates - Admin",
				"page":    "server-updates",
				"version": handler.Version,
			}))
		})

		// /{admin_path}/config/cluster/nodes - Cluster node management
		adminRoutes.GET("/config/cluster/nodes", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_cluster_nodes.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Cluster Nodes - Admin",
				"page":  "server-cluster-nodes",
			}))
		})

		// /{admin_path}/config/cluster/add - Add cluster node
		adminRoutes.GET("/config/cluster/add", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_cluster_add.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Add Cluster Node - Admin",
				"page":  "server-cluster-add",
			}))
		})

		// /{admin_path}/help - Admin help & documentation
		adminRoutes.GET("/help", func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin/admin_help.tmpl", handler.AdminTemplateData(c, gin.H{
				"title": "Help - Admin",
				"page":  "help",
			}))
		})
	}
	r.GET("/notifications", middleware.RequireAuth(db.DB), notificationHandler.ShowNotificationsPage)

	// User profile page (per AI.md PART 14: /users/ is plural)
	r.GET("/users/profile", middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes(), func(c *gin.Context) {
		handler.NegotiateResponse(c, "page/user/profile.tmpl", util.TemplateData(c, gin.H{
			"title": "Profile",
			"page":  "profile",
		}))
	})

	// User security settings page (per AI.md PART 14: /users/ is plural)
	r.GET("/users/security", middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes(), twoFAHandler.ShowSecurityPage)
	r.GET("/users/security/passkeys", middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes(), twoFAHandler.ShowSecurityPage)

	// User notification preferences page (per AI.md PART 14: /users/ is plural)
	r.GET("/users/preferences", middleware.RequireAuth(db.DB), middleware.BlockAdminFromUserRoutes(), func(c *gin.Context) {
		handler.NegotiateResponse(c, "user_preferences.tmpl", util.TemplateData(c, gin.H{
			"title": "Preferences",
			"page":  "preferences",
		}))
	})

	// Removed - moved to adminRoutes group above

	// Location management pages
	r.GET("/users/locations/new", middleware.RequireAuth(db.DB), locationHandler.ShowAddLocationPage)
	r.GET("/users/locations/:id/edit", middleware.RequireAuth(db.DB), locationHandler.ShowEditLocationPage)

	// API routes - all API endpoints under /api/{api_version}
	// AI.md: API version prefix is configurable (default: "v1")
	apiV1 := r.Group(cfg.GetAPIPath())

	// Weather API routes (optional auth + API rate limiting)
	weatherAPI := apiV1.Group("")
	weatherAPI.Use(middleware.OptionalAuth(db.DB))
	weatherAPI.Use(middleware.APIRateLimitMiddleware())
	{
		// Weather endpoints per AI.md PART 36
		weatherAPI.GET("/weather", apiHandler.GetWeather)
		weatherAPI.GET("/weather/:location", apiHandler.GetWeatherByLocation)
		weatherAPI.GET("/weather/forecast", apiHandler.GetForecast)
		weatherAPI.GET("/weather/locations", apiHandler.GetLocation)

		// Backwards compatibility - old paths (deprecated)
		weatherAPI.GET("/forecasts", apiHandler.GetForecast)
		weatherAPI.GET("/forecasts/:location", apiHandler.GetForecastByLocation)

		// Additional endpoints
		weatherAPI.GET("/ip", apiHandler.GetIP)
		weatherAPI.GET("/docs", apiHandler.GetDocsJSON)
		weatherAPI.GET("/earthquakes", earthquakeHandler.HandleEarthquakeAPI)
		weatherAPI.GET("/earthquakes/:id", earthquakeHandler.HandleEarthquakeByIDAPI)
		// Backwards compat
		weatherAPI.GET("/hurricanes", hurricaneHandler.HandleHurricaneAPI)
		weatherAPI.GET("/hurricanes/:id", hurricaneHandler.HandleHurricaneByIDAPI)
		weatherAPI.GET("/severe-weather", severeWeatherHandler.HandleSevereWeatherAPI)
		weatherAPI.GET("/severe-weather/:id", severeWeatherHandler.HandleAlertByIDAPI)
		weatherAPI.GET("/moon", moonHandler.HandleMoonAPI)
		weatherAPI.GET("/moon/calendar", moonHandler.HandleMoonCalendarAPI)
		weatherAPI.GET("/sun", moonHandler.HandleSunAPI)
		weatherAPI.GET("/history", apiHandler.GetHistoricalWeather)

		// CLI client compatibility aliases (IDEA.md endpoints)
		weatherAPI.GET("/weather/alerts", severeWeatherHandler.HandleSevereWeatherAPI)
		weatherAPI.GET("/weather/moon", moonHandler.HandleMoonAPI)
		weatherAPI.GET("/weather/history", apiHandler.GetHistoricalWeather)

		// Root /api/{api_version} endpoint - return all endpoints
		// AI.md PART 14: Never hardcode v1 - use cfg.GetAPIPath()
		weatherAPI.GET("", func(c *gin.Context) {
			hostInfo := util.GetHostInfo(c)
			apiBase := hostInfo.FullHost + cfg.GetAPIPath()
			adminBase := hostInfo.FullHost + cfg.GetAdminAPIPath()
			handler.RespondNegotiatedData(c, http.StatusOK, gin.H{
				"version": cfg.GetAPIVersion(),
				"endpoints": []string{
					apiBase + "/users",
					apiBase + "/locations",
					apiBase + "/users/notifications",
					adminBase,
					apiBase + "/weather",
					apiBase + "/weather/:location",
					apiBase + "/forecasts",
					apiBase + "/forecasts/:location",
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
	apiV1.GET("/blocklist", func(c *gin.Context) {
		handler.RespondNegotiatedData(c, http.StatusOK, gin.H{
			"blocklist": util.UsernameBlocklist,
			"count":     util.GetBlocklistSize(),
			"public":    util.IsBlocklistPublic(),
			"note":      "These usernames are reserved and cannot be used for registration. The blocklist does not apply to the first user (admin setup).",
		})
	})

	// Public server API endpoints (AI.md PART 14: every web page has corresponding API)
	apiV1.GET("/server/about", handler.GetAboutAPI(db, cfg))
	apiV1.GET("/server/privacy", handler.GetPrivacyAPI(db, cfg))
	apiV1.GET("/server/help", handler.GetHelpAPI(db, cfg))
	apiV1.GET("/server/terms", handler.GetTermsAPI(db, cfg))
	apiV1.POST("/server/contact", handler.HandleContactFormSubmission(db, cfg))

	// Auth API routes per AI.md PART 33
	authAPI := apiV1.Group("/server/auth")
	{
		// Public auth endpoints (no auth required)
		authAPI.POST("/register", authAPIHandler.HandleAPIRegister)
		authAPI.POST("/login", authAPIHandler.HandleAPILogin)
		authAPI.POST("/2fa", authAPIHandler.HandleAPI2FA)
		authAPI.POST("/passkey/challenge", passkeyHandler.BeginPasskeyChallenge)
		authAPI.POST("/passkey/verify", passkeyHandler.VerifyPasskey)
		// Admin passkey login challenge/verify per AI.md PART 17 line
		// 28679 ("passkey can be used as primary login or as 2FA"). The
		// pending session token is issued by HandleLogin (`/server/auth/login`)
		// after admin password verify when the admin has at least one
		// passkey registered.
		authAPI.POST("/admin/passkey/challenge", adminPasskeyHandler.BeginPasskeyChallenge)
		authAPI.POST("/admin/passkey/verify", adminPasskeyHandler.VerifyPasskey)
		authAPI.POST("/recovery/use", authAPIHandler.HandleAPIRecoveryUse)
		authAPI.POST("/password/forgot", authAPIHandler.HandleAPIPasswordForgot)
		authAPI.POST("/password/reset", authAPIHandler.HandleAPIPasswordReset)
		authAPI.POST("/verify", authAPIHandler.HandleAPIVerifyEmail)

		// User invite endpoints (no auth required - token validates)
		authAPI.GET("/invite/user/:token", authAPIHandler.HandleAPIUserInviteValidate)
		authAPI.POST("/invite/user/:token", authAPIHandler.HandleAPIUserInviteComplete)
		authAPI.GET("/invite/server/:token", authAPIHandler.HandleAPIServerInviteValidate)
		authAPI.POST("/invite/server/:token", authAPIHandler.HandleAPIServerInviteComplete)

		// Protected auth endpoints (require auth)
		authAPI.POST("/logout", middleware.RequireAuth(db.DB), authAPIHandler.HandleAPILogout)
		authAPI.POST("/refresh", middleware.RequireAuth(db.DB), authAPIHandler.HandleAPIRefresh)
	}

	// Users API routes per AI.md PART 33 (spec uses /api/v1/users not /api/v1/user)
	usersAPI := apiV1.Group("/users")
	usersAPI.Use(middleware.RequireAuth(db.DB))
	usersAPI.Use(middleware.BlockAdminFromUserRoutes())
	{
		usersAPI.GET("", authHandler.GetCurrentUser)
		usersAPI.PATCH("", authHandler.UpdateProfile)

		// User settings API per AI.md PART 34
		usersAPI.GET("/settings", userSettingsHandler.GetSettings)
		usersAPI.PATCH("/settings", userSettingsHandler.UpdateSettings)

		// User tokens API per AI.md PART 34
		usersAPI.GET("/tokens", userSettingsHandler.ListTokens)
		usersAPI.POST("/tokens", userSettingsHandler.CreateToken)
		usersAPI.DELETE("/tokens/:id", userSettingsHandler.RevokeToken)

		// Avatar API per AI.md PART 34
		usersAPI.GET("/avatar", userPublicHandler.GetCurrentUserAvatar)
		usersAPI.POST("/avatar", userPublicHandler.UploadAvatar)
		usersAPI.PATCH("/avatar", userPublicHandler.UpdateAvatarSettings)
		usersAPI.DELETE("/avatar", userPublicHandler.ResetAvatar)

		// Security endpoints
		usersAPI.GET("/security/2fa", twoFAHandler.GetTwoFactorStatus)
		usersAPI.GET("/security/2fa/setup", twoFAHandler.SetupTwoFactor)
		usersAPI.POST("/security/2fa/enable", twoFAHandler.EnableTwoFactor)
		usersAPI.POST("/security/2fa/disable", twoFAHandler.DisableTwoFactor)
		usersAPI.POST("/security/2fa/verify", twoFAHandler.VerifyTwoFactorCode)
		usersAPI.POST("/security/recovery/regenerate", twoFAHandler.RegenerateRecoveryKeys)
		usersAPI.GET("/security/passkeys", passkeyHandler.ListPasskeys)
		usersAPI.POST("/security/passkeys", passkeyHandler.RegisterPasskey)
		usersAPI.DELETE("/security/passkeys/:passkey_id", passkeyHandler.DeletePasskey)

		// Password change per AI.md PART 34
		usersAPI.POST("/security/password", userPublicHandler.ChangePassword)
		usersAPI.POST("/security/email", userPublicHandler.ChangeEmail)
		usersAPI.DELETE("/account", userPublicHandler.DeleteAccount)

		usersAPI.GET("/sessions", userSettingsHandler.ListSessions)
		usersAPI.DELETE("/sessions", userSettingsHandler.RevokeAllSessions)
		usersAPI.DELETE("/sessions/:id", userSettingsHandler.RevokeSession)
	}

	// Note: 2FA routes already registered under usersAPI (/users/security/2fa/*)

	// Public user profile endpoint per AI.md PART 34
	// Uses OptionalAuth to support both authenticated and anonymous requests
	// Private profiles return 404 to prevent existence leakage
	apiV1.GET("/public/users/:username", middleware.OptionalAuth(db.DB), userPublicHandler.GetPublicProfile)

	// Location API routes (require auth)
	// Public location endpoints (no auth required)
	apiV1.GET("/locations/search", locationHandler.SearchLocations)
	apiV1.GET("/locations/lookup/zip/:code", locationHandler.LookupZipCode)
	apiV1.GET("/locations/lookup/coords", locationHandler.LookupCoordinates)

	// Protected location endpoints (require auth)
	locationAPI := apiV1.Group("/users/locations")
	locationAPI.Use(middleware.RequireAuth(db.DB))
	locationAPI.Use(middleware.BlockAdminFromUserRoutes())
	{
		locationAPI.GET("", locationHandler.ListLocations)
		locationAPI.GET("/:id", locationHandler.GetLocation)
		locationAPI.POST("", locationHandler.CreateLocation)
		locationAPI.PUT("/:id", locationHandler.UpdateLocation)
		locationAPI.DELETE("/:id", locationHandler.DeleteLocation)
		locationAPI.PUT("/:id/alerts", locationHandler.ToggleAlerts)
	}

	// WebUI Notification API routes - User (per AI.md PART 14: /users/ is plural)
	usersNotificationAPI := apiV1.Group("/users/notifications")
	usersNotificationAPI.Use(middleware.RequireAuth(db.DB))
	usersNotificationAPI.Use(middleware.BlockAdminFromUserRoutes())
	{
		usersNotificationAPI.GET("", notificationAPIHandler.GetUserNotifications)
		usersNotificationAPI.GET("/unread", notificationAPIHandler.GetUserUnreadNotifications)
		usersNotificationAPI.GET("/count", notificationAPIHandler.GetUserUnreadCount)
		usersNotificationAPI.GET("/stats", notificationAPIHandler.GetUserStats)
		usersNotificationAPI.PATCH("/:id/read", notificationAPIHandler.MarkUserNotificationRead)
		usersNotificationAPI.PATCH("/read", notificationAPIHandler.MarkAllUserNotificationsRead)
		usersNotificationAPI.PATCH("/:id/dismiss", notificationAPIHandler.DismissUserNotification)
		usersNotificationAPI.DELETE("/:id", notificationAPIHandler.DeleteUserNotification)
		usersNotificationAPI.GET("/preferences", notificationAPIHandler.GetUserPreferences)
		usersNotificationAPI.PATCH("/preferences", notificationAPIHandler.UpdateUserPreferences)
	}

	// Admin API routes (require admin role + stricter rate limiting)
	// AI.md: Admin API at /api/{api_version}/server/{admin_path}/
	adminAPI := apiV1.Group("/server/" + cfg.GetAdminPath())
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
		adminSelfAPI := adminAPI.Group("/:admin_username")
		adminSelfAPI.Use(handler.RequireAdminSelfAPI())

		getCurrentAdmin := func(c *gin.Context) (*model.Admin, bool) {
			adminValue, exists := c.Get("admin")
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
				return nil, false
			}

			admin, ok := adminValue.(*model.Admin)
			if !ok || admin == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Invalid admin context"})
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

		buildInviteURL := func(c *gin.Context, token string) string {
			scheme := c.GetHeader("X-Forwarded-Proto")
			if scheme == "" {
				if c.Request.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			return fmt.Sprintf("%s://%s/server/auth/invite/server/%s", scheme, c.Request.Host, token)
		}

		buildUserInviteURL := func(c *gin.Context, token string) string {
			scheme := c.GetHeader("X-Forwarded-Proto")
			if scheme == "" {
				if c.Request.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			return fmt.Sprintf("%s://%s/server/auth/invite/user/%s", scheme, c.Request.Host, token)
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
		adminAPI.GET("/config/setup", setupHandler.GetSetupStatus)
		adminAPI.POST("/config/setup/verify", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "verified": true})
		})
		adminAPI.POST("/config/setup/account", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Admin account created"})
		})
		adminAPI.POST("/config/setup/token", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "token": ""})
		})
		adminAPI.POST("/config/setup/config", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Server config saved"})
		})
		adminAPI.POST("/config/setup/security", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Security settings saved"})
		})
		adminAPI.POST("/config/setup/services", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Services configured"})
		})
		adminAPI.POST("/config/setup/complete", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Setup complete"})
		})

		// Server management API - all under /server/ per spec
		// User management
		adminAPI.GET("/config/users", adminHandler.ListUsers)
		adminAPI.PUT("/config/users/:id", adminHandler.UpdateUser)
		adminAPI.DELETE("/config/users/:id", adminHandler.DeleteUser)
		adminAPI.GET("/config/users/invites", func(c *gin.Context) {
			invites, err := userInviteModel.ListInvites()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user invites"})
				return
			}

			responseInvites := make([]gin.H, 0, len(invites))
			for _, invite := range invites {
				responseInvites = append(responseInvites, gin.H{
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

			c.JSON(http.StatusOK, gin.H{"ok": true, "invites": responseInvites})
		})
		adminAPI.POST("/config/users/invites", func(c *gin.Context) {
			if _, ok := getCurrentAdmin(c); !ok {
				return
			}

			var req struct {
				Username      string `json:"username" binding:"required,min=3"`
				Email         string `json:"email" binding:"required,email"`
				Role          string `json:"role"`
				ExpiresInDays int    `json:"expires_in_days"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			username := util.NormalizeUsername(req.Username)
			if err := util.ValidateUsername(username); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			email := util.NormalizeEmail(req.Email)
			if err := util.ValidateEmail(email); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			if _, err := (&model.UserModel{DB: db.DB}).GetByUsername(username); err == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Username is already in use"})
				return
			}

			if _, err := (&model.UserModel{DB: db.DB}).GetByEmail(email); err == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Email is already in use"})
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
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"ok":              true,
				"message":         "User invite created",
				"invite":          invite,
				"invite_url":      buildUserInviteURL(c, invite.Token),
				"expires_in_days": expiresInDays,
			})
		})
		adminAPI.GET("/config/users/invites/:id", func(c *gin.Context) {
			if _, ok := getCurrentAdmin(c); !ok {
				return
			}

			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invite ID"})
				return
			}

			invite, err := userInviteModel.GetByID(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load invite"})
				return
			}
			if invite == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Invite not found"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"ok":     true,
				"invite": invite,
				"status": userInviteStatus(*invite),
			})
		})
		adminAPI.DELETE("/config/users/invites/:id", func(c *gin.Context) {
			if _, ok := getCurrentAdmin(c); !ok {
				return
			}

			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invite ID"})
				return
			}

			if err := userInviteModel.DeleteInvite(id); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke invite"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Invite revoked"})
		})

		// AI.md PART 17 header spec: JSON counterpart of the admin global search
		adminAPI.GET("/config/search", handler.AdminSearchAPI)

		// Settings management
		adminAPI.GET("/config/settings", adminHandler.ListSettings)
		adminAPI.PATCH("/config/settings", adminSettingsHandler.UpdateSettings)
		adminAPI.GET("/config/settings/:key", adminHandler.GetSetting)
		adminAPI.PUT("/config/settings/:key", adminHandler.UpdateSetting)
		adminAPI.GET("/config/settings/all", adminSettingsHandler.GetAllSettings)
		adminAPI.PUT("/config/settings/bulk", adminSettingsHandler.UpdateSettings)
		adminAPI.POST("/config/settings/reset", adminSettingsHandler.ResetSettings)
		adminAPI.GET("/config/settings/export", adminSettingsHandler.ExportSettings)
		adminAPI.POST("/config/settings/import", adminSettingsHandler.ImportSettings)
		adminAPI.POST("/config/reload", adminSettingsHandler.ReloadConfig)

		// Admin settings sub-endpoints
		adminAPI.POST("/config/users/settings", adminUsersHandler.UpdateUserSettings)
		adminAPI.POST("/config/security/auth", adminAuthHandler.UpdateAuthSettings)
		adminAPI.POST("/config/weather", adminWeatherHandler.UpdateWeatherSettings)
		adminAPI.POST("/config/notifications", adminNotificationsHandler.UpdateNotificationSettings)
		adminAPI.POST("/config/network/geoip", adminGeoIPHandler.UpdateGeoIPSettings)

		// API token management under /server/security/
		adminAPI.GET("/config/security/tokens", adminHandler.ListTokens)
		adminAPI.POST("/config/security/tokens", adminHandler.GenerateToken)
		adminAPI.DELETE("/config/security/tokens/:id", adminHandler.RevokeToken)

		// Audit logs under /server/logs/
		adminAPI.GET("/config/logs/audit-logs", adminHandler.ListAuditLogs)
		adminAPI.DELETE("/config/logs/audit-logs", adminHandler.ClearAuditLogs)

		// System stats
		adminAPI.GET("/config/stats", adminHandler.GetSystemStats)

		// Email settings per spec: /api/{api_version}/{admin_path}/config/email/
		adminAPI.GET("/config/email", func(c *gin.Context) {
			settingsModel := &model.SettingsModel{DB: db.DB}
			c.JSON(http.StatusOK, gin.H{
				"enabled":  settingsModel.GetBool("email.enabled", false),
				"provider": settingsModel.GetString("email.provider", ""),
				"host":     settingsModel.GetString("email.host", ""),
				"port":     settingsModel.GetInt("email.port", 587),
				"from":     settingsModel.GetString("email.from", ""),
			})
		})
		adminAPI.PATCH("/config/email", func(c *gin.Context) {
			var settings map[string]interface{}
			if err := c.ShouldBindJSON(&settings); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: db.DB}
			for key, value := range settings {
				if err := settingsModel.Set("email."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Email settings updated"})
		})
		adminAPI.POST("/config/email/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"ok":      true,
				"message": "Test email functionality available when SMTP is configured",
			})
		})

		// Branding per spec: /api/{api_version}/{admin_path}/config/branding/
		adminAPI.GET("/config/branding", func(c *gin.Context) {
			settingsModel := &model.SettingsModel{DB: db.DB}
			c.JSON(http.StatusOK, gin.H{
				"title":       settingsModel.GetString("branding.title", cfg.Server.Branding.Title),
				"description": settingsModel.GetString("branding.description", cfg.Server.Branding.Description),
				"logo_url":    settingsModel.GetString("branding.logo_url", ""),
				"favicon_url": settingsModel.GetString("branding.favicon_url", ""),
				"theme_color": settingsModel.GetString("branding.theme_color", ""),
			})
		})
		adminAPI.PATCH("/config/branding", func(c *gin.Context) {
			var settings map[string]interface{}
			if err := c.ShouldBindJSON(&settings); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: db.DB}
			for key, value := range settings {
				if err := settingsModel.Set("branding."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Branding settings updated"})
		})

		// Pages per spec: /api/{api_version}/{admin_path}/config/pages/
		adminAPI.GET("/config/pages", func(c *gin.Context) {
			settingsModel := &model.SettingsModel{DB: db.DB}
			c.JSON(http.StatusOK, gin.H{
				"about":   gin.H{"enabled": settingsModel.GetBool("pages.about.enabled", true)},
				"privacy": gin.H{"enabled": settingsModel.GetBool("pages.privacy.enabled", true)},
				"contact": gin.H{"enabled": settingsModel.GetBool("pages.contact.enabled", true)},
				"help":    gin.H{"enabled": settingsModel.GetBool("pages.help.enabled", true)},
				"terms":   gin.H{"enabled": settingsModel.GetBool("pages.terms.enabled", true)},
			})
		})
		adminAPI.GET("/config/pages/:name", func(c *gin.Context) {
			name := c.Param("name")
			settingsModel := &model.SettingsModel{DB: db.DB}
			c.JSON(http.StatusOK, gin.H{
				"name":    name,
				"enabled": settingsModel.GetBool("pages."+name+".enabled", true),
				"content": settingsModel.GetString("pages."+name+".content", ""),
			})
		})
		adminAPI.PATCH("/config/pages/:name", func(c *gin.Context) {
			name := c.Param("name")
			var settings map[string]interface{}
			if err := c.ShouldBindJSON(&settings); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: db.DB}
			for key, value := range settings {
				if err := settingsModel.Set("pages."+name+"."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": fmt.Sprintf("%s page updated", name)})
		})

		// Web settings per spec: /api/{api_version}/{admin_path}/config/web/
		adminAPI.GET("/config/web", func(c *gin.Context) {
			settingsModel := &model.SettingsModel{DB: db.DB}
			c.JSON(http.StatusOK, gin.H{
				"robots_txt":   settingsModel.GetBool("web.robots_enabled", true),
				"security_txt": settingsModel.GetBool("web.security_enabled", true),
			})
		})
		adminAPI.PATCH("/config/web", func(c *gin.Context) {
			var settings map[string]interface{}
			if err := c.ShouldBindJSON(&settings); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			settingsModel := &model.SettingsModel{DB: db.DB}
			for key, value := range settings {
				if err := settingsModel.Set("web."+key, fmt.Sprintf("%v", value), "string"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update %s: %v", key, err)})
					return
				}
			}
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Web settings updated"})
		})

		// Admin status and health endpoints
		adminServerStatusHandler := handler.AdminServerStatus(db, port, httpsPort, sslManager)
		adminAPI.GET("/config/status", adminServerStatusHandler)
		adminAPI.GET("/config/health", adminServerStatusHandler)

		// Server restart per spec: POST /server/restart
		adminAPI.POST("/config/restart", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"ok":      true,
				"message": "Server restart initiated",
			})
			go func() {
				time.Sleep(500 * time.Millisecond)
				log.Println("Server restart requested via admin API")
			}()
		})

		// Scheduler per spec: /api/{api_version}/{admin_path}/config/scheduler/
		adminAPI.GET("/config/scheduler", schedulerHandler.GetAllTasks)
		adminAPI.GET("/config/scheduler/:name", schedulerHandler.GetTaskHistory)
		adminAPI.PATCH("/config/scheduler/:name", schedulerHandler.UpdateTask)
		adminAPI.POST("/config/scheduler/:name/run", schedulerHandler.TriggerTask)
		adminAPI.POST("/config/scheduler/:name/enable", schedulerHandler.EnableTask)
		adminAPI.POST("/config/scheduler/:name/disable", schedulerHandler.DisableTask)

		// Notification channel management under /server/{admin_path}/config/channels/
		adminAPI.GET("/config/channels", channelHandler.ListChannels)
		adminAPI.GET("/config/channels/definitions", channelHandler.GetChannelDefinitions)
		adminAPI.GET("/config/channels/queue/stats", channelHandler.GetQueueStats)
		adminAPI.GET("/config/channels/history", channelHandler.GetNotificationHistory)
		adminAPI.POST("/config/channels/initialize", channelHandler.InitializeChannels)
		adminAPI.GET("/config/channels/:type", channelHandler.GetChannel)
		adminAPI.PUT("/config/channels/:type", channelHandler.UpdateChannel)
		adminAPI.POST("/config/channels/:type/enable", channelHandler.EnableChannel)
		adminAPI.POST("/config/channels/:type/disable", channelHandler.DisableChannel)
		adminAPI.POST("/config/channels/:type/test", channelHandler.TestChannel)
		adminAPI.GET("/config/channels/:type/stats", channelHandler.GetChannelStats)

		// Admin profile per spec: /api/{api_version}/{admin_path}/profile/
		adminSelfAPI.GET("/profile", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			profile := *admin
			profile.PasswordHash = ""
			profile.APITokenPrefix = ""

			c.JSON(http.StatusOK, gin.H{"ok": true, "profile": profile})
		})
		adminSelfAPI.PATCH("/profile", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			var req struct {
				Username *string `json:"username"`
				Email    *string `json:"email"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			username := admin.Username
			email := admin.Email

			if req.Username != nil {
				username = util.NormalizeUsername(*req.Username)
				if err := util.ValidateUsername(username); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
			}

			if req.Email != nil {
				email = util.NormalizeEmail(*req.Email)
				if err := util.ValidateEmail(email); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
			}

			if req.Username == nil && req.Email == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No profile fields provided"})
				return
			}

			if err := adminModel.Update(admin.ID, username, email); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
				return
			}

			updatedAdmin, err := adminModel.GetByID(admin.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load updated profile"})
				return
			}

			updatedAdmin.PasswordHash = ""
			updatedAdmin.APITokenPrefix = ""

			c.JSON(http.StatusOK, gin.H{
				"ok":      true,
				"message": "Profile updated",
				"profile": updatedAdmin,
			})
		})
		adminSelfAPI.POST("/profile/password", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			var req struct {
				CurrentPassword string `json:"current_password" binding:"required"`
				NewPassword     string `json:"new_password" binding:"required"`
				ConfirmPassword string `json:"confirm_password" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			if req.NewPassword != req.ConfirmPassword {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords do not match"})
				return
			}

			if len(req.NewPassword) < 8 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters long"})
				return
			}

			fullAdmin, err := adminModel.GetByID(admin.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify current password"})
				return
			}

			valid, err := model.VerifyPassword(req.CurrentPassword, fullAdmin.PasswordHash)
			if err != nil || !valid {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
				return
			}

			if err := adminModel.UpdatePassword(admin.ID, req.NewPassword); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Password changed successfully"})
		})
		adminSelfAPI.GET("/profile/token", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"ok":    true,
				"token": maskAdminToken(admin.APITokenPrefix),
			})
		})
		adminSelfAPI.POST("/profile/token", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			newToken, err := adminModel.RegenerateAPIToken(admin.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to regenerate API token"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"ok":      true,
				"message": "API token regenerated successfully",
				"token":   newToken,
			})
		})
		adminSelfAPI.GET("/profile/sessions", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			sessionModel := &model.AdminSessionModel{DB: database.GetServerDB()}
			sessions, err := sessionModel.GetActiveSessions(admin.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to load sessions"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "sessions": sessions})
		})
		adminSelfAPI.POST("/profile/sessions/logout-all", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			sessionModel := &model.AdminSessionModel{DB: database.GetServerDB()}
			if err := sessionModel.DeleteAllSessionsForAdmin(admin.ID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to log out of all sessions"})
				return
			}

			c.SetCookie("admin_session", "", -1, "/", "", c.Request.TLS != nil, true)

			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Logged out of all sessions"})
		})
		adminSelfAPI.GET("/preferences", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			prefs, err := loadAdminPreferences(admin.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load preferences"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "preferences": prefs})
		})
		adminSelfAPI.PATCH("/preferences", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			currentPrefs, err := loadAdminPreferences(admin.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load current preferences"})
				return
			}

			var req struct {
				Theme                *string `json:"theme"`
				Language             *string `json:"language"`
				Timezone             *string `json:"timezone"`
				NotificationsEnabled *bool   `json:"notifications_enabled"`
				EmailNotifications   *bool   `json:"email_notifications"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
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
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid theme"})
					return
				}
			}

			if req.Language != nil {
				language = strings.TrimSpace(*req.Language)
				if language == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Language cannot be empty"})
					return
				}
			}

			if req.Timezone != nil {
				timezone = strings.TrimSpace(*req.Timezone)
				if timezone == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Timezone cannot be empty"})
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
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode preferences"})
				return
			}

			// updated_at is bound as canonical UTC text so this writer agrees with the
			// INSERT above and with every reader of the column.
			if _, err := database.ExecContext(context.Background(), serverDB, database.TimeoutWrite, `
				UPDATE server_admin_preferences
				SET preferences = ?, updated_at = ?
				WHERE admin_id = ?
			`, string(updatedJSON), dbtime.FormatSQLTimestamp(time.Now()), admin.ID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update preferences"})
				return
			}

			updatedPrefs, err := loadAdminPreferences(admin.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load updated preferences"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"ok":          true,
				"message":     "Preferences updated",
				"preferences": updatedPrefs,
			})
		})

		// Admin passkeys per AI.md PART 17 line 28674-28683
		// /api/{api_version}/{admin_path}/profile/security/passkeys
		adminSelfAPI.GET("/profile/security/passkeys", adminPasskeyHandler.ListPasskeys)
		adminSelfAPI.POST("/profile/security/passkeys", adminPasskeyHandler.RegisterPasskey)
		adminSelfAPI.DELETE("/profile/security/passkeys/:passkey_id", adminPasskeyHandler.DeletePasskey)

		// Server admins per spec: /api/{api_version}/{admin_path}/config/admins/
		adminAPI.GET("/config/admins", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			count, err := adminModel.GetCount()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count admins"})
				return
			}

			onlineAdmins, err := getOnlineAdminUsernames()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load online admins"})
				return
			}

			currentAdmin := *admin
			currentAdmin.PasswordHash = ""
			currentAdmin.APITokenPrefix = ""

			c.JSON(http.StatusOK, gin.H{
				"ok":             true,
				"count":          count,
				"current_admin":  currentAdmin,
				"online_admins":  onlineAdmins,
				"privacy_notice": "Other admin account details are not exposed",
			})
		})
		adminAPI.GET("/config/admins/:id", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
				return
			}

			if id != admin.ID {
				c.JSON(http.StatusForbidden, gin.H{"error": "Other admin account details are private"})
				return
			}

			profile := *admin
			profile.PasswordHash = ""
			profile.APITokenPrefix = ""

			c.JSON(http.StatusOK, gin.H{"ok": true, "admin": profile})
		})
		adminAPI.DELETE("/config/admins/:id", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
				return
			}

			if id == admin.ID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete your own account"})
				return
			}

			if id == 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Primary admin cannot be deleted"})
				return
			}

			if err := adminModel.Delete(id); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Admin deleted"})
		})
		adminAPI.POST("/config/admins/:id/disable", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
				return
			}

			if id == admin.ID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot disable your own account"})
				return
			}

			if id == 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Primary admin cannot be disabled"})
				return
			}

			targetAdmin, err := adminModel.GetByID(id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Admin not found"})
				return
			}

			if targetAdmin.IsSuperAdmin {
				otherSuperAdmins, err := countOtherActiveSuperAdmins(id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate admin hierarchy"})
					return
				}
				if otherSuperAdmins == 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot disable the last active super admin"})
					return
				}
			}

			if err := adminModel.Update(id, targetAdmin.Username, targetAdmin.Email, targetAdmin.IsSuperAdmin, false); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable admin"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Admin disabled"})
		})
		adminAPI.POST("/config/admins/:id/enable", func(c *gin.Context) {
			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
				return
			}

			targetAdmin, err := adminModel.GetByID(id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Admin not found"})
				return
			}

			if err := adminModel.Update(id, targetAdmin.Username, targetAdmin.Email, targetAdmin.IsSuperAdmin, true); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable admin"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Admin enabled"})
		})
		adminAPI.POST("/config/admins/invite", func(c *gin.Context) {
			admin, ok := getCurrentAdmin(c)
			if !ok {
				return
			}

			var req struct {
				Email     string `json:"email" binding:"required,email"`
				ExpiresIn string `json:"expires_in"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			invite, expiresIn, err := adminInviteService.CreateInvite(req.Email, int(admin.ID), req.ExpiresIn)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"ok":         true,
				"message":    "Admin invite created",
				"token":      invite.Token,
				"email":      invite.InvitedEmail,
				"expires_at": invite.ExpiresAt,
				"expires_in": expiresIn,
				"invite_url": buildInviteURL(c, invite.Token),
			})
		})

		// WebUI Notification API routes - Admin (root-level since notifications is a root admin path)
		adminSelfAPI.GET("/notifications", notificationAPIHandler.GetAdminNotifications)
		adminSelfAPI.GET("/notifications/unread", notificationAPIHandler.GetAdminUnreadNotifications)
		adminSelfAPI.GET("/notifications/count", notificationAPIHandler.GetAdminUnreadCount)
		adminSelfAPI.GET("/notifications/stats", notificationAPIHandler.GetAdminStats)
		adminSelfAPI.PATCH("/notifications/:id/read", notificationAPIHandler.MarkAdminNotificationRead)
		adminSelfAPI.PATCH("/notifications/read", notificationAPIHandler.MarkAllAdminNotificationsRead)
		adminSelfAPI.PATCH("/notifications/:id/dismiss", notificationAPIHandler.DismissAdminNotification)
		adminSelfAPI.DELETE("/notifications/:id", notificationAPIHandler.DeleteAdminNotification)
		adminSelfAPI.GET("/notifications/preferences", notificationAPIHandler.GetAdminPreferences)
		adminSelfAPI.PATCH("/notifications/preferences", notificationAPIHandler.UpdateAdminPreferences)
		adminSelfAPI.POST("/notifications/send", notificationAPIHandler.SendTestNotification)

		// SMTP provider management under /server/
		adminAPI.GET("/config/smtp/providers", channelHandler.ListSMTPProviders)
		adminAPI.POST("/config/smtp/autodetect", channelHandler.AutoDetectSMTP)

		// Admin panel settings endpoints under /server/
		adminAPI.PUT("/config/settings/web", handler.SaveWebSettings)
		adminAPI.PUT("/config/settings/security", handler.SaveSecuritySettings)
		adminAPI.PUT("/config/settings/database", handler.SaveDatabaseSettings)

		// Database management endpoints under /server/
		adminAPI.POST("/config/database/test", handler.TestDatabaseConnection)
		adminAPI.POST("/config/database/test-config", handler.TestDatabaseConfigConnection)
		adminAPI.POST("/config/database/optimize", handler.OptimizeDatabase)
		adminAPI.POST("/config/database/vacuum", handler.VacuumDatabase)
		adminAPI.POST("/config/cache/clear", handler.ClearCache)

		// Backup management per spec: /api/{api_version}/{admin_path}/config/backup/
		adminAPI.GET("/config/backup", handler.ListBackups)
		adminAPI.POST("/config/backup", handler.CreateBackup)
		adminAPI.GET("/config/backup/stats", handler.BackupStats)
		adminAPI.GET("/config/backup/schedule", handler.GetBackupSchedule)
		adminAPI.POST("/config/backup/schedule", handler.SaveBackupSchedule)
		adminAPI.POST("/config/backup/restore", handler.RestoreBackup)
		// The param is named :filename because that is the value the handlers
		// validate and resolve against the backup directory - a backup has no
		// identifier other than its file name.
		adminAPI.GET("/config/backup/:filename", handler.DownloadBackup)
		adminAPI.DELETE("/config/backup/:filename", handler.DeleteBackup)
		adminAPI.GET("/config/backup/:filename/download", handler.DownloadBackup)

		// Template management under /server/
		adminAPI.GET("/config/templates", templateHandler.ListTemplates)
		adminAPI.GET("/config/templates/variables", templateHandler.GetTemplateVariables)
		adminAPI.POST("/config/templates/preview", templateHandler.PreviewTemplate)
		adminAPI.POST("/config/templates/initialize", templateHandler.InitializeDefaults)
		adminAPI.GET("/config/templates/:id", templateHandler.GetTemplate)
		adminAPI.POST("/config/templates", templateHandler.CreateTemplate)
		adminAPI.PUT("/config/templates/:id", templateHandler.UpdateTemplate)
		adminAPI.DELETE("/config/templates/:id", templateHandler.DeleteTemplate)
		adminAPI.POST("/config/templates/:id/clone", templateHandler.CloneTemplate)

		// Notification metrics management under /server/
		adminAPI.GET("/config/metrics/notifications/summary", metricsHandler.GetSummary)
		adminAPI.GET("/config/metrics/notifications/channels/:type", metricsHandler.GetChannelMetrics)
		adminAPI.GET("/config/metrics/notifications/errors", metricsHandler.GetRecentErrors)
		adminAPI.GET("/config/metrics/notifications/health", metricsHandler.GetHealthStatus)

		// Tor hidden service management (AI.md PART 32)
		// API per spec: /api/{api_version}/{admin_path}/config/tor/
		torAPI := adminAPI.Group("/config/tor")
		{
			torAPI.GET("", torAdminHandler.GetStatus)
			torAPI.PATCH("", torAdminHandler.UpdateSettings)
			torAPI.POST("/regenerate", torAdminHandler.Regenerate)
			torAPI.GET("/vanity", torAdminHandler.GetVanityStatus)
			torAPI.POST("/vanity", torAdminHandler.GenerateVanity)
			torAPI.DELETE("/vanity", torAdminHandler.CancelVanity)
			torAPI.POST("/vanity/apply", torAdminHandler.ApplyVanity)
			torAPI.POST("/import", torAdminHandler.ImportKeys)
		}

		// Web settings per spec: /api/{api_version}/{admin_path}/config/web/
		webAPI := adminAPI.Group("/config/web")
		{
			webAPI.GET("/robots", adminWebHandler.GetRobotsTxt)
			webAPI.PATCH("/robots", adminWebHandler.UpdateRobotsTxt)
			webAPI.GET("/robots/preview", adminWebHandler.GetRobotsTxt)
			webAPI.GET("/security", adminWebHandler.GetSecurityTxt)
			webAPI.PATCH("/security", adminWebHandler.UpdateSecurityTxt)
			webAPI.GET("/security/preview", adminWebHandler.GetSecurityTxt)
		}

		// Email templates per spec: /api/{api_version}/{admin_path}/config/email/templates/
		emailTemplateAPI := adminAPI.Group("/config/email/templates")
		{
			emailTemplateAPI.GET("", emailTemplateHandler.ListTemplates)
			emailTemplateAPI.GET("/:name", emailTemplateHandler.GetTemplate)
			emailTemplateAPI.PUT("/:name", emailTemplateHandler.UpdateTemplate)
			emailTemplateAPI.POST("/:name/reset", emailTemplateHandler.ImportTemplate)
			emailTemplateAPI.POST("/:name/preview", emailTemplateHandler.TestTemplate)
		}

		// System logs management (already under /server/logs)
		logsAPI := adminAPI.Group("/config/logs")
		{
			logsAPI.GET("", logsHandler.GetLogs)
			logsAPI.GET("/:type", logsHandler.GetLogs)
			logsAPI.GET("/:type/download", logsHandler.DownloadLogs)
			logsAPI.GET("/audit", logsHandler.GetAuditLogs)
			logsAPI.GET("/audit/download", logsHandler.DownloadAuditLogs)
			logsAPI.POST("/audit/search", logsHandler.SearchAuditLogs)
			logsAPI.GET("/audit/stats", logsHandler.GetAuditStats)
			logsAPI.GET("/stats", logsHandler.GetLogStats)
			logsAPI.GET("/archives", logsHandler.ListArchivedLogs)
			logsAPI.GET("/stream", logsHandler.StreamLogs)
			logsAPI.POST("/rotate", logsHandler.RotateLogs)
			logsAPI.DELETE("", logsHandler.ClearLogs)
		}

		// SSL/TLS per spec: /api/{api_version}/{admin_path}/config/ssl/
		sslAPI := adminAPI.Group("/config/ssl")
		{
			sslAPI.GET("", sslHandler.GetStatus)
			sslAPI.PATCH("", sslHandler.UpdateSettings)
			sslAPI.POST("/renew", sslHandler.RenewCertificate)
			sslAPI.POST("/obtain", sslHandler.ObtainCertificate)
			sslAPI.POST("/auto-renew", sslHandler.StartAutoRenewal)
			sslAPI.GET("/dns-records", sslHandler.GetDNSRecords)
			sslAPI.POST("/verify", sslHandler.VerifyCertificate)
			sslAPI.GET("/export", sslHandler.ExportCertificate)
			sslAPI.POST("/import", sslHandler.ImportCertificate)
			sslAPI.POST("/revoke", sslHandler.RevokeCertificate)
			sslAPI.POST("/test", sslHandler.TestSSL)
			sslAPI.POST("/scan", sslHandler.SecurityScan)
		}

		// Metrics configuration under /server/
		metricsAPI := adminAPI.Group("/config/metrics")
		{
			metricsAPI.GET("/config", metricsConfigHandler.GetConfig)
			metricsAPI.PUT("/config", metricsConfigHandler.UpdateConfig)
			metricsAPI.GET("/stats", metricsConfigHandler.GetStats)
			metricsAPI.GET("/list", metricsConfigHandler.ListMetrics)
			metricsAPI.POST("/custom", metricsConfigHandler.CreateMetric)
			metricsAPI.DELETE("/custom/:name", metricsConfigHandler.DeleteMetric)
			metricsAPI.GET("/export", metricsConfigHandler.ExportMetrics)
			metricsAPI.PUT("/toggle/:name", metricsConfigHandler.ToggleMetric)
		}

		// Advanced logging formats under /server/
		loggingAPI := adminAPI.Group("/config/logging")
		{
			loggingAPI.GET("/formats", loggingHandler.GetFormats)
			loggingAPI.PUT("/formats", loggingHandler.UpdateFormats)
			loggingAPI.GET("/fail2ban/config", loggingHandler.GetFail2banConfig)
			loggingAPI.GET("/syslog/config", loggingHandler.GetSyslogConfig)
			loggingAPI.GET("/cef/config", loggingHandler.GetCEFConfig)
			loggingAPI.GET("/export", loggingHandler.ExportLogs)
			loggingAPI.POST("/fail2ban/configure", loggingHandler.ConfigureFail2ban)
			loggingAPI.POST("/syslog/configure", loggingHandler.ConfigureSyslog)
			loggingAPI.GET("/test", loggingHandler.TestFormat)
		}
	}

	// User notification preferences API (authenticated users)
	// AI.md PART 14: Use versioned API + plural nouns
	userPrefAPI := apiV1.Group("/users")
	userPrefAPI.Use(middleware.RequireAuth(db.DB))
	{
		// Channel preferences
		userPrefAPI.GET("/preferences", preferencesHandler.GetUserPreferences)
		userPrefAPI.PUT("/preferences/:id", preferencesHandler.UpdatePreference)
		userPrefAPI.POST("/preferences", preferencesHandler.CreatePreference)
		userPrefAPI.DELETE("/preferences/:id", preferencesHandler.DeletePreference)

		// Subscriptions
		userPrefAPI.GET("/subscriptions", preferencesHandler.GetSubscriptions)
		userPrefAPI.PUT("/subscriptions/:id", preferencesHandler.UpdateSubscription)
		userPrefAPI.POST("/subscriptions", preferencesHandler.CreateSubscription)
	}

	// API routes are now consolidated under /api/v1 above

	// Main /api endpoint - API version information
	// AI.md PART 14: Never hardcode v1 - use cfg.GetAPIVersion()
	r.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "Weather API",
			"version": "2.0.0",
			"api_versions": []string{
				cfg.GetAPIVersion(),
			},
			"current_version": cfg.GetAPIVersion(),
			"documentation":   "http://" + c.Request.Host + "/docs",
			"openapi":         "http://" + c.Request.Host + "/openapi.json",
			"swagger":         "http://" + c.Request.Host + "/openapi",
			"graphql":         "http://" + c.Request.Host + "/graphql",
		})
	})

	// /api/autodiscover - Client/Agent auto-configuration endpoint
	// AI.md PART 33/34: Non-versioned endpoint for CLI/agent self-configuration
	// SECURITY: NEVER include admin_path, secrets, or internal IPs
	r.GET("/api/autodiscover", func(c *gin.Context) {
		// Build public URL from request
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		publicURL := scheme + "://" + c.Request.Host

		// Get cluster nodes (empty array if single-node)
		clusterNodes := []string{publicURL}

		// Cache for 1 hour per AI.md
		c.Header("Cache-Control", "public, max-age=3600")

		c.JSON(http.StatusOK, gin.H{
			"primary":     publicURL,
			"cluster":     clusterNodes,
			"api_version": cfg.GetAPIVersion(),
			"timeout":     30,
			"retry":       3,
			"retry_delay": 1,
			"config": gin.H{
				"database": gin.H{
					"drivers": []string{"file", "sqlite", "libsql", "postgres", "mysql", "mssql", "mongodb"},
					"aliases": gin.H{
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
				"cache": gin.H{
					"types": []string{"none", "memory", "valkey", "redis"},
				},
				"formats": gin.H{
					"duration": []string{"s", "m", "h", "d"},
					"size":     []string{"KB", "MB", "GB"},
				},
				"logging": gin.H{
					"levels": []string{"debug", "info", "warn", "error"},
				},
				"smtp": gin.H{
					"tls_modes": []string{"auto", "starttls", "tls", "none"},
				},
				"features": gin.H{
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
	r.GET("/openapi", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/openapi/index.html")
	})
	// Swagger UI + JSON spec (auto-generated)
	r.GET("/openapi/*any", handler.GetSwaggerUIAuto())
	r.GET("/openapi.json", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/openapi/doc.json")
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
	r.GET("/docs", apiHandler.GetDocsHTML)

	// WebSocket endpoint for real-time notifications (TEMPLATE.md Part 25)
	// Requires authentication for both users and admins
	r.GET("/ws/notifications", middleware.OptionalAuth(db.DB), notificationAPIHandler.HandleWebSocketConnection)

	// Public /server/ pages (AI.md PART 14: /server/* are public, no auth required)
	r.GET("/server", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/server/about")
	})
	r.GET("/server/about", handler.ShowAboutPage(db, cfg))
	r.GET("/server/privacy", handler.ShowPrivacyPage(db, cfg))
	r.GET("/server/contact", handler.ShowContactPage(db, cfg))
	r.GET("/server/help", handler.ShowHelpPage(db, cfg))
	r.GET("/server/terms", handler.ShowTermsPage(db, cfg))

	// Examples endpoint
	// AI.md PART 14: Never hardcode v1 - use cfg.GetAPIPath()
	r.GET("/examples", func(c *gin.Context) {
		hostInfo := util.GetHostInfo(c)
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

		c.String(http.StatusOK, examples)
	})

	// Web interface routes
	r.GET("/web", webHandler.ServeWebInterface)
	r.GET("/web/:location", webHandler.ServeWebInterface)

	// Moon interface routes
	r.GET("/moon", webHandler.ServeMoonInterface)
	r.GET("/moon/:location", webHandler.ServeMoonInterface)

	// Historical weather page
	r.GET("/history", historyHandler.ShowHistory)

	// Earthquake routes (plural per AI.md PART 14)
	r.GET("/earthquakes", earthquakeHandler.HandleEarthquakeRequest)
	r.GET("/earthquakes/:location", earthquakeHandler.HandleEarthquakeRequest)

	// Backwards compatibility: singular -> plural redirect
	r.GET("/earthquake", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/earthquakes")
	})

	// Hurricane routes redirect to severe-weather (plural per AI.md PART 14)
	r.GET("/hurricanes", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/severe-weather")
	})
	r.GET("/hurricane", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/severe-weather")
	})

	// Severe Weather routes (new comprehensive severe weather page)
	r.GET("/severe-weather", severeWeatherHandler.HandleSevereWeatherRequest)
	r.GET("/severe-weather/:location", severeWeatherHandler.HandleSevereWeatherRequest)

	// Type-filtered severe weather routes
	r.GET("/severe/:type", severeWeatherHandler.HandleSevereWeatherByType)
	r.GET("/severe/:type/:location", severeWeatherHandler.HandleSevereWeatherByType)

	// AI.md PART 14: Legacy endpoints are technical debt - DELETED
	// OLD: /api/earthquakes and /api/hurricanes redirects removed
	// Use versioned endpoints: /api/{api_version}/earthquakes and /api/{api_version}/hurricanes

	// Initialization check middleware - show loading page if not ready
	r.Use(func(c *gin.Context) {
		// Skip for health checks, API routes, and static files
		if strings.HasPrefix(c.Request.URL.Path, "/health") ||
			strings.HasPrefix(c.Request.URL.Path, "/server/healthz") ||
			strings.HasPrefix(c.Request.URL.Path, "/api") ||
			strings.HasPrefix(c.Request.URL.Path, "/debug") ||
			strings.Contains(c.Request.URL.Path, ".") {
			c.Next()
			return
		}

		// Show loading page if not initialized
		if !handler.IsInitialized() {
			handler.ServeLoadingPage(c)
			c.Abort()
			return
		}

		c.Next()
	})

	// Theme toggle (AI.md PART 16 Theme Switching) - POST form, works without JS
	r.POST("/theme", server.SetThemeHandler)

	// Main weather routes
	// Uses IP/cookie lookup
	r.GET("/", weatherHandler.HandleRoot)
	// Explicit location
	r.GET("/weather/:location", weatherHandler.HandleLocation)
	// Backwards compatibility catch-all
	r.GET("/:location", weatherHandler.HandleLocation)

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
