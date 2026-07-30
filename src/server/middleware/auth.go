package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/model"

	"github.com/gin-gonic/gin"
)

const (
	SessionCookieName = "weather_session"
	UserContextKey    = "user"
	SessionContextKey = "session"
)

// AuthMiddleware checks for valid session or API token
func AuthMiddleware(db *sql.DB, required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionModel := &models.SessionModel{DB: db}
		userModel := &models.UserModel{DB: db}
		tokenModel := &models.TokenModel{DB: db}

		var user *models.User
		var session *models.Session

		// First, check for API token in Authorization header
		authHeader := c.GetHeader("Authorization")
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
						c.Set(UserContextKey, user)
						c.Set("auth_method", "api_token")
						c.Next()
						return
					}
				}
			}
		}

		// Check for session cookie
		sessionID, err := c.Cookie(SessionCookieName)
		if err == nil && sessionID != "" {
			session, err = sessionModel.GetByID(sessionID)
			if err == nil {
				user, err = userModel.GetByID(int64(session.UserID))
				if err == nil {
					c.Set(UserContextKey, user)
					c.Set(SessionContextKey, session)
					c.Set("auth_method", "session")
					c.Next()
					return
				}
			}
		}

		// No valid authentication found
		if required {
			// Check if request is from browser or API
			acceptHeader := c.GetHeader("Accept")
			if strings.Contains(acceptHeader, "text/html") {
				c.Redirect(http.StatusFound, "/server/auth/login")
				c.Abort()
				return
			}

			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		// Authentication not required, continue
		c.Next()
	}
}

// RequireAuth is a convenience wrapper for required authentication
func RequireAuth(db *sql.DB) gin.HandlerFunc {
	return AuthMiddleware(db, true)
}

// OptionalAuth is a convenience wrapper for optional authentication
func OptionalAuth(db *sql.DB) gin.HandlerFunc {
	return AuthMiddleware(db, false)
}

// RequireAdmin checks if user has admin role
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userInterface, exists := c.Get(UserContextKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		user, ok := userInterface.(*models.User)
		if !ok || user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Admin access required",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetCurrentUser retrieves the current user from context
func GetCurrentUser(c *gin.Context) (*models.User, bool) {
	userInterface, exists := c.Get(UserContextKey)
	if !exists {
		return nil, false
	}

	user, ok := userInterface.(*models.User)
	return user, ok
}

// GetCurrentSession retrieves the current session from context
func GetCurrentSession(c *gin.Context) (*models.Session, bool) {
	sessionInterface, exists := c.Get(SessionContextKey)
	if !exists {
		return nil, false
	}

	session, ok := sessionInterface.(*models.Session)
	return session, ok
}

// IsAuthenticated checks if user is authenticated
func IsAuthenticated(c *gin.Context) bool {
	_, ok := GetCurrentUser(c)
	return ok
}

// IsAdmin checks if user is admin
func IsAdmin(c *gin.Context) bool {
	user, ok := GetCurrentUser(c)
	return ok && user.Role == "admin"
}

// RestrictAdminToAdminRoutes middleware that forces admins to only access /server/admin routes
// Admins are treated as guest/anonymous on all non-admin routes
func RestrictAdminToAdminRoutes() gin.HandlerFunc {
	// Get admin path from config (AI.md: use configurable admin_path)
	cfg, _ := config.LoadConfig()
	adminPath := "/server/admin"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip this middleware for admin routes, setup routes, API routes, static files, and auth routes
		if strings.HasPrefix(path, adminPath) ||
			strings.HasPrefix(path, "/api") ||
			strings.HasPrefix(path, "/static") ||
			strings.HasPrefix(path, "/server/auth/login") ||
			strings.HasPrefix(path, "/server/auth/logout") ||
			strings.HasPrefix(path, "/server/auth/register") ||
			strings.HasPrefix(path, "/healthz") ||
			strings.HasPrefix(path, "/debug") ||
			strings.HasPrefix(path, "/docs") {
			c.Next()
			return
		}

		// Check if user is admin
		user, ok := GetCurrentUser(c)
		if ok && user.Role == "admin" {
			// Admin accessing non-admin route - treat as guest/anonymous
			// Clear user and session context so they appear as unauthenticated
			c.Set(UserContextKey, nil)
			c.Set(SessionContextKey, nil)
		}

		c.Next()
	}
}

// BlockAdminFromUserRoutes blocks admin users from accessing /users routes
// Admins should only access admin routes
func BlockAdminFromUserRoutes() gin.HandlerFunc {
	// Get admin path from config (AI.md: use configurable admin_path)
	cfg, _ := config.LoadConfig()
	adminPath := "/server/admin"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Only apply to /users routes
		if !strings.HasPrefix(path, "/users") {
			c.Next()
			return
		}

		// Check if user is admin
		user, ok := GetCurrentUser(c)
		if ok && user.Role == "admin" {
			// Admin trying to access user route - block them
			acceptHeader := c.GetHeader("Accept")
			if strings.Contains(acceptHeader, "text/html") {
				// Redirect to admin dashboard for HTML requests
				c.Redirect(http.StatusFound, adminPath)
				c.Abort()
				return
			}

			// Return error for API requests
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Admin users cannot access user routes. Please use /admin routes instead.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
