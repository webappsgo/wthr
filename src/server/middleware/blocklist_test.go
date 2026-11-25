package middleware

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/reqctx"
	_ "modernc.org/sqlite"
)

func openBlocklistTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:blocklist_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestBlocklistMiddlewareBlocksIPAndCIDR(t *testing.T) {
	db := openBlocklistTestDB(t)
	if _, err := db.Exec(`INSERT INTO server_blocklists (type, value, reason) VALUES ('ip', '198.51.100.20', 'manual'), ('cidr', '203.0.113.0/24', 'network')`); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"198.51.100.20", "203.0.113.9"} {
		called := false
		handler := BlocklistMiddleware(storeForDB(db))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
		request := httptest.NewRequest(http.MethodGet, "/weather", nil)
		request.RemoteAddr = ip + ":443"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if called || recorder.Code != http.StatusForbidden {
			t.Fatalf("request from %s was not blocked: next=%t status=%d", ip, called, recorder.Code)
		}
	}
}

func TestBlocklistMiddlewareSkipsAllowlistedIPAndFailsOpenWithoutDatabase(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Security.Allowlist = []config.IPAllowlistEntry{{CIDR: "198.51.100.0/24"}}
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	db := openBlocklistTestDB(t)
	if _, err := db.Exec(`INSERT INTO server_blocklists (type, value) VALUES ('cidr', '198.51.100.0/24')`); err != nil {
		t.Fatal(err)
	}
	called := false
	handler := AllowlistMiddleware()(BlocklistMiddleware(storeForDB(db))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = reqctx.GetBool(r.Context(), "allowlisted")
	})))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "198.51.100.8:443"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if !called || recorder.Code != http.StatusOK {
		t.Fatalf("allowlisted request was blocked: called=%t status=%d", called, recorder.Code)
	}

	called = false
	BlocklistMiddleware(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("request did not continue when blocklist database was unavailable")
	}
}

func TestBlocklistMiddlewareNegotiatesBlockedResponses(t *testing.T) {
	db := openBlocklistTestDB(t)
	if _, err := db.Exec(`INSERT INTO server_blocklists (type, value) VALUES ('ip', '198.51.100.45')`); err != nil {
		t.Fatal(err)
	}
	previousRenderer := RenderHTML
	RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, template string, _ map[string]interface{}) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(template))
	}
	t.Cleanup(func() { RenderHTML = previousRenderer })

	cases := []struct {
		path      string
		accept    string
		userAgent string
		wantType  string
		wantBody  string
	}{
		{path: "/weather", accept: "text/html", wantType: "", wantBody: "page/error.tmpl"},
		{path: "/weather", userAgent: "curl/8.0", wantType: "text/plain; charset=utf-8", wantBody: "Request is blocked"},
		{path: "/api/v1/weather", accept: "application/json", wantType: "application/json; charset=utf-8", wantBody: "IP_BLOCKED"},
		{path: "/api/v1/weather", accept: "text/plain", wantType: "text/plain; charset=utf-8", wantBody: "Request is blocked"},
	}
	for _, test := range cases {
		t.Run(test.path+" "+test.accept+" "+test.userAgent, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.RemoteAddr = "198.51.100.45:443"
			request.Header.Set("Accept", test.accept)
			request.Header.Set("User-Agent", test.userAgent)
			recorder := httptest.NewRecorder()
			BlocklistMiddleware(storeForDB(db))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
			if test.wantType != "" && recorder.Header().Get("Content-Type") != test.wantType {
				t.Errorf("Content-Type = %q, want %q", recorder.Header().Get("Content-Type"), test.wantType)
			}
			if !strings.Contains(recorder.Body.String(), test.wantBody) {
				t.Errorf("body = %q, want it to contain %q", recorder.Body.String(), test.wantBody)
			}
		})
	}
}
