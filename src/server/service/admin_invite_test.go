package service

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// setupAdminInviteServerDB opens a fresh in-memory SQLite database with the
// real production ServerSchema applied (server_admin_credentials and
// server_admin_invites both live there, with a foreign key from invites to
// credentials), uniquely named per test via t.Name().
func setupAdminInviteServerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"_admininvite?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	return db
}

// wireAdminInviteGlobalDB wires database.SetGlobalDualDB since AdminModel
// and AdminInviteModel read/write via database.GetServerDB() rather than
// their injected DB field, and restores nil afterward.
func wireAdminInviteGlobalDB(t *testing.T, serverDB *sql.DB) {
	t.Helper()
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
}

// newAdminInviteTestService builds an AdminInviteService wired to db with no
// email service, so CreateInvite never attempts to touch the network.
func newAdminInviteTestService(db *sql.DB) *AdminInviteService {
	return NewAdminInviteService(db, "https://weather.example.com", nil)
}

// seedInviterAdmin creates a real admin row (the "inviter") and returns its ID.
func seedInviterAdmin(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	am := &models.AdminModel{DB: db}
	admin, err := am.Create("inviter", "inviter@example.com", "correct-horse-battery-staple", true)
	if err != nil {
		t.Fatalf("seed inviter admin: %v", err)
	}
	return admin.ID
}

// TestAdminInvite_ParseAdminInviteExpiration is table-driven across every
// allowed value, the empty-string default, and an invalid value.
func TestAdminInvite_ParseAdminInviteExpiration(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantNorm    string
		wantErr     bool
		wantErrText string
	}{
		{name: "1h", input: "1h", wantNorm: "1h"},
		{name: "6h", input: "6h", wantNorm: "6h"},
		{name: "24h", input: "24h", wantNorm: "24h"},
		{name: "48h", input: "48h", wantNorm: "48h"},
		{name: "7d", input: "7d", wantNorm: "7d"},
		{name: "empty defaults to 24h", input: "", wantNorm: "24h"},
		{
			name:        "invalid value",
			input:       "99h",
			wantErr:     true,
			wantErrText: "invalid invite expiration, allowed values: 1h, 6h, 24h, 48h, 7d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			norm, dur, err := ParseAdminInviteExpiration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Error() != tt.wantErrText {
					t.Errorf("error = %q, want %q", err.Error(), tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if norm != tt.wantNorm {
				t.Errorf("normalized = %q, want %q", norm, tt.wantNorm)
			}
			if dur != adminInviteExpirations[tt.wantNorm] {
				t.Errorf("duration = %v, want %v", dur, adminInviteExpirations[tt.wantNorm])
			}
		})
	}
}

// TestAdminInvite_GenerateInviteToken covers the happy path (non-empty
// output) and that repeated calls never collide (idempotency of the
// generator's uniqueness guarantee).
func TestAdminInvite_GenerateInviteToken(t *testing.T) {
	s := newAdminInviteTestService(nil)

	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		token, err := s.GenerateInviteToken()
		if err != nil {
			t.Fatalf("GenerateInviteToken() unexpected error: %v", err)
		}
		if token == "" {
			t.Fatal("GenerateInviteToken() returned empty string")
		}
		if seen[token] {
			t.Fatalf("GenerateInviteToken() produced a duplicate token: %q", token)
		}
		seen[token] = true
	}
}

// TestAdminInvite_CreateInvite_HappyPath covers the successful path: a valid
// inviter and a fresh email produce a persisted invite with the expected
// fields, and no email is sent since EmailService is nil.
func TestAdminInvite_CreateInvite_HappyPath(t *testing.T) {
	db := setupAdminInviteServerDB(t)
	wireAdminInviteGlobalDB(t, db)
	inviterID := seedInviterAdmin(t, db)

	s := newAdminInviteTestService(db)
	invite, normExpiration, err := s.CreateInvite("New.Admin+test@Example.com", int(inviterID), "48h")
	if err != nil {
		t.Fatalf("CreateInvite() unexpected error: %v", err)
	}
	if invite == nil {
		t.Fatal("CreateInvite() returned nil invite")
	}
	if invite.Token == "" {
		t.Error("expected non-empty invite token")
	}
	if invite.InvitedBy != inviterID {
		t.Errorf("InvitedBy = %d, want %d", invite.InvitedBy, inviterID)
	}
	if normExpiration != "48h" {
		t.Errorf("normalized expiration = %q, want %q", normExpiration, "48h")
	}
}

