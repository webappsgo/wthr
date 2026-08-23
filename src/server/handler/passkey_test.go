package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// newPasskeyHandlerTestSetup wires a PasskeyHandler against a fresh
// in-memory users.db, with the global dual-DB set so package-level model
// calls (UserModel.GetByID, etc.) resolve against the same DB instead of
// nil-panicking.
func newPasskeyHandlerTestSetup(t *testing.T) (*PasskeyHandler, *models.User) {
	t.Helper()
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, usersDB, usersDB)
	user := newPasskeyTestUser(t, usersDB, "pkhandleruser", "password123")
	return NewPasskeyHandler(usersDB), user
}

// withPasskeyHost sets a valid Host on the test request so buildWebAuthn
// can derive an RPID from it.
func withPasskeyHost(r *http.Request) *http.Request {
	r.Host = "example.com"
	return r
}

// withPasskeyUser attaches an authenticated user to the request context,
// the same key middleware.AuthMiddleware sets on a real request.
func withPasskeyUser(r *http.Request, user *models.User) *http.Request {
	return r.WithContext(reqctx.Set(r.Context(), middleware.UserContextKey, user))
}

// withPasskeyURLParam attaches a chi route param to the request context.
func withPasskeyURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// newPasskeyTestRequest builds a plain GET request/recorder pair.
func newPasskeyTestRequest(target string) (*http.Request, *httptest.ResponseRecorder) {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	return r, w
}

// newPasskeyTestJSONRequest builds a request/recorder pair with a JSON
// (or raw string/[]byte passthrough) body, mirroring the shared
// newTestContextJSON test helper's body-encoding behavior.
func newPasskeyTestJSONRequest(t *testing.T, method, target string, body interface{}) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()

	var raw []byte
	switch v := body.(type) {
	case nil:
		raw = nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		var err error
		raw, err = json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

	r := httptest.NewRequest(method, target, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	return r, w
}

func TestPasskeyHandlerListPasskeys(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestRequest("/api/v1/users/security/passkeys")

		h.ListPasskeys(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("authenticated with no passkeys returns empty list", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestRequest("/api/v1/users/security/passkeys")
		r = withPasskeyUser(r, user)

		h.ListPasskeys(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"passkeys":[]`) {
			t.Errorf("expected empty passkeys array, got: %s", w.Body.String())
		}
	})
}

func TestPasskeyHandlerRegisterPasskey(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{})

		h.RegisterPasskey(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/users/security/passkeys", "not json")
		r = withPasskeyUser(r, user)

		h.RegisterPasskey(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing name and password returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{})
		r = withPasskeyUser(r, user)

		h.RegisterPasskey(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{
			"name":     "my key",
			"password": "wrong-password",
		})
		r = withPasskeyUser(r, user)

		h.RegisterPasskey(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid start request returns registration options", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{
			"name":     "my key",
			"password": "password123",
		})
		r = withPasskeyUser(r, user)
		r = withPasskeyHost(r)

		h.RegisterPasskey(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"options"`) {
			t.Errorf("expected options in response, got: %s", w.Body.String())
		}
	})

	t.Run("registration completion with unknown ceremony returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{
			"response": map[string]interface{}{"id": "abc"},
		})
		r = withPasskeyUser(r, user)

		h.RegisterPasskey(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestPasskeyHandlerDeletePasskey(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestRequest("/api/v1/users/security/passkeys/1")
		r = withPasskeyURLParam(r, "passkey_id", "1")

		h.DeletePasskey(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid passkey id returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestRequest("/api/v1/users/security/passkeys/abc")
		r = withPasskeyUser(r, user)
		r = withPasskeyURLParam(r, "passkey_id", "abc")

		h.DeletePasskey(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown passkey returns 404", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestRequest("/api/v1/users/security/passkeys/999")
		r = withPasskeyUser(r, user)
		r = withPasskeyURLParam(r, "passkey_id", "999")

		h.DeletePasskey(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})
}

func TestPasskeyHandlerBeginPasskeyChallenge(t *testing.T) {
	t.Run("malformed body returns 400", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", "not json")
		r = withPasskeyHost(r)

		h.BeginPasskeyChallenge(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no session token performs discoverable login", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", map[string]interface{}{})
		r = withPasskeyHost(r)

		h.BeginPasskeyChallenge(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"options"`) {
			t.Errorf("expected options in response, got: %s", w.Body.String())
		}
	})

	t.Run("unknown session token returns 401", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", map[string]interface{}{
			"session_token": "no-such-session",
		})
		r = withPasskeyHost(r)

		h.BeginPasskeyChallenge(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("user with no passkeys returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		pending, err := createPendingTwoFactorSession(h.DB, user.ID)
		if err != nil {
			t.Fatalf("create pending session: %v", err)
		}

		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", map[string]interface{}{
			"session_token": pending.ID,
		})
		r = withPasskeyHost(r)

		h.BeginPasskeyChallenge(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestPasskeyHandlerVerifyPasskey(t *testing.T) {
	t.Run("empty body returns 400", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/server/auth/passkey/verify", nil)

		h.VerifyPasskey(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no ceremony cookie returns 400", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		r, w := newPasskeyTestJSONRequest(t, http.MethodPost, "/api/v1/server/auth/passkey/verify", map[string]interface{}{})

		h.VerifyPasskey(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestParsePasskeyUserHandle(t *testing.T) {
	tests := []struct {
		name    string
		handle  []byte
		want    int64
		wantErr bool
	}{
		{name: "valid handle", handle: []byte("usr:42"), want: 42},
		{name: "missing prefix", handle: []byte("42"), wantErr: true},
		{name: "empty handle", handle: []byte(""), wantErr: true},
		{name: "non-numeric id", handle: []byte("usr:abc"), wantErr: true},
		{name: "zero id", handle: []byte("usr:0"), wantErr: true},
		{name: "negative id", handle: []byte("usr:-5"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePasskeyUserHandle(tt.handle)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got id=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("id = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPasskeyUserWebAuthnAccessors(t *testing.T) {
	user := &models.User{ID: 7, Username: "waid", Email: "waid@example.com"}
	pu := &passkeyUser{user: user}

	if string(pu.WebAuthnID()) != "usr:7" {
		t.Errorf("WebAuthnID() = %q, want %q", string(pu.WebAuthnID()), "usr:7")
	}
	if pu.WebAuthnName() != "waid" {
		t.Errorf("WebAuthnName() = %q, want %q", pu.WebAuthnName(), "waid")
	}
	if pu.WebAuthnDisplayName() != "waid" {
		t.Errorf("WebAuthnDisplayName() = %q, want %q", pu.WebAuthnDisplayName(), "waid")
	}
	if creds := pu.WebAuthnCredentials(); creds != nil {
		t.Errorf("WebAuthnCredentials() = %v, want nil for zero-value passkeyUser", creds)
	}
}
