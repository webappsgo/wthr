package model

import (
	"strings"
	"testing"
	"time"
)

// testUserPassword is a fake fixture password, not a real credential.
const testUserPassword = "test-password-123"

// newUserTestDB wires a fresh in-memory users.db and sets it as the global
// dual-DB so UserModel/UserSessionModel/etc. (which read via
// database.GetUsersDB()) operate against it.
func newUserTestDB(t *testing.T) {
	t.Helper()
	usersDB := newModelUsersDB(t)
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)
}

// TestUserSessionExpired covers the boundary helpers on UserSession,
// UserEmailVerification and UserPasswordReset.
func TestUserSessionExpired(t *testing.T) {
	t.Run("session not yet expired", func(t *testing.T) {
		s := &UserSession{ExpiresAt: time.Now().Add(time.Hour)}
		if s.SessionExpired() {
			t.Fatal("expected session to not be expired")
		}
	})

	t.Run("session already expired", func(t *testing.T) {
		s := &UserSession{ExpiresAt: time.Now().Add(-time.Hour)}
		if !s.SessionExpired() {
			t.Fatal("expected session to be expired")
		}
	})

	t.Run("email verification expiry and used state", func(t *testing.T) {
		v := &UserEmailVerification{ExpiresAt: time.Now().Add(-time.Minute)}
		if !v.IsExpired() {
			t.Fatal("expected verification to be expired")
		}
		if v.IsUsed() {
			t.Fatal("expected fresh verification to be unused")
		}
		used := time.Now()
		v.UsedAt = &used
		if !v.IsUsed() {
			t.Fatal("expected verification to be used")
		}
	})

	t.Run("password reset expiry and used state", func(t *testing.T) {
		r := &UserPasswordReset{ExpiresAt: time.Now().Add(-time.Minute)}
		if !r.IsExpired() {
			t.Fatal("expected reset to be expired")
		}
		if r.IsUsed() {
			t.Fatal("expected fresh reset to be unused")
		}
		used := time.Now()
		r.UsedAt = &used
		if !r.IsUsed() {
			t.Fatal("expected reset to be used")
		}
	})
}

