package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	models "github.com/webappsgo/wthr/src/server/model"
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

// withPasskeyHost sets a valid Host header on the test request so
// buildWebAuthn can derive an RPID from it.
func withPasskeyHost(c *gin.Context) {
	c.Request.Host = "example.com"
}

func TestPasskeyHandlerListPasskeys(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		c, w := newAPITestContext("/api/v1/users/security/passkeys")

		h.ListPasskeys(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("authenticated with no passkeys returns empty list", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newAPITestContext("/api/v1/users/security/passkeys")
		c.Set("user", user)

		h.ListPasskeys(c)

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
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{})

		h.RegisterPasskey(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/passkeys", "not json")
		c.Set("user", user)

		h.RegisterPasskey(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing name and password returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{})
		c.Set("user", user)

		h.RegisterPasskey(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{
			"name":     "my key",
			"password": "wrong-password",
		})
		c.Set("user", user)

		h.RegisterPasskey(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid start request returns registration options", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{
			"name":     "my key",
			"password": "password123",
		})
		c.Set("user", user)
		withPasskeyHost(c)

		h.RegisterPasskey(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"options"`) {
			t.Errorf("expected options in response, got: %s", w.Body.String())
		}
	})

	t.Run("registration completion with unknown ceremony returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/passkeys", map[string]interface{}{
			"response": map[string]interface{}{"id": "abc"},
		})
		c.Set("user", user)

		h.RegisterPasskey(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestPasskeyHandlerDeletePasskey(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		c, w := newAPITestContext("/api/v1/users/security/passkeys/1")
		c.Params = gin.Params{{Key: "passkey_id", Value: "1"}}

		h.DeletePasskey(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid passkey id returns 400", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newAPITestContext("/api/v1/users/security/passkeys/abc")
		c.Set("user", user)
		c.Params = gin.Params{{Key: "passkey_id", Value: "abc"}}

		h.DeletePasskey(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown passkey returns 404", func(t *testing.T) {
		h, user := newPasskeyHandlerTestSetup(t)
		c, w := newAPITestContext("/api/v1/users/security/passkeys/999")
		c.Set("user", user)
		c.Params = gin.Params{{Key: "passkey_id", Value: "999"}}

		h.DeletePasskey(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})
}

func TestPasskeyHandlerBeginPasskeyChallenge(t *testing.T) {
	t.Run("malformed body returns 400", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", "not json")
		withPasskeyHost(c)

		h.BeginPasskeyChallenge(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no session token performs discoverable login", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", map[string]interface{}{})
		withPasskeyHost(c)

		h.BeginPasskeyChallenge(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"options"`) {
			t.Errorf("expected options in response, got: %s", w.Body.String())
		}
	})

	t.Run("unknown session token returns 401", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", map[string]interface{}{
			"session_token": "no-such-session",
		})
		withPasskeyHost(c)

		h.BeginPasskeyChallenge(c)

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

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/passkey/challenge", map[string]interface{}{
			"session_token": pending.ID,
		})
		withPasskeyHost(c)

		h.BeginPasskeyChallenge(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestPasskeyHandlerVerifyPasskey(t *testing.T) {
	t.Run("empty body returns 400", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/passkey/verify", nil)

		h.VerifyPasskey(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no ceremony cookie returns 400", func(t *testing.T) {
		h, _ := newPasskeyHandlerTestSetup(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/passkey/verify", map[string]interface{}{})

		h.VerifyPasskey(c)

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
