package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/webappsgo/wthr/src/server/reqctx"
)

// RequestID middleware generates or extracts a unique request ID for each HTTP request
// Per TEMPLATE.md requirements:
// - Generates UUID v4 if no ID provided
// - Checks multiple header variants (X-Request-ID, X-Request-Id, X-Correlation-ID, etc.)
// - Sets request ID in context for logging
// - Includes request ID in response headers
//
// Request ID helps with:
// - Distributed tracing across services
// - Correlating logs for a single request
// - Debugging issues in production
const (
	// Context key for storing request ID
	RequestIDKey = "request_id"

	// Header names to check for existing request ID (in priority order)
	HeaderXRequestID     = "X-Request-ID"
	HeaderXRequestId     = "X-Request-Id"
	HeaderXCorrelationID = "X-Correlation-ID"
	HeaderXCorrelationId = "X-Correlation-Id"
	HeaderRequestID      = "Request-ID"
	HeaderRequestId      = "Request-Id"
	// Cloudflare request ID
	HeaderCFRay = "CF-Ray"
	// AWS request ID
	HeaderXAmznTraceID = "X-Amzn-Trace-Id"
	// GCP request ID
	HeaderXCloudTraceCtx = "X-Cloud-Trace-Context"
	// Zipkin/B3 trace ID
	HeaderXB3TraceID = "X-B3-TraceId"
)

// RequestID returns a middleware that manages request IDs
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var requestID string

			// Try to extract existing request ID from headers (check multiple variants)
			// This allows request IDs to be passed through from proxies, load balancers, or upstream services
			requestID = extractRequestID(r)

			// If no request ID found, generate a new UUID v4
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Store request ID in context for use in handlers and logging
			ctx := reqctx.Set(r.Context(), RequestIDKey, requestID)

			// Add request ID to response headers
			// Use standard X-Request-ID header for response
			w.Header().Set(HeaderXRequestID, requestID)

			// Continue processing request
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractRequestID tries to extract request ID from various headers
func extractRequestID(r *http.Request) string {
	// Check standard request ID headers in priority order
	headers := []string{
		HeaderXRequestID,
		HeaderXRequestId,
		HeaderXCorrelationID,
		HeaderXCorrelationId,
		HeaderRequestID,
		HeaderRequestId,
		HeaderCFRay,
		HeaderXAmznTraceID,
		HeaderXCloudTraceCtx,
		HeaderXB3TraceID,
	}

	for _, header := range headers {
		if id := r.Header.Get(header); id != "" {
			return id
		}
	}

	return ""
}

// GetRequestID retrieves the request ID from the context
// Returns empty string if not found
func GetRequestID(ctx context.Context) string {
	return reqctx.GetString(ctx, RequestIDKey)
}

// MustGetRequestID retrieves the request ID from the context or panics
// Use this only in handlers where you're certain the middleware has run
func MustGetRequestID(ctx context.Context) string {
	return reqctx.MustGet(ctx, RequestIDKey).(string)
}
