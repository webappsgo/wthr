package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// uniqueTestIP returns a distinct RemoteAddr per call so tests exercising
// the package-level, init()-time httprate limiters (shared/cumulative state
// for the whole test binary, keyed by client IP via httprate.KeyByIP) don't
// bleed rate-limit state into each other.
var ratelimitTestIPCounter int

func uniqueTestIP() string {
	ratelimitTestIPCounter++
	return fmt.Sprintf("203.0.%d.%d:12345", (ratelimitTestIPCounter>>8)&0xff, ratelimitTestIPCounter&0xff)
}

// TestRegistrationRateLimitMiddleware_BlocksAfterLimit actually exceeds the
// configured limit (RegistrationRequestsPerWindow = 5/hour) from a single
// source IP and verifies the 6th request is rejected with 429, while the
// first 5 succeed - this is the cheapest limiter to exhaust in a unit test.
//
// This documents a real production bug: wrapRateLimiter (ratelimit.go:
// 164-205) assumes httprate's RateLimiter.Handler calls next.ServeHTTP even
// when the limit is exceeded, and tries to detect that case from inside the
// inner handler by checking the "X-RateLimit-Remaining" header. But
// httprate's Handler (limiter.go:136-150) calls RespondOnLimit and returns
// immediately WITHOUT calling next.ServeHTTP once the limit is exceeded -
// the default limit handler writes 429 directly via http.Error. wrapRate
// Limiter's rateLimitResponseWriter.WriteHeader (ratelimit.go:214-221)
// swallows that WriteHeader(429) call ("don't write header yet, let Gin
// handle it") betting that the inner handler will run afterward and trigger
// its own c.JSON(429, ...) - but since the inner handler never runs on the
// exceeded path, rateLimitExceeded stays false, nothing is ever written,
// c.Abort() is never called, and gin falls through to the real route
// handler, returning 200 for every rate-limited request. Rate limiting is
// completely non-functional in production. This test encodes the CORRECT
// expected behavior (429 once the limit is exceeded) and is expected to
// FAIL against the current implementation.
func TestRegistrationRateLimitMiddleware_BlocksAfterLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ip := uniqueTestIP()

	router := gin.New()
	router.Use(RegistrationRateLimitMiddleware())
	router.POST("/server/auth/register", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	for i := 1; i <= RegistrationRequestsPerWindow; i++ {
		req := httptest.NewRequest(http.MethodPost, "/server/auth/register", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (within limit)", i, w.Code)
		}
	}

	// One more request beyond the limit must be rejected.
	req := httptest.NewRequest(http.MethodPost, "/server/auth/register", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request beyond limit: status = %d, want 429", w.Code)
	}
}

// TestLoginRateLimitMiddleware_BlocksAfterLimit exercises the
// login-specific limiter (5/15min) similarly, from an isolated IP.
//
// Same underlying bug as TestRegistrationRateLimitMiddleware_BlocksAfterLimit
// above: wrapRateLimiter (ratelimit.go:164-205) never observes a rate-limit
// rejection from httprate.RateLimiter.Handler, so it never writes 429 or
// aborts, and the request falls through to the real route handler. This test
// encodes the CORRECT expected behavior and is expected to FAIL against the
// current implementation.
func TestLoginRateLimitMiddleware_BlocksAfterLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ip := uniqueTestIP()

	router := gin.New()
	router.Use(LoginRateLimitMiddleware())
	router.POST("/server/auth/login", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	for i := 1; i <= LoginRequestsPerWindow; i++ {
		req := httptest.NewRequest(http.MethodPost, "/server/auth/login", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (within limit)", i, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/server/auth/login", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request beyond limit: status = %d, want 429", w.Code)
	}
}

// TestAPIRateLimitMiddleware_AppliesUnauthenticatedLimitByDefault verifies
// an ordinary request with no auth context is subject to the unauthenticated
// limit (APIUnauthRequestsPerWindow = 20/min), reflected in the
// X-RateLimit-Limit response header.
func TestAPIRateLimitMiddleware_AppliesUnauthenticatedLimitByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ip := uniqueTestIP()

	router := gin.New()
	router.Use(APIRateLimitMiddleware())
	router.GET("/api/v1/weather", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("X-RateLimit-Limit"); got != fmt.Sprintf("%d", APIUnauthRequestsPerWindow) {
		t.Errorf("X-RateLimit-Limit = %q, want %d", got, APIUnauthRequestsPerWindow)
	}
}

// TestAPIRateLimitMiddleware_IgnoresUserIDContextKey documents a real
// production bug: APIRateLimitMiddleware (ratelimit.go:134-147) decides
// between the authenticated (100/min) and unauthenticated (20/min) limiter
// by checking c.Get("user_id"). No middleware in this package ever sets
// that key - auth.go's AuthMiddleware sets "user" (a *models.User) and
// admin_auth.go sets "admin_id", never "user_id". Consequently every
// authenticated request - regardless of how legitimately it authenticated -
// is throttled at the stricter 20/min unauthenticated rate instead of the
// intended 100/min, needlessly rate-limiting real logged-in traffic.
//
// This test encodes CORRECT expected behavior (a request from a context
// that has gone through real session authentication, i.e. has "user" set,
// should receive the authenticated 100/min limit) and is expected to FAIL
// against the current implementation, which only ever looks for "user_id".
func TestAPIRateLimitMiddleware_IgnoresUserIDContextKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ip := uniqueTestIP()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Mirrors what auth.go's AuthMiddleware actually sets on a
		// successfully authenticated request.
		c.Set("user", "some-authenticated-user")
		c.Next()
	})
	router.Use(APIRateLimitMiddleware())
	router.GET("/api/v1/weather", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("X-RateLimit-Limit"); got != fmt.Sprintf("%d", APIAuthRequestsPerWindow) {
		t.Errorf("X-RateLimit-Limit = %q, want %d - ratelimit.go:134-147 checks "+
			"c.Get(\"user_id\"), a key no middleware ever sets, so authenticated requests "+
			"are always throttled at the unauthenticated rate instead", got, APIAuthRequestsPerWindow)
	}
}

// TestGlobalRateLimitMiddleware_AllowsWithinBurst is a smoke test that the
// global limiter (GlobalRPS=100/GlobalBurst=200) does not reject ordinary,
// low-volume traffic.
func TestGlobalRateLimitMiddleware_AllowsWithinBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ip := uniqueTestIP()

	router := gin.New()
	router.Use(GlobalRateLimitMiddleware())
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
