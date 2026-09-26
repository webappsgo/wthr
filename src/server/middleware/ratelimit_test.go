package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/reqctx"
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

// rateLimitOKHandler writes a plain 200 "ok" body, mirroring the gin version's
// c.String(http.StatusOK, "ok") route handlers.
func rateLimitOKHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// TestRegistrationRateLimitMiddleware_BlocksAfterLimit actually exceeds the
// configured limit (RegistrationRequestsPerWindow = 5/hour) from a single
// source IP and verifies the 6th request is rejected with 429, while the
// first 5 succeed - this is the cheapest limiter to exhaust in a unit test.
func TestRegistrationRateLimitMiddleware_BlocksAfterLimit(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(RegistrationRateLimitMiddleware())
	router.Post("/server/auth/register", rateLimitOKHandler)

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
func TestLoginRateLimitMiddleware_BlocksAfterLimit(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(LoginRateLimitMiddleware())
	router.Post("/server/auth/login", rateLimitOKHandler)

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
// an ordinary request with no auth context is subject to the read bucket
// (server.rate_limit.read, 120/min by default), reflected in the
// X-RateLimit-Limit response header.
func TestAPIRateLimitMiddleware_AppliesUnauthenticatedLimitByDefault(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(APIRateLimitMiddleware())
	router.Get("/api/v1/weather", rateLimitOKHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	want := resolveRateLimitBuckets().ReadLimit
	if got := w.Header().Get("X-RateLimit-Limit"); got != fmt.Sprintf("%d", want) {
		t.Errorf("X-RateLimit-Limit = %q, want %d", got, want)
	}
}

// TestAPIRateLimitMiddleware_AppliesAuthenticatedLimit is a regression test
// for a real production bug: APIRateLimitMiddleware chose between the
// authenticated and unauthenticated limiter by checking a bare "user_id"
// context key that no middleware in this package ever set, so every
// authenticated request was throttled as anonymous.
//
// The check now keys on UserContextKey, and AuthMiddleware sets both
// UserContextKey and UserIDContextKey at every authentication site. This test
// mirrors that pair and asserts the request is served from the read bucket.
func TestAPIRateLimitMiddleware_AppliesAuthenticatedLimit(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		// Mirrors what auth.go's AuthMiddleware sets on a successfully
		// authenticated request: both the user object and its numeric id.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reqctx.SetValue(r.Context(), UserContextKey, "some-authenticated-user")
			ctx = reqctx.SetValue(ctx, UserIDContextKey, 7)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Use(APIRateLimitMiddleware())
	router.Get("/api/v1/weather", rateLimitOKHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	want := resolveRateLimitBuckets().ReadLimit
	if got := w.Header().Get("X-RateLimit-Limit"); got != fmt.Sprintf("%d", want) {
		t.Errorf("X-RateLimit-Limit = %q, want %d - an authenticated request must get the "+
			"read limit; if this fails, AuthMiddleware has stopped setting "+
			"UserContextKey and every logged-in caller is throttled as anonymous",
			got, want)
	}
}

// TestResolveRateLimitBuckets_FallsBackToSpecDefaults verifies that an unset
// or non-positive config value is replaced by the AI.md PART 12 default
// instead of disabling the limit.
func TestResolveRateLimitBuckets_FallsBackToSpecDefaults(t *testing.T) {
	original := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(original) })

	config.SetGlobalConfig(&config.AppConfig{
		Server: config.ServerConfig{
			RateLimit: config.RateLimitConfig{
				Enabled: true,
				Read:    config.RateLimitBucket{Requests: 0, Window: 0},
			},
		},
	})

	buckets := resolveRateLimitBuckets()
	if buckets.ReadLimit != 120 || buckets.ReadWindow != time.Minute {
		t.Errorf("read bucket = %d/%v, want 120/1m", buckets.ReadLimit, buckets.ReadWindow)
	}
	if buckets.WriteLimit != 10 {
		t.Errorf("write bucket = %d, want 10", buckets.WriteLimit)
	}
	if buckets.GlobalLimit != GlobalBurst {
		t.Errorf("global burst = %d, want %d", buckets.GlobalLimit, GlobalBurst)
	}
	if buckets.LoginLimit != LoginRequestsPerWindow {
		t.Errorf("login bucket = %d, want %d", buckets.LoginLimit, LoginRequestsPerWindow)
	}
}

// TestResolveRateLimitBuckets_UsesConfiguredValues verifies server.rate_limit.*
// actually reaches the limiters instead of being ignored.
func TestResolveRateLimitBuckets_UsesConfiguredValues(t *testing.T) {
	original := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(original) })

	config.SetGlobalConfig(&config.AppConfig{
		Server: config.ServerConfig{
			RateLimit: config.RateLimitConfig{
				Enabled:     true,
				Read:        config.RateLimitBucket{Requests: 7, Window: 30},
				Write:       config.RateLimitBucket{Requests: 3, Window: 30},
				Health:      config.RateLimitBucket{Requests: 9, Window: 30},
				GlobalBurst: 11,
				Auth: config.RateLimitAuthConfig{
					Login:         config.RateLimitBucket{Requests: 2, Window: 60},
					PasswordReset: config.RateLimitBucket{Requests: 4, Window: 120},
					Registration:  config.RateLimitBucket{Requests: 6, Window: 180},
				},
			},
		},
	})

	buckets := resolveRateLimitBuckets()
	if buckets.ReadLimit != 7 || buckets.ReadWindow != 30*time.Second {
		t.Errorf("read bucket = %d/%v, want 7/30s", buckets.ReadLimit, buckets.ReadWindow)
	}
	if buckets.HealthLimit != 9 || buckets.HealthWindow != 30*time.Second {
		t.Errorf("health bucket = %d/%v, want 9/30s", buckets.HealthLimit, buckets.HealthWindow)
	}
	if buckets.WriteLimit != 3 {
		t.Errorf("write bucket = %d, want 3", buckets.WriteLimit)
	}
	if buckets.GlobalLimit != 11 {
		t.Errorf("global burst = %d, want 11", buckets.GlobalLimit)
	}
	if buckets.LoginLimit != 2 || buckets.LoginWindow != time.Minute {
		t.Errorf("login bucket = %d/%v, want 2/1m", buckets.LoginLimit, buckets.LoginWindow)
	}
	if buckets.PasswordReset != 4 || buckets.PasswordResetT != 2*time.Minute {
		t.Errorf("password reset bucket = %d/%v, want 4/2m", buckets.PasswordReset, buckets.PasswordResetT)
	}
	if buckets.Registration != 6 || buckets.RegistrationT != 3*time.Minute {
		t.Errorf("registration bucket = %d/%v, want 6/3m", buckets.Registration, buckets.RegistrationT)
	}
}

