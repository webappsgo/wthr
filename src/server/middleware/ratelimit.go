package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/httprate"
	"github.com/webappsgo/wthr/src/config"
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

	// Global rate limit (DDoS protection) - requests per window
	GlobalRPS   = 100
	GlobalBurst = 200
)

// rateLimitBuckets holds the effective limits for every limiter, resolved once
// from the loaded configuration per AI.md PART 12 ("All limits are
// configurable under `server.rate_limit.*` in `server.yml` and via the admin
// panel"). Classes the schema does not cover (file upload) keep their
// PART 1 defaults.
type rateLimitBuckets struct {
	Enabled        bool
	GlobalLimit    int
	GlobalWindow   time.Duration
	ReadLimit      int
	ReadWindow     time.Duration
	HealthLimit    int
	HealthWindow   time.Duration
	WriteLimit     int
	WriteWindow    time.Duration
	LoginLimit     int
	LoginWindow    time.Duration
	PasswordReset  int
	PasswordResetT time.Duration
	Registration   int
	RegistrationT  time.Duration
	FileUpload     int
	FileUploadT    time.Duration
}

// resolveRateLimitBuckets reads server.rate_limit.* from the loaded config,
// substituting the PART 1/PART 12 defaults for any unset or non-positive value.
func resolveRateLimitBuckets() rateLimitBuckets {
	limits := config.DefaultRateLimitConfig()
	if cfg := config.GetGlobalConfig(); cfg != nil {
		limits = cfg.Server.RateLimit
	}

	bucket := func(requests, window, defRequests, defWindow int) (int, time.Duration) {
		if requests <= 0 {
			requests = defRequests
		}
		if window <= 0 {
			window = defWindow
		}
		return requests, time.Duration(window) * time.Second
	}

	readLimit, readWindow := bucket(limits.Read.Requests, limits.Read.Window, 120, 60)
	healthLimit, healthWindow := bucket(limits.Health.Requests, limits.Health.Window, 120, 60)
	writeLimit, writeWindow := bucket(limits.Write.Requests, limits.Write.Window, 10, 60)
	loginLimit, loginWindow := bucket(limits.Auth.Login.Requests, limits.Auth.Login.Window, LoginRequestsPerWindow, 900)
	resetLimit, resetWindow := bucket(limits.Auth.PasswordReset.Requests, limits.Auth.PasswordReset.Window, PasswordResetRequestsPerWindow, 3600)
	regLimit, regWindow := bucket(limits.Auth.Registration.Requests, limits.Auth.Registration.Window, RegistrationRequestsPerWindow, 3600)

	globalLimit := limits.GlobalBurst
	if globalLimit <= 0 {
		globalLimit = GlobalBurst
	}

	return rateLimitBuckets{
		Enabled:        limits.Enabled,
		GlobalLimit:    globalLimit,
		GlobalWindow:   time.Minute,
		ReadLimit:      readLimit,
		ReadWindow:     readWindow,
		HealthLimit:    healthLimit,
		HealthWindow:   healthWindow,
		WriteLimit:     writeLimit,
		WriteWindow:    writeWindow,
		LoginLimit:     loginLimit,
		LoginWindow:    loginWindow,
		PasswordReset:  resetLimit,
		PasswordResetT: resetWindow,
		Registration:   regLimit,
		RegistrationT:  regWindow,
		FileUpload:     FileUploadRequestsPerWindow,
		FileUploadT:    FileUploadWindowDuration,
	}
}

type rateLimiterSet struct {
	global        *httprate.RateLimiter
	read          *httprate.RateLimiter
	health        *httprate.RateLimiter
	write         *httprate.RateLimiter
	login         *httprate.RateLimiter
	passwordReset *httprate.RateLimiter
	registration  *httprate.RateLimiter
	fileUpload    *httprate.RateLimiter
}

var (
	limiterOnce sync.Once
	limiters    rateLimiterSet
)

