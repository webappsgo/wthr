package handler

import (
	"database/sql"
	"testing"

	models "github.com/webappsgo/wthr/src/server/model"
)

// newAdminPasskeyTestAdmin creates a real admin row in an isolated in-memory
// server DB so admin passkey helper functions that load/verify admins have
// something to operate against. AdminModel/AdminSessionModel/AdminPasskeyModel
// all read through database.GetServerDB() rather than an injected field, so
// the global dual-DB must be wired first.
func newAdminPasskeyTestAdmin(t *testing.T, serverDB *sql.DB, username, password string) *models.Admin {
	t.Helper()
	setGlobalTestDualDB(t, serverDB, serverDB)
	admin, err := (&models.AdminModel{}).Create(username, username+"@example.com", password, false)
	if err != nil {
		t.Fatalf("failed to create test admin: %v", err)
	}
	return admin
}

func TestSummarizeAdminPasskey(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := summarizeAdminPasskey(nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("without last used", func(t *testing.T) {
		pk := &models.AdminPasskey{ID: 5, Name: "office key"}
		got := summarizeAdminPasskey(pk)
		if got == nil {
			t.Fatalf("expected non-nil summary")
		}
		if got.ID != 5 || got.Name != "office key" {
			t.Errorf("unexpected summary: %+v", got)
		}
		if got.LastUsedAt != nil {
			t.Errorf("expected nil LastUsedAt, got %v", *got.LastUsedAt)
		}
		if got.Raw != pk {
			t.Errorf("expected Raw to reference original passkey")
		}
	})
}

func TestListAdminPasskeys(t *testing.T) {
	serverDB := newTestServerDB(t)
	admin := newAdminPasskeyTestAdmin(t, serverDB, "listadmin", "password123")

	t.Run("no passkeys returns empty slice", func(t *testing.T) {
		summaries, err := ListAdminPasskeys(serverDB, admin.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(summaries) != 0 {
			t.Errorf("expected 0 summaries, got %d", len(summaries))
		}
	})

	t.Run("db error surfaces", func(t *testing.T) {
		badDB := newTestServerDB(t)
		if _, err := badDB.Exec("DROP TABLE server_admin_passkeys"); err != nil {
			t.Fatalf("failed to drop table: %v", err)
		}
		if _, err := ListAdminPasskeys(badDB, admin.ID); err == nil {
			t.Fatalf("expected error from missing table")
		}
	})
}

func TestDeleteAdminPasskey(t *testing.T) {
	serverDB := newTestServerDB(t)
	admin := newAdminPasskeyTestAdmin(t, serverDB, "deladmin", "password123")

	t.Run("deleting non-existent passkey errors", func(t *testing.T) {
		if err := DeleteAdminPasskey(serverDB, admin.ID, 999); err == nil {
			t.Fatalf("expected error deleting non-existent passkey")
		}
	})
}

func TestBeginAdminPasskeyRegistrationToken(t *testing.T) {
	serverDB := newTestServerDB(t)
	admin := newAdminPasskeyTestAdmin(t, serverDB, "regadmin", "password123")
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("empty name errors", func(t *testing.T) {
		if _, err := BeginAdminPasskeyRegistrationToken(serverDB, admin, env, "", "password123"); err == nil {
			t.Fatalf("expected error for empty name")
		}
	})

	t.Run("empty password errors", func(t *testing.T) {
		if _, err := BeginAdminPasskeyRegistrationToken(serverDB, admin, env, "my key", ""); err == nil {
			t.Fatalf("expected error for empty password")
		}
	})

	t.Run("wrong password errors", func(t *testing.T) {
		if _, err := BeginAdminPasskeyRegistrationToken(serverDB, admin, env, "my key", "wrong-password"); err == nil {
			t.Fatalf("expected error for wrong password")
		}
	})

	t.Run("invalid envelope host errors", func(t *testing.T) {
		if _, err := BeginAdminPasskeyRegistrationToken(serverDB, admin, PasskeyEnvelope{}, "my key", "password123"); err == nil {
			t.Fatalf("expected error for missing host")
		}
	})

	t.Run("valid inputs succeed", func(t *testing.T) {
		result, err := BeginAdminPasskeyRegistrationToken(serverDB, admin, env, "my key", "password123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CeremonyToken == "" {
			t.Errorf("expected non-empty ceremony token")
		}
		if result.Options == nil {
			t.Errorf("expected non-nil options")
		}
		passkeyCeremonyCache.Delete(result.CeremonyToken)
	})
}

func TestFinishAdminPasskeyRegistrationToken(t *testing.T) {
	serverDB := newTestServerDB(t)
	admin := newAdminPasskeyTestAdmin(t, serverDB, "finregadmin", "password123")
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("unknown ceremony token errors", func(t *testing.T) {
		if _, err := FinishAdminPasskeyRegistrationToken(serverDB, admin, env, "no-such-token", []byte(`{}`)); err == nil {
			t.Fatalf("expected error for unknown ceremony token")
		}
	})

	t.Run("wrong ceremony kind errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindLogin, UserID: admin.ID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishAdminPasskeyRegistrationToken(serverDB, admin, env, token, []byte(`{}`)); err == nil {
			t.Fatalf("expected error for mismatched ceremony kind")
		}
	})

	t.Run("mismatched admin id errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindRegistration, UserID: admin.ID + 999})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishAdminPasskeyRegistrationToken(serverDB, admin, env, token, []byte(`{}`)); err == nil {
			t.Fatalf("expected error for mismatched admin id")
		}
	})

	t.Run("malformed body fails ceremony", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindRegistration, UserID: admin.ID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishAdminPasskeyRegistrationToken(serverDB, admin, env, token, []byte(`not json`)); err == nil {
			t.Fatalf("expected error for malformed credential body")
		}
	})
}