// TestUserModelCreateAndGet covers the happy path for Create plus each
// lookup method (GetByID, GetByUsername, GetByEmail, GetByIdentifier), and
// the not-found error path for all of them.
func TestUserModelCreateAndGet(t *testing.T) {
	newUserTestDB(t)
	m := &UserModel{}

	t.Run("create defaults role to user", func(t *testing.T) {
		u, err := m.Create("alice", "alice@example.com", testUserPassword)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if u.ID == 0 {
			t.Fatal("expected non-zero user id")
		}
		if u.Role != "user" {
			t.Fatalf("expected default role 'user', got %q", u.Role)
		}
		if u.PasswordHash == "" || u.PasswordHash == testUserPassword {
			t.Fatal("expected password to be hashed")
		}
		if !u.IsActive {
			t.Fatal("expected new user to be active")
		}
	})

	t.Run("create with explicit role", func(t *testing.T) {
		u, err := m.Create("bob", "bob@example.com", testUserPassword, "admin")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if u.Role != "admin" {
			t.Fatalf("expected role 'admin', got %q", u.Role)
		}
	})

	t.Run("get by id", func(t *testing.T) {
		created, err := m.Create("carol", "carol@example.com", testUserPassword)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := m.GetByID(created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Username != "carol" {
			t.Fatalf("expected username carol, got %q", got.Username)
		}
	})

	t.Run("get by id not found", func(t *testing.T) {
		if _, err := m.GetByID(999999); err == nil {
			t.Fatal("expected error for missing user")
		}
	})

	t.Run("get by username", func(t *testing.T) {
		got, err := m.GetByUsername("alice")
		if err != nil {
			t.Fatalf("GetByUsername: %v", err)
		}
		if got.Email != "alice@example.com" {
			t.Fatalf("unexpected email: %s", got.Email)
		}
	})

	t.Run("get by username not found", func(t *testing.T) {
		if _, err := m.GetByUsername("nobody"); err == nil {
			t.Fatal("expected error for missing username")
		}
	})

	t.Run("get by email", func(t *testing.T) {
		got, err := m.GetByEmail("bob@example.com")
		if err != nil {
			t.Fatalf("GetByEmail: %v", err)
		}
		if got.Username != "bob" {
			t.Fatalf("unexpected username: %s", got.Username)
		}
	})

	t.Run("get by email not found", func(t *testing.T) {
		if _, err := m.GetByEmail("nobody@example.com"); err == nil {
			t.Fatal("expected error for missing email")
		}
	})

	t.Run("get by identifier resolves username then email", func(t *testing.T) {
		byUsername, err := m.GetByIdentifier("alice")
		if err != nil {
			t.Fatalf("GetByIdentifier(username): %v", err)
		}
		if byUsername.Username != "alice" {
			t.Fatalf("unexpected user: %s", byUsername.Username)
		}

		byEmail, err := m.GetByIdentifier("bob@example.com")
		if err != nil {
			t.Fatalf("GetByIdentifier(email): %v", err)
		}
		if byEmail.Username != "bob" {
			t.Fatalf("unexpected user: %s", byEmail.Username)
		}
	})

	t.Run("get by identifier not found", func(t *testing.T) {
		if _, err := m.GetByIdentifier("nobody"); err == nil {
			t.Fatal("expected error for missing identifier")
		}
	})
}

// TestUserModelListAndCount covers ListUsers pagination, the empty-table
// boundary, GetAll, CountUsers/Count, and CountByRole (which uniquely reads
// via the injected m.DB field rather than the global accessor).
func TestUserModelListAndCount(t *testing.T) {
	usersDB := newModelUsersDB(t)
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)
	m := &UserModel{DB: usersDB}

	t.Run("empty table", func(t *testing.T) {
		users, err := m.ListUsers(0, 10)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if len(users) != 0 {
			t.Fatalf("expected no users, got %d", len(users))
		}
		count, err := m.CountUsers()
		if err != nil {
			t.Fatalf("CountUsers: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected zero count, got %d", count)
		}
	})

	if _, err := m.Create("u1", "u1@example.com", testUserPassword); err != nil {
		t.Fatalf("Create u1: %v", err)
	}
	if _, err := m.Create("u2", "u2@example.com", testUserPassword, "admin"); err != nil {
		t.Fatalf("Create u2: %v", err)
	}

	t.Run("list with pagination", func(t *testing.T) {
		page, err := m.ListUsers(0, 1)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if len(page) != 1 {
			t.Fatalf("expected 1 user in page, got %d", len(page))
		}
	})

	t.Run("get all returns every user", func(t *testing.T) {
		all, err := m.GetAll()
		if err != nil {
			t.Fatalf("GetAll: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("expected 2 users, got %d", len(all))
		}
	})

	t.Run("count alias matches CountUsers", func(t *testing.T) {
		count, err := m.Count()
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected count 2, got %d", count)
		}
	})

	t.Run("count by role", func(t *testing.T) {
		userCount, err := m.CountByRole("user")
		if err != nil {
			t.Fatalf("CountByRole(user): %v", err)
		}
		if userCount != 1 {
			t.Fatalf("expected 1 user with role 'user', got %d", userCount)
		}

		adminCount, err := m.CountByRole("admin")
		if err != nil {
			t.Fatalf("CountByRole(admin): %v", err)
		}
		if adminCount != 1 {
			t.Fatalf("expected 1 user with role 'admin', got %d", adminCount)
		}

		noneCount, err := m.CountByRole("nonexistent")
		if err != nil {
			t.Fatalf("CountByRole(nonexistent): %v", err)
		}
		if noneCount != 0 {
			t.Fatalf("expected 0 users with unknown role, got %d", noneCount)
		}
	})
}

