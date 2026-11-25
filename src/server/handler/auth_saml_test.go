package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
)

func newSAMLHandlerTest(t *testing.T, enabled bool) *SAMLAuthHandler {
	t.Helper()
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("create server config: %v", err)
	}
	t.Setenv("CONFIG_DIR", configDir)
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
	if err := config.WriteSetting("server.auth.saml.enabled", enabled); err != nil {
		t.Fatalf("configure SAML: %v", err)
	}
	return &SAMLAuthHandler{DB: storeForDB(serverDB), Service: service.NewSAMLService()}
}

func samlProviderRequest(method string) *http.Request {
	r := httptest.NewRequest(method, "/server/auth/saml/missing", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "missing")
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestSAMLAuthHandlerDisabledRoutes(t *testing.T) {
	h := newSAMLHandlerTest(t, false)

	t.Run("browser login is unavailable", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.StartLogin(w, samlProviderRequest(http.MethodGet))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "disabled") {
			t.Fatalf("StartLogin() = %d %q, want disabled response", w.Code, w.Body.String())
		}
	})
	t.Run("API login is unavailable", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.StartAPILogin(w, samlProviderRequest(http.MethodPost))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("StartAPILogin() = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
	})
	t.Run("provider listing is empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ListAPIProviders(w, httptest.NewRequest(http.MethodGet, "/server/auth/saml/providers", nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data": []`) {
			t.Fatalf("ListAPIProviders() = %d %q, want an empty provider list", w.Code, w.Body.String())
		}
	})
	t.Run("assertion consumer is unavailable", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ACS(w, samlProviderRequest(http.MethodPost))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("ACS() = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestSAMLAuthHandlerRejectsUnknownProviderAndMalformedExchange(t *testing.T) {
	h := newSAMLHandlerTest(t, true)

	for _, test := range []struct {
		name   string
		method string
		call   func(http.ResponseWriter, *http.Request)
		want   int
	}{
		{name: "browser login", method: http.MethodGet, call: h.StartLogin, want: http.StatusNotFound},
		{name: "API login", method: http.MethodPost, call: h.StartAPILogin, want: http.StatusNotFound},
		{name: "metadata", method: http.MethodGet, call: h.Metadata, want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			test.call(w, samlProviderRequest(test.method))
			if w.Code != test.want {
				t.Fatalf("handler status = %d, want %d; body=%q", w.Code, test.want, w.Body.String())
			}
		})
	}

	t.Run("ACS requires POST", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ACS(w, samlProviderRequest(http.MethodGet))
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("ACS() = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
	t.Run("invalid provider fails authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ACS(w, samlProviderRequest(http.MethodPost))
		if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "SAML authentication failed") {
			t.Fatalf("ACS() = %d %q, want authentication failure", w.Code, w.Body.String())
		}
	})
	t.Run("exchange rejects malformed request", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := samlProviderRequest(http.MethodPost)
		r.Body = http.NoBody
		h.ExchangeAPILogin(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("ExchangeAPILogin() = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
	t.Run("exchange requires request ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := samlProviderRequest(http.MethodPost)
		r.Body = io.NopCloser(strings.NewReader(`{"code":"code-without-id"}`))
		h.ExchangeAPILogin(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("ExchangeAPILogin() = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
	t.Run("API auth rejects missing bearer token", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		w := httptest.NewRecorder()
		h.APIAuthMiddleware(next).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/server/auth/saml/logout", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("APIAuthMiddleware() = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

func TestSAMLAuthHandlerCompletesPendingUserIdentity(t *testing.T) {
	h := newSAMLHandlerTest(t, true)
	const pendingToken = "pending-saml-user"
	h.identities = map[string]pendingSAMLIdentity{
		pendingToken: {
			Provider: "idp", ExternalID: "subject-42", Email: "samluser@example.test",
			Name: "SAML User", Groups: []string{"members"}, Role: "user", Expires: time.Now().Add(time.Minute),
		},
	}
	form := url.Values{"username": {"samluser"}}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/external/username", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "saml_pending_identity", Value: pendingToken})
	w := httptest.NewRecorder()
	h.CompleteExternalUsername(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("CompleteExternalUsername() = %d %q; body=%q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if _, ok := h.identities[pendingToken]; ok {
		t.Fatal("completed SAML identity remained pending")
	}
	usersDB := database.GetUsersDB()
	var username, mappedExternalID string
	if err := usersDB.QueryRow(`SELECT username FROM users WHERE email=?`, "samluser@example.test").Scan(&username); err != nil {
		t.Fatalf("load created user: %v", err)
	}
	if err := usersDB.QueryRow(`SELECT external_id FROM user_saml_mappings WHERE provider_name=? AND email=?`, "idp", "samluser@example.test").Scan(&mappedExternalID); err != nil {
		t.Fatalf("load SAML mapping: %v", err)
	}
	if username != "samluser" || mappedExternalID != "subject-42" {
		t.Fatalf("created username/mapping = %q/%q", username, mappedExternalID)
	}
	if len(w.Result().Cookies()) < 2 {
		t.Fatalf("session response cookies = %#v, want session and pending-identity cookies", w.Result().Cookies())
	}
}

func TestSAMLAuthHandlerShowsUsernameChoice(t *testing.T) {
	h := newSAMLHandlerTest(t, true)
	const pendingToken = "pending-saml-username"
	h.identities = map[string]pendingSAMLIdentity{
		pendingToken: {
			Provider: "idp", ExternalID: "subject-43", Email: "choice@example.test",
			Username: "choiceuser", Expires: time.Now().Add(time.Minute),
		},
	}
	previousRenderer := middleware.RenderHTML
	var rendered map[string]interface{}
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, name string, data map[string]interface{}) {
		if name != "page/external_username.tmpl" {
			t.Errorf("template = %q, want external username page", name)
		}
		rendered = data
		w.WriteHeader(status)
	}
	t.Cleanup(func() { middleware.RenderHTML = previousRenderer })
	r := httptest.NewRequest(http.MethodGet, "/server/auth/external/username", nil)
	r.AddCookie(&http.Cookie{Name: "saml_pending_identity", Value: pendingToken})
	w := httptest.NewRecorder()
	h.ShowExternalUsername(w, r)
	if w.Code != http.StatusOK || rendered["username"] != "choiceuser" || rendered["IsAdmin"] != false {
		t.Fatalf("ShowExternalUsername() = %d %#v, want user choice page", w.Code, rendered)
	}
}

func TestSAMLAuthHandlerPendingIdentityLifecycle(t *testing.T) {
	now := time.Now()
	h := &SAMLAuthHandler{
		requests: map[string]pendingSAMLLogin{
			"expired-request": {Provider: "idp", Expires: now.Add(-time.Minute)},
			"active-request":  {Provider: "idp", Expires: now.Add(time.Minute)},
		},
		identities: map[string]pendingSAMLIdentity{
			"expired-identity": {Provider: "idp", Expires: now.Add(-time.Minute)},
			"active-identity":  {Provider: "idp", Expires: now.Add(time.Minute)},
		},
	}
	h.prune()
	if len(h.requests) != 1 || h.requests["active-request"].Provider != "idp" {
		t.Fatalf("pending requests after prune = %#v", h.requests)
	}
	if len(h.identities) != 1 || h.identities["active-identity"].Provider != "idp" {
		t.Fatalf("pending identities after prune = %#v", h.identities)
	}
	expired := httptest.NewRequest(http.MethodGet, "/", nil)
	expired.AddCookie(&http.Cookie{Name: "saml_pending_identity", Value: "expired-identity"})
	if _, ok := h.pendingIdentity(expired); ok || len(h.identities) != 1 {
		t.Fatalf("expired pending identity remained available: %#v", h.identities)
	}
	active := httptest.NewRequest(http.MethodGet, "/", nil)
	active.AddCookie(&http.Cookie{Name: "saml_pending_identity", Value: "active-identity"})
	if identity, ok := h.pendingIdentity(active); !ok || identity.Provider != "idp" {
		t.Fatalf("active pending identity = %#v, %t", identity, ok)
	}
	h.consumePending(active)
	if len(h.identities) != 0 {
		t.Fatalf("pending identity remained after consume: %#v", h.identities)
	}
}

func TestNormalizeSAMLUsername(t *testing.T) {
	tests := []struct{ candidate, email, want string }{
		{candidate: "@Alice.Example", want: "aliceexample"},
		{email: "river@example.org", want: "river"},
		{candidate: "", email: "", want: "user"},
	}
	for _, test := range tests {
		if got := normalizeSAMLUsername(test.candidate, test.email); got != test.want {
			t.Errorf("normalizeSAMLUsername(%q, %q) = %q, want %q", test.candidate, test.email, got, test.want)
		}
	}
}

func TestMappedSAMLRole(t *testing.T) {
	mapping := map[string][]string{"admin": {"server-admins"}, "moderator": {"support"}, "user": {"members"}}
	for _, test := range []struct {
		groups []string
		want   string
	}{
		{groups: []string{"members"}, want: "user"},
		{groups: []string{"support", "members"}, want: "moderator"},
		{groups: []string{"server-admins", "members"}, want: "admin"},
		{groups: []string{"unknown"}, want: "user"},
	} {
		if got := mappedSAMLRole(test.groups, mapping); got != test.want {
			t.Errorf("mappedSAMLRole(%v) = %q, want %q", test.groups, got, test.want)
		}
	}
}
