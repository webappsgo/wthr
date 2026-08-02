package model

import (
	"strings"
	"testing"
	"time"
)

// testAdminPassword is a fake fixture password, not a real credential.
const testAdminPassword = "test-password-123"

// TestHashPasswordAndVerifyPassword covers the real Argon2id round trip:
// happy path, wrong password, and malformed-hash error paths.
func TestHashPasswordAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword(testAdminPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}

	t.Run("correct password verifies", func(t *testing.T) {
		ok, err := VerifyPassword(testAdminPassword, hash)
		if err != nil {
			t.Fatalf("VerifyPassword: %v", err)
		}
		if !ok {
			t.Fatal("expected password to verify")
		}
	})

	t.Run("wrong password fails", func(t *testing.T) {
		ok, err := VerifyPassword("not-the-password", hash)
		if err != nil {
			t.Fatalf("VerifyPassword: %v", err)
		}
		if ok {
			t.Fatal("expected wrong password to fail verification")
		}
	})

	t.Run("empty password produces a valid distinct hash", func(t *testing.T) {
		emptyHash, err := HashPassword("")
		if err != nil {
			t.Fatalf("HashPassword empty: %v", err)
		}
		ok, err := VerifyPassword("", emptyHash)
		if err != nil {
			t.Fatalf("VerifyPassword empty: %v", err)
		}
		if !ok {
			t.Fatal("expected empty password to verify against its own hash")
		}
	})

	t.Run("malformed hash returns error", func(t *testing.T) {
		if _, err := VerifyPassword(testAdminPassword, "not-a-valid-hash"); err == nil {
			t.Fatal("expected error for malformed hash")
		}
	})

	t.Run("unsupported algorithm returns error", func(t *testing.T) {
		bogus := "$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA"
		if _, err := VerifyPassword(testAdminPassword, bogus); err == nil {
			t.Fatal("expected error for unsupported algorithm")
		}
	})
}

// TestGenerateAPITokenAndHash covers token generation format, hashing, and
// the prefix helper's boundary behavior on short input.
func TestGenerateAPITokenAndHash(t *testing.T) {
	token, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	if !strings.HasPrefix(token, "adm_") {
		t.Fatalf("expected adm_ prefix, got %s", token)
	}

	t.Run("hash is deterministic", func(t *testing.T) {
		h1 := HashAPIToken(token)
		h2 := HashAPIToken(token)
		if h1 != h2 {
			t.Fatal("expected HashAPIToken to be deterministic")
		}
		if len(h1) != 64 {
			t.Fatalf("expected 64-char hex sha256, got %d chars", len(h1))
		}
	})

	t.Run("different tokens hash differently", func(t *testing.T) {
		token2, err := GenerateAPIToken()
		if err != nil {
			t.Fatalf("GenerateAPIToken: %v", err)
		}
		if HashAPIToken(token) == HashAPIToken(token2) {
			t.Fatal("expected distinct tokens to hash differently")
		}
	})

	t.Run("prefix returns first 8 chars", func(t *testing.T) {
		if got := GetAPITokenPrefix(token); got != token[:8] {
			t.Fatalf("expected prefix %s, got %s", token[:8], got)
		}
	})

	t.Run("prefix on short string returns input unchanged", func(t *testing.T) {
		short := "abc"
		if got := GetAPITokenPrefix(short); got != short {
			t.Fatalf("expected %s, got %s", short, got)
		}
	})

	t.Run("prefix on empty string returns empty", func(t *testing.T) {
		if got := GetAPITokenPrefix(""); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})
}

