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

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
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

// setAdminTestRenderer installs a stub RenderHTML - RequireAdminAuth/
// AdminLoginHandler render the "admin/login.tmpl" name literally on every
// reject path, so a renderer must be wired up or those calls panic on a nil
// func value.
func setAdminTestRenderer(t *testing.T) {
	t.Helper()
	tmpl := template.Must(template.New("admin/login.tmpl").Parse("login stub {{.error}}"))
	orig := RenderHTML
	RenderHTML = func(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{}) {
		w.WriteHeader(status)
		_ = tmpl.Execute(w, data)
	}
	t.Cleanup(func() { RenderHTML = orig })
}

// withDB returns a handler that stores db in the request context under the
// same "db" key GetDB reads, mirroring gin's router.Use(func(c *gin.Context)
// { c.Set("db", db); c.Next() }) stub.
func withDB(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := reqctx.Set(r.Context(), "db", db)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func seedAdminCredential(t *testing.T, db *sql.DB, username, password string) int64 {
	t.Helper()
	hash, err := models.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// Bound through dbtime rather than left to CURRENT_TIMESTAMP so the fixture
	// uses the same writer convention as production code.
	res, err := db.Exec(`
		INSERT INTO server_admin_credentials (username, email, password_hash, created_at)
		VALUES (?, ?, ?, ?)
	`, username, username+"@example.com", hash, dbtime.FormatSQLTimestamp(time.Now()))
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
	setAdminTestRenderer(t)
	db := openAdminAuthTestServerDB(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("wrapped handler reached, want login page instead")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})
	handler := withDB(db, RequireAdminAuth()(next))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (login page) for missing admin_session cookie", w.Code)
	}
}

// TestRequireAdminAuth_NoDBReturns503 verifies that when GetDB(r) can't find
// a db in context and database.GetGlobalDualDB hasn't been set either, the
// middleware fails closed with 503 instead of panicking or silently passing
// through.
func TestRequireAdminAuth_NoDBReturns503(t *testing.T) {
	setAdminTestRenderer(t)
	database.SetGlobalDualDB(nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("wrapped handler reached, want 503 instead")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})
	handler := RequireAdminAuth()(next)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "whatever"})
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when no db is reachable", w.Code)
	}
}

// TestRequireAdminAuth_InvalidSessionShowsLoginPage verifies a cookie value
// with no matching, unexpired row in server_admin_sessions is rejected back
// to the login page rather than authenticating.
func TestRequireAdminAuth_InvalidSessionShowsLoginPage(t *testing.T) {
	setAdminTestRenderer(t)
	serverDB := openAdminAuthTestServerDB(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("wrapped handler reached, want login page instead")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})
	handler := withDB(serverDB, RequireAdminAuth()(next))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "does-not-exist"})
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (login page) for an unknown session token", w.Code)
	}
}

