package middleware

import (
	"context"
	"crypto/rand"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// AuditLogger logs admin actions to the server_audit_log table
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
		status := "success"
		if !success {
			status = "failure"
		}

		actorType := "anonymous"
		var actorID string
		if userID != nil {
			actorType = "user"
			actorID = strconv.FormatInt(*userID, 10)
		}

		now := time.Now()
		id := ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()

		_, err := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
			INSERT INTO server_audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, ip_address, user_agent, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, now, actorType, actorID, action, resource, "", clientIP, userAgent, status)

		if err != nil {
			// Log error but don't fail the request
			c.Error(err)
		}
	}
}

// isAdminRoute checks if the path is an admin route, using the configured
// admin.path (AI.md config-rules.md: admin path is configurable, default
// "admin") rather than a hardcoded "/server/admin" prefix - matching the
// resolution pattern already used by auth.go's RestrictAdminToAdminRoutes.
func isAdminRoute(path string) bool {
	adminPath := "/server/admin"
	if cfg, err := config.LoadConfig(); err == nil && cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}
	apiAdminPath := "/api/v1" + adminPath

	return strings.HasPrefix(path, adminPath) || strings.HasPrefix(path, apiAdminPath)
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