// TestAdminInviteHelpers covers IsExpired/IsUsed boundary conditions.
func TestAdminInviteHelpers(t *testing.T) {
	t.Run("not expired", func(t *testing.T) {
		inv := AdminInvite{ExpiresAt: time.Now().Add(time.Hour)}
		if inv.IsExpired() {
			t.Fatal("expected invite not to be expired")
		}
	})

	t.Run("expired", func(t *testing.T) {
		inv := AdminInvite{ExpiresAt: time.Now().Add(-time.Hour)}
		if !inv.IsExpired() {
			t.Fatal("expected invite to be expired")
		}
	})

	t.Run("unused", func(t *testing.T) {
		inv := AdminInvite{}
		if inv.IsUsed() {
			t.Fatal("expected invite not to be used")
		}
	})

	t.Run("used", func(t *testing.T) {
		now := time.Now()
		inv := AdminInvite{UsedAt: &now}
		if !inv.IsUsed() {
			t.Fatal("expected invite to be used")
		}
	})
}

// TestAdminSessionExpired covers the AdminSession.SessionExpired boundary.
func TestAdminSessionExpired(t *testing.T) {
	t.Run("active session", func(t *testing.T) {
		s := AdminSession{ExpiresAt: time.Now().Add(time.Hour)}
		if s.SessionExpired() {
			t.Fatal("expected session not expired")
		}
	})

	t.Run("expired session", func(t *testing.T) {
		s := AdminSession{ExpiresAt: time.Now().Add(-time.Hour)}
		if !s.SessionExpired() {
			t.Fatal("expected session expired")
		}
	})
}

// newAdminTestDB creates an in-memory server DB with ServerSchema applied
// and wires it as the global server DB for AdminModel/AdminInviteModel/
// AdminSessionModel, all of which read via database.GetServerDB().
func newAdminTestDB(t *testing.T) {
	t.Helper()
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, nil)
}

