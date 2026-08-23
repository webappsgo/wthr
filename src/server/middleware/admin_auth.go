package middleware

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// RenderHTML renders a named template to w for the given request. It is
// wired up by main.go during startup (dependency injection), since the
// template registry gin's LoadHTMLGlob populated is owned by main.go and
// middleware cannot import the handler package (handler already imports
// middleware, so importing it back here would create an import cycle).
var RenderHTML func(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{})

// RequireAdminAuth checks if user is authenticated as admin
// Shows admin login page if not authenticated per AI.md PART 18
func RequireAdminAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Load config for branding
			cfg, err := config.LoadConfig()
			var title string
			if err == nil && cfg.Server.Branding.Title != "" {
				title = cfg.Server.Branding.Title
			} else {
				title = "Weather"
			}

			// Get version from main package
			version := GetVersion()

			// Check for admin session
			sessionCookie, err := r.Cookie("admin_session")
			if err != nil || sessionCookie.Value == "" {
				// No session - show login page per AI.md PART 18
				RenderHTML(w, r, http.StatusOK, "admin/login.tmpl", map[string]interface{}{
					"branding": map[string]interface{}{
						"Title": title,
					},
					"version": version,
				})
				return
			}
			session := sessionCookie.Value

			// Validate session against database (AI.md PART 18 requirement)
			// Check if session exists in server_admin_sessions table and is not expired
			db := GetDB(r)
			if db == nil {
				// Database not available - reject
				RenderHTML(w, r, http.StatusServiceUnavailable, "admin/login.tmpl", map[string]interface{}{
					"error": "Service temporarily unavailable",
					"branding": map[string]interface{}{
						"Title": title,
					},
					"version": version,
				})
				return
			}

			// Query server_admin_sessions table for the session row.
			//
			// expires_at is judged in Go, never by SQL. "expires_at >
			// CURRENT_TIMESTAMP" is a raw lexicographic TEXT comparison, and this
			// column can hold either canonical UTC text or the local-zone
			// time.Time.String() form an older build bound directly. A row written
			// in a zone behind UTC therefore compared as still valid for hours
			// after it had really expired (authentication bypass), and one written
			// ahead of UTC compared as expired while still live (denial of
			// service); wrapping the column in datetime() instead yields NULL for
			// the local-zone layout, so the predicate matches nothing at all.
			// dbtime.IsAfter resolves both layouts to the same absolute instant and
			// reports false for a NULL or unparseable value, so a session this
			// project cannot interpret is rejected rather than trusted.
			var adminID int
			var storedExpiresAt interface{}
			err = database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, `
				SELECT admin_id, expires_at
				FROM server_admin_sessions
				WHERE id = ?
			`, session).Scan(&adminID, &storedExpiresAt)

			if err != nil || !dbtime.IsAfter(storedExpiresAt, time.Now().UTC()) {
				// Invalid or expired session - show login
				RenderHTML(w, r, http.StatusOK, "admin/login.tmpl", map[string]interface{}{
					"branding": map[string]interface{}{
						"Title": title,
					},
					"version": version,
				})
				return
			}

			// Valid session - store admin_id in context for handlers
			ctx := reqctx.Set(r.Context(), "admin_id", adminID)
			r = r.WithContext(ctx)

			// Session valid - continue to admin panel
			next.ServeHTTP(w, r)
		})
	}
}

// GetVersion returns the application version (helper for middleware)
func GetVersion() string {
	// This will be injected by main package
	// Fallback, overridden by main.Version
	return "1.0.0"
}

// GetDB returns the database connection from context (helper for middleware)
func GetDB(r *http.Request) *sql.DB {
	if db, exists := reqctx.Get(r.Context(), "db"); exists {
		if sqlDB, ok := db.(*sql.DB); ok {
			return sqlDB
		}
	}
	return database.GetServerDB()
}

