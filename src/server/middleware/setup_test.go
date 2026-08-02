package middleware

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/path"
	utils "github.com/webappsgo/wthr/src/util"
	_ "modernc.org/sqlite"
)

// setupTestConfigDir is fixed once for the whole test binary, before
// path.GetInstance()'s sync.Once fires for the first time. paths.Initialize
// (src/path/paths.go:373-388) is process-global and one-shot: whichever
// CONFIG_DIR value is present the first time any code calls
// paths.GetInstance()/paths.GetConfigDir() wins for the remainder of the
// process, so per-test os.Setenv/t.Setenv calls after that point are
// silently ignored. TestMain pins it to a throwaway temp directory (never
// the project tree, per PART 29) before any test runs, so setup.go's calls
// to paths.GetConfigDir() are test-isolated instead of touching a real
// system config path.
var setupTestConfigDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wthr-middleware-setup-test-")
	if err != nil {
		panic(err)
	}
	setupTestConfigDir = dir
	os.Setenv("CONFIG_DIR", dir)
	// Force initialization now, under our controlled CONFIG_DIR, before any
	// test can race path.GetInstance() with a different expectation.
	_ = paths.GetConfigDir()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func openSetupTestServerDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:setup_server_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open server sqlite: %v", err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// SetupTokenRequired/BlockSetupAfterComplete/BlockSetupAfterAdminExists
	// all query database.GetServerDB() directly rather than the db parameter
	// they're constructed with (the same dead DB parameter pattern as
	// auth.go/server_context.go/admin.go's GetByAPIToken).
	database.SetGlobalDualDB(&database.DualDB{Server: db})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	return db
}

func seedSetupAdmin(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO server_admin_credentials (username, email, password_hash, created_at)
		VALUES ('root', 'root@example.com', 'x', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed admin credential: %v", err)
	}
}

// testAppConfig returns a zero-value config; AppConfig.GetAdminPath()
// (src/config/config.go:339-344) already falls back to "admin" when
// Server.AdminPath is empty, matching the CLAUDE.md default admin path.
func testAppConfig() *config.AppConfig {
	return &config.AppConfig{}
}

// removeSetupToken guarantees a clean, token-file-absent starting state
// regardless of what a prior test in this file left behind.
func removeSetupToken(t *testing.T) {
	t.Helper()
	os.Remove(setupTestConfigDir + "/setup_token.txt")
}

func writeSetupToken(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(setupTestConfigDir+"/setup_token.txt", []byte("test-token"), 0o600); err != nil {
		t.Fatalf("write setup_token.txt: %v", err)
	}
	t.Cleanup(func() { removeSetupToken(t) })
}

func newSetupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// TestSetupTokenRequired_NonAdminRouteSkipped verifies a request outside the
// configured admin path prefix passes straight through untouched.
func TestSetupTokenRequired_NonAdminRouteSkipped(t *testing.T) {
	db := openSetupTestServerDB(t)
	cfg := testAppConfig()

	router := newSetupTestRouter()
	router.Use(SetupTokenRequired(db, cfg))
	router.GET("/api/v1/weather", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a non-admin route", w.Code)
	}
}

// TestSetupTokenRequired_SetupRouteSkipped verifies the setup wizard route
// itself is exempted from this middleware (it handles its own auth).
func TestSetupTokenRequired_SetupRouteSkipped(t *testing.T) {
	db := openSetupTestServerDB(t)
	cfg := testAppConfig()

	router := newSetupTestRouter()
	router.Use(SetupTokenRequired(db, cfg))
	router.GET("/server/admin/server/setup", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/server/setup", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for the setup wizard route", w.Code)
	}
}

// TestSetupTokenRequired_AdminExistsPassesThrough verifies that once an
// admin account exists, the middleware steps aside for the normal auth flow.
func TestSetupTokenRequired_AdminExistsPassesThrough(t *testing.T) {
	db := openSetupTestServerDB(t)
	seedSetupAdmin(t, db)
	cfg := testAppConfig()

	router := newSetupTestRouter()
	router.Use(SetupTokenRequired(db, cfg))
	router.GET("/server/admin/dashboard", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/dashboard", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (normal auth flow) once an admin exists", w.Code)
	}
}

