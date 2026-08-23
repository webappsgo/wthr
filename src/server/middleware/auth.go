package middleware

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

const (
	SessionCookieName = "weather_session"
	UserContextKey    = "user"
	// UserIDContextKey carries the authenticated user's numeric id; handlers, CSRF and rate limiting all read this key
	UserIDContextKey  = "user_id"
	SessionContextKey = "session"
)

// AuthMiddleware checks for valid session or API token
func AuthMiddleware(db *sql.DB, required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionModel := &model.SessionModel{DB: db}
			userModel := &model.UserModel{DB: db}
			tokenModel := &model.TokenModel{DB: db}

			var user *model.User
			var session *model.Session

			// First, check for API token in Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				// Extract token from "Bearer <token>" format
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && parts[0] == "Bearer" {
					token := parts[1]
					apiToken, err := tokenModel.GetByToken(token)
					if err == nil {
						// Valid API token found
						user, err = userModel.GetByID(int64(apiToken.UserID))
						if err == nil {
							// Update last used timestamp asynchronously
							go tokenModel.UpdateLastUsed(apiToken.ID)
							ctx := reqctx.Set(r.Context(), UserContextKey, user)
							// Handlers read the numeric id via reqctx.GetInt(UserIDContextKey); model.User.ID is int64, which GetInt cannot assert
							ctx = reqctx.Set(ctx, UserIDContextKey, int(user.ID))
							ctx = reqctx.Set(ctx, "auth_method", "api_token")
							r = r.WithContext(ctx)
							next.ServeHTTP(w, r)
							return
						}
					}
				}
			}

			// Check for session cookie
			sessionCookie, err := r.Cookie(SessionCookieName)
			if err == nil && sessionCookie.Value != "" {
				sessionID := sessionCookie.Value
				session, err = sessionModel.GetByID(sessionID)
				if err == nil {
					user, err = userModel.GetByID(int64(session.UserID))
					if err == nil {
						ctx := reqctx.Set(r.Context(), UserContextKey, user)
						// Handlers read the numeric id via reqctx.GetInt(UserIDContextKey); model.User.ID is int64, which GetInt cannot assert
						ctx = reqctx.Set(ctx, UserIDContextKey, int(user.ID))
						ctx = reqctx.Set(ctx, SessionContextKey, session)
						ctx = reqctx.Set(ctx, "auth_method", "session")
						r = r.WithContext(ctx)
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			// No valid authentication found
			if required {
				// Check if request is from browser or API
				acceptHeader := r.Header.Get("Accept")
				if strings.Contains(acceptHeader, "text/html") {
					http.Redirect(w, r, "/server/auth/login", http.StatusFound)
					return
				}

				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "Authentication required",
				})
				return
			}

			// Authentication not required, continue
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth is a convenience wrapper for required authentication
func RequireAuth(db *sql.DB) func(http.Handler) http.Handler {
	return AuthMiddleware(db, true)
}

// OptionalAuth is a convenience wrapper for optional authentication
func OptionalAuth(db *sql.DB) func(http.Handler) http.Handler {
	return AuthMiddleware(db, false)
}

// RequireAdmin checks if user has admin role
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userInterface, exists := reqctx.Get(r.Context(), UserContextKey)
			if !exists {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "Authentication required",
				})
				return
			}

			user, ok := userInterface.(*model.User)
			if !ok || user.Role != "admin" {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "Admin access required",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetCurrentUser retrieves the current user from context
func GetCurrentUser(r *http.Request) (*model.User, bool) {
	userInterface, exists := reqctx.Get(r.Context(), UserContextKey)
	if !exists {
		return nil, false
	}

	user, ok := userInterface.(*model.User)
	return user, ok
}

// GetCurrentSession retrieves the current session from context
func GetCurrentSession(r *http.Request) (*model.Session, bool) {
	sessionInterface, exists := reqctx.Get(r.Context(), SessionContextKey)
	if !exists {
		return nil, false
	}

	session, ok := sessionInterface.(*model.Session)
	return session, ok
}

// IsAuthenticated checks if user is authenticated
func IsAuthenticated(r *http.Request) bool {
	_, ok := GetCurrentUser(r)
	return ok
}

// IsAdmin checks if user is admin
func IsAdmin(r *http.Request) bool {
	user, ok := GetCurrentUser(r)
	return ok && user.Role == "admin"
}

// RestrictAdminToAdminRoutes middleware that forces admins to only access /server/admin routes
// Admins are treated as guest/anonymous on all non-admin routes
func RestrictAdminToAdminRoutes() func(http.Handler) http.Handler {
	// Get admin path from config (AI.md: use configurable admin_path)
	cfg, _ := config.LoadConfig()
	adminPath := "/server/admin"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Skip this middleware for admin routes, setup routes, API routes, static files, and auth routes
			if strings.HasPrefix(path, adminPath) ||
				strings.HasPrefix(path, "/api") ||
				strings.HasPrefix(path, "/static") ||
				strings.HasPrefix(path, "/server/auth/login") ||
				strings.HasPrefix(path, "/server/auth/logout") ||
				strings.HasPrefix(path, "/server/auth/register") ||
				strings.HasPrefix(path, "/server/healthz") ||
				strings.HasPrefix(path, "/healthz") ||
				strings.HasPrefix(path, "/debug") ||
				strings.HasPrefix(path, "/docs") {
				next.ServeHTTP(w, r)
				return
			}

			// Check if user is admin
			user, ok := GetCurrentUser(r)
			if ok && user.Role == "admin" {
				// Admin accessing non-admin route - treat as guest/anonymous
				// Clear user and session context so they appear as unauthenticated
				ctx := reqctx.Set(r.Context(), UserContextKey, nil)
				ctx = reqctx.Set(ctx, SessionContextKey, nil)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// BlockAdminFromUserRoutes blocks admin users from accessing /users routes
// Admins should only access admin routes
func BlockAdminFromUserRoutes() func(http.Handler) http.Handler {
	// Get admin path from config (AI.md: use configurable admin_path)
	cfg, _ := config.LoadConfig()
	adminPath := "/server/admin"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Only apply to /users routes
			if !strings.HasPrefix(path, "/users") {
				next.ServeHTTP(w, r)
				return
			}

			// Check if user is admin
			user, ok := GetCurrentUser(r)
			if ok && user.Role == "admin" {
				// Admin trying to access user route - block them
				acceptHeader := r.Header.Get("Accept")
				if strings.Contains(acceptHeader, "text/html") {
					// Redirect to admin dashboard for HTML requests
					http.Redirect(w, r, adminPath, http.StatusFound)
					return
				}

				// Return error for API requests
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "Admin users cannot access user routes. Please use /admin routes instead.",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
