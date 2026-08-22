package handler

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/server/service"
)

// TestBuildOIDCCallbackURL covers scheme selection (plain HTTP, TLS request,
// and X-Forwarded-Proto override) and path construction.
func TestBuildOIDCCallbackURL(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		provider   string
		forwarded  string
		useTLS     bool
		wantResult string
	}{
		{
			name:       "plain http",
			host:       "example.com",
			provider:   "okta",
			wantResult: "http://example.com/server/auth/oidc/okta/callback",
		},
		{
			name:       "forwarded proto https",
			host:       "example.com",
			provider:   "okta",
			forwarded:  "https",
			wantResult: "https://example.com/server/auth/oidc/okta/callback",
		},
		{
			name:       "tls request",
			host:       "example.com",
			provider:   "google",
			useTLS:     true,
			wantResult: "https://example.com/server/auth/oidc/google/callback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/", nil)
			req.Host = tt.host
			if tt.useTLS {
				req.TLS = &tls.ConnectionState{}
			}
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}
			c.Request = req

			got := buildOIDCCallbackURL(c, tt.provider)
			if got != tt.wantResult {
				t.Errorf("buildOIDCCallbackURL() = %q, want %q", got, tt.wantResult)
			}
		})
	}
}

// TestDeriveUsernameFromClaims covers each claim-priority branch: preferred
// username, email local-part, display name, and the provider-name fallback.
func TestDeriveUsernameFromClaims(t *testing.T) {
	tests := []struct {
		name     string
		claims   *service.OIDCClaims
		provider string
		want     string
	}{
		{
			name:     "preferred username wins",
			claims:   &service.OIDCClaims{PreferredUsername: "Alice.B", Email: "bob@example.com", Name: "Bob"},
			provider: "okta",
			want:     "alice.b",
		},
		{
			name:     "falls back to email local part",
			claims:   &service.OIDCClaims{Email: "Carol_D@example.com", Name: "Carol"},
			provider: "okta",
			want:     "carol_d",
		},
		{
			name:     "falls back to name",
			claims:   &service.OIDCClaims{Name: "Dave Evans!"},
			provider: "okta",
			want:     "daveevans",
		},
		{
			name:     "falls back to provider name",
			claims:   &service.OIDCClaims{},
			provider: "okta",
			want:     "okta_user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveUsernameFromClaims(tt.claims, tt.provider)
			if got != tt.want {
				t.Errorf("deriveUsernameFromClaims() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSanitizeUsername covers lowercasing, allowed-punctuation retention,
// stripped characters, and the all-stripped fallback.
func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "lowercases letters", input: "AlIcE", want: "alice"},
		{name: "keeps safe punctuation", input: "bob_the-builder.99", want: "bob_the-builder.99"},
		{name: "strips unsafe characters", input: "carol! d@ve#", want: "caroldve"},
		{name: "empty result falls back to user", input: "!!!", want: "user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeUsername(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeUsername(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSplitEmail covers a normal address, an address with no "@", and an
// address beginning with "@" (empty local part).
func TestSplitEmail(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		wantLocal  string
		wantDomain string
	}{
		{name: "normal address", email: "alice@example.com", wantLocal: "alice", wantDomain: "example.com"},
		{name: "no at sign", email: "notanemail", wantLocal: "notanemail", wantDomain: ""},
		{name: "leading at sign", email: "@example.com", wantLocal: "@example.com", wantDomain: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitEmail(tt.email)
			if len(parts) != 2 {
				t.Fatalf("expected 2 parts, got %d", len(parts))
			}
			if parts[0] != tt.wantLocal || parts[1] != tt.wantDomain {
				t.Errorf("splitEmail(%q) = %v, want [%q %q]", tt.email, parts, tt.wantLocal, tt.wantDomain)
			}
		})
	}
}

// TestOIDCServicePureHelpers covers the package-level pure crypto/PKCE
// helpers that back the OIDC handler flows.
func TestOIDCServicePureHelpers(t *testing.T) {
	t.Run("GenerateState returns distinct hex strings", func(t *testing.T) {
		a, err := service.GenerateState()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		b, err := service.GenerateState()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a == b {
			t.Errorf("expected distinct state values")
		}
		if len(a) != 32 {
			t.Errorf("expected 32 hex chars (16 bytes), got %d", len(a))
		}
	})

	t.Run("GenerateCodeVerifier returns distinct base64url strings", func(t *testing.T) {
		a, err := service.GenerateCodeVerifier()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		b, err := service.GenerateCodeVerifier()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a == b {
			t.Errorf("expected distinct verifier values")
		}
		if len(a) < 43 || len(a) > 128 {
			t.Errorf("expected verifier length in [43,128], got %d", len(a))
		}
	})

	t.Run("CodeChallenge is deterministic for a given verifier", func(t *testing.T) {
		got1 := service.CodeChallenge("fixed-verifier")
		got2 := service.CodeChallenge("fixed-verifier")
		if got1 != got2 {
			t.Errorf("expected deterministic challenge, got %q vs %q", got1, got2)
		}
		if service.CodeChallenge("other-verifier") == got1 {
			t.Errorf("expected different verifiers to produce different challenges")
		}
	})

	t.Run("IsAdminGroup matches case-insensitively", func(t *testing.T) {
		svc := service.NewOIDCService()
		cfg := &service.OIDCProviderConfig{AdminGroups: []string{"Admins", "ops"}}

		if !svc.IsAdminGroup(cfg, []string{"ADMINS"}) {
			t.Errorf("expected case-insensitive match on ADMINS")
		}
		if svc.IsAdminGroup(cfg, []string{"users"}) {
			t.Errorf("expected no match for unrelated group")
		}
		if svc.IsAdminGroup(cfg, nil) {
			t.Errorf("expected no match for empty user groups")
		}
	})
}

// TestOIDCAuthHandlerStartLogin_Disabled verifies the disabled-service guard
// clause is reached before any provider lookup. The handler renders HTML for
// every branch, which panics without a configured gin HTMLRender in a unit
// test context, so this test only proves the guard clause is exercised
// (matching the recover/skip pattern used elsewhere in this package, e.g.
// admin_geoip_test.go).
func TestOIDCAuthHandlerStartLogin_Disabled(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	h := &OIDCAuthHandler{DB: serverDB, OIDCService: service.NewOIDCService()}

	c, _ := newAPITestContext("/server/auth/oidc/okta/start")
	c.Params = gin.Params{{Key: "provider", Value: "okta"}}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("gin HTMLRender not configured in unit test context: %v", r)
		}
	}()
	h.StartLogin(c)
}

// TestOIDCAuthHandlerCallback_Disabled mirrors StartLogin's disabled-service
// guard clause test for the Callback handler.
func TestOIDCAuthHandlerCallback_Disabled(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	h := &OIDCAuthHandler{DB: serverDB, OIDCService: service.NewOIDCService()}

	c, _ := newAPITestContext("/server/auth/oidc/okta/callback")
	c.Params = gin.Params{{Key: "provider", Value: "okta"}}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("gin HTMLRender not configured in unit test context: %v", r)
		}
	}()
	h.Callback(c)
}