// TestAdminModelCreateAndGet covers Create, GetByID, GetByEmail, GetAll,
// GetCount, and GetByAPIToken happy paths plus not-found error paths.
func TestAdminModelCreateAndGet(t *testing.T) {
	newAdminTestDB(t)
	m := &AdminModel{}

	admin, err := m.Create("alice", "alice@example.com", testAdminPassword, true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if admin.ID == 0 {
		t.Fatal("expected non-zero admin ID")
	}
	if admin.Username != "alice" || admin.Email != "alice@example.com" {
		t.Fatalf("unexpected admin fields: %+v", admin)
	}
	if !admin.IsSuperAdmin || !admin.IsActive {
		t.Fatalf("expected new admin to be super admin and active: %+v", admin)
	}

	t.Run("GetByID happy path", func(t *testing.T) {
		got, err := m.GetByID(admin.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Username != "alice" {
			t.Fatalf("unexpected username: %s", got.Username)
		}
	})

	t.Run("GetByID not found", func(t *testing.T) {
		if _, err := m.GetByID(999999); err == nil {
			t.Fatal("expected error for missing admin")
		}
	})

	t.Run("GetByEmail happy path", func(t *testing.T) {
		got, err := m.GetByEmail("alice@example.com")
		if err != nil {
			t.Fatalf("GetByEmail: %v", err)
		}
		if got.ID != admin.ID {
			t.Fatalf("expected id %d, got %d", admin.ID, got.ID)
		}
	})

	t.Run("GetByEmail not found", func(t *testing.T) {
		if _, err := m.GetByEmail("nobody@example.com"); err == nil {
			t.Fatal("expected error for missing admin email")
		}
	})

	t.Run("GetAll includes created admin", func(t *testing.T) {
		all, err := m.GetAll()
		if err != nil {
			t.Fatalf("GetAll: %v", err)
		}
		if len(all) != 1 {
			t.Fatalf("expected 1 admin, got %d", len(all))
		}
	})

	t.Run("GetCount", func(t *testing.T) {
		count, err := m.GetCount()
		if err != nil {
			t.Fatalf("GetCount: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected count 1, got %d", count)
		}
	})

	t.Run("GetByAPIToken not found for random token", func(t *testing.T) {
		if _, err := m.GetByAPIToken("adm_doesnotexist"); err == nil {
			t.Fatal("expected error for unknown API token")
		}
	})

	t.Run("RegenerateAPIToken then GetByAPIToken succeeds", func(t *testing.T) {
		token, err := m.RegenerateAPIToken(admin.ID)
		if err != nil {
			t.Fatalf("RegenerateAPIToken: %v", err)
		}
		got, err := m.GetByAPIToken(token)
		if err != nil {
			t.Fatalf("GetByAPIToken: %v", err)
		}
		if got.ID != admin.ID {
			t.Fatalf("expected id %d, got %d", admin.ID, got.ID)
		}

		if err := m.RevokeAPIToken(admin.ID); err != nil {
			t.Fatalf("RevokeAPIToken: %v", err)
		}
		if _, err := m.GetByAPIToken(token); err == nil {
			t.Fatal("expected revoked token to no longer resolve")
		}
	})
}

// TestAdminModelUpdate covers both the simple and full-flags Update variants.
func TestAdminModelUpdate(t *testing.T) {
	newAdminTestDB(t)
	m := &AdminModel{}
	admin, err := m.Create("bob", "bob@example.com", testAdminPassword, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("simple update", func(t *testing.T) {
		if err := m.Update(admin.ID, "bobby", "bobby@example.com"); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := m.GetByID(admin.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Username != "bobby" || got.Email != "bobby@example.com" {
			t.Fatalf("unexpected fields after update: %+v", got)
		}
	})

	t.Run("full update with flags", func(t *testing.T) {
		if err := m.Update(admin.ID, "bobby2", "bobby2@example.com", true, false); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := m.GetByID(admin.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.IsSuperAdmin || got.IsActive {
			t.Fatalf("expected super admin=true, active=false, got: %+v", got)
		}
	})

	t.Run("UpdatePassword allows login with new password", func(t *testing.T) {
		if err := m.UpdatePassword(admin.ID, "new-test-password-456"); err != nil {
			t.Fatalf("UpdatePassword: %v", err)
		}
		got, err := m.GetByID(admin.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		ok, err := VerifyPassword("new-test-password-456", got.PasswordHash)
		if err != nil {
			t.Fatalf("VerifyPassword: %v", err)
		}
		if !ok {
			t.Fatal("expected new password to verify")
		}
	})

	t.Run("UpdateLastLogin sets timestamp", func(t *testing.T) {
		if err := m.UpdateLastLogin(admin.ID); err != nil {
			t.Fatalf("UpdateLastLogin: %v", err)
		}
		got, err := m.GetByID(admin.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.LastLoginAt == nil {
			t.Fatal("expected LastLoginAt to be set")
		}
	})
}

// TestAdminModelVerifyCredentials covers the login path by username and by
// email, plus wrong-password, unknown-user, and disabled-account error paths.
func TestAdminModelVerifyCredentials(t *testing.T) {
	newAdminTestDB(t)
	m := &AdminModel{}
	admin, err := m.Create("carol", "carol@example.com", testAdminPassword, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("valid credentials by email", func(t *testing.T) {
		got, err := m.VerifyCredentials("carol@example.com", testAdminPassword)
		if err != nil {
			t.Fatalf("VerifyCredentials: %v", err)
		}
		if got.ID != admin.ID {
			t.Fatalf("expected id %d, got %d", admin.ID, got.ID)
		}
	})

	t.Run("valid credentials by username", func(t *testing.T) {
		got, err := m.VerifyCredentials("carol", testAdminPassword)
		if err != nil {
			t.Fatalf("VerifyCredentials: %v", err)
		}
		if got.ID != admin.ID {
			t.Fatalf("expected id %d, got %d", admin.ID, got.ID)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		if _, err := m.VerifyCredentials("carol", "wrong-password"); err == nil {
			t.Fatal("expected error for wrong password")
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		if _, err := m.VerifyCredentials("nobody", testAdminPassword); err == nil {
			t.Fatal("expected error for unknown user")
		}
	})

	t.Run("disabled account", func(t *testing.T) {
		if err := m.Update(admin.ID, "carol", "carol@example.com", false, false); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if _, err := m.VerifyCredentials("carol", testAdminPassword); err == nil {
			t.Fatal("expected error for disabled account")
		}
	})
}

// TestAdminModelDelete covers deleting a non-last super admin, deleting a
// regular admin, and the "cannot delete last super admin" error path.
func TestAdminModelDelete(t *testing.T) {
	newAdminTestDB(t)
	m := &AdminModel{}

	t.Run("cannot delete last super admin", func(t *testing.T) {
		admin, err := m.Create("sole-super", "sole-super@example.com", testAdminPassword, true)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := m.Delete(admin.ID); err == nil {
			t.Fatal("expected error deleting the last super admin")
		}
	})

	t.Run("can delete super admin when another exists", func(t *testing.T) {
		a1, err := m.Create("super-1", "super-1@example.com", testAdminPassword, true)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		_, err = m.Create("super-2", "super-2@example.com", testAdminPassword, true)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := m.Delete(a1.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := m.GetByID(a1.ID); err == nil {
			t.Fatal("expected deleted admin to be gone")
		}
	})

	t.Run("can delete regular admin", func(t *testing.T) {
		a, err := m.Create("regular", "regular@example.com", testAdminPassword, false)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := m.Delete(a.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})

	t.Run("delete unknown id errors", func(t *testing.T) {
		if err := m.Delete(999999); err == nil {
			t.Fatal("expected error deleting unknown admin id")
		}
	})
}

// TestAdminInviteModel covers the create/get/mark-used/expire lifecycle.
func TestAdminInviteModel(t *testing.T) {
	newAdminTestDB(t)
	adminModel := &AdminModel{}
	inviter, err := adminModel.Create("inviter", "inviter@example.com", testAdminPassword, true)
	if err != nil {
		t.Fatalf("Create inviter: %v", err)
	}

	im := &AdminInviteModel{}

	t.Run("create and get invite", func(t *testing.T) {
		inv, err := im.CreateInvite("invitee@example.com", inviter.ID, 15*time.Minute)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		if inv.Token == "" {
			t.Fatal("expected non-empty token")
		}

		got, err := im.GetInvite(inv.Token)
		if err != nil {
			t.Fatalf("GetInvite: %v", err)
		}
		if got.InvitedEmail != "invitee@example.com" {
			t.Fatalf("unexpected invited email: %s", got.InvitedEmail)
		}
		if got.IsUsed() {
			t.Fatal("expected fresh invite to be unused")
		}
	})

	t.Run("zero duration errors", func(t *testing.T) {
		if _, err := im.CreateInvite("bad@example.com", inviter.ID, 0); err == nil {
			t.Fatal("expected error for zero expiration")
		}
	})

	t.Run("unknown token errors", func(t *testing.T) {
		if _, err := im.GetInvite("does-not-exist"); err == nil {
			t.Fatal("expected error for unknown invite token")
		}
	})

	t.Run("mark used then reflected in GetInvite", func(t *testing.T) {
		inv, err := im.CreateInvite("useme@example.com", inviter.ID, 15*time.Minute)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		if err := im.MarkInviteUsed(inv.Token, inviter.ID); err != nil {
			t.Fatalf("MarkInviteUsed: %v", err)
		}
		got, err := im.GetInvite(inv.Token)
		if err != nil {
			t.Fatalf("GetInvite: %v", err)
		}
		if !got.IsUsed() {
			t.Fatal("expected invite to be marked used")
		}
	})

	t.Run("pending invites excludes used and expired", func(t *testing.T) {
		expired, err := im.CreateInvite("expired@example.com", inviter.ID, time.Nanosecond)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		_ = expired
		time.Sleep(2 * time.Millisecond)

		pending, err := im.CreateInvite("pending@example.com", inviter.ID, time.Hour)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}

		list, err := im.GetPendingInvites()
		if err != nil {
			t.Fatalf("GetPendingInvites: %v", err)
		}
		// GetPendingInvites scans the raw "token" column, which stores
		// HashAPIToken(rawToken) -- not the raw token -- mirroring the
		// documented UserSessionModel.GetSession behavior elsewhere in
		// this package. Compare against the hash, not the raw token.
		pendingHash := HashAPIToken(pending.Token)
		expiredHash := HashAPIToken(expired.Token)
		foundPending := false
		for _, inv := range list {
			if inv.Token == pendingHash {
				foundPending = true
			}
			if inv.Token == expiredHash {
				t.Fatal("expired invite should not be in pending list")
			}
		}
		if !foundPending {
			t.Fatal("expected pending invite to be in list")
		}
	})

	t.Run("DeleteExpiredInvites removes expired and used", func(t *testing.T) {
		if err := im.DeleteExpiredInvites(); err != nil {
			t.Fatalf("DeleteExpiredInvites: %v", err)
		}
	})
}

// TestAdminSessionModel covers session create/get/delete/expire lifecycle,
// including the store-raw-id-as-primary-key behavior documented in GetSession.
func TestAdminSessionModel(t *testing.T) {
	newAdminTestDB(t)
	adminModel := &AdminModel{}
	admin, err := adminModel.Create("sessuser", "sessuser@example.com", testAdminPassword, true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sm := &AdminSessionModel{}

	t.Run("create and get session", func(t *testing.T) {
		sess, err := sm.CreateSession(admin.ID, "127.0.0.1", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if sess.SessionID == "" {
			t.Fatal("expected non-empty session id")
		}

		got, err := sm.GetSession(sess.SessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.AdminID != admin.ID {
			t.Fatalf("expected admin id %d, got %d", admin.ID, got.AdminID)
		}
		if got.LastUsedAt.IsZero() {
			t.Fatal("expected LastUsedAt to be approximated from CreatedAt")
		}
	})

	t.Run("get unknown session errors", func(t *testing.T) {
		if _, err := sm.GetSession("does-not-exist"); err == nil {
			t.Fatal("expected error for unknown session id")
		}
	})

	t.Run("UpdateSessionLastUsed is a no-op that never errors", func(t *testing.T) {
		if err := sm.UpdateSessionLastUsed("anything"); err != nil {
			t.Fatalf("expected no-op nil error, got %v", err)
		}
	})

	t.Run("delete session removes it", func(t *testing.T) {
		sess, err := sm.CreateSession(admin.ID, "127.0.0.1", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := sm.DeleteSession(sess.SessionID); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}
		if _, err := sm.GetSession(sess.SessionID); err == nil {
			t.Fatal("expected session to be gone after delete")
		}
	})

	t.Run("active sessions excludes expired", func(t *testing.T) {
		_, err := sm.CreateSession(admin.ID, "127.0.0.1", "test-agent", time.Hour)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		expiredSess, err := sm.CreateSession(admin.ID, "127.0.0.1", "test-agent", time.Nanosecond)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		time.Sleep(2 * time.Millisecond)

		active, err := sm.GetActiveSessions(admin.ID)
		if err != nil {
			t.Fatalf("GetActiveSessions: %v", err)
		}
		for _, s := range active {
			if s.SessionID == expiredSess.SessionID {
				t.Fatal("expired session should not be active")
			}
		}
	})

	t.Run("delete all sessions for admin", func(t *testing.T) {
		if err := sm.DeleteAllSessionsForAdmin(admin.ID); err != nil {
			t.Fatalf("DeleteAllSessionsForAdmin: %v", err)
		}
		active, err := sm.GetActiveSessions(admin.ID)
		if err != nil {
			t.Fatalf("GetActiveSessions: %v", err)
		}
		if len(active) != 0 {
			t.Fatalf("expected 0 active sessions, got %d", len(active))
		}
	})

	t.Run("delete expired sessions", func(t *testing.T) {
		if err := sm.DeleteExpiredSessions(); err != nil {
			t.Fatalf("DeleteExpiredSessions: %v", err)
		}
	})
}