// TestRateLimitEnabled_FalseDisablesLimiting verifies server.rate_limit.enabled
// is honored end to end: with it off, the request chain is reached no matter how
// many times the same IP hits the route.
func TestRateLimitEnabled_FalseDisablesLimiting(t *testing.T) {
	original := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(original) })

	config.SetGlobalConfig(&config.AppConfig{
		Server: config.ServerConfig{
			RateLimit: config.RateLimitConfig{Enabled: false},
		},
	})

	router := chi.NewRouter()
	router.Use(GlobalRateLimitMiddleware())
	router.Get("/ping", rateLimitOKHandler)

	for i := 1; i <= 25; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "198.51.100.7:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (limiting disabled)", i, w.Code)
		}
	}
}

// TestWriteRateLimitMiddleware_BlocksAfterLimit exercises the write bucket
// from a single IP and asserts the request past the limit is rejected.
func TestWriteRateLimitMiddleware_BlocksAfterLimit(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(WriteRateLimitMiddleware())
	router.Post("/server/config", rateLimitOKHandler)

	limit := resolveRateLimitBuckets().WriteLimit
	for i := 1; i <= limit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/server/config", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (within limit)", i, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/server/config", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request beyond limit: status = %d, want 429", w.Code)
	}
}

// TestRateLimitResponse_RetryAfterHeader verifies the 429 carries the
// canonical RATE_LIMITED body plus the Retry-After header, per AI.md PART 12.
func TestRateLimitResponse_RetryAfterHeader(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(RegistrationRateLimitMiddleware())
	router.Post("/server/auth/register", rateLimitOKHandler)

	for i := 0; i <= resolveRateLimitBuckets().Registration; i++ {
		req := httptest.NewRequest(http.MethodPost, "/server/auth/register", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			if got := w.Header().Get("Retry-After"); got == "" {
				t.Error("Retry-After header missing on 429")
			}
			var body struct {
				OK      bool   `json:"ok"`
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.OK || body.Error != "RATE_LIMITED" || body.Message == "" {
				t.Errorf("body = %+v, want ok=false error=RATE_LIMITED with a message", body)
			}
			return
		}
	}
	t.Error("limiter never returned 429")
}

// TestRateLimitMiddleware_CanonicalRejectionShape verifies the 429 response
// body follows AI.md PART 12, which names this exact shape: the canonical
// PART 9/14 error envelope ({"ok":false,"error":"RATE_LIMITED"}) plus a
// top-level "retry_after" field, carried alongside the Retry-After header.
// PART 14's general ban on ad-hoc top-level body fields does not apply here —
// the more specific rate-limit section governs its own response shape.
func TestRateLimitMiddleware_CanonicalRejectionShape(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(RegistrationRateLimitMiddleware())
	router.Post("/server/auth/register", rateLimitOKHandler)

	// Exhaust the limit so the next request is rejected.
	for i := 1; i <= RegistrationRequestsPerWindow; i++ {
		req := httptest.NewRequest(http.MethodPost, "/server/auth/register", nil)
		req.RemoteAddr = ip
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodPost, "/server/auth/register", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	retryHeader := w.Header().Get("Retry-After")
	if retryHeader == "" {
		t.Fatal("Retry-After header missing on the 429")
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %v", err)
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Errorf("body[\"ok\"] = true, want false")
	}
	if got, _ := body["error"].(string); got != "RATE_LIMITED" {
		t.Errorf("body[\"error\"] = %q, want \"RATE_LIMITED\"", got)
	}
	// AI.md PART 12 requires the retry window in BOTH carriers: the header
	// and a top-level retry_after field. They must agree.
	bodyRetry, present := body["retry_after"]
	if !present {
		t.Fatal("body missing the retry_after field required by AI.md PART 12")
	}
	seconds, ok := bodyRetry.(float64)
	if !ok {
		t.Fatalf("body[\"retry_after\"] = %#v, want a JSON number", bodyRetry)
	}
	if got, want := strconv.Itoa(int(seconds)), retryHeader; got != want {
		t.Errorf("body retry_after = %s, Retry-After header = %s; the two must carry the same window", got, want)
	}
	if want := resolveRateLimitBuckets().RegistrationT; time.Duration(int(seconds)*int(time.Second)) != want {
		t.Errorf("body retry_after = %ds, want the bucket window %v", int(seconds), want)
	}
}

// TestGlobalRateLimitMiddleware_AllowsWithinBurst is a smoke test that the
// global limiter (GlobalBurst=240 per AI.md PART 12) does not reject ordinary,
// low-volume traffic.
func TestGlobalRateLimitMiddleware_AllowsWithinBurst(t *testing.T) {
	ip := uniqueTestIP()

	router := chi.NewRouter()
	router.Use(GlobalRateLimitMiddleware())
	router.Get("/", rateLimitOKHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestRateLimitMiddleware_LiveReloadsConfigChange verifies the AI.md PART 5
// live-reload requirement: a rate limit changed at runtime takes effect
// without rebuilding the router or restarting the process.
//
// This is the regression test for the real bug the rework fixed. chi evaluates
// r.Use() exactly once at registration, so the previous implementation — which
// captured the limiter, the limit, and the window when the middleware
// constructor ran — kept enforcing the configuration that was live at startup
// forever. The middleware now resolves the buckets inside the request handler
// and rebuilds the cached limiter set when the resolved config differs, so the
// very same router enforces the new limit.
func TestRateLimitMiddleware_LiveReloadsConfigChange(t *testing.T) {
	original := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(original) })

	setWriteLimit := func(requests int) {
		config.SetGlobalConfig(&config.AppConfig{
			Server: config.ServerConfig{
				RateLimit: config.RateLimitConfig{
					Enabled: true,
					Write:   config.RateLimitBucket{Requests: requests, Window: 60},
				},
			},
		})
	}

	// One router, built once, reused for both phases. If the limit were still
	// captured at registration time, phase two would enforce the phase-one
	// value and this test would fail.
	router := chi.NewRouter()
	router.Use(WriteRateLimitMiddleware())
	router.Post("/server/config", rateLimitOKHandler)

	// exhaust sends limit+1 POSTs from one IP and returns the status of the
	// last one: 200 while within budget, 429 once the limit is exceeded.
	exhaust := func(limit int) int {
		ip := uniqueTestIP()
		last := 0
		for i := 0; i <= limit; i++ {
			req := httptest.NewRequest(http.MethodPost, "/server/config", nil)
			req.RemoteAddr = ip
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			last = w.Code
		}
		return last
	}

	setWriteLimit(2)
	if got := exhaust(2); got != http.StatusTooManyRequests {
		t.Fatalf("with write limit 2, the third request returned %d, want 429", got)
	}

	// Live reload: the same router must now honor the new limit. A single
	// request is enough because the new limit of 1 rejects the second one.
	setWriteLimit(1)
	if got := exhaust(1); got != http.StatusTooManyRequests {
		t.Errorf("after lowering the write limit to 1, the second request returned %d, want 429 "+
			"— the change did not reach the already-registered router", got)
	}

	// And raising it must be honored too, proving the cache is not simply
	// re-frozen at the first configuration it happens to observe.
	setWriteLimit(3)
	if got := exhaust(3); got != http.StatusTooManyRequests {
		t.Errorf("with write limit 3, the fourth request returned %d, want 429", got)
	}
}
