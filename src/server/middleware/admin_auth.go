package middleware

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/casapps/wthr/src/config"
	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/server/model"
)

// RequireAdminAuth checks if user is authenticated as admin
// Shows admin login page if not authenticated per AI.md PART 18
func RequireAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Load config for branding
		cfg, err := config.LoadConfig()
		var title string
		if err == nil && cfg.Server.Branding.Title != "" {
			title = cfg.Server.Branding.Title
		} else {
			title = "Weather Service"
		}

		// Get version from main package
		version := GetVersion()

		// Check for admin session
		session, err := c.Cookie("admin_session")
		if err != nil || session == "" {
			// No session - show login page per AI.md PART 18
			c.HTML(http.StatusOK, "admin/login.tmpl", gin.H{
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			c.Abort()
			return
		}

		// Validate session against database (AI.md PART 18 requirement)
		// Check if session exists in server_admin_sessions table and is not expired
		db := GetDB(c)
		if db == nil {
			// Database not available - reject
			c.HTML(http.StatusServiceUnavailable, "admin/login.tmpl", gin.H{
				"error": "Service temporarily unavailable",
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			c.Abort()
			return
		}

		// Query server_admin_sessions table for valid session
		var adminID int
		err = db.QueryRow(`
			SELECT admin_id
			FROM server_admin_sessions
			WHERE id = ? AND expires_at > CURRENT_TIMESTAMP
		`, session).Scan(&adminID)

		if err != nil {
			// Invalid or expired session - show login
			c.HTML(http.StatusOK, "admin/login.tmpl", gin.H{
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			c.Abort()
			return
		}

		// Valid session - store admin_id in context for handlers
		c.Set("admin_id", adminID)

		// Session valid - continue to admin panel
		c.Next()
	}
}

// GetVersion returns the application version (helper for middleware)
func GetVersion() string {
	// This will be injected by main package
	// Fallback, overridden by main.Version
	return "1.0.0"
}

// GetDB returns the database connection from context (helper for middleware)
func GetDB(c *gin.Context) *sql.DB {
	if db, exists := c.Get("db"); exists {
		if sqlDB, ok := db.(*sql.DB); ok {
			return sqlDB
		}
	}
	return database.GetServerDB()
}

// AdminLoginHandler handles admin login per AI.md PART 18
func AdminLoginHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Load config for branding and HTTPS detection
		cfg, _ := config.LoadConfig()
		title := "Weather Service"
		if cfg != nil && cfg.Server.Branding.Title != "" {
			title = cfg.Server.Branding.Title
		}
		version := GetVersion()

		username := c.PostForm("username")
		password := c.PostForm("password")
		remember := c.PostForm("remember") == "1"

		// Input validation
		if username == "" || password == "" {
			c.HTML(http.StatusUnauthorized, "admin/login.tmpl", gin.H{
				"error": "Invalid username or password",
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		// Query users.db server_admin_credentials table for username (AI.md PART 23)
		var adminID int
		var passwordHash string
		err := db.QueryRow(`
			SELECT id, password_hash
			FROM server_admin_credentials
			WHERE username = ?
		`, username).Scan(&adminID, &passwordHash)

		if err == sql.ErrNoRows {
			// Admin not found - generic error to prevent enumeration
			c.HTML(http.StatusUnauthorized, "admin/login.tmpl", gin.H{
				"error": "Invalid credentials",
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		if err != nil {
			// Database error
			c.HTML(http.StatusInternalServerError, "admin/login.tmpl", gin.H{
				"error": "An error occurred. Please try again.",
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		// Verify password hash with Argon2id (AI.md PART 3 requirement)
		// Password verification handled by admin model
		if !verifyPasswordHash(password, passwordHash) {
			// Invalid password - generic error
			c.HTML(http.StatusUnauthorized, "admin/login.tmpl", gin.H{
				"error": "Invalid credentials",
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		// Generate real session token (secure random, AI.md requirement)
		sessionToken := generateSessionToken()

		// Calculate expiry time
		// Remember me: 90 days, normal: 30 days per AI.md PART 18
		// 30 days in seconds
		maxAge := 30 * 24 * 60 * 60
		if remember {
			// 90 days
			maxAge = 90 * 24 * 60 * 60
		}

		// Create session in server_admin_sessions table (AI.md PART 5)
		expiresAt := time.Now().Unix() + int64(maxAge)
		_, err = db.Exec(`
			INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, expires_at, created_at)
			VALUES (?, ?, ?, ?, datetime(?, 'unixepoch'), CURRENT_TIMESTAMP)
		`, sessionToken, adminID, c.ClientIP(), c.Request.UserAgent(), expiresAt)

		if err != nil {
			c.HTML(http.StatusInternalServerError, "admin/login.tmpl", gin.H{
				"error": "Failed to create session. Please try again.",
				"branding": gin.H{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		// Detect if HTTPS is being used
		isHTTPS := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

		// Get admin path from config (AI.md: use configurable admin_path)
		adminPath := "/admin"
		if cfg != nil {
			adminPath = "/" + cfg.GetAdminPath()
		}

		// Set admin_session cookie with proper security (AI.md PART 18)
		c.SetCookie(
			"admin_session",
			sessionToken,
			maxAge,
			adminPath,
			"",
			// Secure flag - true if HTTPS
			isHTTPS,
			// HttpOnly - prevent JavaScript access
			true,
		)

		// Update last_login timestamp
		db.Exec("UPDATE server_admin_credentials SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?", adminID)

		// Redirect to admin dashboard
		c.Redirect(http.StatusFound, adminPath+"/dashboard")
	}
}

// AdminLogoutHandler handles admin logout per AI.md PART 18
func AdminLogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get admin path from config (AI.md: use configurable admin_path)
		cfg, _ := config.LoadConfig()
		adminPath := "/admin"
		if cfg != nil {
			adminPath = "/" + cfg.GetAdminPath()
		}

		// Clear admin session cookie
		c.SetCookie(
			"admin_session",
			"",
			-1,
			adminPath,
			"",
			false,
			true,
		)

		// Redirect to admin login
		c.Redirect(http.StatusFound, adminPath)
	}
}

// generateSessionToken generates a cryptographically secure session token
// Per AI.md PART 18: session tokens must be secure random
func generateSessionToken() string {
	// Use UUID for session token (secure random)
	// AI.md PART 18 requirement: cryptographically random session tokens
	// 32 bytes = 256 bits
	return generateSecureToken(32)
}

// generateSecureToken generates a secure random token of specified byte length.
// Fails closed: panics if crypto/rand is unavailable rather than falling back to
// a weak RNG. Per AI.md PART 18: session tokens MUST be cryptographically random.
func generateSecureToken(byteLength int) string {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable — cannot generate secure token: " + err.Error())
	}
	token := make([]byte, hex.EncodedLen(len(b)))
	hex.Encode(token, b)
	return string(token)
}

// verifyPasswordHash verifies a password against its Argon2id hash
// Per AI.md PART 3: MUST use Argon2id for password verification
func verifyPasswordHash(password, hash string) bool {
	// Use proper Argon2id verification from model package
	// AI.md PART 3 requirement: MUST use Argon2id with constant-time comparison
	valid, err := models.VerifyPassword(password, hash)
	if err != nil {
		// Log error but don't expose to user
		return false
	}
	return valid
}