// AdminLoginHandler handles admin login per AI.md PART 18
func AdminLoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Load config for branding and HTTPS detection
		cfg, _ := config.LoadConfig()
		title := "Weather"
		if cfg != nil && cfg.Server.Branding.Title != "" {
			title = cfg.Server.Branding.Title
		}
		version := GetVersion()

		username := r.PostFormValue("username")
		password := r.PostFormValue("password")
		remember := r.PostFormValue("remember") == "1"

		// Input validation
		if username == "" || password == "" {
			RenderHTML(w, r, http.StatusUnauthorized, "admin/login.tmpl", map[string]interface{}{
				"error": "Invalid username or password",
				"branding": map[string]interface{}{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		// Query users.db server_admin_credentials table for username (AI.md PART 23)
		var adminID int
		var passwordHash string
		err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, `
			SELECT id, password_hash
			FROM server_admin_credentials
			WHERE username = ?
		`, username).Scan(&adminID, &passwordHash)

		if err == sql.ErrNoRows {
			// Admin not found - generic error to prevent enumeration
			RenderHTML(w, r, http.StatusUnauthorized, "admin/login.tmpl", map[string]interface{}{
				"error": "Invalid credentials",
				"branding": map[string]interface{}{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		if err != nil {
			// Database error
			RenderHTML(w, r, http.StatusInternalServerError, "admin/login.tmpl", map[string]interface{}{
				"error": "An error occurred. Please try again.",
				"branding": map[string]interface{}{
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
			RenderHTML(w, r, http.StatusUnauthorized, "admin/login.tmpl", map[string]interface{}{
				"error": "Invalid credentials",
				"branding": map[string]interface{}{
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

		// Create session in server_admin_sessions table (AI.md PART 5).
		//
		// Both timestamps are bound as canonical UTC text produced in Go rather
		// than by datetime(?, 'unixepoch')/CURRENT_TIMESTAMP: those are SQLite
		// spellings that do not exist on PostgreSQL or MySQL, and binding the
		// value keeps this writer in the one layout every reader parses.
		now := time.Now()
		expiresAt := now.Add(time.Duration(maxAge) * time.Second)
		_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
			INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, expires_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, sessionToken, adminID, util.TrustedGetClientIP(r), r.UserAgent(), dbtime.FormatSQLTimestamp(expiresAt), dbtime.FormatSQLTimestamp(now))

		if err != nil {
			RenderHTML(w, r, http.StatusInternalServerError, "admin/login.tmpl", map[string]interface{}{
				"error": "Failed to create session. Please try again.",
				"branding": map[string]interface{}{
					"Title": title,
				},
				"version": version,
			})
			return
		}

		// Detect if HTTPS is being used
		isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

		// Get admin path from config (AI.md: use configurable admin_path)
		adminPath := "/server/admin"
		if cfg != nil {
			adminPath = "/server/" + cfg.GetAdminPath()
		}

		// Set admin_session cookie with proper security (AI.md PART 18)
		http.SetCookie(w, &http.Cookie{
			Name:   "admin_session",
			Value:  sessionToken,
			MaxAge: maxAge,
			Path:   adminPath,
			Domain: "",
			// Secure flag - true if HTTPS
			Secure: isHTTPS,
			// HttpOnly - prevent JavaScript access
			HttpOnly: true,
		})

		// Update last_login timestamp, bound as canonical UTC text so the value
		// is identical on every driver rather than whatever CURRENT_TIMESTAMP
		// means to the backend in use.
		database.ExecContext(context.Background(), db, database.TimeoutWrite, "UPDATE server_admin_credentials SET last_login_at = ? WHERE id = ?", dbtime.FormatSQLTimestamp(now), adminID)

		// Redirect to admin dashboard
		http.Redirect(w, r, adminPath+"/dashboard", http.StatusFound)
	}
}

// AdminLogoutHandler handles admin logout per AI.md PART 18
func AdminLogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get admin path from config (AI.md: use configurable admin_path)
		cfg, _ := config.LoadConfig()
		adminPath := "/server/admin"
		if cfg != nil {
			adminPath = "/server/" + cfg.GetAdminPath()
		}

		// Clear admin session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "admin_session",
			Value:    "",
			MaxAge:   -1,
			Path:     adminPath,
			Domain:   "",
			Secure:   false,
			HttpOnly: true,
		})

		// Redirect to admin login
		http.Redirect(w, r, adminPath, http.StatusFound)
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
	valid, err := model.VerifyPassword(password, hash)
	if err != nil {
		// Log error but don't expose to user
		return false
	}
	return valid
}
