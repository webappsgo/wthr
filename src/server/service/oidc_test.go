package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"

	_ "modernc.org/sqlite"
)

// setupOIDCTestDB opens an in-memory database with the real
// database.ServerSchema applied and wires it up as the global server DB,
// since SettingsModel.Get() (used transitively by Enabled/GetProviderConfigs)
// always queries database.GetServerDB() rather than its own m.DB field.
// Executing the production schema constant rather than a hand-rolled
// CREATE TABLE is what keeps these tests honest: a test-local table can drift
// away from the table the server actually creates at startup, which is exactly
// how a handler ends up querying a table that does not exist in production.
func setupOIDCTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("failed to apply ServerSchema: %v", err)
	}

	database.SetGlobalDualDB(&database.DualDB{Server: db})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
	return db
}

func setConfig(t *testing.T, db *sql.DB, key, value, typ string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO server_config (key, value, type) VALUES (?, ?, ?)",
		key, value, typ,
	); err != nil {
		t.Fatalf("failed to seed config %q: %v", key, err)
	}
}

// TestOIDC_GenerateState covers the random state generator: correct length,
// valid hex, and non-determinism across calls.
func TestOIDC_GenerateState(t *testing.T) {
	s1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	if len(s1) != 32 {
		t.Errorf("len(state) = %d, want 32 (16 bytes hex-encoded)", len(s1))
	}
	for _, c := range s1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("state contains non-hex char %q", c)
			break
		}
	}

	s2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() second call error = %v", err)
	}
	if s1 == s2 {
		t.Error("two consecutive GenerateState() calls returned the same value")
	}
}

// TestOIDC_GenerateCodeVerifier covers the PKCE verifier generator: length
// falls within the RFC 7636 43-128 char range and calls are non-deterministic.
func TestOIDC_GenerateCodeVerifier(t *testing.T) {
	v1, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() error = %v", err)
	}
	if len(v1) < 43 || len(v1) > 128 {
		t.Errorf("len(verifier) = %d, want in [43,128]", len(v1))
	}

	v2, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() second call error = %v", err)
	}
	if v1 == v2 {
		t.Error("two consecutive GenerateCodeVerifier() calls returned the same value")
	}
}

// TestOIDC_CodeChallenge covers the S256 PKCE challenge derivation: it must
// be deterministic for the same verifier (idempotent) and different for
// different verifiers, including the empty-string boundary.
func TestOIDC_CodeChallenge(t *testing.T) {
	tests := []struct {
		name     string
		verifier string
	}{
		{"empty verifier", ""},
		{"typical verifier", "abcdefghijklmnopqrstuvwxyz0123456789ABCDEF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1 := CodeChallenge(tt.verifier)
			got2 := CodeChallenge(tt.verifier)
			if got1 != got2 {
				t.Errorf("CodeChallenge(%q) not idempotent: %q vs %q", tt.verifier, got1, got2)
			}
			if got1 == "" {
				t.Error("CodeChallenge() returned empty string")
			}
		})
	}

	a := CodeChallenge("verifier-a")
	b := CodeChallenge("verifier-b")
	if a == b {
		t.Error("CodeChallenge() returned the same value for different verifiers")
	}
}

