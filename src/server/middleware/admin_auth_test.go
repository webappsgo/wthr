package middleware

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	_ "modernc.org/sqlite"
)

// openAdminAuthTestServerDB opens an in-memory server.db seeded with the
// real ServerSchema (which defines server_admin_credentials and
// server_admin_sessions - the two tables admin_auth.go queries directly via
// its own db parameter, unlike auth.go/server_context.go/setup.go which
// bypass their db parameter entirely via database.GetServerDB()).
func openAdminAuthTestServerDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:admin_auth_server_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open server sqlite: %v", err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newAdminTestRouter builds a gin router with a stub admin/login.tmpl -
// RequireAdminAuth/AdminLoginHandler render that template name literally via
// c.HTML on every reject path, so it must be registered or gin panics.
func newAdminTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	tmpl := template.Must(template.New("admin/login.tmpl").Parse("login stub {{.error}}"))
	router.SetHTMLTemplate(tmpl)
	return router
}

func seedAdminCredential(t *testing.T, db *sql.DB, username, password string) int64 {
	t.Helper()
	hash, err := models.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	res, err := db.Exec(`
		INSERT INTO server_admin_credentials (username, email, password_hash, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, username, username+"@example.com", hash)
	if err != nil {
		t.Fatalf("seed admin credential: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// TestRequireAdminAuth_NoCookieShowsLoginPage verifies an admin route
// request with no admin_session cookie renders the login page (200) rather
// than reaching the wrapped handler.
func TestRequireAdminAuth_NoCookieShowsLoginPage(t *testing.T) {
	router := newAdminTestRouter()
	router.Use(func(c *gin.Context) {
		c.Set("db", openAdminAuthTestServerDB(t))
		c.Next()
	})
	router.Use(RequireAdminAuth())
	router.GET("/server/admin/", func(c *gin.Context) {
		t.Error("wrapped handler reached, want login page instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (login page) for missing admin_session cookie", w.Code)
	}
}

// TestRequireAdminAuth_NoDBReturns503 verifies that when GetDB(c) can't find
// a db in context and database.GetGlobalDualDB hasn't been set either, the
// middleware fails closed with 503 instead of panicking or silently passing
// through.
func TestRequireAdminAuth_NoDBReturns503(t *testing.T) {
	database.SetGlobalDualDB(nil)

	router := newAdminTestRouter()
	router.Use(RequireAdminAuth())
	router.GET("/server/admin/", func(c *gin.Context) {
		t.Error("wrapped handler reached, want 503 instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "whatever"})
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when no db is reachable", w.Code)
	}
}

// TestRequireAdminAuth_InvalidSessionShowsLoginPage verifies a cookie value
// with no matching, unexpired row in server_admin_sessions is rejected back
// to the login page rather than authenticating.
func TestRequireAdminAuth_InvalidSessionShowsLoginPage(t *testing.T) {
	serverDB := openAdminAuthTestServerDB(t)

	router := newAdminTestRouter()
	router.Use(func(c *gin.Context) {
		c.Set("db", serverDB)
		c.Next()
	})
	router.Use(RequireAdminAuth())
	router.GET("/server/admin/", func(c *gin.Context) {
		t.Error("wrapped handler reached, want login page instead")
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "does-not-exist"})
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (login page) for an unknown session token", w.Code)
	}
}

// TestRequireAdminAuth_ValidSessionReachesHandler seeds a real
// server_admin_sessions row with a future expiry and verifies the request
// reaches the wrapped handler with admin_id populated in context.
func TestRequireAdminAuth_ValidSessionReachesHandler(t *testing.T) {
	serverDB := openAdminAuthTestServerDB(t)
	adminID := seedAdminCredential(t, serverDB, "root", "hunter2-hunter2")

	const token = "a-valid-looking-session-token"
	expires := time.Now().Add(time.Hour).Unix()
	if _, err := serverDB.Exec(`
		INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, expires_at, created_at)
		VALUES (?, ?, ?, ?, datetime(?, 'unixepoch'), CURRENT_TIMESTAMP)
	`, token, adminID, "127.0.0.1", "test-agent", expires); err != nil {
		t.Fatalf("seed admin session: %v", err)
	}

	router := newAdminTestRouter()
	router.Use(func(c *gin.Context) {
		c.Set("db", serverDB)
		c.Next()
	})
	router.Use(RequireAdminAuth())
	router.GET("/server/admin/", func(c *gin.Context) {
		got, exists := c.Get("admin_id")
		if !exists {
			t.Error("admin_id not set in context for a valid session")
		} else if got.(int) != int(adminID) {
			t.Errorf("admin_id = %v, want %d", got, adminID)
		}
		c.String(http.StatusOK, "reached")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: token})
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 from the wrapped handler", w.Code)
	}
	if !strings.Contains(w.Body.String(), "reached") {
		t.Errorf("body = %q, want the wrapped handler's response", w.Body.String())
	}
}

// TestAdminLoginHandler covers empty credentials, unknown username, wrong
// password, and a fully correct login (new session row + cookie + redirect).
func TestAdminLoginHandler(t *testing.T) {
	t.Run("empty credentials rejected", func(t *testing.T) {
		serverDB := openAdminAuthTestServerDB(t)
		router := newAdminTestRouter()
		router.POST("/server/admin/login", AdminLoginHandler(serverDB))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(url.Values{}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 for empty username/password", w.Code)
		}
	})

	t.Run("unknown username rejected generically", func(t *testing.T) {
		serverDB := openAdminAuthTestServerDB(t)
		router := newAdminTestRouter()
		router.POST("/server/admin/login", AdminLoginHandler(serverDB))

		form := url.Values{"username": {"nobody"}, "password": {"whatever123"}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 for unknown username", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Invalid credentials") {
			t.Errorf("body = %q, want the generic 'Invalid credentials' message (no user enumeration)", w.Body.String())
		}
	})

	t.Run("wrong password rejected generically", func(t *testing.T) {
		serverDB := openAdminAuthTestServerDB(t)
		seedAdminCredential(t, serverDB, "root", "correct-password-123")
		router := newAdminTestRouter()
		router.POST("/server/admin/login", AdminLoginHandler(serverDB))

		form := url.Values{"username": {"root"}, "password": {"wrong-password"}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 for wrong password", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Invalid credentials") {
			t.Errorf("body = %q, want the generic 'Invalid credentials' message", w.Body.String())
		}
	})

	t.Run("correct credentials create a session and redirect", func(t *testing.T) {
		serverDB := openAdminAuthTestServerDB(t)
		seedAdminCredential(t, serverDB, "root", "correct-password-123")
		router := newAdminTestRouter()
		router.POST("/server/admin/login", AdminLoginHandler(serverDB))

		form := url.Values{"username": {"root"}, "password": {"correct-password-123"}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("status = %d, want 302 redirect for correct credentials", w.Code)
		}

		var cookieFound bool
		for _, c := range w.Result().Cookies() {
			if c.Name == "admin_session" && c.Value != "" {
				cookieFound = true
			}
		}
		if !cookieFound {
			t.Error("admin_session cookie not set (or empty) after a successful login")
		}

		var count int
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_admin_sessions").Scan(&count); err != nil {
			t.Fatalf("count sessions: %v", err)
		}
		if count != 1 {
			t.Errorf("server_admin_sessions row count = %d, want 1 after a successful login", count)
		}
	})
}

// TestAdminLogoutHandler verifies logout clears the admin_session cookie
// (maxAge < 0) and redirects to the admin path.
func TestAdminLogoutHandler(t *testing.T) {
	router := newAdminTestRouter()
	router.GET("/server/admin/logout", AdminLogoutHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/logout", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect on logout", w.Code)
	}

	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "admin_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("admin_session cookie was not cleared (MaxAge < 0) on logout")
	}
}
