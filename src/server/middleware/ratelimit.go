package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/httprate"
	"github.com/webappsgo/wthr/src/config"
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

	// Global rate limit (DDoS protection) - absolute ceiling across all
	// request types, per AI.md PART 12's rate limit table.
	GlobalRPS   = 100
	GlobalBurst = 240
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
	// limitersMu guards the cached limiter set. The limiters themselves are
	// goroutine-safe once built; only the pointer handoff needs the lock.
	limitersMu sync.Mutex
	// cachedBuckets records the resolved configuration the cached set was
	// built from, so a change can be detected without rebuilding per request.
	cachedBuckets rateLimitBuckets
	limiters      rateLimiterSet
)

// getRateLimiters returns the limiter set for the given buckets, rebuilding it
// whenever the resolved configuration differs from the one the cached set was
// built from. AI.md PART 5 makes live reload mandatory for every
// server.rate_limit.* setting, so a set built once at startup from the
// pre-reload config would silently ignore the admin panel's changes. The
// comparison is a plain struct equality (every field is a bool, int, or
// time.Duration), which costs far less than rebuilding a limiter per request.
func getRateLimiters(b rateLimitBuckets) rateLimiterSet {
	limitersMu.Lock()
	defer limitersMu.Unlock()

	if limiters.read != nil && cachedBuckets == b {
		return limiters
	}

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
	cachedBuckets = b
	return limiters
}

// pick returns the named limiter from a set the caller already holds. Every
// middleware calls this inside its request handler rather than capturing a
// limiter at construction time: chi's r.Use() evaluates a middleware
// constructor exactly once at registration, so a limiter captured then would
// stay frozen at its startup configuration for the life of the process no
// matter how the cache underneath it was managed.
func (s rateLimiterSet) pick(name rateLimiterName) *httprate.RateLimiter {
	switch name {
	case rateLimitGlobal:
		return s.global
	case rateLimitRead:
		return s.read
	case rateLimitHealth:
		return s.health
	case rateLimitWrite:
		return s.write
	case rateLimitLogin:
		return s.login
	case rateLimitPasswordReset:
		return s.passwordReset
	case rateLimitRegistration:
		return s.registration
	case rateLimitFileUpload:
		return s.fileUpload
	}
	return nil
}

// rateLimiterName identifies one limiter within the cached set.
type rateLimiterName int

const (
	rateLimitGlobal rateLimiterName = iota
	rateLimitRead
	rateLimitHealth
	rateLimitWrite
	rateLimitLogin
	rateLimitPasswordReset
	rateLimitRegistration
	rateLimitFileUpload
)

// bucket returns the effective limit and window for one limiter class.
func (b rateLimitBuckets) bucket(name rateLimiterName) (int, time.Duration) {
	switch name {
	case rateLimitGlobal:
		return b.GlobalLimit, b.GlobalWindow
	case rateLimitRead:
		return b.ReadLimit, b.ReadWindow
	case rateLimitHealth:
		return b.HealthLimit, b.HealthWindow
	case rateLimitWrite:
		return b.WriteLimit, b.WriteWindow
	case rateLimitLogin:
		return b.LoginLimit, b.LoginWindow
	case rateLimitPasswordReset:
		return b.PasswordReset, b.PasswordResetT
	case rateLimitRegistration:
		return b.Registration, b.RegistrationT
	case rateLimitFileUpload:
		return b.FileUpload, b.FileUploadT
	}
	return 0, 0
}

// newRateLimitMiddleware builds the shared per-class middleware. The
// configuration is resolved inside the request handler on every request, not
// when the constructor runs: chi evaluates r.Use() exactly once at
// registration, so anything captured then — the limiter, the limit, the
// window, even the enabled flag — is frozen for the process lifetime and no
// amount of cache management underneath it can make live reload work.
func newRateLimitMiddleware(name rateLimiterName) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b := resolveRateLimitBuckets()
			if !b.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			_, window := b.bucket(name)
			limiter := getRateLimiters(b).pick(name)
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			serveRateLimited(w, r, next, limiter, window)
		})
	}
}

