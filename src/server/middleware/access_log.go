package middleware

import (
	"time"

	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
)

// AccessLogger creates middleware for logging HTTP requests
// TEMPLATE.md Part 25: Supports 7 log formats
func AccessLogger(logger *utils.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Process request
		c.Next()

		// Calculate request duration
		duration := time.Since(start)

		// Extract request details
		clientIP := c.ClientIP()
		method := c.Request.Method
		path := c.Request.URL.Path
		protocol := c.Request.Proto
		statusCode := c.Writer.Status()
		bodySize := int64(c.Writer.Size())
		referer := c.Request.Referer()
		userAgent := c.Request.UserAgent()

		// Get username from context (if authenticated)
		username := ""
		if user, exists := c.Get(UserContextKey); exists {
			if u, ok := user.(*models.User); ok && u != nil {
				username = u.Username
			}
		}

		// Log access (legacy method for backward compatibility)
		logger.Access(clientIP, username, method, path, protocol, statusCode, bodySize, referer, userAgent)

		// Also log slow requests as warnings
		if duration > 1*time.Second {
			logger.Error("Slow request: %s %s took %v", method, path, duration)
		}
	}
}

// AccessLoggerWithFormat creates middleware for logging HTTP requests with configurable format
// TEMPLATE.md Part 25: Support 7 log formats (apache, nginx, json, fail2ban, syslog, cef, text)
func AccessLoggerWithFormat(logger *utils.Logger, formatter *service.LogFormatter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Process request
		c.Next()

		// Extract log entry from request
		entry := service.ExtractLogEntry(c, start, c.Writer.Size())

		// Get username from context (if authenticated)
		if user, exists := c.Get(UserContextKey); exists {
			if u, ok := user.(*models.User); ok && u != nil {
				entry.Username = u.Username
			}
		}

		// Format and write log
		logLine := formatter.Format(entry)
		logger.Write(logLine)

		// Also log slow requests as warnings
		if entry.RequestTime > 1.0 {
			logger.Error("Slow request: %s %s took %.3fs", entry.Method, entry.Path, entry.RequestTime)
		}
	}
}