// TestAdminInvite_CreateInvite_ErrorPaths is table-driven over every guard
// clause in CreateInvite: invalid email, invalid expiration, inviter not
// found, and email already registered as admin.
func TestAdminInvite_CreateInvite_ErrorPaths(t *testing.T) {
	tests := []struct {
		name          string
		email         string
		expiration    string
		useInviter    bool
		seedExisting  bool
		wantErrSubstr string
	}{
		{
			name:          "invalid email",
			email:         "not-an-email",
			expiration:    "24h",
			useInviter:    true,
			wantErrSubstr: "email",
		},
		{
			name:          "invalid expiration",
			email:         "fresh@example.com",
			expiration:    "99h",
			useInviter:    true,
			wantErrSubstr: "invalid invite expiration",
		},
		{
			name:          "inviter not found",
			email:         "fresh2@example.com",
			expiration:    "24h",
			useInviter:    false,
			wantErrSubstr: "inviter admin not found",
		},
		{
			name:          "email already registered as admin",
			email:         "inviter@example.com",
			expiration:    "24h",
			useInviter:    true,
			seedExisting:  true,
			wantErrSubstr: "email is already registered as admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupAdminInviteServerDB(t)
			wireAdminInviteGlobalDB(t, db)

			var inviterID int64 = 999999
			if tt.useInviter {
				inviterID = seedInviterAdmin(t, db)
			}

			s := newAdminInviteTestService(db)
			_, _, err := s.CreateInvite(tt.email, int(inviterID), tt.expiration)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
			}
		})
	}
}

// TestAdminInvite_VerifyInvite covers the happy path, not-found token,
// already-used invite, and expired invite.
func TestAdminInvite_VerifyInvite(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		db := setupAdminInviteServerDB(t)
		wireAdminInviteGlobalDB(t, db)
		inviterID := seedInviterAdmin(t, db)

		s := newAdminInviteTestService(db)
		created, _, err := s.CreateInvite("valid@example.com", int(inviterID), "24h")
		if err != nil {
			t.Fatalf("CreateInvite() unexpected error: %v", err)
		}

		invite, err := s.VerifyInvite(created.Token)
		if err != nil {
			t.Fatalf("VerifyInvite() unexpected error: %v", err)
		}
		if invite.InvitedEmail != "valid@example.com" {
			t.Errorf("InvitedEmail = %q, want %q", invite.InvitedEmail, "valid@example.com")
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := setupAdminInviteServerDB(t)
		wireAdminInviteGlobalDB(t, db)

		s := newAdminInviteTestService(db)
		_, err := s.VerifyInvite("this-token-does-not-exist")
		if err == nil {
			t.Fatal("expected error for unknown token, got nil")
		}
		if err.Error() != "invalid invite token" {
			t.Errorf("error = %q, want %q", err.Error(), "invalid invite token")
		}
	})

	t.Run("already used", func(t *testing.T) {
		db := setupAdminInviteServerDB(t)
		wireAdminInviteGlobalDB(t, db)
		inviterID := seedInviterAdmin(t, db)

		s := newAdminInviteTestService(db)
		created, _, err := s.CreateInvite("used@example.com", int(inviterID), "24h")
		if err != nil {
			t.Fatalf("CreateInvite() unexpected error: %v", err)
		}
		if err := s.InviteModel.MarkInviteUsed(created.Token, inviterID); err != nil {
			t.Fatalf("MarkInviteUsed() unexpected error: %v", err)
		}

		_, err = s.VerifyInvite(created.Token)
		if err == nil {
			t.Fatal("expected error for used token, got nil")
		}
		if err.Error() != "invite token has already been used" {
			t.Errorf("error = %q, want %q", err.Error(), "invite token has already been used")
		}
	})

	t.Run("expired", func(t *testing.T) {
		db := setupAdminInviteServerDB(t)
		wireAdminInviteGlobalDB(t, db)
		inviterID := seedInviterAdmin(t, db)

		s := newAdminInviteTestService(db)
		created, _, err := s.CreateInvite("expired@example.com", int(inviterID), "1h")
		if err != nil {
			t.Fatalf("CreateInvite() unexpected error: %v", err)
		}

		// Force the stored expiry into the past directly, bypassing the
		// service's own duration math (which only offers forward-looking
		// durations).
		if _, err := db.Exec(
			`UPDATE server_admin_invites SET expires_at = datetime('now', '-1 hour') WHERE token = ?`,
			models.HashAPIToken(created.Token),
		); err != nil {
			t.Fatalf("force-expire invite: %v", err)
		}

		_, err = s.VerifyInvite(created.Token)
		if err == nil {
			t.Fatal("expected error for expired token, got nil")
		}
		if err.Error() != "invite token has expired" {
			t.Errorf("error = %q, want %q", err.Error(), "invite token has expired")
		}
	})
}

