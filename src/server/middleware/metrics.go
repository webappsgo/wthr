// Package middleware provides HTTP middleware for security and request processing
// per AI.md PART 21: METRICS - HTTP Metrics Middleware
package middleware

import (
	"regexp"
	"strconv"
	"time"

	"github.com/casapps/wthr/src/server/metrics"
	"github.com/gin-gonic/gin"
)

var (
	// Regex patterns for normalizing paths (cardinality control)
	uuidRegex      = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	numericIDRegex = regexp.MustCompile(`/\d+(?:/|$)`)
	ulIDRegex      = regexp.MustCompile(`[0-9A-HJKMNP-TV-Z]{26}`)
)

// MetricsMiddleware records HTTP metrics for all requests per AI.md PART 21
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Track active requests
		metrics.HTTPActiveRequests.Inc()
		defer metrics.HTTPActiveRequests.Dec()

		// Get normalized path (remove IDs for cardinality control)
		path := normalizeMetricPath(c.FullPath())
		if path == "" {
			path = normalizeMetricPath(c.Request.URL.Path)
		}

		// Record request size
		if c.Request.ContentLength > 0 {
			metrics.HTTPRequestSize.WithLabelValues(c.Request.Method, path).Observe(float64(c.Request.ContentLength))
		}

		// Process request
		c.Next()

		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		responseSize := float64(c.Writer.Size())
		if responseSize < 0 {
			responseSize = 0
		}

		metrics.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
		metrics.HTTPResponseSize.WithLabelValues(c.Request.Method, path).Observe(responseSize)
	}
}

// normalizeMetricPath normalizes URL path for consistent metric labels
// Replaces dynamic segments (UUIDs, IDs) with placeholders
func normalizeMetricPath(path string) string {
	if path == "" {
		return "/"
	}
	// Replace UUIDs
	path = uuidRegex.ReplaceAllString(path, ":id")
	// Replace ULIDs
	path = ulIDRegex.ReplaceAllString(path, ":id")
	// Replace numeric IDs
	path = numericIDRegex.ReplaceAllString(path, "/:id/")
	return path
}
