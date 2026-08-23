package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/httprate"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// Rate limit constants per AI.md PART 1: Security-First Design
const (
	// Login attempts: 5 per 15 minutes
	LoginRequestsPerWindow = 5
	LoginWindowDuration    = 15 * time.Minute

	// Password reset: 3 per 1 hour
	PasswordResetRequestsPerWindow = 3
	PasswordResetWindowDuration    = 1 * time.Hour

	// API (authenticated): 100 per 1 minute per AI.md PART 1
	APIAuthRequestsPerWindow = 100
	APIAuthWindowDuration    = 1 * time.Minute

	// API (unauthenticated): 20 per 1 minute per AI.md PART 1
	APIUnauthRequestsPerWindow = 20
	APIUnauthWindowDuration    = 1 * time.Minute

	// Registration: 5 per 1 hour
	RegistrationRequestsPerWindow = 5
	RegistrationWindowDuration    = 1 * time.Hour

	// File upload: 10 per 1 hour
	FileUploadRequestsPerWindow = 10
	FileUploadWindowDuration    = 1 * time.Hour

	// Admin: 30 per 15 minutes
	AdminRequestsPerWindow = 30
	AdminWindowDuration    = 15 * time.Minute

	// Global rate limit (DDoS protection)
	GlobalRPS   = 100
	GlobalBurst = 200
)

var (
	// Rate limiters initialized in init()
	globalLimiter        *httprate.RateLimiter
	loginLimiter         *httprate.RateLimiter
	passwordResetLimiter *httprate.RateLimiter
	apiAuthLimiter       *httprate.RateLimiter
	apiUnauthLimiter     *httprate.RateLimiter
	registrationLimiter  *httprate.RateLimiter
	fileUploadLimiter    *httprate.RateLimiter
	adminLimiter         *httprate.RateLimiter
)

func init() {
	// Initialize all rate limiters per AI.md PART 1 specifications
	globalLimiter = httprate.NewRateLimiter(
		GlobalRPS,
		time.Second,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	loginLimiter = httprate.NewRateLimiter(
		LoginRequestsPerWindow,
		LoginWindowDuration,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	passwordResetLimiter = httprate.NewRateLimiter(
		PasswordResetRequestsPerWindow,
		PasswordResetWindowDuration,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	apiAuthLimiter = httprate.NewRateLimiter(
		APIAuthRequestsPerWindow,
		APIAuthWindowDuration,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	apiUnauthLimiter = httprate.NewRateLimiter(
		APIUnauthRequestsPerWindow,
		APIUnauthWindowDuration,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	registrationLimiter = httprate.NewRateLimiter(
		RegistrationRequestsPerWindow,
		RegistrationWindowDuration,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	fileUploadLimiter = httprate.NewRateLimiter(
		FileUploadRequestsPerWindow,
		FileUploadWindowDuration,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)

	adminLimiter = httprate.NewRateLimiter(
		AdminRequestsPerWindow,
		AdminWindowDuration,
		httprate.WithKeyFuncs(httprate.KeyByIP),
	)
}

// GlobalRateLimitMiddleware applies global rate limiting (100 req/s)
func GlobalRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(globalLimiter, GlobalRPS, time.Second)
}

// LoginRateLimitMiddleware applies login rate limiting (5 req/15min)
func LoginRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(loginLimiter, LoginRequestsPerWindow, LoginWindowDuration)
}

// PasswordResetRateLimitMiddleware applies password reset rate limiting (3 req/1hr)
func PasswordResetRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(passwordResetLimiter, PasswordResetRequestsPerWindow, PasswordResetWindowDuration)
}

// APIAuthRateLimitMiddleware applies authenticated API rate limiting (100 req/1min)
func APIAuthRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(apiAuthLimiter, APIAuthRequestsPerWindow, APIAuthWindowDuration)
}

// APIUnauthRateLimitMiddleware applies unauthenticated API rate limiting (20 req/1min)
func APIUnauthRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(apiUnauthLimiter, APIUnauthRequestsPerWindow, APIUnauthWindowDuration)
}

// APIRateLimitMiddleware applies API rate limiting based on authentication status
func APIRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		authLimited := wrapRateLimiter(apiAuthLimiter, APIAuthRequestsPerWindow, APIAuthWindowDuration)(next)
		unauthLimited := wrapRateLimiter(apiUnauthLimiter, APIUnauthRequestsPerWindow, APIUnauthWindowDuration)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if user is authenticated
			_, exists := reqctx.Get(r.Context(), UserContextKey)
			if exists {
				// Authenticated: 100 req/min
				authLimited.ServeHTTP(w, r)
			} else {
				// Unauthenticated: 20 req/min
				unauthLimited.ServeHTTP(w, r)
			}
		})
	}
}