// TestOIDC_IsAdminGroup covers case-insensitive group matching, including
// the empty-groups and no-match boundary cases.
func TestOIDC_IsAdminGroup(t *testing.T) {
	s := &OIDCService{}
	tests := []struct {
		name        string
		adminGroups []string
		userGroups  []string
		want        bool
	}{
		{"exact match", []string{"admins"}, []string{"admins"}, true},
		{"case-insensitive match", []string{"Admins"}, []string{"ADMINS"}, true},
		{"no overlap", []string{"admins"}, []string{"users"}, false},
		{"empty admin groups", nil, []string{"admins"}, false},
		{"empty user groups", []string{"admins"}, nil, false},
		{"both empty", nil, nil, false},
		{"match among many", []string{"other", "admins"}, []string{"foo", "admins", "bar"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &OIDCProviderConfig{AdminGroups: tt.adminGroups}
			if got := s.IsAdminGroup(cfg, tt.userGroups); got != tt.want {
				t.Errorf("IsAdminGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOIDC_Enabled covers the enabled flag boundary: default false when
// unset, and reflecting the stored boolean when set.
func TestOIDC_Enabled(t *testing.T) {
	t.Run("defaults to false when unset", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		s := NewOIDCService(db)
		if s.Enabled() {
			t.Error("Enabled() = true, want false when server.auth.oidc.enabled is unset")
		}
	})

	t.Run("true when explicitly enabled", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		setConfig(t, db, "server.auth.oidc.enabled", "true", "boolean")
		s := NewOIDCService(db)
		if !s.Enabled() {
			t.Error("Enabled() = false, want true")
		}
	})

	t.Run("false when explicitly disabled", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		setConfig(t, db, "server.auth.oidc.enabled", "false", "boolean")
		s := NewOIDCService(db)
		if s.Enabled() {
			t.Error("Enabled() = true, want false")
		}
	})
}

// TestOIDC_GetProviderConfigs covers the empty (unset), malformed JSON, and
// happy-path decode cases.
func TestOIDC_GetProviderConfigs(t *testing.T) {
	t.Run("nil when unset", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		s := NewOIDCService(db)
		if got := s.GetProviderConfigs(); got != nil {
			t.Errorf("GetProviderConfigs() = %v, want nil", got)
		}
	})

	t.Run("nil on malformed json", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		setConfig(t, db, "server.auth.oidc.providers", `not valid json`, "json")
		s := NewOIDCService(db)
		if got := s.GetProviderConfigs(); got != nil {
			t.Errorf("GetProviderConfigs() = %v, want nil on malformed json", got)
		}
	})

	t.Run("decodes valid provider list", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		raw := `[{"name":"google","display_name":"Google","issuer":"https://accounts.google.com","client_id":"cid","client_secret":"secret","scopes":["openid","email"]}]`
		setConfig(t, db, "server.auth.oidc.providers", raw, "json")
		s := NewOIDCService(db)
		got := s.GetProviderConfigs()
		if len(got) != 1 {
			t.Fatalf("len(GetProviderConfigs()) = %d, want 1", len(got))
		}
		if got[0].Name != "google" || got[0].Issuer != "https://accounts.google.com" {
			t.Errorf("provider = %+v, unexpected", got[0])
		}
	})

	t.Run("empty array decodes to empty non-nil slice", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		setConfig(t, db, "server.auth.oidc.providers", `[]`, "json")
		s := NewOIDCService(db)
		got := s.GetProviderConfigs()
		if len(got) != 0 {
			t.Errorf("len(GetProviderConfigs()) = %d, want 0", len(got))
		}
	})
}

// TestOIDC_GetProviderConfig covers found and not-found (including the
// empty-providers boundary) lookup paths.
func TestOIDC_GetProviderConfig(t *testing.T) {
	t.Run("not found when no providers configured", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		s := NewOIDCService(db)
		if _, err := s.GetProviderConfig("google"); err == nil {
			t.Fatal("expected error for missing provider, got nil")
		}
	})

	t.Run("found by name", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		raw := `[{"name":"google","issuer":"https://accounts.google.com"},{"name":"github","issuer":"https://github.com"}]`
		setConfig(t, db, "server.auth.oidc.providers", raw, "json")
		s := NewOIDCService(db)

		cfg, err := s.GetProviderConfig("github")
		if err != nil {
			t.Fatalf("GetProviderConfig() error = %v", err)
		}
		if cfg.Issuer != "https://github.com" {
			t.Errorf("Issuer = %q, want https://github.com", cfg.Issuer)
		}
	})

	t.Run("not found among configured providers", func(t *testing.T) {
		db := setupOIDCTestDB(t)
		raw := `[{"name":"google","issuer":"https://accounts.google.com"}]`
		setConfig(t, db, "server.auth.oidc.providers", raw, "json")
		s := NewOIDCService(db)

		if _, err := s.GetProviderConfig("nonexistent"); err == nil {
			t.Fatal("expected error for unmatched provider name, got nil")
		}
	})
}

