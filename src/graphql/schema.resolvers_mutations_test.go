package graphql

import (
	"context"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/pquerna/otp/totp"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// --- Shared test fixtures -------------------------------------------------
// These resolvers exercise real handler/model logic against a real sqlite
// DualDB, since several model methods (UserModel.Create, AdminModel.Create,
// AdminInviteModel.CreateInvite) bypass their own DB field and always hit
// the database.GetServerDB()/GetUsersDB() package globals. Every test below
// therefore sets the global via database.SetGlobalDualDB and resets it in
// t.Cleanup. These tests cannot run t.Parallel with each other.

func newAuthMutationTestDB(t *testing.T) *database.DualDB {
	t.Helper()
	ddb, err := database.InitDualDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDualDB: %v", err)
	}
	database.SetGlobalDualDB(ddb)
	t.Cleanup(func() {
		database.SetGlobalDualDB(nil)
		ddb.Close()
	})
	return ddb
}

func setOpenRegistrationConfig(t *testing.T) {
	t.Helper()
	config.SetGlobalConfig(&config.AppConfig{
		Users: config.UsersConfig{
			Enabled: true,
			Registration: config.RegistrationConfig{
				Mode: "open",
			},
		},
	})
	t.Cleanup(func() { config.SetGlobalConfig(nil) })
}

func seedGraphQLUser(t *testing.T, ddb *database.DualDB, username, email, password string) *models.User {
	t.Helper()
	user, err := (&models.UserModel{DB: ddb.Users}).Create(username, email, password, "user")
	if err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
	return user
}

// seedPendingTwoFactorSession replicates handler.createPendingTwoFactorSession
// (unexported) using only the public SessionModel API, since it isn't
// reachable from the graphql package.
func seedPendingTwoFactorSession(t *testing.T, ddb *database.DualDB, userID int64) string {
	t.Helper()
	sessionModel := &models.SessionModel{DB: ddb.Users}
	session, err := sessionModel.Create(userID, 900)
	if err != nil {
		t.Fatalf("create pending session: %v", err)
	}
	if err := sessionModel.UpdateData(session.ID, map[string]interface{}{
		"auth_stage":        "pending_2fa",
		"requires_2fa":      true,
		"temporary_session": true,
	}); err != nil {
		t.Fatalf("update pending session data: %v", err)
	}
	return session.ID
}

// --- RegisterUser -----------------------------------------------------------