// TestRequireAdminAuth_ValidSessionReachesHandler seeds a real
// server_admin_sessions row with a future expiry and verifies the request
// reaches the wrapped handler with admin_id populated in context.
func TestRequireAdminAuth_ValidSessionReachesHandler(t *testing.T) {
	setAdminTestRenderer(t)
	serverDB := openAdminAuthTestServerDB(t)
	adminID := seedAdminCredential(t, serverDB, "root", "hunter2-hunter2")

	const token = "a-valid-looking-session-token"
	now := time.Now()
	// Both timestamps go in as canonical UTC text via dbtime, matching what
	// AdminLoginHandler itself writes.
	if _, err := serverDB.Exec(`
		INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, token, adminID, "127.0.0.1", "test-agent", dbtime.FormatSQLTimestamp(now.Add(time.Hour)), dbtime.FormatSQLTimestamp(now)); err != nil {
		t.Fatalf("seed admin session: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, exists := reqctx.Get(r.Context(), "admin_id")
		if !exists {
			t.Error("admin_id not set in context for a valid session")
		} else if got.(int) != int(adminID) {
			t.Errorf("admin_id = %v, want %d", got, adminID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})
	handler := withDB(serverDB, RequireAdminAuth()(next))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: token})
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 from the wrapped handler", w.Code)
	}
	if !strings.Contains(w.Body.String(), "reached") {
		t.Errorf("body = %q, want the wrapped handler's response", w.Body.String())
	}
}

// Fixed zones far from UTC in both directions, so the assertions below hold on
// a host in any timezone: the stored wall-clock text of a west-zone row reads
// as long past while the instant is still in the future, and the text of an
// east-zone row reads as far future while the instant has already gone by.
// The names are short, all-uppercase and digit-free because the "MST" element
// of adminAuthLocalLayout can only read names of that shape - a digit-carrying
// name would make both rows unparseable and collapse them into the separate
// unparseable case below.
var (
	adminAuthZoneWest = time.FixedZone("WST", -11*60*60)
	adminAuthZoneEast = time.FixedZone("EAT", 13*60*60)
)

// adminAuthLocalLayout is the layout modernc.org/sqlite writes when a Go
// time.Time is bound directly, which is how older builds stored expires_at.
const adminAuthLocalLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// TestRequireAdminAuth_ExpiryIsJudgedByInstantNotText proves the session
// expiry decision is made on the absolute instant in Go rather than by a
// lexicographic TEXT comparison in SQL.
//
// Against the previous "WHERE ... AND expires_at > CURRENT_TIMESTAMP" query
// both zone cases failed: the still-valid west-zone session sorted below
// CURRENT_TIMESTAMP and was thrown back to the login page (denial of service),
// and the already-expired east-zone session sorted above it and authenticated
// (authentication bypass). The unparseable case proves the replacement fails
// closed instead of trusting a value it cannot interpret.
func TestRequireAdminAuth_ExpiryIsJudgedByInstantNotText(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name        string
		expiresAt   string
		wantHandler bool
		explanation string
	}{
		{
			name:        "live-west-zone-text-reads-as-past",
			expiresAt:   now.Add(time.Hour).In(adminAuthZoneWest).Format(adminAuthLocalLayout),
			wantHandler: true,
			explanation: "session expires an hour from now and must authenticate",
		},
		{
			name:        "expired-east-zone-text-reads-as-future",
			expiresAt:   now.Add(-time.Hour).In(adminAuthZoneEast).Format(adminAuthLocalLayout),
			wantHandler: false,
			explanation: "session expired an hour ago and must be rejected",
		},
		{
			name:        "unparseable-is-rejected",
			expiresAt:   "whenever-o-clock",
			wantHandler: false,
			explanation: "an expiry this project cannot parse must fail closed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setAdminTestRenderer(t)
			serverDB := openAdminAuthTestServerDB(t)
			adminID := seedAdminCredential(t, serverDB, "root", "hunter2-hunter2")

			const token = "zone-safety-session-token"
			if _, err := serverDB.Exec(`
				INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, token, adminID, "127.0.0.1", "test-agent", tc.expiresAt, dbtime.FormatSQLTimestamp(now)); err != nil {
				t.Fatalf("seed admin session: %v", err)
			}

			var reached bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("reached"))
			})
			handler := withDB(serverDB, RequireAdminAuth()(next))

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/server/admin/", nil)
			req.AddCookie(&http.Cookie{Name: "admin_session", Value: token})
			handler.ServeHTTP(w, req)

			if reached != tc.wantHandler {
				t.Errorf("wrapped handler reached = %v, want %v (%s)", reached, tc.wantHandler, tc.explanation)
			}

			// Both outcomes are a 200: the reject path renders the login page
			// rather than a distinguishable status, so nothing about whether the
			// session exists leaks.
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", w.Code)
			}
		})
	}
}

// TestAdminLoginHandler covers empty credentials, unknown username, wrong
// password, and a fully correct login (new session row + cookie + redirect).
func TestAdminLoginHandler(t *testing.T) {
	setAdminTestRenderer(t)

	t.Run("empty credentials rejected", func(t *testing.T) {
		serverDB := openAdminAuthTestServerDB(t)
		handler := AdminLoginHandler(serverDB)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(url.Values{}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 for empty username/password", w.Code)
		}
	})

	t.Run("unknown username rejected generically", func(t *testing.T) {
		serverDB := openAdminAuthTestServerDB(t)
		handler := AdminLoginHandler(serverDB)

		form := url.Values{"username": {"nobody"}, "password": {"whatever123"}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler.ServeHTTP(w, req)

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
		handler := AdminLoginHandler(serverDB)

		form := url.Values{"username": {"root"}, "password": {"wrong-password"}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler.ServeHTTP(w, req)

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
		handler := AdminLoginHandler(serverDB)

		form := url.Values{"username": {"root"}, "password": {"correct-password-123"}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler.ServeHTTP(w, req)

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
	handler := AdminLogoutHandler()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/admin/logout", nil)
	handler.ServeHTTP(w, req)

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