// newMethodRateLimitMiddleware limits only the request methods in methods and
// lets every other method through untouched.
func newMethodRateLimitMiddleware(name rateLimiterName, methods ...string) func(http.Handler) http.Handler {
	limited := newRateLimitMiddleware(name)
	return func(next http.Handler) http.Handler {
		limitedNext := limited(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, method := range methods {
				if r.Method == method {
					limitedNext.ServeHTTP(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GlobalRateLimitMiddleware applies the configured global burst ceiling.
func GlobalRateLimitMiddleware() func(http.Handler) http.Handler {
	return newRateLimitMiddleware(rateLimitGlobal)
}

// LoginRateLimitMiddleware applies the configured login rate limit.
func LoginRateLimitMiddleware() func(http.Handler) http.Handler {
	return newRateLimitMiddleware(rateLimitLogin)
}

// PasswordResetRateLimitMiddleware applies the configured password reset rate limit.
func PasswordResetRateLimitMiddleware() func(http.Handler) http.Handler {
	return newRateLimitMiddleware(rateLimitPasswordReset)
}

// APIRateLimitMiddleware applies the read bucket's limit to API routes. AI.md
// PART 12's table gives GET/HEAD one budget for every caller, authenticated
// or not, so there is no per-authentication branch here.
func APIRateLimitMiddleware() func(http.Handler) http.Handler {
	return newRateLimitMiddleware(rateLimitRead)
}

// RegistrationRateLimitMiddleware applies the configured registration rate limit.
func RegistrationRateLimitMiddleware() func(http.Handler) http.Handler {
	return newRateLimitMiddleware(rateLimitRegistration)
}

// FileUploadRateLimitMiddleware applies file upload rate limiting (10 req/1hr)
func FileUploadRateLimitMiddleware() func(http.Handler) http.Handler {
	return newRateLimitMiddleware(rateLimitFileUpload)
}

// HealthRateLimitMiddleware applies the configured health/status rate limit.
func HealthRateLimitMiddleware() func(http.Handler) http.Handler {
	return newRateLimitMiddleware(rateLimitHealth)
}

// ReadRateLimitMiddleware applies the configured read rate limit to safe
// requests (GET, HEAD, OPTIONS), per AI.md PART 12's Read endpoint class.
func ReadRateLimitMiddleware() func(http.Handler) http.Handler {
	return newMethodRateLimitMiddleware(rateLimitRead, http.MethodGet, http.MethodHead, http.MethodOptions)
}

// WriteRateLimitMiddleware applies the configured write rate limit to
// mutating requests (POST, PUT, PATCH, DELETE) per AI.md PART 12; safe
// methods pass through to the read bucket's budget.
func WriteRateLimitMiddleware() func(http.Handler) http.Handler {
	return newMethodRateLimitMiddleware(rateLimitWrite, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete)
}

// serveRateLimited applies one httprate.RateLimiter to a request and, when the
// request is within the limit, continues the chain; when it is not, it writes
// the AI.md PART 12 429 response in place of calling next.
//
// httprate.RateLimiter.Handler only invokes the wrapped inner handler when
// the request is WITHIN the limit - when the limit is exceeded it calls
// RespondOnLimit (default: http.Error with 429) and returns without ever
// calling the inner handler. So the inner handler must only be used to
// detect the "allowed" case; the "exceeded" case is everything else. This
// bridge behavior (and the response-writer swallow-the-429 trick below) is a
// pre-existing pattern carried over verbatim from the gin version - it is not
// a behavior change, just a framework-API translation of the same bridge.
func serveRateLimited(w http.ResponseWriter, r *http.Request, next http.Handler, limiter *httprate.RateLimiter, window time.Duration) {
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
		// AI.md PART 12 mandates both carriers for the retry window:
		// the Retry-After header and a top-level retry_after field in
		// the body. PART 14's general "no ad-hoc top-level fields" rule
		// is overridden here by the rate-limit section, which names
		// this exact body shape.
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		// AI.md PART 9/14: canonical error shape with the RATE_LIMITED code.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":          false,
			"error":       "RATE_LIMITED",
			"message":     setupTranslate(r, "errors.rate_limit.too_many_requests"),
			"retry_after": retryAfter,
		})
		return
	}

	// Set rate limit headers for allowed requests, then continue the
	// real chain.
	w.Header().Set("X-RateLimit-Limit", writer.Header().Get("X-RateLimit-Limit"))
	w.Header().Set("X-RateLimit-Remaining", writer.Header().Get("X-RateLimit-Remaining"))
	w.Header().Set("X-RateLimit-Reset", writer.Header().Get("X-RateLimit-Reset"))
	next.ServeHTTP(w, r)
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
