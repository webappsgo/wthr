package middleware

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/reqctx"
	_ "modernc.org/sqlite"
)

// openServerContextTestDB opens an in-memory server.db seeded with the real
// ServerSchema (defines server_config, which SettingsModel.Get queries) and
// registers it as the global dual DB. InjectServerContext constructs a
// SettingsModel with the *sql.DB passed to it, but SettingsModel.Get/GetString
// (src/server/model/settings.go:35) ignore that field entirely and query
// database.GetServerDB() directly - the same dead DB parameter pattern found
// in auth.go/setup.go/admin.go's GetByAPIToken. Without registering the
// global dual DB, GetServerDB() returns nil and QueryRow on it panics.
func openServerContextTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:server_context_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open server sqlite: %v", err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	database.SetGlobalDualDB(&database.DualDB{Server: db})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	return db
}

// TestInjectServerContext_DefaultsWhenNoSettingsStored verifies that with an
// empty server_config table, InjectServerContext falls back to its hardcoded
// defaults (title, tagline, description) and defaults Lang to "en" when no
// upstream "lang" key exists in context.
func TestInjectServerContext_DefaultsWhenNoSettingsStored(t *testing.T) {
	db := openServerContextTestDB(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, exists := GetServerContext(r.Context())
		if !exists {
			t.Fatal("server context not set by InjectServerContext")
		}
		if ctx.Title != "Weather" {
			t.Errorf("Title = %q, want default %q", ctx.Title, "Weather")
		}
		if ctx.Tagline != "Your personal weather dashboard" {
			t.Errorf("Tagline = %q, want the default tagline", ctx.Tagline)
		}
		if ctx.Version != "1.2.3" {
			t.Errorf("Version = %q, want %q (passed to InjectServerContext)", ctx.Version, "1.2.3")
		}
		if ctx.Lang != "en" {
			t.Errorf("Lang = %q, want default %q when no lang was set upstream", ctx.Lang, "en")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := InjectServerContext(db, "1.2.3")(next)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestInjectServerContext_UsesStoredSettingsAndUpstreamLang verifies that
// values stored in server_config override the hardcoded defaults, the
// Twitter handle gets a "@" prefix normalized on, and an upstream "lang"
// context value (set by an earlier i18n middleware) is honored instead of
// the "en" fallback.
func TestInjectServerContext_UsesStoredSettingsAndUpstreamLang(t *testing.T) {
	db := openServerContextTestDB(t)

	seed := []struct{ key, value string }{
		{"server.title", "My Weather Site"},
		{"server.tagline", "Storms ahead"},
		{"seo.twitter_handle", "wthrapp"},
	}
	for _, s := range seed {
		if _, err := db.Exec(
			"INSERT INTO server_config (key, value, type) VALUES (?, ?, 'string')",
			s.key, s.value,
		); err != nil {
			t.Fatalf("seed server_config %q: %v", s.key, err)
		}
	}

	setLang := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reqctx.Set(r.Context(), "lang", "fr")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, exists := GetServerContext(r.Context())
		if !exists {
			t.Fatal("server context not set by InjectServerContext")
		}
		if ctx.Title != "My Weather Site" {
			t.Errorf("Title = %q, want the stored setting %q", ctx.Title, "My Weather Site")
		}
		if ctx.Tagline != "Storms ahead" {
			t.Errorf("Tagline = %q, want the stored setting %q", ctx.Tagline, "Storms ahead")
		}
		if ctx.TwitterHandle != "@wthrapp" {
			t.Errorf("TwitterHandle = %q, want %q (normalized with a leading @)", ctx.TwitterHandle, "@wthrapp")
		}
		if ctx.Lang != "fr" {
			t.Errorf("Lang = %q, want the upstream context value %q", ctx.Lang, "fr")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := setLang(InjectServerContext(db, "1.2.3")(next))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestGetServerContext_MissingReturnsSafeDefaults verifies GetServerContext
// on a context where InjectServerContext never ran returns a safe zero-value
// fallback (exists=false) rather than panicking on a failed type assertion.
func TestGetServerContext_MissingReturnsSafeDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	ctx, exists := GetServerContext(req.Context())
	if exists {
		t.Error("exists = true, want false when InjectServerContext never ran")
	}
	if ctx.Title != "Weather" {
		t.Errorf("Title = %q, want the safe fallback default %q", ctx.Title, "Weather")
	}
	if ctx.Version != "unknown" {
		t.Errorf("Version = %q, want the safe fallback %q", ctx.Version, "unknown")
	}
}
