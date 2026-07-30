package handler

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"

	"github.com/webappsgo/wthr/src/config"
	models "github.com/webappsgo/wthr/src/server/model"
)

// newAuthAPITestHandler wires an AuthAPIHandler against fresh in-memory
// server.db/users.db instances and primes the database.GetServerDB/
// GetUsersDB globals the package-level LoginAPIUser/RegisterAPIUser/etc.
// helpers read directly.
func newAuthAPITestHandler(t *testing.T) *AuthAPIHandler {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return &AuthAPIHandler{DB: usersDB}
}

// TestHandleAPILogin covers the happy path (no 2FA), rejection of bad
// credentials, and the whitespace-padded-password 400 guard.
func TestHandleAPILogin(t *testing.T) {
	t.Run("valid credentials return a session token", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		seedAuthUser(t, h.DB, "apiuser", "apiuser@example.com", "correcthorse123")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/login", APILoginRequest{
			Identifier: "apiuser",
			Password:   "correcthorse123",
		})
		h.HandleAPILogin(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body struct {
			OK   bool `json:"ok"`
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !body.OK || body.Data.Token == "" {
			t.Fatalf("expected ok=true with a session token, got %s", w.Body.String())
		}
	})

	t.Run("wrong password returns generic invalid credentials", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		seedAuthUser(t, h.DB, "apiuser2", "apiuser2@example.com", "correcthorse123")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/login", APILoginRequest{
			Identifier: "apiuser2",
			Password:   "totallywrong",
		})
		h.HandleAPILogin(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("whitespace-padded password is rejected with 400 before any DB lookup", func(t *testing.T) {
		h := newAuthAPITestHandler(t)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/login", APILoginRequest{
			Identifier: "nobody",
			Password:   " padded ",
		})
		h.HandleAPILogin(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/login", "{not json")
		h.HandleAPILogin(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("2FA-enabled account returns a pending session instead of a token", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "apiuser3", "apiuser3@example.com", "correcthorse123")
		secret := "JBSWY3DPEHPK3PXP"
		if _, err := h.DB.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = ? WHERE id = ?`, secret, userID); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/login", APILoginRequest{
			Identifier: "apiuser3",
			Password:   "correcthorse123",
		})
		h.HandleAPILogin(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Data struct {
				RequiresTwoFactor bool   `json:"requires_2fa"`
				SessionToken      string `json:"session_token"`
				Token             string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !body.Data.RequiresTwoFactor || body.Data.SessionToken == "" || body.Data.Token != "" {
			t.Fatalf("expected pending 2fa session without a full token, got %s", w.Body.String())
		}
	})
}

// TestHandleAPIRegister covers the created-account happy path and the
// registration-disabled 404 gate.
func TestHandleAPIRegister(t *testing.T) {
	t.Run("registration disabled returns 404", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/register", APIRegisterRequest{
			Username: "newapiuser",
			Email:    "newapiuser@example.com",
			Password: "correcthorse123",
		})
		h.HandleAPIRegister(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("valid registration creates an account", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{Users: config.UsersConfig{Enabled: true, Registration: config.RegistrationConfig{Mode: "open"}}})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/register", APIRegisterRequest{
			Username: "newapiuser2",
			Email:    "newapiuser2@example.com",
			Password: "correcthorse123",
		})
		h.HandleAPIRegister(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := h.DB.QueryRow(`SELECT COUNT(*) FROM user_accounts WHERE username = ?`, "newapiuser2").Scan(&count); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected exactly one account row, got %d", count)
		}
	})

	t.Run("duplicate username returns 409", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{Users: config.UsersConfig{Enabled: true, Registration: config.RegistrationConfig{Mode: "open"}}})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })
		seedAuthUser(t, h.DB, "dupeuser", "dupeuser@example.com", "correcthorse123")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/register", APIRegisterRequest{
			Username: "dupeuser",
			Email:    "other@example.com",
			Password: "correcthorse123",
		})
		h.HandleAPIRegister(c)

		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleAPILogout covers the authenticated success path and the
// missing-session 401.
func TestHandleAPILogout(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/logout", nil)
		h.HandleAPILogout(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("active session is deleted", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "logoutuser", "logoutuser@example.com", "correcthorse123")
		sessionModel := &models.SessionModel{DB: h.DB}
		session, err := sessionModel.Create(userID, 3600)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/logout", nil)
		c.Set("session", session)
		h.HandleAPILogout(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if _, err := sessionModel.GetByID(session.ID); err == nil {
			t.Fatalf("expected session to be deleted after logout")
		}
	})
}

// TestHandleAPI2FA covers completing a pending login with a real TOTP code
// and rejecting an invalid one.
func TestHandleAPI2FA(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"

	setupPending := func(t *testing.T, h *AuthAPIHandler) (userID int64, pendingToken string) {
		t.Helper()
		userID = seedAuthUser(t, h.DB, "twofauser", "twofauser@example.com", "correcthorse123")
		if _, err := h.DB.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = ? WHERE id = ?`, secret, userID); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}
		pending, err := createPendingTwoFactorSession(h.DB, userID)
		if err != nil {
			t.Fatalf("create pending session: %v", err)
		}
		return userID, pending.ID
	}

	t.Run("valid TOTP code completes login", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		_, pendingToken := setupPending(t, h)
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp code: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/2fa", API2FARequest{
			SessionToken:  pendingToken,
			TwoFactorCode: code,
		})
		h.HandleAPI2FA(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong TOTP code is rejected with 401", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		_, pendingToken := setupPending(t, h)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/2fa", API2FARequest{
			SessionToken:  pendingToken,
			TwoFactorCode: "000000",
		})
		h.HandleAPI2FA(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown pending session token is rejected with 401", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/2fa", API2FARequest{
			SessionToken:  "does-not-exist",
			TwoFactorCode: "123456",
		})
		h.HandleAPI2FA(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleAPIRecoveryUse covers completing a pending login with a real
// recovery key and rejecting an invalid one.
func TestHandleAPIRecoveryUse(t *testing.T) {
	t.Run("valid recovery key completes login", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "recoveryuser", "recoveryuser@example.com", "correcthorse123")
		if _, err := h.DB.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = 'JBSWY3DPEHPK3PXP' WHERE id = ?`, userID); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}
		keys, err := (&models.RecoveryKeyModel{DB: h.DB}).GenerateRecoveryKeys(int(userID))
		if err != nil || len(keys) == 0 {
			t.Fatalf("generate recovery keys: %v", err)
		}
		pending, err := createPendingTwoFactorSession(h.DB, userID)
		if err != nil {
			t.Fatalf("create pending session: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/recovery/use", APIRecoveryUseRequest{
			SessionToken: pending.ID,
			RecoveryKey:  keys[0],
		})
		h.HandleAPIRecoveryUse(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid recovery key is rejected with 401", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "recoveryuser2", "recoveryuser2@example.com", "correcthorse123")
		pending, err := createPendingTwoFactorSession(h.DB, userID)
		if err != nil {
			t.Fatalf("create pending session: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/recovery/use", APIRecoveryUseRequest{
			SessionToken: pending.ID,
			RecoveryKey:  "wrong-key",
		})
		h.HandleAPIRecoveryUse(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleAPIRefresh covers extending an authenticated session and the
// unauthenticated 401 path.
func TestHandleAPIRefresh(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/refresh", nil)
		h.HandleAPIRefresh(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("authenticated session is refreshed", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "refreshuser", "refreshuser@example.com", "correcthorse123")
		sessionModel := &models.SessionModel{DB: h.DB}
		session, err := sessionModel.Create(userID, 3600)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		userModel := &models.UserModel{DB: h.DB}
		user, err := userModel.GetByID(userID)
		if err != nil {
			t.Fatalf("get user: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/refresh", nil)
		c.Set("session", session)
		c.Set("user", user)
		h.HandleAPIRefresh(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if _, err := sessionModel.GetByID(session.ID); err == nil {
			t.Fatalf("expected old session to be invalidated after refresh")
		}
	})
}

// TestHandleAPIVerifyEmail covers the success path, the invalid-token 400,
// and documents a real production bug (see BUG note below): a genuine DB
// failure while marking the account verified is misreported as a 400
// instead of a 500 because the handler compares against a capitalized
// error string that the underlying function never produces.
func TestHandleAPIVerifyEmail(t *testing.T) {
	t.Run("valid token marks the account verified", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "verifyuser", "verifyuser@example.com", "correcthorse123")
		if _, err := h.DB.Exec(`UPDATE user_accounts SET email_verified = 0 WHERE id = ?`, userID); err != nil {
			t.Fatalf("reset verified flag: %v", err)
		}
		if _, err := h.DB.Exec(`
			CREATE TABLE IF NOT EXISTS user_email_verifications (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				email TEXT NOT NULL,
				token TEXT NOT NULL,
				expires_at DATETIME NOT NULL
			)`); err != nil {
			t.Fatalf("create verifications table: %v", err)
		}
		if _, err := h.DB.Exec(`INSERT INTO user_email_verifications (user_id, email, token, expires_at) VALUES (?, ?, ?, datetime('now','+1 hour'))`,
			userID, "verifyuser@example.com", "good-token"); err != nil {
			t.Fatalf("seed verification row: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/verify", APIVerifyEmailRequest{Token: "good-token"})
		h.HandleAPIVerifyEmail(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var verified bool
		if err := h.DB.QueryRow(`SELECT email_verified FROM user_accounts WHERE id = ?`, userID).Scan(&verified); err != nil {
			t.Fatalf("query verified flag: %v", err)
		}
		if !verified {
			t.Fatalf("expected email_verified = true after successful verification")
		}
	})

	t.Run("unknown token returns 400", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/verify", APIVerifyEmailRequest{Token: "bogus"})
		h.HandleAPIVerifyEmail(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	// BUG: auth_api.go VerifyAPIUserEmail (~line 491) returns
	// fmt.Errorf("failed to verify email") (lowercase) on a DB UPDATE
	// failure, but HandleAPIVerifyEmail (auth_api.go:1001) compares
	// err.Error() == "Failed to verify email" (capitalized). The
	// case mismatch means the branch never triggers, so a genuine
	// database failure is reported as 400 Bad Request instead of the
	// correct 500 Internal Server Error. This test encodes the CORRECT
	// expected behavior and is expected to FAIL against current code.
	t.Run("BUG: DB failure during verification should return 500, not 400", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "verifybuguser", "verifybuguser@example.com", "correcthorse123")
		if _, err := h.DB.Exec(`
			CREATE TABLE IF NOT EXISTS user_email_verifications (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				email TEXT NOT NULL,
				token TEXT NOT NULL,
				expires_at DATETIME NOT NULL
			)`); err != nil {
			t.Fatalf("create verifications table: %v", err)
		}
		if _, err := h.DB.Exec(`INSERT INTO user_email_verifications (user_id, email, token, expires_at) VALUES (?, ?, ?, datetime('now','+1 hour'))`,
			userID, "verifybuguser@example.com", "bug-token"); err != nil {
			t.Fatalf("seed verification row: %v", err)
		}
		// Force the subsequent UPDATE user_accounts to fail by dropping the
		// table out from under it after the verification row lookup succeeds.
		if _, err := h.DB.Exec(`DROP TABLE user_accounts`); err != nil {
			t.Fatalf("drop user_accounts to force update failure: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/verify", APIVerifyEmailRequest{Token: "bug-token"})
		h.HandleAPIVerifyEmail(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("KNOWN BUG (auth_api.go:1001): status = %d, want 500 for a DB failure; got body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleAPIPasswordForgot covers the always-200 enumeration-safe
// response and the malformed-body 400.
func TestHandleAPIPasswordForgot(t *testing.T) {
	t.Run("valid email always returns 200 regardless of existence", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/password/forgot", APIPasswordForgotRequest{Email: "nobody@example.com"})
		h.HandleAPIPasswordForgot(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/password/forgot", "{not json")
		h.HandleAPIPasswordForgot(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// TestHandleAPIPasswordReset covers the success path, the invalid-token
// 400, and documents a second instance of the same case-sensitivity bug
// class as HandleAPIVerifyEmail.
func TestHandleAPIPasswordReset(t *testing.T) {
	seedResetRow := func(t *testing.T, h *AuthAPIHandler, userID int64, token string) {
		t.Helper()
		if _, err := h.DB.Exec(`
			CREATE TABLE IF NOT EXISTS user_password_resets (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				token TEXT NOT NULL,
				expires_at DATETIME NOT NULL
			)`); err != nil {
			t.Fatalf("create resets table: %v", err)
		}
		if _, err := h.DB.Exec(`INSERT INTO user_password_resets (user_id, token, expires_at) VALUES (?, ?, datetime('now','+1 hour'))`,
			userID, token); err != nil {
			t.Fatalf("seed reset row: %v", err)
		}
	}

	t.Run("valid token resets the password and revokes sessions", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "resetuser", "resetuser@example.com", "oldpassword1")
		sessionModel := &models.SessionModel{DB: h.DB}
		oldSession, err := sessionModel.Create(userID, 3600)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		seedResetRow(t, h, userID, "reset-token")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/password/reset", APIPasswordResetRequest{
			Token:    "reset-token",
			Password: "brandnewpassword1",
		})
		h.HandleAPIPasswordReset(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if _, err := sessionModel.GetByID(oldSession.ID); err == nil {
			t.Fatalf("expected all sessions to be revoked after a password reset")
		}
	})

	t.Run("unknown token returns 400", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/password/reset", APIPasswordResetRequest{
			Token:    "bogus",
			Password: "brandnewpassword1",
		})
		h.HandleAPIPasswordReset(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("short password returns 400 before touching the DB", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/password/reset", "{\"token\":\"x\",\"password\":\"short\"}")
		h.HandleAPIPasswordReset(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	// BUG: auth_api.go ResetAPIUserPassword (~line 592) returns
	// fmt.Errorf("failed to reset password") (lowercase) on a DB UPDATE
	// failure, but HandleAPIPasswordReset (auth_api.go:1080) compares
	// against the capitalized "Failed to reset password" /
	// "Failed to process password". The mismatch means a genuine
	// database failure during password reset is reported as 400 Bad
	// Request instead of 500 Internal Server Error. This test encodes
	// the CORRECT expected behavior and is expected to FAIL against
	// current code.
	t.Run("BUG: DB failure during reset should return 500, not 400", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		userID := seedAuthUser(t, h.DB, "resetbuguser", "resetbuguser@example.com", "oldpassword1")
		seedResetRow(t, h, userID, "bug-reset-token")
		if _, err := h.DB.Exec(`DROP TABLE user_accounts`); err != nil {
			t.Fatalf("drop user_accounts to force update failure: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/password/reset", APIPasswordResetRequest{
			Token:    "bug-reset-token",
			Password: "brandnewpassword1",
		})
		h.HandleAPIPasswordReset(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("KNOWN BUG (auth_api.go:1080): status = %d, want 500 for a DB failure; got body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleAPIUserInviteValidate covers a valid invite token and the
// empty-token 400 path (also affected by the same case-mismatch bug class,
// noted inline).
func TestHandleAPIUserInviteValidate(t *testing.T) {
	t.Run("valid invite token returns invite details", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		invite, err := (&models.UserInviteModel{DB: h.DB}).CreateInvite("inviteduser", "invited@example.com", "user", 7)
		if err != nil {
			t.Fatalf("create invite: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/auth/invite/user/"+invite.Token, nil)
		c.Params = []gin.Param{{Key: "token", Value: invite.Token}}
		h.HandleAPIUserInviteValidate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown invite token returns 410", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/auth/invite/user/bogus", nil)
		c.Params = []gin.Param{{Key: "token", Value: "bogus"}}
		h.HandleAPIUserInviteValidate(c)
		if w.Code != http.StatusGone {
			t.Fatalf("status = %d, want 410; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleAPIUserInviteComplete covers accepting a valid invite and
// creating the resulting account.
func TestHandleAPIUserInviteComplete(t *testing.T) {
	t.Run("valid invite completion creates an account", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		invite, err := (&models.UserInviteModel{DB: h.DB}).CreateInvite("", "completeinvite@example.com", "user", 7)
		if err != nil {
			t.Fatalf("create invite: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/invite/user/"+invite.Token, map[string]string{
			"username": "completedinviteuser",
			"password": "correcthorse123",
		})
		c.Params = []gin.Param{{Key: "token", Value: invite.Token}}
		h.HandleAPIUserInviteComplete(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := h.DB.QueryRow(`SELECT COUNT(*) FROM user_accounts WHERE username = ?`, "completedinviteuser").Scan(&count); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected exactly one account row, got %d", count)
		}
	})

	t.Run("expired/unknown invite token returns an error status", func(t *testing.T) {
		h := newAuthAPITestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/auth/invite/user/bogus", map[string]string{
			"username": "whoever",
			"password": "correcthorse123",
		})
		c.Params = []gin.Param{{Key: "token", Value: "bogus"}}
		h.HandleAPIUserInviteComplete(c)
		if w.Code < 400 {
			t.Fatalf("status = %d, want a 4xx error", w.Code)
		}
	})
}