// TestAdminInvite_AcceptInvite covers the happy path (a real admin account
// is created and the invite is marked used), an invalid username, and that
// invite-verification failures (expired/used/not-found) pass through
// unchanged.
func TestAdminInvite_AcceptInvite(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		db := setupAdminInviteServerDB(t)
		wireAdminInviteGlobalDB(t, db)
		inviterID := seedInviterAdmin(t, db)

		s := newAdminInviteTestService(db)
		created, _, err := s.CreateInvite("newadmin@example.com", int(inviterID), "24h")
		if err != nil {
			t.Fatalf("CreateInvite() unexpected error: %v", err)
		}

		admin, err := s.AcceptInvite(created.Token, "alice2026", "correct-horse-battery-staple")
		if err != nil {
			t.Fatalf("AcceptInvite() unexpected error: %v", err)
		}
		if admin.Username != "alice2026" {
			t.Errorf("Username = %q, want %q", admin.Username, "alice2026")
		}
		if admin.Email != "newadmin@example.com" {
			t.Errorf("Email = %q, want %q", admin.Email, "newadmin@example.com")
		}
		if admin.IsSuperAdmin {
			t.Error("expected invite-created admin to not be super admin")
		}

		// The invite must now be reported as used.
		refreshed, err := s.InviteModel.GetInvite(created.Token)
		if err != nil {
			t.Fatalf("GetInvite() unexpected error: %v", err)
		}
		if refreshed.UsedBy == nil {
			t.Fatal("expected UsedBy to be set after AcceptInvite")
		}
		if *refreshed.UsedBy != admin.ID {
			t.Errorf("UsedBy = %d, want %d", *refreshed.UsedBy, admin.ID)
		}
	})

	t.Run("invalid username", func(t *testing.T) {
		db := setupAdminInviteServerDB(t)
		wireAdminInviteGlobalDB(t, db)
		inviterID := seedInviterAdmin(t, db)

		s := newAdminInviteTestService(db)
		created, _, err := s.CreateInvite("badusername@example.com", int(inviterID), "24h")
		if err != nil {
			t.Fatalf("CreateInvite() unexpected error: %v", err)
		}

		_, err = s.AcceptInvite(created.Token, "!", "correct-horse-battery-staple")
		if err == nil {
			t.Fatal("expected error for invalid username, got nil")
		}
	})

	t.Run("verification failure passes through", func(t *testing.T) {
		db := setupAdminInviteServerDB(t)
		wireAdminInviteGlobalDB(t, db)

		s := newAdminInviteTestService(db)
		_, err := s.AcceptInvite("unknown-token", "someone", "correct-horse-battery-staple")
		if err == nil {
			t.Fatal("expected error for unknown token, got nil")
		}
		if err.Error() != "invalid invite token" {
			t.Errorf("error = %q, want %q", err.Error(), "invalid invite token")
		}
	})
}

