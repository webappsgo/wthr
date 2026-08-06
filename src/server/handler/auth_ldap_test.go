package handler

import (
	"net/http"
	"testing"

	"github.com/webappsgo/wthr/src/server/service"
)

// TestLDAPAuthHandlerLogin_MissingCredentials verifies a request missing the
// required username/password fields is rejected before any LDAP config is
// consulted.
func TestLDAPAuthHandlerLogin_MissingCredentials(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	h := &LDAPAuthHandler{DB: serverDB, LDAPService: service.NewLDAPService(serverDB)}

	c, w := newTestContextJSON(t, http.MethodPost, "/auth/ldap", map[string]string{"username": "alice"})

	h.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestLDAPAuthHandlerLogin_MalformedBody verifies malformed JSON is rejected
// with a 400 rather than panicking.
func TestLDAPAuthHandlerLogin_MalformedBody(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	h := &LDAPAuthHandler{DB: serverDB, LDAPService: service.NewLDAPService(serverDB)}

	c, w := newTestContextJSON(t, http.MethodPost, "/auth/ldap", "not json")

	h.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestLDAPAuthHandlerLogin_Disabled verifies that with LDAP disabled
// (the default with no server_config row), the handler returns 503 without
// attempting a network connection.
func TestLDAPAuthHandlerLogin_Disabled(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	h := &LDAPAuthHandler{DB: serverDB, LDAPService: service.NewLDAPService(serverDB)}

	c, w := newTestContextJSON(t, http.MethodPost, "/auth/ldap", LDAPLoginRequest{
		Username: "alice",
		Password: "secret",
	})

	h.Login(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}
