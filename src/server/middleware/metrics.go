// Package middleware provides HTTP middleware for security and request processing
// per AI.md PART 21: METRICS - HTTP Metrics Middleware
package middleware

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/webappsgo/wthr/src/server/metric"
)

var (
	// Regex patterns for normalizing paths (cardinality control)
	uuidRegex      = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	numericIDRegex = regexp.MustCompile(`/\d+(?:/|$)`)
	ulIDRegex      = regexp.MustCompile(`[0-9A-HJKMNP-TV-Z]{26}`)
)

// MetricsMiddleware records HTTP metrics for all requests per AI.md PART 21
func MetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Track active requests
			metric.HTTPActiveRequests.Inc()
			defer metric.HTTPActiveRequests.Dec()

			// Get normalized path (remove IDs for cardinality control).
			// chi.RouteContext(...).RoutePattern() is the FullPath()
			// equivalent - only populated once chi has matched a route, so
			// this still falls back to the raw URL path exactly as the gin
			// version fell back when FullPath() was empty (e.g. no matching
			// route, or middleware running before routing per PART 0 rule 6).
			path := normalizeMetricPath(chi.RouteContext(r.Context()).RoutePattern())
			if path == "" {
				path = normalizeMetricPath(r.URL.Path)
			}

			// Record request size
			if r.ContentLength > 0 {
				metric.HTTPRequestSize.WithLabelValues(r.Method, path).Observe(float64(r.ContentLength))
			}

			// Process request
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(ww.Status())
			responseSize := float64(ww.BytesWritten())
			if responseSize < 0 {
				responseSize = 0
			}

			metric.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
			metric.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
			metric.HTTPResponseSize.WithLabelValues(r.Method, path).Observe(responseSize)
		})
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
