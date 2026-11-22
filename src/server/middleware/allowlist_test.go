package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

func TestAllowlistMarksRequestsAndSkipsRateLimiting(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Security.Allowlist = []config.IPAllowlistEntry{{CIDR: "203.0.113.27"}}
	cfg.Server.RateLimit = config.DefaultRateLimitConfig()
	cfg.Server.RateLimit.Enabled = true
	cfg.Server.RateLimit.GlobalBurst = 1
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	calls := 0
	handler := AllowlistMiddleware()(GlobalRateLimitMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !reqctx.GetBool(r.Context(), "allowlisted") {
			t.Error("allowlisted request was not marked in context")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	for range 3 {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "203.0.113.27:443"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("allowlisted response = %d, want %d", recorder.Code, http.StatusNoContent)
		}
	}
	if calls != 3 {
		t.Fatalf("next handler calls = %d, want 3", calls)
	}
}