// TestSetupTokenRequired_NoAdminNoTokenFileShows503 verifies that with no
// admin account and no setup token file present, the middleware fails
// closed with a 503 error page rather than silently proceeding.
func TestSetupTokenRequired_NoAdminNoTokenFileShows503(t *testing.T) {
	db := openSetupTestServerDB(t)
	cfg := testAppConfig()
	removeSetupToken(t)

	router := newSetupTestRouter()
	router.SetHTMLTemplate(template.Must(template.New("error.tmpl").Parse("error stub {{.error}}")))
	router.Use(SetupTokenRequired(db, cfg))
	router.GET("/server/admin", func(c *gin.Context) {
		t.Error("wrapped handler reached, want the 503 error page instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when no admin exists and no setup token file is present", w.Code)
	}
}

// TestSetupTokenRequired_NoAdminTokenFileUnverifiedShowsForm verifies that
// with a setup token file present but no verified cookie, the token entry
// form is rendered.
func TestSetupTokenRequired_NoAdminTokenFileUnverifiedShowsForm(t *testing.T) {
	db := openSetupTestServerDB(t)
	cfg := testAppConfig()
	writeSetupToken(t)

	router := newSetupTestRouter()
	router.SetHTMLTemplate(template.Must(template.New("admin/setup_token.tmpl").Parse("token form stub")))
	router.Use(SetupTokenRequired(db, cfg))
	router.GET("/server/admin", func(c *gin.Context) {
		t.Error("wrapped handler reached, want the token entry form instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (token entry form)", w.Code)
	}
}

// TestSetupTokenRequired_VerifiedTokenCookieRedirectsToWizard verifies that
// with the setup_token_verified cookie set to "true", the request is
// redirected straight to the setup wizard.
func TestSetupTokenRequired_VerifiedTokenCookieRedirectsToWizard(t *testing.T) {
	db := openSetupTestServerDB(t)
	cfg := testAppConfig()
	writeSetupToken(t)

	router := newSetupTestRouter()
	router.Use(SetupTokenRequired(db, cfg))
	router.GET("/server/admin", func(c *gin.Context) {
		t.Error("wrapped handler reached, want a redirect instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	req.AddCookie(&http.Cookie{Name: "setup_token_verified", Value: "true"})
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect to the setup wizard", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin/server/setup" {
		t.Errorf("Location = %q, want %q", loc, "/server/admin/server/setup")
	}
}

// TestBlockSetupAfterComplete_AdminExistsRedirects verifies that once an
// admin exists, setup routes redirect to the admin dashboard instead of
// letting the setup wizard run again.
func TestBlockSetupAfterComplete_AdminExistsRedirects(t *testing.T) {
	db := openSetupTestServerDB(t)
	seedSetupAdmin(t, db)
	cfg := testAppConfig()

	router := newSetupTestRouter()
	router.Use(BlockSetupAfterComplete(db, cfg))
	router.GET("/server/admin/server/setup", func(c *gin.Context) {
		t.Error("wrapped handler reached, want a redirect instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/server/setup", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect once setup is complete", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin/dashboard" {
		t.Errorf("Location = %q, want %q", loc, "/server/admin/dashboard")
	}
}

// TestBlockSetupAfterComplete_NoAdminTokenPresentPassesThrough verifies
// setup is allowed to proceed when no admin exists yet and the setup token
// file is still present.
func TestBlockSetupAfterComplete_NoAdminTokenPresentPassesThrough(t *testing.T) {
	db := openSetupTestServerDB(t)
	cfg := testAppConfig()
	writeSetupToken(t)

	router := newSetupTestRouter()
	router.Use(BlockSetupAfterComplete(db, cfg))
	router.GET("/server/admin/server/setup", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/server/setup", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 to let setup proceed", w.Code)
	}
}

// TestBlockSetupAfterComplete_NoAdminNoTokenRedirectsToAdminRoot verifies
// the inconsistent-state case (no admin, no token file) redirects back to
// the admin root rather than crashing or looping.
func TestBlockSetupAfterComplete_NoAdminNoTokenRedirectsToAdminRoot(t *testing.T) {
	db := openSetupTestServerDB(t)
	cfg := testAppConfig()
	removeSetupToken(t)

	router := newSetupTestRouter()
	router.Use(BlockSetupAfterComplete(db, cfg))
	router.GET("/server/admin/server/setup", func(c *gin.Context) {
		t.Error("wrapped handler reached, want a redirect instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/server/setup", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect for the inconsistent no-admin/no-token state", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin" {
		t.Errorf("Location = %q, want %q", loc, "/server/admin")
	}
}

// TestRequireSetupTokenVerified covers both the verified and unverified
// cookie branches; this middleware never touches the DB or filesystem.
func TestRequireSetupTokenVerified(t *testing.T) {
	cfg := testAppConfig()

	tests := []struct {
		name       string
		cookie     *http.Cookie
		wantStatus int
		wantReach  bool
	}{
		{"no cookie redirects", nil, http.StatusFound, false},
		{"wrong value redirects", &http.Cookie{Name: "setup_token_verified", Value: "nope"}, http.StatusFound, false},
		{"verified cookie reaches handler", &http.Cookie{Name: "setup_token_verified", Value: "true"}, http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newSetupTestRouter()
			reached := false
			router.Use(RequireSetupTokenVerified(cfg))
			router.GET("/server/admin/server/setup", func(c *gin.Context) {
				reached = true
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/server/admin/server/setup", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if reached != tt.wantReach {
				t.Errorf("handler reached = %v, want %v", reached, tt.wantReach)
			}
		})
	}
}

// TestBlockSetupAfterAdminExists covers both branches: an existing admin
// redirects to the dashboard, no admin lets the request through.
func TestBlockSetupAfterAdminExists(t *testing.T) {
	t.Run("admin exists redirects", func(t *testing.T) {
		db := openSetupTestServerDB(t)
		seedSetupAdmin(t, db)
		cfg := testAppConfig()

		router := newSetupTestRouter()
		router.Use(BlockSetupAfterAdminExists(db, cfg))
		router.GET("/server/admin/server/setup", func(c *gin.Context) {
			t.Error("wrapped handler reached, want a redirect instead")
			c.String(http.StatusOK, "reached")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/server/admin/server/setup", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("status = %d, want 302 redirect when an admin already exists", w.Code)
		}
	})

	t.Run("no admin passes through", func(t *testing.T) {
		db := openSetupTestServerDB(t)
		cfg := testAppConfig()

		router := newSetupTestRouter()
		router.Use(BlockSetupAfterAdminExists(db, cfg))
		router.GET("/server/admin/server/setup", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/server/admin/server/setup", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 to let setup proceed when no admin exists", w.Code)
		}
	})
}

// sanity-check that utils.SetupTokenExists observes the same directory these
// tests write to/remove from, guarding against a future paths.GetConfigDir()
// refactor silently breaking this file's assumptions.
func TestSetupTestHarness_TokenFileToggleObservedByConfigDir(t *testing.T) {
	if paths.GetConfigDir() != setupTestConfigDir {
		t.Fatalf("paths.GetConfigDir() = %q, want the TestMain-pinned %q - "+
			"CONFIG_DIR override was not honored, likely because something "+
			"else in this test binary called path.GetInstance() before "+
			"TestMain set CONFIG_DIR", paths.GetConfigDir(), setupTestConfigDir)
	}

	removeSetupToken(t)
	if utils.SetupTokenExists(setupTestConfigDir) {
		t.Fatal("SetupTokenExists = true after removal, want false")
	}
	writeSetupToken(t)
	if !utils.SetupTokenExists(setupTestConfigDir) {
		t.Fatal("SetupTokenExists = false after writing the token file, want true")
	}
}