func TestMutationResolver_RegisterUser(t *testing.T) {
	t.Run("registration not available with no config", func(t *testing.T) {
		ddb := newAuthMutationTestDB(t)
		m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

		_, err := m.RegisterUser(context.Background(), "newuser", "newuser@example.com", "password123")
		if err == nil || err.Error() != "registration is not available" {
			t.Fatalf("err = %v, want %q", err, "registration is not available")
		}
	})

	t.Run("validation and happy path", func(t *testing.T) {
		ddb := newAuthMutationTestDB(t)
		setOpenRegistrationConfig(t)
		m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

		tests := []struct {
			name        string
			username    string
			email       string
			password    string
			wantErr     string
			checkResult bool
		}{
			{name: "invalid email", username: "aliceuser", email: "not-an-email", password: "password123", wantErr: "invalid email format"},
			{name: "short password", username: "bobuser", email: "bob@example.com", password: "short", wantErr: "password must be at least 8 characters"},
			{name: "happy path", username: "carolreg", email: "carol@example.com", password: "password123", checkResult: true},
			{name: "duplicate username or email", username: "carolreg", email: "different@example.com", password: "password123", wantErr: "username or email already exists"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := m.RegisterUser(context.Background(), tt.username, tt.email, tt.password)
				if tt.wantErr != "" {
					if err == nil || err.Error() != tt.wantErr {
						t.Fatalf("err = %v, want %q", err, tt.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !tt.checkResult {
					return
				}
				if result == nil || result.User == nil {
					t.Fatal("expected AuthResult with User set")
				}
				if result.User.Username != tt.username {
					t.Fatalf("Username = %q, want %q", result.User.Username, tt.username)
				}
				if result.VerificationRequired {
					t.Fatal("VerificationRequired = true, want false (RequireEmailVerification not set)")
				}
				if result.Token == nil || *result.Token == "" {
					t.Fatal("expected a non-empty session token on happy-path registration")
				}
			})
		}
	})
}

// --- LoginUser ---------------------------------------------------------------

func TestMutationResolver_LoginUser(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	seedGraphQLUser(t, ddb, "loginuser", "loginuser@example.com", "correctpass1")

	t.Run("invalid credentials wrong password", func(t *testing.T) {
		_, err := m.LoginUser(context.Background(), "loginuser", "wrongpass1", nil, nil)
		if err == nil || err.Error() != "invalid credentials" {
			t.Fatalf("err = %v, want %q", err, "invalid credentials")
		}
	})

	t.Run("invalid credentials unknown identifier", func(t *testing.T) {
		_, err := m.LoginUser(context.Background(), "nosuchuser", "whatever1", nil, nil)
		if err == nil || err.Error() != "invalid credentials" {
			t.Fatalf("err = %v, want %q", err, "invalid credentials")
		}
	})

	t.Run("happy path returns session token", func(t *testing.T) {
		result, err := m.LoginUser(context.Background(), "loginuser", "correctpass1", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.Token == nil || *result.Token == "" {
			t.Fatal("expected a non-empty session token")
		}
		if result.RequiresTwoFactor {
			t.Fatal("RequiresTwoFactor = true, want false (2FA not enabled)")
		}
	})

	t.Run("two factor required returns pending session token", func(t *testing.T) {
		user := seedGraphQLUser(t, ddb, "tfauser", "tfauser@example.com", "correctpass1")
		secret := "JBSWY3DPEHPK3PXP"
		if _, err := ddb.Users.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = ? WHERE id = ?`, secret, user.ID); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}

		result, err := m.LoginUser(context.Background(), "tfauser", "correctpass1", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || !result.RequiresTwoFactor {
			t.Fatal("expected RequiresTwoFactor = true")
		}
		if result.SessionToken == nil || *result.SessionToken == "" {
			t.Fatal("expected a non-empty pending SessionToken")
		}

		t.Run("completing with valid totp code logs in", func(t *testing.T) {
			code, err := totp.GenerateCode(secret, time.Now())
			if err != nil {
				t.Fatalf("generate totp code: %v", err)
			}
			loginResult, err := m.LoginUser(context.Background(), "tfauser", "correctpass1", &code, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if loginResult == nil || loginResult.RequiresTwoFactor {
				t.Fatal("expected a full login result with RequiresTwoFactor = false")
			}
			if loginResult.Token == nil || *loginResult.Token == "" {
				t.Fatal("expected a non-empty session token")
			}
		})
	})
}

// --- CompleteUserTwoFactor ----------------------------------------------------

func TestMutationResolver_CompleteUserTwoFactor(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	secret := "JBSWY3DPEHPK3PXP"
	user := seedGraphQLUser(t, ddb, "twofauser", "twofauser@example.com", "correctpass1")
	if _, err := ddb.Users.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = ? WHERE id = ?`, secret, user.ID); err != nil {
		t.Fatalf("enable 2fa: %v", err)
	}

	t.Run("invalid session token", func(t *testing.T) {
		_, err := m.CompleteUserTwoFactor(context.Background(), "bogus-token", "123456")
		if err == nil || err.Error() != "invalid session token" {
			t.Fatalf("err = %v, want %q", err, "invalid session token")
		}
	})

	t.Run("invalid two-factor code", func(t *testing.T) {
		pendingToken := seedPendingTwoFactorSession(t, ddb, user.ID)
		_, err := m.CompleteUserTwoFactor(context.Background(), pendingToken, "000000")
		if err == nil || err.Error() != "invalid two-factor code" {
			t.Fatalf("err = %v, want %q", err, "invalid two-factor code")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		pendingToken := seedPendingTwoFactorSession(t, ddb, user.ID)
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp code: %v", err)
		}

		result, err := m.CompleteUserTwoFactor(context.Background(), pendingToken, code)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.Token == nil || *result.Token == "" {
			t.Fatal("expected a non-empty session token")
		}

		t.Run("reusing the consumed pending session fails", func(t *testing.T) {
			_, err := m.CompleteUserTwoFactor(context.Background(), pendingToken, code)
			if err == nil || err.Error() != "invalid session token" {
				t.Fatalf("err = %v, want %q (pending session should have been deleted)", err, "invalid session token")
			}
		})
	})
}

// --- UseUserRecoveryKey -------------------------------------------------------

func TestMutationResolver_UseUserRecoveryKey(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "recoveryuser", "recoveryuser@example.com", "correctpass1")
	keys, err := (&models.RecoveryKeyModel{DB: ddb.Users}).GenerateRecoveryKeys(int(user.ID))
	if err != nil {
		t.Fatalf("generate recovery keys: %v", err)
	}

	t.Run("invalid session token", func(t *testing.T) {
		_, err := m.UseUserRecoveryKey(context.Background(), "bogus-token", keys[0])
		if err == nil || err.Error() != "invalid session token" {
			t.Fatalf("err = %v, want %q", err, "invalid session token")
		}
	})

	t.Run("invalid recovery key", func(t *testing.T) {
		pendingToken := seedPendingTwoFactorSession(t, ddb, user.ID)
		_, err := m.UseUserRecoveryKey(context.Background(), pendingToken, "0000-0000")
		if err == nil || err.Error() != "invalid recovery key" {
			t.Fatalf("err = %v, want %q", err, "invalid recovery key")
		}
	})

	t.Run("happy path consumes key and reports remaining", func(t *testing.T) {
		pendingToken := seedPendingTwoFactorSession(t, ddb, user.ID)
		result, err := m.UseUserRecoveryKey(context.Background(), pendingToken, keys[0])
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.Token == nil || *result.Token == "" {
			t.Fatal("expected a non-empty session token")
		}
		if result.RemainingKeys == nil || *result.RemainingKeys != 9 {
			t.Fatalf("RemainingKeys = %v, want 9", result.RemainingKeys)
		}

		t.Run("reusing the same recovery key fails (idempotency)", func(t *testing.T) {
			pendingToken2 := seedPendingTwoFactorSession(t, ddb, user.ID)
			_, err := m.UseUserRecoveryKey(context.Background(), pendingToken2, keys[0])
			if err == nil || err.Error() != "invalid recovery key" {
				t.Fatalf("err = %v, want %q (key already used)", err, "invalid recovery key")
			}
		})
	})
}

// --- RequestPasswordReset -----------------------------------------------------

func TestMutationResolver_RequestPasswordReset(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("invalid email format", func(t *testing.T) {
		_, err := m.RequestPasswordReset(context.Background(), "not-an-email")
		if err == nil || err.Error() != "invalid email format" {
			t.Fatalf("err = %v, want %q", err, "invalid email format")
		}
	})

	t.Run("valid email returns success synchronously regardless of account existing", func(t *testing.T) {
		result, err := m.RequestPasswordReset(context.Background(), "nosuchaccount@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || !result.Success {
			t.Fatal("expected Success = true (response never reveals account existence)")
		}
	})

	t.Run("valid email for existing account queues a reset row asynchronously", func(t *testing.T) {
		user := seedGraphQLUser(t, ddb, "resetuser", "resetuser@example.com", "correctpass1")

		result, err := m.RequestPasswordReset(context.Background(), "resetuser@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || !result.Success {
			t.Fatal("expected Success = true")
		}

		// The DB insert happens in a detached goroutine; poll briefly with a
		// bounded deadline instead of sleeping a fixed amount.
		deadline := time.Now().Add(200 * time.Millisecond)
		var count int
		for time.Now().Before(deadline) {
			if err := ddb.Users.QueryRow(`SELECT COUNT(*) FROM user_password_resets WHERE user_id = ?`, user.ID).Scan(&count); err != nil {
				t.Fatalf("query reset rows: %v", err)
			}
			if count > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if count == 0 {
			t.Fatal("expected a user_password_resets row to be created asynchronously within 200ms")
		}
	})
}

// --- ResetUserPassword --------------------------------------------------------

func TestMutationResolver_ResetUserPassword(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("short password", func(t *testing.T) {
		_, err := m.ResetUserPassword(context.Background(), "sometoken", "short")
		if err == nil || err.Error() != "invalid request format" {
			t.Fatalf("err = %v, want %q", err, "invalid request format")
		}
	})

	t.Run("invalid or expired reset token", func(t *testing.T) {
		_, err := m.ResetUserPassword(context.Background(), "bogus-token", "newpassword1")
		if err == nil || err.Error() != "invalid or expired reset token" {
			t.Fatalf("err = %v, want %q", err, "invalid or expired reset token")
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		user := seedGraphQLUser(t, ddb, "expreset", "expreset@example.com", "correctpass1")
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_password_resets (user_id, token, ip_address, created_at, expires_at)
			VALUES (?, 'expired-tok', '127.0.0.1', ?, ?)
		`, user.ID, time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour)); err != nil {
			t.Fatalf("seed expired reset: %v", err)
		}

		_, err := m.ResetUserPassword(context.Background(), "expired-tok", "newpassword1")
		if err == nil || err.Error() != "invalid or expired reset token" {
			t.Fatalf("err = %v, want %q", err, "invalid or expired reset token")
		}
	})

	t.Run("happy path resets password and invalidates sessions", func(t *testing.T) {
		user := seedGraphQLUser(t, ddb, "resetok", "resetok@example.com", "correctpass1")
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_password_resets (user_id, token, ip_address, created_at, expires_at)
			VALUES (?, 'valid-tok', '127.0.0.1', ?, ?)
		`, user.ID, time.Now(), time.Now().Add(1*time.Hour)); err != nil {
			t.Fatalf("seed valid reset: %v", err)
		}
		session, err := (&models.SessionModel{DB: ddb.Users}).Create(user.ID, 900)
		if err != nil {
			t.Fatalf("seed session: %v", err)
		}

		result, err := m.ResetUserPassword(context.Background(), "valid-tok", "newpassword1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || !result.Success {
			t.Fatal("expected Success = true")
		}

		if _, err := (&models.SessionModel{DB: ddb.Users}).GetByID(session.ID); err == nil {
			t.Fatal("expected prior session to be invalidated after password reset")
		}

		t.Run("reusing the consumed token fails (idempotency)", func(t *testing.T) {
			_, err := m.ResetUserPassword(context.Background(), "valid-tok", "anotherpass1")
			if err == nil || err.Error() != "invalid or expired reset token" {
				t.Fatalf("err = %v, want %q (token already consumed)", err, "invalid or expired reset token")
			}
		})
	})
}

// --- VerifyUserEmail -----------------------------------------------------------

func TestMutationResolver_VerifyUserEmail(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("invalid or expired verification token", func(t *testing.T) {
		_, err := m.VerifyUserEmail(context.Background(), "bogus-token")
		if err == nil || err.Error() != "invalid or expired verification token" {
			t.Fatalf("err = %v, want %q", err, "invalid or expired verification token")
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		user := seedGraphQLUser(t, ddb, "expverify", "expverify@example.com", "correctpass1")
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_email_verifications (user_id, email, token, created_at, expires_at)
			VALUES (?, ?, 'expired-verify-tok', ?, ?)
		`, user.ID, user.Email, time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour)); err != nil {
			t.Fatalf("seed expired verification: %v", err)
		}

		_, err := m.VerifyUserEmail(context.Background(), "expired-verify-tok")
		if err == nil || err.Error() != "invalid or expired verification token" {
			t.Fatalf("err = %v, want %q", err, "invalid or expired verification token")
		}
	})

	t.Run("happy path marks email verified", func(t *testing.T) {
		user := seedGraphQLUser(t, ddb, "verifyok", "verifyok@example.com", "correctpass1")
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_email_verifications (user_id, email, token, created_at, expires_at)
			VALUES (?, ?, 'valid-verify-tok', ?, ?)
		`, user.ID, user.Email, time.Now(), time.Now().Add(1*time.Hour)); err != nil {
			t.Fatalf("seed valid verification: %v", err)
		}

		result, err := m.VerifyUserEmail(context.Background(), "valid-verify-tok")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || !result.Success {
			t.Fatal("expected Success = true")
		}

		var verified bool
		if err := ddb.Users.QueryRow(`SELECT email_verified FROM user_accounts WHERE id = ?`, user.ID).Scan(&verified); err != nil {
			t.Fatalf("query verified flag: %v", err)
		}
		if !verified {
			t.Fatal("expected email_verified = 1 after successful verification")
		}

		t.Run("reusing the consumed token fails (idempotency)", func(t *testing.T) {
			_, err := m.VerifyUserEmail(context.Background(), "valid-verify-tok")
			if err == nil || err.Error() != "invalid or expired verification token" {
				t.Fatalf("err = %v, want %q (token already consumed and deleted)", err, "invalid or expired verification token")
			}
		})
	})
}

// --- CompleteUserInvite --------------------------------------------------------

func TestMutationResolver_CompleteUserInvite(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}
	inviteModel := &models.UserInviteModel{DB: ddb.Users}

	t.Run("empty token", func(t *testing.T) {
		_, err := m.CompleteUserInvite(context.Background(), "  ", "someuser", "password123")
		if err == nil || err.Error() != "token required" {
			t.Fatalf("err = %v, want %q", err, "token required")
		}
	})

	t.Run("invalid invite token", func(t *testing.T) {
		_, err := m.CompleteUserInvite(context.Background(), "bogus-invite-tok", "someuser", "password123")
		if err == nil {
			t.Fatal("expected an error for a nonexistent invite token")
		}
	})

	t.Run("invite username does not match", func(t *testing.T) {
		invite, err := inviteModel.CreateInvite("expecteduser", "invitee1@example.com", "user", 7)
		if err != nil {
			t.Fatalf("seed invite: %v", err)
		}

		_, err = m.CompleteUserInvite(context.Background(), invite.Token, "wronguser", "password123")
		if err == nil || err.Error() != "invite username does not match" {
			t.Fatalf("err = %v, want %q", err, "invite username does not match")
		}
	})

	t.Run("invite missing email address", func(t *testing.T) {
		invite, err := inviteModel.CreateInvite("", "", "user", 7)
		if err != nil {
			t.Fatalf("seed invite: %v", err)
		}

		_, err = m.CompleteUserInvite(context.Background(), invite.Token, "anyusername", "password123")
		if err == nil || err.Error() != "invite is missing an email address" {
			t.Fatalf("err = %v, want %q", err, "invite is missing an email address")
		}
	})

	t.Run("happy path creates account and marks invite used", func(t *testing.T) {
		invite, err := inviteModel.CreateInvite("", "invitee2@example.com", "user", 7)
		if err != nil {
			t.Fatalf("seed invite: %v", err)
		}

		result, err := m.CompleteUserInvite(context.Background(), invite.Token, "inviteduser", "password123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.User == nil {
			t.Fatal("expected UserInviteCompletion with User set")
		}
		if result.User.Username != "inviteduser" {
			t.Fatalf("Username = %q, want %q", result.User.Username, "inviteduser")
		}
		if result.Token == nil || *result.Token == "" {
			t.Fatal("expected a non-empty session token")
		}

		t.Run("reusing the consumed invite token fails (idempotency)", func(t *testing.T) {
			_, err := m.CompleteUserInvite(context.Background(), invite.Token, "seconduser", "password123")
			if err == nil {
				t.Fatal("expected an error reusing an already-used invite token")
			}
		})
	})
}

// --- CompleteServerInvite -------------------------------------------------------

func TestMutationResolver_CompleteServerInvite(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{ServerDB: ddb.Server}}

	// server_admin_invites.invited_by is FK-enforced against
	// server_admin_credentials(id), so a real inviter admin must exist first.
	inviter, err := (&models.AdminModel{DB: ddb.Server}).Create("inviteradmin", "inviter@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed inviter admin: %v", err)
	}
	inviteModel := &models.AdminInviteModel{DB: ddb.Server}

	t.Run("empty token", func(t *testing.T) {
		_, err := m.CompleteServerInvite(context.Background(), "  ", "newadmin", "password123")
		if err == nil || err.Error() != "token required" {
			t.Fatalf("err = %v, want %q", err, "token required")
		}
	})

	t.Run("invalid invite token", func(t *testing.T) {
		_, err := m.CompleteServerInvite(context.Background(), "bogus-admin-invite", "newadmin", "password123")
		if err == nil {
			t.Fatal("expected an error for a nonexistent invite token")
		}
	})

	t.Run("expired invite token", func(t *testing.T) {
		invite, err := inviteModel.CreateInvite("expiredadmin@example.com", inviter.ID, 1*time.Millisecond)
		if err != nil {
			t.Fatalf("seed expired invite: %v", err)
		}
		time.Sleep(5 * time.Millisecond)

		_, err = m.CompleteServerInvite(context.Background(), invite.Token, "expiredadminuser", "password123")
		if err == nil || err.Error() != "invite token has expired" {
			t.Fatalf("err = %v, want %q", err, "invite token has expired")
		}
	})

	t.Run("happy path creates admin account", func(t *testing.T) {
		invite, err := inviteModel.CreateInvite("newadmin@example.com", inviter.ID, 24*time.Hour)
		if err != nil {
			t.Fatalf("seed invite: %v", err)
		}

		result, err := m.CompleteServerInvite(context.Background(), invite.Token, "newlyinvited", "password123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.Admin == nil {
			t.Fatal("expected ServerInviteCompletion with Admin set")
		}
		if result.Admin.Username != "newlyinvited" {
			t.Fatalf("Username = %q, want %q", result.Admin.Username, "newlyinvited")
		}

		t.Run("reusing the consumed invite token fails (idempotency)", func(t *testing.T) {
			_, err := m.CompleteServerInvite(context.Background(), invite.Token, "secondadmin", "password123")
			if err == nil || err.Error() != "invite token has already been used" {
				t.Fatalf("err = %v, want %q", err, "invite token has already been used")
			}
		})
	})
}

// --- UploadUserAvatar ------------------------------------------------------------

func TestMutationResolver_UploadUserAvatar(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}
	user := seedGraphQLUser(t, ddb, "avataruser", "avataruser@example.com", "correctpass1")

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := m.UploadUserAvatar(context.Background(), graphql.Upload{Filename: "a.png", Size: 10, ContentType: "image/png"})
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	authedCtx := context.WithValue(context.Background(), "user_id", int(user.ID))

	t.Run("file too large", func(t *testing.T) {
		_, err := m.UploadUserAvatar(authedCtx, graphql.Upload{Filename: "big.png", Size: 3 * 1024 * 1024, ContentType: "image/png"})
		if err == nil || err.Error() != "file too large (max 2MB)" {
			t.Fatalf("err = %v, want %q", err, "file too large (max 2MB)")
		}
	})

	t.Run("invalid image type", func(t *testing.T) {
		_, err := m.UploadUserAvatar(authedCtx, graphql.Upload{Filename: "doc.pdf", Size: 100, ContentType: "application/pdf"})
		if err == nil || err.Error() != "invalid image type" {
			t.Fatalf("err = %v, want %q", err, "invalid image type")
		}
	})

	t.Run("happy path uploads avatar", func(t *testing.T) {
		avatar, err := m.UploadUserAvatar(authedCtx, graphql.Upload{Filename: "me.png", Size: 100, ContentType: "image/png"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if avatar == nil || avatar.Type != "upload" {
			t.Fatalf("avatar = %+v, want Type = %q", avatar, "upload")
		}
	})
}

// --- PublicUserProfile (query) ----------------------------------------------------

func TestQueryResolver_PublicUserProfile(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	owner := seedGraphQLUser(t, ddb, "profileowner", "profileowner@example.com", "correctpass1")
	seedGraphQLUser(t, ddb, "otherviewer", "otherviewer@example.com", "correctpass1")

	t.Run("unknown username returns nil, nil", func(t *testing.T) {
		profile, err := q.PublicUserProfile(context.Background(), "nosuchusername")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile != nil {
			t.Fatalf("profile = %+v, want nil", profile)
		}
	})

	t.Run("public profile visible to anyone including unauthenticated", func(t *testing.T) {
		profile, err := q.PublicUserProfile(context.Background(), "profileowner")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile == nil || profile.Username != "profileowner" {
			t.Fatalf("profile = %+v, want Username = %q", profile, "profileowner")
		}
	})

	t.Run("private profile hidden from non-owner returns nil, nil", func(t *testing.T) {
		if _, err := ddb.Users.Exec(`UPDATE user_accounts SET visibility = 'private' WHERE id = ?`, owner.ID); err != nil {
			t.Fatalf("set private visibility: %v", err)
		}

		anonProfile, err := q.PublicUserProfile(context.Background(), "profileowner")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if anonProfile != nil {
			t.Fatalf("profile = %+v, want nil for unauthenticated viewer of a private profile", anonProfile)
		}

		otherCtx := context.WithValue(context.Background(), "user_id", int(seedGraphQLUser(t, ddb, "anothersnooper", "anothersnooper@example.com", "correctpass1").ID))
		otherProfile, err := q.PublicUserProfile(otherCtx, "profileowner")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if otherProfile != nil {
			t.Fatalf("profile = %+v, want nil for a different logged-in viewer of a private profile", otherProfile)
		}

		ownerCtx := context.WithValue(context.Background(), "user_id", int(owner.ID))
		ownProfile, err := q.PublicUserProfile(ownerCtx, "profileowner")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ownProfile == nil || ownProfile.Username != "profileowner" {
			t.Fatalf("profile = %+v, want the owner to see their own private profile", ownProfile)
		}
	})
}
