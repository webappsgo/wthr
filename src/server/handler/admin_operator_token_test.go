package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
)

// operatorTokenFixture installs a global config holding a token and a scratch CONFIG_DIR.
func operatorTokenFixture(t *testing.T, token string) {
	t.Helper()
	withTestGlobalConfig(t)
	config.GetGlobalConfig().Server.Token = token
	middleware.SetOperatorToken(token)
	t.Cleanup(func() { middleware.SetOperatorToken("") })
}

func TestRegenerateOperatorTokenReplacesTheValue(t *testing.T) {
	newAdminsTestHandler(t)
	admin := newAdminsTestAdmin(t, "operatortokenadmin", "password123")
	operatorTokenFixture(t, "tok_oldoldoldoldoldoldoldoldoldoldol")
	h := NewAdminOperatorTokenHandler()

	r, w := newAdminsJSONRequest(t, http.MethodPost, "/api/v1/server/administration/security/operator-token", map[string]string{
		"current_password": "password123",
	})
	r = withAdminID(r, int(admin.ID))
	h.RegenerateOperatorToken(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	fresh := config.GetGlobalConfig().Server.Token
	if !middleware.IsOperatorTokenFormat(fresh) || fresh == "tok_oldoldoldoldoldoldoldoldoldoldol" {
		t.Fatalf("a new token in the tok_ format must be saved, got %q", fresh)
	}
	if !strings.Contains(w.Body.String(), fresh) {
		t.Error("the new value must be returned once in the response")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("the response carrying the token must not be cached, got %q", w.Header().Get("Cache-Control"))
	}
	if middleware.IsOperatorToken("tok_oldoldoldoldoldoldoldoldoldoldol") || !middleware.IsOperatorToken(fresh) {
		t.Error("the previous value must stop working at once and the new one must work")
	}
}

func TestRegenerateOperatorTokenNeedsTheCorrectPassword(t *testing.T) {
	newAdminsTestHandler(t)
	admin := newAdminsTestAdmin(t, "operatortokenadmin2", "password123")
	operatorTokenFixture(t, "tok_keepkeepkeepkeepkeepkeepkeepkeep")
	h := NewAdminOperatorTokenHandler()

	r, w := newAdminsJSONRequest(t, http.MethodPost, "/api/v1/server/administration/security/operator-token", map[string]string{
		"current_password": "wrong-password",
	})
	r = withAdminID(r, int(admin.ID))
	h.RegenerateOperatorToken(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	if config.GetGlobalConfig().Server.Token != "tok_keepkeepkeepkeepkeepkeepkeepkeep" || !middleware.IsOperatorToken("tok_keepkeepkeepkeepkeepkeepkeepkeep") {
		t.Error("a wrong password must leave the token unchanged")
	}

	empty, wEmpty := newAdminsJSONRequest(t, http.MethodPost, "/api/v1/server/administration/security/operator-token", map[string]string{"current_password": " "})
	empty = withAdminID(empty, int(admin.ID))
	h.RegenerateOperatorToken(wEmpty, empty)
	if wEmpty.Code != http.StatusBadRequest {
		t.Errorf("a blank password = %d, want 400", wEmpty.Code)
	}
}

func TestRegenerateOperatorTokenNeedsAnAdminSession(t *testing.T) {
	newAdminsTestHandler(t)
	operatorTokenFixture(t, "tok_nosessionnosessionnosessionnosess")
	h := NewAdminOperatorTokenHandler()

	r, w := newAdminsJSONRequest(t, http.MethodPost, "/api/v1/server/administration/security/operator-token", map[string]string{"current_password": "x"})
	h.RegenerateOperatorToken(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetOperatorTokenNeverReturnsTheValue(t *testing.T) {
	const token = "tok_abcdefghijklmnopqrstuvwxyzABCDEF"
	operatorTokenFixture(t, token)
	h := NewAdminOperatorTokenHandler()

	r, w := newAPITestContext("/api/v1/server/administration/security/operator-token")
	h.GetOperatorToken(w, r)
	body := w.Body.String()
	if w.Code != http.StatusOK || strings.Contains(body, token) || strings.Contains(body, "abcdefghijklmnop") {
		t.Errorf("only the masked form may be returned, status=%d body=%s", w.Code, body)
	}
	if !strings.Contains(body, "ABCDEF"[2:]) {
		t.Errorf("the last characters must be shown, body=%s", body)
	}
}
