package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/casapps/wthr/src/config"
	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/path"
	"github.com/casapps/wthr/src/util"

	"github.com/gin-gonic/gin"
)

// SetupTokenRequired shows setup token entry form at /server/admin when no admin exists
// AI.md: User navigates to /server/admin → User enters setup token → Redirect to /server/{admin_path}/server/setup
// AI.md: Admin panel (/server/admin) - YES (requires setup token) - accessible before setup
func SetupTokenRequired(db *sql.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		adminPath := "/server/" + cfg.GetAdminPath()

		// Only apply to admin routes
		if !strings.HasPrefix(path, adminPath) {
			c.Next()
			return
		}

		// Skip check for setup routes (setup wizard handles its own auth)
		setupPath := adminPath + "/server/setup"
		if strings.HasPrefix(path, setupPath) {
			c.Next()
			return
		}

		// Check if any admin exists
		var count int
		err := database.GetServerDB().QueryRow("SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
		if err != nil {
			c.Next()
			return
		}

		// If admin exists, setup is complete - use normal auth flow
		if count > 0 {
			c.Next()
			return
		}

		// No admin exists - check for setup token file
		configDir := paths.GetConfigDir()
		if !utils.SetupTokenExists(configDir) {
			// No setup token file - setup was somehow skipped, show error
			c.HTML(http.StatusServiceUnavailable, "error.tmpl", gin.H{
				"error":   "Server setup incomplete",
				"message": "Please restart the server to generate a setup token.",
			})
			c.Abort()
			return
		}

		// Check if user has valid setup token cookie
		tokenVerified, _ := c.Cookie("setup_token_verified")
		if tokenVerified == "true" {
			// Token verified - redirect to setup wizard to create admin account
			c.Redirect(http.StatusFound, setupPath)
			c.Abort()
			return
		}

		// No admin, setup token exists, no verified cookie - show token entry form at /admin
		// AI.md: Step 2: User navigates to /admin → Step 3: User enters setup token
		title := "Weather"
		cfgLoaded, _ := config.LoadConfig()
		if cfgLoaded != nil && cfgLoaded.Server.Branding.Title != "" {
			title = cfgLoaded.Server.Branding.Title
		}

		c.HTML(http.StatusOK, "admin/setup_token.tmpl", gin.H{
			"title":      title + " - Setup",
			"admin_path": adminPath,
			"branding": gin.H{
				"Title": title,
			},
		})
		c.Abort()
	}
}

// BlockSetupAfterComplete blocks access to setup routes after server setup is complete
// AI.md: Setup token file deleted after successful setup completion
func BlockSetupAfterComplete(db *sql.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if any admin exists - setup is complete when admin exists
		var count int
		err := database.GetServerDB().QueryRow("SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
		if err == nil && count > 0 {
			// Admin exists - setup complete, redirect to admin dashboard
			adminPath := "/server/" + cfg.GetAdminPath()
			c.Redirect(http.StatusFound, adminPath+"/dashboard")
			c.Abort()
			return
		}

		// Check if setup token file still exists
		configDir := paths.GetConfigDir()
		if !utils.SetupTokenExists(configDir) {
			// No setup token file and no admin - should not happen
			adminPath := "/server/" + cfg.GetAdminPath()
			c.Redirect(http.StatusFound, adminPath)
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireSetupTokenVerified ensures the setup token has been verified before accessing setup wizard
// AI.md: Step 3: User enters setup token → Step 4: Redirect to /{admin_path}/server/setup
func RequireSetupTokenVerified(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for setup_token_verified cookie
		tokenVerified, _ := c.Cookie("setup_token_verified")
		if tokenVerified != "true" {
			// No verified token - redirect to /server/admin to enter token
			adminPath := "/server/" + cfg.GetAdminPath()
			c.Redirect(http.StatusFound, adminPath)
			c.Abort()
			return
		}

		c.Next()
	}
}

// BlockSetupAfterAdminExists blocks access to admin setup if admin account already exists
func BlockSetupAfterAdminExists(db *sql.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if admin exists in server_admin_credentials
		var count int
		err := database.GetServerDB().QueryRow("SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			c.Abort()
			return
		}

		// If admin exists, redirect to admin dashboard
		if count > 0 {
			adminPath := "/server/" + cfg.GetAdminPath()
			c.Redirect(http.StatusFound, adminPath+"/dashboard")
			c.Abort()
			return
		}

		c.Next()
	}
}