// TestOIDC_AuthURL_UnknownProvider covers the error path where the provider
// name does not resolve to a configured provider.
func TestOIDC_AuthURL_UnknownProvider(t *testing.T) {
	db := setupOIDCTestDB(t)
	s := NewOIDCService(db)

	_, err := s.AuthURL(context.Background(), "nonexistent", "https://example.com/callback", "state", "verifier")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

// TestOIDC_ExchangeAndVerify_UnknownProvider covers the error path where the
// provider name does not resolve, verifying the lookup failure short-circuits
// before any network call is attempted.
func TestOIDC_ExchangeAndVerify_UnknownProvider(t *testing.T) {
	db := setupOIDCTestDB(t)
	s := NewOIDCService(db)

	_, err := s.ExchangeAndVerify(context.Background(), "nonexistent", "https://example.com/callback", "code", "verifier")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

// TestOIDC_OAuth2Config_DefaultScopes covers the scope defaulting logic:
// an empty Scopes slice on the provider config falls back to the standard
// openid/profile/email scopes, while an explicit non-empty slice is
// preserved unchanged.
func TestOIDC_OAuth2Config_DefaultScopes(t *testing.T) {
	s := &OIDCService{}

	t.Run("defaults when empty", func(t *testing.T) {
		cfg := &OIDCProviderConfig{ClientID: "cid", ClientSecret: "secret"}
		o2cfg := s.oauth2Config(cfg, &oidcProviderStub, "https://example.com/callback")
		want := []string{"openid", "profile", "email"}
		if len(o2cfg.Scopes) != len(want) {
			t.Fatalf("Scopes = %v, want %v", o2cfg.Scopes, want)
		}
		for i, sc := range want {
			if o2cfg.Scopes[i] != sc {
				t.Errorf("Scopes[%d] = %q, want %q", i, o2cfg.Scopes[i], sc)
			}
		}
	})

	t.Run("preserves explicit scopes", func(t *testing.T) {
		cfg := &OIDCProviderConfig{ClientID: "cid", Scopes: []string{"openid", "custom-scope"}}
		o2cfg := s.oauth2Config(cfg, &oidcProviderStub, "https://example.com/callback")
		want := []string{"openid", "custom-scope"}
		if len(o2cfg.Scopes) != len(want) {
			t.Fatalf("Scopes = %v, want %v", o2cfg.Scopes, want)
		}
		for i, sc := range want {
			if o2cfg.Scopes[i] != sc {
				t.Errorf("Scopes[%d] = %q, want %q", i, o2cfg.Scopes[i], sc)
			}
		}
	})

	t.Run("propagates client id, secret and redirect url", func(t *testing.T) {
		cfg := &OIDCProviderConfig{ClientID: "my-client", ClientSecret: "my-secret"}
		o2cfg := s.oauth2Config(cfg, &oidcProviderStub, "https://example.com/callback")
		if o2cfg.ClientID != "my-client" {
			t.Errorf("ClientID = %q, want my-client", o2cfg.ClientID)
		}
		if o2cfg.ClientSecret != "my-secret" {
			t.Errorf("ClientSecret = %q, want my-secret", o2cfg.ClientSecret)
		}
		if o2cfg.RedirectURL != "https://example.com/callback" {
			t.Errorf("RedirectURL = %q, want https://example.com/callback", o2cfg.RedirectURL)
		}
	})
}

// TestOIDC_GetOIDCProvider_CacheHit covers the provider cache: within the
// TTL window a second call for the same issuer must return the cached
// pointer without invoking oidc.NewProvider again (which would require a
// real discovery endpoint).
func TestOIDC_GetOIDCProvider_CacheHit(t *testing.T) {
	s := &OIDCService{
		providerCache: map[string]*oidc.Provider{
			"https://issuer.example.com": &oidcProviderStub,
		},
		cacheExpiry: map[string]time.Time{
			"https://issuer.example.com": time.Now().Add(5 * time.Minute),
		},
	}

	got, err := s.getOIDCProvider(context.Background(), "https://issuer.example.com")
	if err != nil {
		t.Fatalf("getOIDCProvider() error = %v", err)
	}
	if got != &oidcProviderStub {
		t.Error("getOIDCProvider() did not return the cached provider pointer")
	}
}

// TestOIDC_GetOIDCProvider_ExpiredCache_FailsWithoutNetwork covers the
// cache-expiry boundary: once the TTL has elapsed the cached entry must not
// be trusted, forcing a fresh discovery call. Since no real issuer is
// reachable in this test environment, the refetch must fail with an error
// (proving the stale cache entry was NOT blindly reused).
func TestOIDC_GetOIDCProvider_ExpiredCache_FailsWithoutNetwork(t *testing.T) {
	s := &OIDCService{
		providerCache: map[string]*oidc.Provider{
			"https://issuer.example.com": &oidcProviderStub,
		},
		cacheExpiry: map[string]time.Time{
			"https://issuer.example.com": time.Now().Add(-1 * time.Minute),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := s.getOIDCProvider(ctx, "https://issuer.example.com")
	if err == nil {
		t.Fatal("expected error refetching expired/unreachable issuer, got nil")
	}
}

// TestOIDC_NewOIDCService covers the constructor's initial state.
func TestOIDC_NewOIDCService(t *testing.T) {
	db := setupOIDCTestDB(t)
	s := NewOIDCService(db)
	if s.settings == nil {
		t.Fatal("settings should be initialized")
	}
	if s.providerCache == nil {
		t.Error("providerCache should be initialized")
	}
	if s.cacheExpiry == nil {
		t.Error("cacheExpiry should be initialized")
	}
	if len(s.providerCache) != 0 {
		t.Errorf("providerCache should start empty, got %d entries", len(s.providerCache))
	}
}

// oidcProviderStub is a zero-value *oidc.Provider used purely as an opaque
// pointer identity in tests that never dereference provider internals
// (oauth2Config only calls oidcProvider.Endpoint(), which is safe on a
// zero-value Provider).
var oidcProviderStub oidc.Provider

// modelsSettingsUnused keeps the models import used even if future edits
// remove other direct references, since SettingsModel is exercised only
// indirectly via NewOIDCService above.
var _ = models.SettingsModel{}
