package middleware

import (
	"context"
	"database/sql"
	"log"
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
// Redirects to the shared /server/auth/login form when there is no valid admin session (AI.md PART 17)
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
				redirectAdminUnauthenticated(w, r)
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
				SELECT sessions.admin_id, sessions.expires_at
				FROM server_admin_sessions AS sessions
				INNER JOIN server_admin_credentials AS admins
					ON admins.id = sessions.admin_id AND admins.is_active = 1
				WHERE sessions.token_hash = ?
			`, model.HashAdminSessionToken(session)).Scan(&adminID, &storedExpiresAt)

			if err != nil || !dbtime.IsAfter(storedExpiresAt, time.Now().UTC()) {
				// Invalid or expired session
				redirectAdminUnauthenticated(w, r)
				return
			}

			// Valid session - store admin_id in context for handlers
			ctx := reqctx.SetValue(r.Context(), "admin_id", adminID)
			r = r.WithContext(ctx)

			// Session valid - continue to admin panel
			next.ServeHTTP(w, r)
		})
	}
}

// redirectAdminUnauthenticated sends a request without a valid admin session
// away from the admin panel with no admin-specific hint (AI.md PART 17):
// an authenticated regular user goes to their own dashboard, everyone else to
// the shared login form.
func redirectAdminUnauthenticated(w http.ResponseWriter, r *http.Request) {
	if IsAuthenticated(r) {
		http.Redirect(w, r, "/users/dashboard", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/server/auth/login", http.StatusFound)
}

// GetVersion returns the application version (helper for middleware)
func GetVersion() string {
	// This will be injected by main package
	// Fallback, overridden by main.Version
	return "1.0.0"
}

// GetDB returns the database connection from context (helper for middleware)
func GetDB(r *http.Request) *sql.DB {
	if db, exists := reqctx.GetValue(r.Context(), "db"); exists {
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
			WHERE username = ? AND is_active = 1
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

		// Calculate expiry time
		// Remember me: 90 days, normal: 30 days per AI.md PART 18
		// 30 days in seconds
		maxAge := 30 * 24 * 60 * 60
		if remember {
			// 90 days
			maxAge = 90 * 24 * 60 * 60
		}

		sessionModel := &model.AdminSessionModel{DB: db}
		adminSession, err := sessionModel.CreateSession(int64(adminID), util.TrustedGetClientIP(r), r.UserAgent(), time.Duration(maxAge)*time.Second)
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
		sessionToken := adminSession.SessionID
		now := time.Now()

		// Detect if HTTPS is being used
		isHTTPS := util.TrustedIsHTTPS(r)

		// Get admin path from config (AI.md: use configurable admin_path)
		adminPath := "/server/administration"
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
		adminPath := "/server/administration"
		if cfg != nil {
			adminPath = "/server/" + cfg.GetAdminPath()
		}
		if cookie, err := r.Cookie("admin_session"); err == nil && cookie.Value != "" {
			sessionModel := &model.AdminSessionModel{DB: GetDB(r)}
			if err := sessionModel.DeleteSession(cookie.Value); err != nil {
				log.Printf("WARNING: admin-logout: failed to delete admin session: %v", err)
			}
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