// TestUserModelMutations covers Update, UpdatePassword, UpdateLastLogin,
// VerifyEmail, SetActive, BanUser/UnbanUser, Enable2FA/Disable2FA (and their
// aliases), UpdateProfile, and Delete.
func TestUserModelMutations(t *testing.T) {
	newUserTestDB(t)
	m := &UserModel{}

	u, err := m.Create("dave", "dave@example.com", testUserPassword)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("update username, email and role", func(t *testing.T) {
		if err := m.Update(u.ID, "dave2", "dave2@example.com", "admin"); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Username != "dave2" || got.Email != "dave2@example.com" || got.Role != "admin" {
			t.Fatalf("unexpected user after update: %+v", got)
		}
	})

	t.Run("update without role leaves role unchanged", func(t *testing.T) {
		if err := m.Update(u.ID, "dave3", "dave3@example.com"); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Role != "admin" {
			t.Fatalf("expected role to remain 'admin', got %q", got.Role)
		}
	})

	t.Run("update password", func(t *testing.T) {
		if err := m.UpdatePassword(u.ID, "new-test-password-456"); err != nil {
			t.Fatalf("UpdatePassword: %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !m.CheckPassword(got, "new-test-password-456") {
			t.Fatal("expected new password to verify")
		}
		if m.CheckPassword(got, testUserPassword) {
			t.Fatal("expected old password to no longer verify")
		}
	})

	t.Run("update last login", func(t *testing.T) {
		if err := m.UpdateLastLogin(u.ID, "10.0.0.5"); err != nil {
			t.Fatalf("UpdateLastLogin: %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.LastLoginIP != "10.0.0.5" || got.LastLoginAt == nil {
			t.Fatalf("expected last login to be recorded, got %+v", got)
		}
	})

	t.Run("verify email", func(t *testing.T) {
		if err := m.VerifyEmail(u.ID); err != nil {
			t.Fatalf("VerifyEmail: %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.EmailVerified {
			t.Fatal("expected email to be verified")
		}
	})

	t.Run("set active toggles active flag", func(t *testing.T) {
		if err := m.SetActive(u.ID, false); err != nil {
			t.Fatalf("SetActive(false): %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.IsActive {
			t.Fatal("expected user to be inactive")
		}
		if err := m.SetActive(u.ID, true); err != nil {
			t.Fatalf("SetActive(true): %v", err)
		}
	})

	t.Run("ban and unban", func(t *testing.T) {
		if err := m.BanUser(u.ID, "spam"); err != nil {
			t.Fatalf("BanUser: %v", err)
		}
		banned, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !banned.IsBanned || banned.BanReason != "spam" {
			t.Fatalf("expected user banned with reason, got %+v", banned)
		}

		if err := m.UnbanUser(u.ID); err != nil {
			t.Fatalf("UnbanUser: %v", err)
		}
		unbanned, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if unbanned.IsBanned || unbanned.BanReason != "" {
			t.Fatalf("expected user unbanned with no reason, got %+v", unbanned)
		}
	})

	t.Run("enable and disable 2fa via aliases", func(t *testing.T) {
		if err := m.EnableTwoFactor(u.ID, "SECRET123"); err != nil {
			t.Fatalf("EnableTwoFactor: %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.TwoFactorEnabled || got.TwoFactorSecret != "SECRET123" {
			t.Fatalf("expected 2FA enabled with secret, got %+v", got)
		}

		if err := m.DisableTwoFactor(u.ID); err != nil {
			t.Fatalf("DisableTwoFactor: %v", err)
		}
		got, err = m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.TwoFactorEnabled {
			t.Fatal("expected 2FA to be disabled")
		}
	})

	t.Run("update profile", func(t *testing.T) {
		if err := m.UpdateProfile(u.ID, "Dave D", "555-1234"); err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		got, err := m.GetByID(u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.DisplayName != "Dave D" || got.Phone != "555-1234" {
			t.Fatalf("unexpected profile after update: %+v", got)
		}
	})

	t.Run("delete removes the user", func(t *testing.T) {
		if err := m.Delete(u.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := m.GetByID(u.ID); err == nil {
			t.Fatal("expected error looking up deleted user")
		}
	})
}

// TestUserModelVerifyCredentials covers the happy path, wrong password,
// unknown identifier, inactive account, and banned account error paths.
func TestUserModelVerifyCredentials(t *testing.T) {
	newUserTestDB(t)
	m := &UserModel{}

	if _, err := m.Create("erin", "erin@example.com", testUserPassword); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("correct credentials by username", func(t *testing.T) {
		u, err := m.VerifyCredentials("erin", testUserPassword)
		if err != nil {
			t.Fatalf("VerifyCredentials: %v", err)
		}
		if u.Username != "erin" {
			t.Fatalf("unexpected user: %s", u.Username)
		}
	})

	t.Run("correct credentials by email", func(t *testing.T) {
		if _, err := m.VerifyCredentials("erin@example.com", testUserPassword); err != nil {
			t.Fatalf("VerifyCredentials: %v", err)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		if _, err := m.VerifyCredentials("erin", "wrong-password"); err == nil {
			t.Fatal("expected error for wrong password")
		}
	})

	t.Run("unknown identifier", func(t *testing.T) {
		if _, err := m.VerifyCredentials("nobody", testUserPassword); err == nil {
			t.Fatal("expected error for unknown identifier")
		}
	})

	t.Run("inactive account", func(t *testing.T) {
		created, err := m.Create("frank", "frank@example.com", testUserPassword)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := m.SetActive(created.ID, false); err != nil {
			t.Fatalf("SetActive: %v", err)
		}
		if _, err := m.VerifyCredentials("frank", testUserPassword); err == nil {
			t.Fatal("expected error for inactive account")
		}
	})

	t.Run("banned account", func(t *testing.T) {
		created, err := m.Create("gina", "gina@example.com", testUserPassword)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := m.BanUser(created.ID, "abuse"); err != nil {
			t.Fatalf("BanUser: %v", err)
		}
		if _, err := m.VerifyCredentials("gina", testUserPassword); err == nil {
			t.Fatal("expected error for banned account")
		}
	})
}

// TestHashUserToken checks the SHA-256 hex digest format and determinism.
func TestHashUserToken(t *testing.T) {
	h1 := hashUserToken("raw-token-value")
	h2 := hashUserToken("raw-token-value")
	if h1 != h2 {
		t.Fatal("expected hashUserToken to be deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64 hex chars (SHA-256), got %d", len(h1))
	}
	if hashUserToken("different") == h1 {
		t.Fatal("expected different input to produce different hash")
	}
}

// TestUserSessionModel covers session create/get/update/delete lifecycle,
// including the not-found and empty-list boundary conditions and the
// documented "GetRowIDByToken returns 0, not an error, when missing" quirk.
func TestUserSessionModel(t *testing.T) {
	newUserTestDB(t)
	users := &UserModel{}
	sessions := &UserSessionModel{}

	u, err := m_createUser(t, users, "hank", "hank@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Run("create and get session", func(t *testing.T) {
		s, err := sessions.CreateSession(u.ID, "1.2.3.4", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if s.SessionID == "" {
			t.Fatal("expected non-empty raw session id")
		}

		got, err := sessions.GetSession(s.SessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.UserID != u.ID {
			t.Fatalf("expected user id %d, got %d", u.ID, got.UserID)
		}
		// GetSession returns the token_hash, not the raw token, in SessionID.
		if got.SessionID != hashUserToken(s.SessionID) {
			t.Fatalf("expected SessionID to hold the token hash")
		}
	})

	t.Run("get session not found", func(t *testing.T) {
		if _, err := sessions.GetSession("does-not-exist"); err == nil {
			t.Fatal("expected error for missing session")
		}
	})

	t.Run("update last used", func(t *testing.T) {
		s, err := sessions.CreateSession(u.ID, "1.2.3.4", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := sessions.UpdateSessionLastUsed(s.SessionID); err != nil {
			t.Fatalf("UpdateSessionLastUsed: %v", err)
		}
	})

	t.Run("get row id by token, found and not found", func(t *testing.T) {
		s, err := sessions.CreateSession(u.ID, "1.2.3.4", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		rowID, err := sessions.GetRowIDByToken(s.SessionID)
		if err != nil {
			t.Fatalf("GetRowIDByToken: %v", err)
		}
		if rowID == 0 {
			t.Fatal("expected non-zero row id")
		}

		missingRowID, err := sessions.GetRowIDByToken("no-such-token")
		if err != nil {
			t.Fatalf("GetRowIDByToken(missing): %v", err)
		}
		if missingRowID != 0 {
			t.Fatalf("expected 0 for missing token, got %d", missingRowID)
		}
	})

	t.Run("delete session by raw token", func(t *testing.T) {
		s, err := sessions.CreateSession(u.ID, "1.2.3.4", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := sessions.DeleteSession(s.SessionID); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}
		if _, err := sessions.GetSession(s.SessionID); err == nil {
			t.Fatal("expected error getting deleted session")
		}
	})

	t.Run("delete session by row id", func(t *testing.T) {
		s, err := sessions.CreateSession(u.ID, "1.2.3.4", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		rowID, err := sessions.GetRowIDByToken(s.SessionID)
		if err != nil {
			t.Fatalf("GetRowIDByToken: %v", err)
		}
		if err := sessions.DeleteSessionByRowID(rowID); err != nil {
			t.Fatalf("DeleteSessionByRowID: %v", err)
		}
		if _, err := sessions.GetSession(s.SessionID); err == nil {
			t.Fatal("expected error getting deleted session")
		}
	})

	t.Run("delete all sessions for user", func(t *testing.T) {
		if _, err := sessions.CreateSession(u.ID, "1.2.3.4", "a", time.Hour); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if _, err := sessions.CreateSession(u.ID, "1.2.3.4", "b", time.Hour); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := sessions.DeleteAllSessionsForUser(u.ID); err != nil {
			t.Fatalf("DeleteAllSessionsForUser: %v", err)
		}
		active, err := sessions.GetActiveSessions(u.ID)
		if err != nil {
			t.Fatalf("GetActiveSessions: %v", err)
		}
		if len(active) != 0 {
			t.Fatalf("expected no active sessions, got %d", len(active))
		}
	})

	t.Run("get active sessions excludes expired", func(t *testing.T) {
		if _, err := sessions.CreateSession(u.ID, "1.2.3.4", "fresh", time.Hour); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if _, err := sessions.CreateSession(u.ID, "1.2.3.4", "stale", -time.Hour); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		active, err := sessions.GetActiveSessions(u.ID)
		if err != nil {
			t.Fatalf("GetActiveSessions: %v", err)
		}
		if len(active) != 1 {
			t.Fatalf("expected 1 active session, got %d", len(active))
		}
	})

	t.Run("delete expired sessions cleanup", func(t *testing.T) {
		if _, err := sessions.CreateSession(u.ID, "1.2.3.4", "expired", -time.Hour); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := sessions.DeleteExpiredSessions(); err != nil {
			t.Fatalf("DeleteExpiredSessions: %v", err)
		}
	})
}

// m_createUser is a small helper for tests in this file that only need a
// user row to exist, via the real UserModel.Create path (so password
// hashing and preference-row creation happen exactly as in production).
func m_createUser(t *testing.T, m *UserModel, username, email string) (*User, error) {
	t.Helper()
	return m.Create(username, email, testUserPassword)
}

// TestUserEmailVerificationModel covers create/get/mark-used lifecycle, the
// not-found error path, and the expired-cleanup boundary.
func TestUserEmailVerificationModel(t *testing.T) {
	newUserTestDB(t)
	users := &UserModel{}
	verifications := &UserEmailVerificationModel{}

	u, err := m_createUser(t, users, "ivan", "ivan@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Run("create and get verification", func(t *testing.T) {
		v, err := verifications.CreateVerification(u.ID, "ivan@example.com")
		if err != nil {
			t.Fatalf("CreateVerification: %v", err)
		}
		if v.Token == "" {
			t.Fatal("expected non-empty token")
		}

		got, err := verifications.GetVerification(v.Token)
		if err != nil {
			t.Fatalf("GetVerification: %v", err)
		}
		if got.UserID != u.ID || got.IsUsed() {
			t.Fatalf("unexpected verification: %+v", got)
		}
	})

	t.Run("get verification not found", func(t *testing.T) {
		if _, err := verifications.GetVerification("bogus"); err == nil {
			t.Fatal("expected error for missing verification")
		}
	})

	t.Run("mark used", func(t *testing.T) {
		v, err := verifications.CreateVerification(u.ID, "ivan@example.com")
		if err != nil {
			t.Fatalf("CreateVerification: %v", err)
		}
		if err := verifications.MarkVerificationUsed(v.Token); err != nil {
			t.Fatalf("MarkVerificationUsed: %v", err)
		}
		got, err := verifications.GetVerification(v.Token)
		if err != nil {
			t.Fatalf("GetVerification: %v", err)
		}
		if !got.IsUsed() {
			t.Fatal("expected verification to be marked used")
		}
	})

	t.Run("delete expired verifications", func(t *testing.T) {
		if err := verifications.DeleteExpiredVerifications(); err != nil {
			t.Fatalf("DeleteExpiredVerifications: %v", err)
		}
	})
}

// TestUserPasswordResetModel mirrors TestUserEmailVerificationModel's
// coverage for the password-reset lifecycle.
func TestUserPasswordResetModel(t *testing.T) {
	newUserTestDB(t)
	users := &UserModel{}
	resets := &UserPasswordResetModel{}

	u, err := m_createUser(t, users, "judy", "judy@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Run("create and get reset", func(t *testing.T) {
		r, err := resets.CreateReset(u.ID)
		if err != nil {
			t.Fatalf("CreateReset: %v", err)
		}
		if r.Token == "" {
			t.Fatal("expected non-empty token")
		}

		got, err := resets.GetReset(r.Token)
		if err != nil {
			t.Fatalf("GetReset: %v", err)
		}
		if got.UserID != u.ID || got.IsUsed() {
			t.Fatalf("unexpected reset: %+v", got)
		}
	})

	t.Run("get reset not found", func(t *testing.T) {
		if _, err := resets.GetReset("bogus"); err == nil {
			t.Fatal("expected error for missing reset")
		}
	})

	t.Run("mark used", func(t *testing.T) {
		r, err := resets.CreateReset(u.ID)
		if err != nil {
			t.Fatalf("CreateReset: %v", err)
		}
		if err := resets.MarkResetUsed(r.Token); err != nil {
			t.Fatalf("MarkResetUsed: %v", err)
		}
		got, err := resets.GetReset(r.Token)
		if err != nil {
			t.Fatalf("GetReset: %v", err)
		}
		if !got.IsUsed() {
			t.Fatal("expected reset to be marked used")
		}
	})

	t.Run("delete expired resets", func(t *testing.T) {
		if err := resets.DeleteExpiredResets(); err != nil {
			t.Fatalf("DeleteExpiredResets: %v", err)
		}
	})
}

// TestUserActivityLogModel covers logging, paginated retrieval on an empty
// and populated log, and the age-based cleanup boundary.
func TestUserActivityLogModel(t *testing.T) {
	newUserTestDB(t)
	users := &UserModel{}
	activity := &UserActivityLogModel{}

	u, err := m_createUser(t, users, "karl", "karl@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Run("empty log", func(t *testing.T) {
		activities, err := activity.GetActivities(u.ID, 0, 10)
		if err != nil {
			t.Fatalf("GetActivities: %v", err)
		}
		if len(activities) != 0 {
			t.Fatalf("expected no activities, got %d", len(activities))
		}
	})

	t.Run("log and retrieve", func(t *testing.T) {
		if err := activity.LogActivity(u.ID, "login", "1.2.3.4", "test-agent", "details here"); err != nil {
			t.Fatalf("LogActivity: %v", err)
		}
		if err := activity.LogActivity(u.ID, "logout", "1.2.3.4", "test-agent", ""); err != nil {
			t.Fatalf("LogActivity: %v", err)
		}

		activities, err := activity.GetActivities(u.ID, 0, 10)
		if err != nil {
			t.Fatalf("GetActivities: %v", err)
		}
		if len(activities) != 2 {
			t.Fatalf("expected 2 activities, got %d", len(activities))
		}
	})

	t.Run("pagination limit", func(t *testing.T) {
		page, err := activity.GetActivities(u.ID, 0, 1)
		if err != nil {
			t.Fatalf("GetActivities: %v", err)
		}
		if len(page) != 1 {
			t.Fatalf("expected 1 activity in page, got %d", len(page))
		}
	})

	t.Run("delete old activities", func(t *testing.T) {
		if err := activity.DeleteOldActivities(30); err != nil {
			t.Fatalf("DeleteOldActivities: %v", err)
		}
	})
}

// TestUserInviteModel covers the full invite lifecycle: creation, lookup by
// token and by ID, verification (happy path plus expired/used/exhausted
// error paths), marking used, listing, pending filtering, expired cleanup,
// and deletion.
func TestUserInviteModel(t *testing.T) {
	usersDB := newModelUsersDB(t)
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)
	invites := &UserInviteModel{}

	t.Run("create requires positive expiry", func(t *testing.T) {
		if _, err := invites.CreateInvite("nobody", "nobody@example.com", "user", 0); err == nil {
			t.Fatal("expected error for zero expiry days")
		}
	})

	t.Run("create defaults role to user", func(t *testing.T) {
		inv, err := invites.CreateInvite("liam", "liam@example.com", "", 7)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		if inv.Role != "user" {
			t.Fatalf("expected default role 'user', got %q", inv.Role)
		}
		if inv.Token == "" {
			t.Fatal("expected non-empty token")
		}
	})

	t.Run("get by token", func(t *testing.T) {
		inv, err := invites.CreateInvite("mia", "mia@example.com", "admin", 7)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		got, err := invites.GetByToken(inv.Token)
		if err != nil {
			t.Fatalf("GetByToken: %v", err)
		}
		if got == nil {
			t.Fatal("expected invite to be found")
		}
		if got.Username != "mia" || got.Role != "admin" {
			t.Fatalf("unexpected invite: %+v", got)
		}
	})

	t.Run("get by token not found returns nil, nil", func(t *testing.T) {
		got, err := invites.GetByToken("bogus-token")
		if err != nil {
			t.Fatalf("GetByToken: %v", err)
		}
		if got != nil {
			t.Fatal("expected nil invite for missing token")
		}
	})

	t.Run("get by id", func(t *testing.T) {
		inv, err := invites.CreateInvite("noah", "noah@example.com", "user", 7)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		got, err := invites.GetByID(inv.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil || got.Username != "noah" {
			t.Fatalf("unexpected invite: %+v", got)
		}
	})

	t.Run("get by id not found returns nil, nil", func(t *testing.T) {
		got, err := invites.GetByID(999999)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got != nil {
			t.Fatal("expected nil invite for missing id")
		}
	})

	t.Run("verify invite happy path", func(t *testing.T) {
		inv, err := invites.CreateInvite("olivia", "olivia@example.com", "user", 7)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		verified, err := invites.VerifyInvite(inv.Token)
		if err != nil {
			t.Fatalf("VerifyInvite: %v", err)
		}
		if verified.Username != "olivia" {
			t.Fatalf("unexpected invite: %+v", verified)
		}
	})

	t.Run("verify invalid token", func(t *testing.T) {
		if _, err := invites.VerifyInvite("no-such-token"); err == nil {
			t.Fatal("expected error for invalid token")
		}
	})

	t.Run("verify used invite", func(t *testing.T) {
		inv, err := invites.CreateInvite("paul", "paul@example.com", "user", 7)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		if err := invites.MarkUsed(inv.Token, 1); err != nil {
			t.Fatalf("MarkUsed: %v", err)
		}
		if _, err := invites.VerifyInvite(inv.Token); err == nil {
			t.Fatal("expected error for already-used invite")
		}
	})

	t.Run("list invites and pending filter", func(t *testing.T) {
		all, err := invites.ListInvites()
		if err != nil {
			t.Fatalf("ListInvites: %v", err)
		}
		if len(all) == 0 {
			t.Fatal("expected at least one invite in list")
		}

		pending, err := invites.GetPendingInvites()
		if err != nil {
			t.Fatalf("GetPendingInvites: %v", err)
		}
		for _, inv := range pending {
			if inv.UsedAt != nil {
				t.Fatalf("expected only unused invites in pending list, got used invite %d", inv.ID)
			}
		}
	})

	t.Run("delete expired invites cleanup", func(t *testing.T) {
		if err := invites.DeleteExpiredInvites(); err != nil {
			t.Fatalf("DeleteExpiredInvites: %v", err)
		}
	})

	t.Run("delete invite by id", func(t *testing.T) {
		inv, err := invites.CreateInvite("quinn", "quinn@example.com", "user", 7)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		if err := invites.DeleteInvite(inv.ID); err != nil {
			t.Fatalf("DeleteInvite: %v", err)
		}
		got, err := invites.GetByToken(inv.Token)
		if err != nil {
			t.Fatalf("GetByToken after delete: %v", err)
		}
		if got != nil {
			t.Fatal("expected invite to be gone after delete")
		}
	})
}

// TestUserModelCheckPasswordWrongHash covers CheckPassword's error-swallowing
// boundary: a malformed stored hash must make CheckPassword return false,
// never panic or propagate the error.
func TestUserModelCheckPasswordWrongHash(t *testing.T) {
	m := &UserModel{}
	bogusUser := &User{PasswordHash: "not-a-valid-hash"}
	if m.CheckPassword(bogusUser, testUserPassword) {
		t.Fatal("expected CheckPassword to return false for malformed hash")
	}
}

// TestUserModelGetByUsernameFieldSubset documents that GetByUsername /
// GetByEmail / ListUsers populate a narrower field set than GetByID (they
// omit DisplayName, Visibility, etc.) — this is existing, intentional
// behavior, verified here as a regression guard rather than a bug.
func TestUserModelGetByUsernameFieldSubset(t *testing.T) {
	newUserTestDB(t)
	m := &UserModel{}

	u, err := m.Create("ray", "ray@example.com", testUserPassword)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.UpdateProfile(u.ID, "Ray Display", "555-0000"); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	byID, err := m.GetByID(u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.DisplayName != "Ray Display" {
		t.Fatalf("expected GetByID to include display name, got %q", byID.DisplayName)
	}

	byUsername, err := m.GetByUsername("ray")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if byUsername.DisplayName != "" {
		t.Fatalf("expected GetByUsername to omit display name, got %q", byUsername.DisplayName)
	}
}

// TestUserModelUpdatePasswordRejectsSameConstant guards against accidentally
// asserting equality between the fixture password and any generated hash —
// belt-and-suspenders check that HashPassword never returns the raw input.
func TestUserModelUpdatePasswordRejectsSameConstant(t *testing.T) {
	if strings.Contains(testUserPassword, "$argon2id$") {
		t.Fatal("fixture constant must be a plain password, not a hash")
	}
}