// RegistrationRateLimitMiddleware applies registration rate limiting (5 req/1hr)
func RegistrationRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(registrationLimiter, RegistrationRequestsPerWindow, RegistrationWindowDuration)
}

// FileUploadRateLimitMiddleware applies file upload rate limiting (10 req/1hr)
func FileUploadRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(fileUploadLimiter, FileUploadRequestsPerWindow, FileUploadWindowDuration)
}

// AdminRateLimitMiddleware applies admin rate limiting (30 req/15min)
func AdminRateLimitMiddleware() func(http.Handler) http.Handler {
	return wrapRateLimiter(adminLimiter, AdminRequestsPerWindow, AdminWindowDuration)
}

// wrapRateLimiter wraps httprate.RateLimiter for net/http
//
// httprate.RateLimiter.Handler only invokes the wrapped inner handler when
// the request is WITHIN the limit - when the limit is exceeded it calls
// RespondOnLimit (default: http.Error with 429) and returns without ever
// calling the inner handler. So the inner handler must only be used to
// detect the "allowed" case; the "exceeded" case is everything else. This
// bridge behavior (and the response-writer swallow-the-429 trick below) is a
// pre-existing pattern carried over verbatim from the gin version - it is not
// a behavior change, just a framework-API translation of the same bridge.
func wrapRateLimiter(limiter *httprate.RateLimiter, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled := false

			// Response writer wrapper: swallow the 429 that httprate's default
			// limit handler writes directly, so this middleware can produce its
			// own JSON body below instead.
			writer := &rateLimitResponseWriter{ResponseWriter: w}

			handler := limiter.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
			}))

			// Call httprate handler against the wrapper, not w directly, so a
			// rate-limited response never reaches the real writer.
			handler.ServeHTTP(writer, r)

			// If httprate never called the inner handler, the request was
			// rejected as over the limit.
			if !nextCalled {
				retryAfter := int(window.Seconds())
				// AI.md PART 14: operational metadata (retry timing) goes in the
				// Retry-After header, never an ad-hoc top-level body field.
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				// AI.md PART 9/14: canonical error shape with the RATE_LIMITED code.
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "RATE_LIMITED",
					"message": "Too many requests. Please try again later.",
				})
				return
			}

			// Set rate limit headers for allowed requests, then continue the
			// real chain.
			w.Header().Set("X-RateLimit-Limit", writer.Header().Get("X-RateLimit-Limit"))
			w.Header().Set("X-RateLimit-Remaining", writer.Header().Get("X-RateLimit-Remaining"))
			w.Header().Set("X-RateLimit-Reset", writer.Header().Get("X-RateLimit-Reset"))
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitResponseWriter wraps http.ResponseWriter to work with httprate
type rateLimitResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *rateLimitResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	if statusCode == http.StatusTooManyRequests {
		// Don't write header yet, let the calling middleware handle it
		return
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *rateLimitResponseWriter) Write(b []byte) (int, error) {
	if w.statusCode == http.StatusTooManyRequests {
		// Don't write body for rate limited requests
		// The calling middleware will handle the JSON response
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
