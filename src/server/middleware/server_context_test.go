package middleware

import (
	"bytes"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
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
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close server database: %v", err)
		}
	})

	database.SetGlobalDualDB(&database.DualDB{Server: db})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	return db
}

// TestInjectServerContext_DefaultsWhenNoSettingsStored verifies branding
// defaults and the language fallback.
func TestInjectServerContext_DefaultsWhenNoSettingsStored(t *testing.T) {
	db := openServerContextTestDB(t)
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(nil)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, exists := GetServerContext(r.Context())
		if !exists {
			t.Fatal("server context not set by InjectServerContext")
		}
		if ctx.Title != "wthr" {
			t.Errorf("Title = %q, want default %q", ctx.Title, "wthr")
		}
		if ctx.Tagline != "" {
			t.Errorf("Tagline = %q, want empty default", ctx.Tagline)
		}
		if ctx.Version != "1.2.3" {
			t.Errorf("Version = %q, want %q (passed to InjectServerContext)", ctx.Version, "1.2.3")
		}
		if ctx.GitOrg != "webappsgo" || ctx.GitRepo != "wthr" {
			t.Errorf("GitHub source = %q/%q, want webappsgo/wthr", ctx.GitOrg, ctx.GitRepo)
		}
		if ctx.Lang != "en" {
			t.Errorf("Lang = %q, want default %q when no lang was set upstream", ctx.Lang, "en")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := InjectServerContext(storeForDB(db), "1.2.3")(next)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestInjectServerContext_UsesBrandingConfigAndUpstreamLang verifies typed
// branding settings and an upstream language context.
func TestInjectServerContext_UsesStoredSettingsAndUpstreamLang(t *testing.T) {
	db := openServerContextTestDB(t)
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Branding.Title = "My Weather Site"
	cfg.Server.Branding.Tagline = "Storms ahead"
	cfg.Server.SEO.TwitterHandle = "wthrapp"
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	setLang := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reqctx.SetValue(r.Context(), "lang", "fr")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, exists := GetServerContext(r.Context())
		if !exists {
			t.Fatal("server context not set by InjectServerContext")
		}
		if ctx.Title != "My Weather Site" {
			t.Errorf("Title = %q, want configured title", ctx.Title)
		}
		if ctx.Tagline != "Storms ahead" {
			t.Errorf("Tagline = %q, want configured tagline", ctx.Tagline)
		}
		if ctx.TwitterHandle != "@wthrapp" {
			t.Errorf("TwitterHandle = %q, want %q (normalized with a leading @)", ctx.TwitterHandle, "@wthrapp")
		}
		if ctx.Lang != "fr" {
			t.Errorf("Lang = %q, want the upstream context value %q", ctx.Lang, "fr")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := setLang(InjectServerContext(storeForDB(db), "1.2.3")(next))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestInjectServerContextUsesGeneratedBrandingVariants(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_DIR", configDir)
	imageSource := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < imageSource.Bounds().Dy(); y++ {
		for x := 0; x < imageSource.Bounds().Dx(); x++ {
			imageSource.Set(x, y, color.NRGBA{R: 40, G: 90, B: 180, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, imageSource); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveBrandingAsset("brand.png", encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	variants, err := GenerateBrandingImageVariants("brand", encoded.Bytes(), "image/png")
	if err != nil {
		t.Fatalf("generate branding variants: %v", err)
	}
	for _, variant := range variants {
		if err := config.SaveBrandingAsset(variant.Name, variant.Data); err != nil {
			t.Fatalf("save %s: %v", variant.Name, err)
		}
	}
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Branding.Logo = "brand.png"
	cfg.Server.Branding.Favicon = "brand.png"
	cfg.Server.SEO.OGImage = "brand.png"
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	db := openServerContextTestDB(t)

	var got ServerContext
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = GetServerContext(r.Context())
		if !ok {
			t.Fatal("server context missing")
		}
	})
	InjectServerContext(storeForDB(db), "test")(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if got.Logo200 != "/assets/brand-logo-200.png" || got.Logo50 != "/assets/brand-logo-50.png" {
		t.Fatalf("logo variants = %q, %q", got.Logo200, got.Logo50)
	}
	if got.Favicon16 != "/assets/brand-favicon-16.png" || got.Favicon512 != "/assets/brand-favicon-512.png" {
		t.Fatalf("favicon variants = %q, %q", got.Favicon16, got.Favicon512)
	}
	if got.OGImage1200 != "/assets/brand-og-1200x630.jpg" {
		t.Fatalf("OpenGraph image variant = %q", got.OGImage1200)
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
	if ctx.Title != "wthr" {
		t.Errorf("Title = %q, want the safe fallback default %q", ctx.Title, "wthr")
	}
	if ctx.Version != "unknown" {
		t.Errorf("Version = %q, want the safe fallback %q", ctx.Version, "unknown")
	}
	if ctx.GitOrg != "webappsgo" || ctx.GitRepo != "wthr" {
		t.Errorf("GitHub source = %q/%q, want webappsgo/wthr", ctx.GitOrg, ctx.GitRepo)
	}
}
