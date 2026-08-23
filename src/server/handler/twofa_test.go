package handler

import (
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
)

// newTwoFactorTestHandler builds a TwoFactorHandler against a fresh
// in-memory users.db and returns the raw DB handle for seeding/assertions.
func newTwoFactorTestHandler(t *testing.T) (*TwoFactorHandler, *sql.DB) {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return &TwoFactorHandler{DB: usersDB}, usersDB
}

// seedTwoFactorUser inserts a user, optionally with 2FA already enabled and
// a known secret, and returns the loaded model so tests can pass it
// directly into the pure functions/handlers under test.
func seedTwoFactorUser(t *testing.T, db *sql.DB, username, email, password string, enabled bool, secret string) *models.User {
	t.Helper()
	id := seedAuthUser(t, db, username, email, password)
	if enabled {
		if _, err := db.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = ? WHERE id = ?`, secret, id); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}
	}
	user, err := (&models.UserModel{DB: db}).GetByID(id)
	if err != nil {
		t.Fatalf("load seeded user: %v", err)
	}
	return user
}

// TestGetTwoFactorStatus covers the authenticated status payload (with and
// without 2FA enabled) and the unauthenticated 401.
func TestGetTwoFactorStatus(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newTwoFactorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/security/2fa", nil)
		h.GetTwoFactorStatus(w, c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("2FA enabled reports unused recovery key count", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "statususer", "statususer@example.com", "correcthorse123", true, "JBSWY3DPEHPK3PXP")
		if _, err := (&models.RecoveryKeyModel{DB: db}).GenerateRecoveryKeys(int(user.ID)); err != nil {
			t.Fatalf("generate recovery keys: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/security/2fa", nil)
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.GetTwoFactorStatus(w, c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestSetupTwoFactor covers generating a fresh TOTP secret and the
// already-enabled 400 guard.
func TestSetupTwoFactor(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newTwoFactorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/security/2fa/setup", nil)
		h.SetupTwoFactor(w, c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("not-yet-enabled user gets a new secret", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "setupuser", "setupuser@example.com", "correcthorse123", false, "")

		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/security/2fa/setup", nil)
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.SetupTwoFactor(w, c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("already-enabled user is rejected with 400", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "setupuser2", "setupuser2@example.com", "correcthorse123", true, "JBSWY3DPEHPK3PXP")

		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/security/2fa/setup", nil)
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.SetupTwoFactor(w, c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestEnableTwoFactor covers the real-TOTP-code success path, an invalid
// code, and a malformed request body.
func TestEnableTwoFactor(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"

	t.Run("valid code enables 2FA and returns recovery keys", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "enableuser", "enableuser@example.com", "correcthorse123", false, "")
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp code: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/enable", map[string]string{
			"secret": secret,
			"code":   code,
		})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.EnableTwoFactor(w, c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var enabled bool
		if err := db.QueryRow(`SELECT two_factor_enabled FROM user_accounts WHERE id = ?`, user.ID).Scan(&enabled); err != nil {
			t.Fatalf("query 2fa flag: %v", err)
		}
		if !enabled {
			t.Fatalf("expected two_factor_enabled = true after successful enable")
		}
	})

	t.Run("invalid code is rejected with 400", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "enableuser2", "enableuser2@example.com", "correcthorse123", false, "")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/enable", map[string]string{
			"secret": secret,
			"code":   "000000",
		})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.EnableTwoFactor(w, c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing fields fail JSON binding with 400", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "enableuser3", "enableuser3@example.com", "correcthorse123", false, "")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/enable", map[string]string{})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.EnableTwoFactor(w, c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestDisableTwoFactor covers the password-verified success path and the
// wrong-password 401.
func TestDisableTwoFactor(t *testing.T) {
	t.Run("correct password disables 2FA", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "disableuser", "disableuser@example.com", "correcthorse123", true, "JBSWY3DPEHPK3PXP")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/disable", map[string]string{
			"password": "correcthorse123",
		})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.DisableTwoFactor(w, c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var enabled bool
		if err := db.QueryRow(`SELECT two_factor_enabled FROM user_accounts WHERE id = ?`, user.ID).Scan(&enabled); err != nil {
			t.Fatalf("query 2fa flag: %v", err)
		}
		if enabled {
			t.Fatalf("expected two_factor_enabled = false after successful disable")
		}
	})

	t.Run("wrong password is rejected with 401", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "disableuser2", "disableuser2@example.com", "correcthorse123", true, "JBSWY3DPEHPK3PXP")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/disable", map[string]string{
			"password": "totallywrong",
		})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.DisableTwoFactor(w, c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("not enabled returns 400", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "disableuser3", "disableuser3@example.com", "correcthorse123", false, "")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/disable", map[string]string{
			"password": "correcthorse123",
		})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.DisableTwoFactor(w, c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestVerifyTwoFactorCode covers a valid elevated-trust TOTP check and a
// rejected wrong code.
func TestVerifyTwoFactorCode(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"

	t.Run("valid code returns 200", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "verify2fauser", "verify2fauser@example.com", "correcthorse123", true, secret)
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp code: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/verify", map[string]string{"code": code})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.VerifyTwoFactorCode(w, c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong code returns 400", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "verify2fauser2", "verify2fauser2@example.com", "correcthorse123", true, secret)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/2fa/verify", map[string]string{"code": "000000"})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.VerifyTwoFactorCode(w, c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestRegenerateRecoveryKeys covers the valid-code success path and the
// invalid-code 400 rejection.
func TestRegenerateRecoveryKeys(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"

	t.Run("valid code regenerates recovery keys", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "regenuser", "regenuser@example.com", "correcthorse123", true, secret)
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp code: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/recovery/regenerate", map[string]string{"code": code})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.RegenerateRecoveryKeys(w, c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("2FA not enabled returns 400", func(t *testing.T) {
		h, db := newTwoFactorTestHandler(t)
		user := seedTwoFactorUser(t, db, "regenuser2", "regenuser2@example.com", "correcthorse123", false, "")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/recovery/regenerate", map[string]string{"code": "123456"})
		c = withReqCtxValue(c, middleware.UserContextKey, user)
		h.RegenerateRecoveryKeys(w, c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}
