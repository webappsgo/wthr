package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crewjam/saml"
	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

func setSAMLLogoutProvider(t *testing.T, provider service.SAMLProviderConfig) *SAMLAuthHandler {
	t.Helper()
	h := newSAMLHandlerTest(t, true)
	settings := &model.SettingsModel{Store: database.GetServerStore()}
	if err := settings.SetJSON("server.auth.saml.providers", []service.SAMLProviderConfig{provider}); err != nil {
		t.Fatal(err)
	}
	return h
}

func samlLogoutRequest(method, provider string) *http.Request {
	r := httptest.NewRequest(method, "/server/auth/saml/"+provider+"/slo", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", provider)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestSAMLLogoutRoutesRejectUnknownProviderAndMissingSession(t *testing.T) {
	h := setSAMLLogoutProvider(t, service.SAMLProviderConfig{Name: "example", SLOEnabled: true})

	w := httptest.NewRecorder()
	h.StartSAMLLogout(w, samlLogoutRequest(http.MethodGet, "unknown"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown SP logout provider status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.StartSAMLLogout(w, samlLogoutRequest(http.MethodGet, "example"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("SP logout without a mapped session status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.StartAPISAMLLogout(w, samlLogoutRequest(http.MethodPost, "unknown"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown API logout provider status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.StartAPISAMLLogout(w, samlLogoutRequest(http.MethodPost, "example"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("API logout without a mapped session status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.SAMLLogout(w, samlLogoutRequest(http.MethodGet, "unknown"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown IdP logout provider status = %d", w.Code)
	}
}

func TestTryStartSAMLLogoutWithoutCurrentProviderIsNoop(t *testing.T) {
	h := newSAMLHandlerTest(t, true)
	w := httptest.NewRecorder()
	if h.TryStartSAMLLogout(w, httptest.NewRequest(http.MethodPost, "/logout", nil)) {
		t.Fatal("TryStartSAMLLogout claimed to start without a current SAML provider")
	}
	if w.Body.Len() != 0 {
		t.Fatalf("no-op logout wrote a response: %q", w.Body.String())
	}
}

func TestSAMLSessionIndexStrings(t *testing.T) {
	got := samlSessionIndexStrings([]saml.SessionIndex{{Value: "one"}, {Value: "two"}})
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("session indexes = %#v", got)
	}
	if empty := samlSessionIndexStrings(nil); empty == nil || len(empty) != 0 {
		t.Fatalf("empty session indexes = %#v", empty)
	}
}