// getRateLimiters builds every limiter from the configured buckets on first
// use, which happens after the config has been loaded in main().
func getRateLimiters() rateLimiterSet {
	limiterOnce.Do(func() {
		b := resolveRateLimitBuckets()
		newLimiter := func(limit int, window time.Duration) *httprate.RateLimiter {
			return httprate.NewRateLimiter(limit, window, httprate.WithKeyFuncs(httprate.KeyByIP))
		}
		limiters = rateLimiterSet{
			global:        newLimiter(b.GlobalLimit, b.GlobalWindow),
			read:          newLimiter(b.ReadLimit, b.ReadWindow),
			health:        newLimiter(b.HealthLimit, b.HealthWindow),
			write:         newLimiter(b.WriteLimit, b.WriteWindow),
			login:         newLimiter(b.LoginLimit, b.LoginWindow),
			passwordReset: newLimiter(b.PasswordReset, b.PasswordResetT),
			registration:  newLimiter(b.Registration, b.RegistrationT),
			fileUpload:    newLimiter(b.FileUpload, b.FileUploadT),
		}
	})
	return limiters
}

// rateLimitEnabled reports whether server.rate_limit.enabled is set.
func rateLimitEnabled() bool {
	return resolveRateLimitBuckets().Enabled
}

// passthroughMiddleware returns a chain that never rate limits, used when
// server.rate_limit.enabled is false.
func passthroughMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return next
	}
}

// GlobalRateLimitMiddleware applies the configured global burst ceiling.
func GlobalRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	return wrapRateLimiter(getRateLimiters().global, b.GlobalLimit, b.GlobalWindow)
}

// LoginRateLimitMiddleware applies the configured login rate limit.
func LoginRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	return wrapRateLimiter(getRateLimiters().login, b.LoginLimit, b.LoginWindow)
}

// PasswordResetRateLimitMiddleware applies the configured password reset rate limit.
func PasswordResetRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	return wrapRateLimiter(getRateLimiters().passwordReset, b.PasswordReset, b.PasswordResetT)
}

// APIAuthRateLimitMiddleware applies the configured read rate limit for
// authenticated API callers.
func APIAuthRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	return wrapRateLimiter(getRateLimiters().read, b.ReadLimit, b.ReadWindow)
}

// APIUnauthRateLimitMiddleware applies the configured read rate limit for
// unauthenticated API callers.
func APIUnauthRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	return wrapRateLimiter(getRateLimiters().read, b.ReadLimit, b.ReadWindow)
}

// APIRateLimitMiddleware applies API rate limiting based on authentication status
func APIRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		authLimited := APIAuthRateLimitMiddleware()(next)
		unauthLimited := APIUnauthRateLimitMiddleware()(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if user is authenticated
			_, exists := reqctx.GetValue(r.Context(), UserContextKey)
			if exists {
				authLimited.ServeHTTP(w, r)
			} else {
				unauthLimited.ServeHTTP(w, r)
			}
		})
	}
}

// RegistrationRateLimitMiddleware applies the configured registration rate limit.
func RegistrationRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	return wrapRateLimiter(getRateLimiters().registration, b.Registration, b.RegistrationT)
}

// FileUploadRateLimitMiddleware applies file upload rate limiting (10 req/1hr)
func FileUploadRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	return wrapRateLimiter(getRateLimiters().fileUpload, FileUploadRequestsPerWindow, FileUploadWindowDuration)
}

// HealthRateLimitMiddleware applies the configured health/status rate limit.
func HealthRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	return wrapRateLimiter(getRateLimiters().health, b.HealthLimit, b.HealthWindow)
}

// ReadRateLimitMiddleware applies the configured read rate limit to safe
// requests (GET, HEAD, OPTIONS), per AI.md PART 12's Read endpoint class.
func ReadRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	limited := wrapRateLimiter(getRateLimiters().read, b.ReadLimit, b.ReadWindow)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				limited(next).ServeHTTP(w, r)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// WriteRateLimitMiddleware applies the configured write rate limit to
// mutating requests (POST, PUT, PATCH, DELETE) per AI.md PART 12; safe
// methods pass through to the read bucket's budget.
func WriteRateLimitMiddleware() func(http.Handler) http.Handler {
	if !rateLimitEnabled() {
		return passthroughMiddleware()
	}
	b := resolveRateLimitBuckets()
	limited := wrapRateLimiter(getRateLimiters().write, b.WriteLimit, b.WriteWindow)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				limited(next).ServeHTTP(w, r)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
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