// TestAdminInvite_GetPendingInvites covers the boundary of zero pending
// invites and the happy path of one pending invite, and confirms a used
// invite is excluded (idempotency: repeated reads return the same set with
// no side effects).
func TestAdminInvite_GetPendingInvites(t *testing.T) {
	db := setupAdminInviteServerDB(t)
	wireAdminInviteGlobalDB(t, db)
	inviterID := seedInviterAdmin(t, db)
	s := newAdminInviteTestService(db)

	empty, err := s.GetPendingInvites()
	if err != nil {
		t.Fatalf("GetPendingInvites() unexpected error: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 pending invites, got %d", len(empty))
	}

	if _, _, err := s.CreateInvite("pending@example.com", int(inviterID), "24h"); err != nil {
		t.Fatalf("CreateInvite() unexpected error: %v", err)
	}
	used, _, err := s.CreateInvite("used-pending@example.com", int(inviterID), "24h")
	if err != nil {
		t.Fatalf("CreateInvite() unexpected error: %v", err)
	}
	if err := s.InviteModel.MarkInviteUsed(used.Token, inviterID); err != nil {
		t.Fatalf("MarkInviteUsed() unexpected error: %v", err)
	}

	for i := 0; i < 2; i++ {
		invites, err := s.GetPendingInvites()
		if err != nil {
			t.Fatalf("GetPendingInvites() call %d unexpected error: %v", i, err)
		}
		if len(invites) != 1 {
			t.Fatalf("call %d: expected 1 pending invite, got %d", i, len(invites))
		}
		if invites[0].InvitedEmail != "pending@example.com" {
			t.Errorf("call %d: InvitedEmail = %q, want %q", i, invites[0].InvitedEmail, "pending@example.com")
		}
	}
}

// TestAdminInvite_CleanupExpiredInvites covers removal of an expired invite
// while a still-valid invite survives, and that running cleanup twice in a
// row (idempotency) is safe and returns no error either time.
func TestAdminInvite_CleanupExpiredInvites(t *testing.T) {
	db := setupAdminInviteServerDB(t)
	wireAdminInviteGlobalDB(t, db)
	inviterID := seedInviterAdmin(t, db)
	s := newAdminInviteTestService(db)

	expired, _, err := s.CreateInvite("expired-cleanup@example.com", int(inviterID), "1h")
	if err != nil {
		t.Fatalf("CreateInvite() unexpected error: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE server_admin_invites SET expires_at = datetime('now', '-1 hour') WHERE token = ?`,
		models.HashAPIToken(expired.Token),
	); err != nil {
		t.Fatalf("force-expire invite: %v", err)
	}

	if _, _, err := s.CreateInvite("still-valid@example.com", int(inviterID), "24h"); err != nil {
		t.Fatalf("CreateInvite() unexpected error: %v", err)
	}

	if err := s.CleanupExpiredInvites(); err != nil {
		t.Fatalf("CleanupExpiredInvites() unexpected error: %v", err)
	}
	// Idempotency: a second run with nothing left to clean must not error.
	if err := s.CleanupExpiredInvites(); err != nil {
		t.Fatalf("CleanupExpiredInvites() second call unexpected error: %v", err)
	}

	if _, err := s.InviteModel.GetInvite(expired.Token); err == nil {
		t.Error("expected expired invite to be deleted")
	}

	pending, err := s.GetPendingInvites()
	if err != nil {
		t.Fatalf("GetPendingInvites() unexpected error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 surviving pending invite, got %d", len(pending))
	}
	if pending[0].InvitedEmail != "still-valid@example.com" {
		t.Errorf("surviving invite email = %q, want %q", pending[0].InvitedEmail, "still-valid@example.com")
	}
}

// TestAdminInvite_RevokeInvite covers the happy path (a revoked invite can
// no longer be verified, mirroring the "already used" outcome since
// RevokeInvite is implemented as MarkInviteUsed(token, 0)) and idempotency
// of calling it twice.
func TestAdminInvite_RevokeInvite(t *testing.T) {
	db := setupAdminInviteServerDB(t)
	wireAdminInviteGlobalDB(t, db)
	inviterID := seedInviterAdmin(t, db)
	s := newAdminInviteTestService(db)

	created, _, err := s.CreateInvite("revoke-me@example.com", int(inviterID), "24h")
	if err != nil {
		t.Fatalf("CreateInvite() unexpected error: %v", err)
	}

	if err := s.RevokeInvite(created.Token); err != nil {
		t.Fatalf("RevokeInvite() unexpected error: %v", err)
	}
	// Idempotency: revoking an already-revoked invite must not error.
	if err := s.RevokeInvite(created.Token); err != nil {
		t.Fatalf("RevokeInvite() second call unexpected error: %v", err)
	}

	_, err = s.VerifyInvite(created.Token)
	if err == nil {
		t.Fatal("expected revoked invite to fail verification")
	}
	if err.Error() != "invite token has already been used" {
		t.Errorf("error = %q, want %q", err.Error(), "invite token has already been used")
	}
}
