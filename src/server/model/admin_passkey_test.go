package model

import (
	"testing"
)

// TestAdminPasskeyModel_CreateListCountDelete mirrors the user passkey
// lifecycle test but against the server-admin table/DB, verifying the two
// models don't cross-contaminate storage.
func TestAdminPasskeyModel_CreateListCountDelete(t *testing.T) {
	db := newModelServerDB(t)
	model := &AdminPasskeyModel{DB: db}
	adminID := int64(1)

	t.Run("empty before creation", func(t *testing.T) {
		has, err := model.HasPasskeys(adminID)
		if err != nil {
			t.Fatalf("HasPasskeys() error = %v", err)
		}
		if has {
			t.Error("HasPasskeys() = true, want false")
		}
	})

	created, err := model.Create(adminID, "Admin Key", testCredential(1))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.AdminID != adminID {
		t.Errorf("Create() AdminID = %d, want %d", created.AdminID, adminID)
	}
	if _, err := model.Create(adminID, "Second Admin Key", testCredential(2)); err != nil {
		t.Fatalf("Create() second key error = %v", err)
	}

	t.Run("ListByAdminID", func(t *testing.T) {
		list, err := model.ListByAdminID(adminID)
		if err != nil {
			t.Fatalf("ListByAdminID() error = %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("ListByAdminID() = %d passkeys, want 2", len(list))
		}
	})

	t.Run("CountByAdminID and HasPasskeys", func(t *testing.T) {
		count, err := model.CountByAdminID(adminID)
		if err != nil {
			t.Fatalf("CountByAdminID() error = %v", err)
		}
		if count != 2 {
			t.Errorf("CountByAdminID() = %d, want 2", count)
		}
		has, err := model.HasPasskeys(adminID)
		if err != nil {
			t.Fatalf("HasPasskeys() error = %v", err)
		}
		if !has {
			t.Error("HasPasskeys() = false, want true")
		}
	})

	t.Run("ListCredentialsByAdminID reconstructs webauthn.Credential", func(t *testing.T) {
		creds, err := model.ListCredentialsByAdminID(adminID)
		if err != nil {
			t.Fatalf("ListCredentialsByAdminID() error = %v", err)
		}
		if len(creds) != 2 {
			t.Fatalf("ListCredentialsByAdminID() = %d credentials, want 2", len(creds))
		}
	})

	t.Run("UpdateCredential", func(t *testing.T) {
		cred := testCredential(1)
		cred.Authenticator.SignCount = 7
		if err := model.UpdateCredential(adminID, cred); err != nil {
			t.Fatalf("UpdateCredential() error = %v", err)
		}
		list, err := model.ListByAdminID(adminID)
		if err != nil {
			t.Fatalf("ListByAdminID() error = %v", err)
		}
		found := false
		for _, p := range list {
			if p.ID == created.ID {
				found = true
				if p.SignCount != 7 {
					t.Errorf("SignCount = %d, want 7", p.SignCount)
				}
			}
		}
		if !found {
			t.Fatal("updated admin passkey not found in list")
		}
	})

	t.Run("UpdateCredential unknown credential", func(t *testing.T) {
		if err := model.UpdateCredential(adminID, testCredential(200)); err == nil {
			t.Error("UpdateCredential() expected error for unknown credential")
		}
	})

	t.Run("DeleteByID", func(t *testing.T) {
		if err := model.DeleteByID(adminID, created.ID); err != nil {
			t.Fatalf("DeleteByID() error = %v", err)
		}
		count, err := model.CountByAdminID(adminID)
		if err != nil {
			t.Fatalf("CountByAdminID() error = %v", err)
		}
		if count != 1 {
			t.Errorf("CountByAdminID() after delete = %d, want 1", count)
		}
	})

	t.Run("DeleteByID not found", func(t *testing.T) {
		if err := model.DeleteByID(adminID, 999999); err == nil {
			t.Error("DeleteByID() expected error for missing passkey")
		}
	})
}

// TestAdminPasskeyModel_GetDB_FallsBackToGlobal mirrors the user-passkey
// fallback test for the server DB accessor.
func TestAdminPasskeyModel_GetDB_FallsBackToGlobal(t *testing.T) {
	db := newModelServerDB(t)
	model := &AdminPasskeyModel{DB: db}
	if model.getDB() != db {
		t.Error("getDB() should return the injected DB")
	}

	setModelGlobalDualDB(t, db, nil)
	fallback := &AdminPasskeyModel{}
	if fallback.getDB() != db {
		t.Error("getDB() should fall back to database.GetServerDB() when DB field is nil")
	}
}
