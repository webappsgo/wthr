package middleware

import (
	"database/sql"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditLogger logs admin actions to the audit_log table
func AuditLogger(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only log admin routes
		if !isAdminRoute(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Skip GET requests (only log modifications)
		if c.Request.Method == "GET" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		// Get user info
		user, exists := GetCurrentUser(c)
		var userID *int64
		if exists {
			userID = &user.ID
		}

		// Get client info
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// Capture the response
		c.Next()

		// Determine action from method and path
		action := getActionFromRequest(c.Request.Method, c.Request.URL.Path)
		resource := getResourceFromPath(c.Request.URL.Path)

		// Log the action
		success := c.Writer.Status() >= 200 && c.Writer.Status() < 400

		_, err := db.Exec(`
			INSERT INTO audit_log (user_id, action, resource, ip_address, user_agent, created_at, success)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, userID, action, resource, clientIP, userAgent, time.Now(), success)

		if err != nil {
			// Log error but don't fail the request
			c.Error(err)
		}
	}
}

// isAdminRoute checks if the path is an admin route
func isAdminRoute(path string) bool {
	return len(path) >= 13 && path[:13] == "/server/admin" ||
		len(path) >= 20 && path[:20] == "/api/v1/server/admin"
}

// getActionFromRequest determines the action type from method and path
func getActionFromRequest(method, path string) string {
	switch method {
	case "POST":
		if contains(path, "/login") {
			return "login"
		}
		if contains(path, "/logout") {
			return "logout"
		}
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return method
	}
}

// getResourceFromPath extracts the resource name from the path
func getResourceFromPath(path string) string {
	// Remove /api/v1/server/admin or /server/admin prefix
	if len(path) >= 20 && path[:20] == "/api/v1/server/admin" {
		path = path[20:]
	} else if len(path) >= 13 && path[:13] == "/server/admin" {
		path = path[13:]
	}

	// Remove leading slash
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	// Extract first path segment
	for i, c := range path {
		if c == '/' {
			return path[:i]
		}
	}

	if path == "" {
		return "dashboard"
	}

	return path
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
