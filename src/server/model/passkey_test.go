package model

import (
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// TestPasskeyTransportRoundTrip covers marshal/unmarshal of the transport
// list, including the empty and nil edge cases used by both user and admin
// passkey models.
func TestPasskeyTransportRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		transports []protocol.AuthenticatorTransport
	}{
		{name: "empty", transports: nil},
		{name: "single", transports: []protocol.AuthenticatorTransport{protocol.USB}},
		{name: "multiple", transports: []protocol.AuthenticatorTransport{protocol.USB, protocol.NFC, protocol.Internal}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := marshalPasskeyTransport(tt.transports)
			if err != nil {
				t.Fatalf("marshalPasskeyTransport() error = %v", err)
			}
			decoded, err := unmarshalPasskeyTransport(encoded)
			if err != nil {
				t.Fatalf("unmarshalPasskeyTransport() error = %v", err)
			}
			if len(decoded) != len(tt.transports) {
				t.Errorf("round trip length = %d, want %d", len(decoded), len(tt.transports))
			}
		})
	}

	t.Run("unmarshal empty string", func(t *testing.T) {
		decoded, err := unmarshalPasskeyTransport("")
		if err != nil {
			t.Fatalf("unmarshalPasskeyTransport() error = %v", err)
		}
		if decoded != nil {
			t.Errorf("unmarshalPasskeyTransport(\"\") = %v, want nil", decoded)
		}
	})

	t.Run("unmarshal invalid json", func(t *testing.T) {
		if _, err := unmarshalPasskeyTransport("not-json"); err == nil {
			t.Error("unmarshalPasskeyTransport() expected error for invalid JSON")
		}
	})
}

// TestPasskeyBytesRoundTrip covers the base64url encode/decode helpers,
// including the empty-input and invalid-input edge cases.
func TestPasskeyBytesRoundTrip(t *testing.T) {
	data := []byte{1, 2, 3, 255, 0}
	encoded := encodePasskeyBytes(data)
	decoded, err := decodePasskeyBytes(encoded)
	if err != nil {
		t.Fatalf("decodePasskeyBytes() error = %v", err)
	}
	if string(decoded) != string(data) {
		t.Errorf("round trip = %v, want %v", decoded, data)
	}

	t.Run("empty input", func(t *testing.T) {
		decoded, err := decodePasskeyBytes("")
		if err != nil {
			t.Fatalf("decodePasskeyBytes() error = %v", err)
		}
		if decoded != nil {
			t.Errorf("decodePasskeyBytes(\"\") = %v, want nil", decoded)
		}
	})

	t.Run("invalid base64", func(t *testing.T) {
		if _, err := decodePasskeyBytes("not valid base64!!!"); err == nil {
			t.Error("decodePasskeyBytes() expected error for invalid input")
		}
	})
}

func testCredential(id byte) *webauthn.Credential {
	return &webauthn.Credential{
		ID:              []byte{id, id, id},
		PublicKey:       []byte{id + 1, id + 1},
		AttestationType: "none",
		Transport:       []protocol.AuthenticatorTransport{protocol.USB},
		Flags: webauthn.CredentialFlags{
			BackupEligible: true,
			BackupState:    false,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    []byte{9, 9},
			SignCount: 1,
		},
	}
}

// TestUserPasskeyModel_CreateListCountDelete exercises the full lifecycle:
// schema self-migration on first use, create, list, count, HasPasskeys,
// credential reconstruction for WebAuthn, update, and delete (including
// not-found).
func TestUserPasskeyModel_CreateListCountDelete(t *testing.T) {
	db := newModelUsersDB(t)
	userID := insertTestUser(t, db, "pk-user", "pk-user@example.com")
	model := &UserPasskeyModel{DB: db}

	t.Run("empty before creation", func(t *testing.T) {
		has, err := model.HasPasskeys(userID)
		if err != nil {
			t.Fatalf("HasPasskeys() error = %v", err)
		}
		if has {
			t.Error("HasPasskeys() = true, want false")
		}
		count, err := model.CountByUserID(userID)
		if err != nil {
			t.Fatalf("CountByUserID() error = %v", err)
		}
		if count != 0 {
			t.Errorf("CountByUserID() = %d, want 0", count)
		}
	})

	created, err := model.Create(userID, "My Key", testCredential(1))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == 0 {
		t.Error("Create() returned zero ID")
	}
	if created.Name != "My Key" {
		t.Errorf("Create() name = %q, want %q", created.Name, "My Key")
	}
	if _, err := model.Create(userID, "Second Key", testCredential(2)); err != nil {
		t.Fatalf("Create() second key error = %v", err)
	}

	t.Run("ListByUserID", func(t *testing.T) {
		list, err := model.ListByUserID(userID)
		if err != nil {
			t.Fatalf("ListByUserID() error = %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("ListByUserID() = %d passkeys, want 2", len(list))
		}
	})

	t.Run("CountByUserID and HasPasskeys", func(t *testing.T) {
		count, err := model.CountByUserID(userID)
		if err != nil {
			t.Fatalf("CountByUserID() error = %v", err)
		}
		if count != 2 {
			t.Errorf("CountByUserID() = %d, want 2", count)
		}
		has, err := model.HasPasskeys(userID)
		if err != nil {
			t.Fatalf("HasPasskeys() error = %v", err)
		}
		if !has {
			t.Error("HasPasskeys() = false, want true")
		}
	})

	t.Run("ListCredentialsByUserID reconstructs webauthn.Credential", func(t *testing.T) {
		creds, err := model.ListCredentialsByUserID(userID)
		if err != nil {
			t.Fatalf("ListCredentialsByUserID() error = %v", err)
		}
		if len(creds) != 2 {
			t.Fatalf("ListCredentialsByUserID() = %d credentials, want 2", len(creds))
		}
		if len(creds[0].ID) == 0 {
			t.Error("reconstructed credential has empty ID")
		}
		if !creds[0].Flags.BackupEligible {
			t.Error("reconstructed credential should have BackupEligible=true")
		}
	})

	t.Run("UpdateCredential", func(t *testing.T) {
		cred := testCredential(1)
		cred.Authenticator.SignCount = 42
		if err := model.UpdateCredential(userID, cred); err != nil {
			t.Fatalf("UpdateCredential() error = %v", err)
		}
		list, err := model.ListByUserID(userID)
		if err != nil {
			t.Fatalf("ListByUserID() error = %v", err)
		}
		found := false
		for _, p := range list {
			if p.ID == created.ID {
				found = true
				if p.SignCount != 42 {
					t.Errorf("SignCount = %d, want 42", p.SignCount)
				}
			}
		}
		if !found {
			t.Fatal("updated passkey not found in list")
		}
	})

	t.Run("UpdateCredential unknown credential", func(t *testing.T) {
		cred := testCredential(200)
		if err := model.UpdateCredential(userID, cred); err == nil {
			t.Error("UpdateCredential() expected error for unknown credential")
		}
	})

	t.Run("DeleteByID", func(t *testing.T) {
		if err := model.DeleteByID(userID, created.ID); err != nil {
			t.Fatalf("DeleteByID() error = %v", err)
		}
		count, err := model.CountByUserID(userID)
		if err != nil {
			t.Fatalf("CountByUserID() error = %v", err)
		}
		if count != 1 {
			t.Errorf("CountByUserID() after delete = %d, want 1", count)
		}
	})

	t.Run("DeleteByID not found", func(t *testing.T) {
		if err := model.DeleteByID(userID, 999999); err == nil {
			t.Error("DeleteByID() expected error for missing passkey")
		}
	})
}

// TestUserPasskeyModel_GetDB_FallsBackToGlobal verifies getDB() prefers the
// injected DB field but falls back to the global accessor when nil.
func TestUserPasskeyModel_GetDB_FallsBackToGlobal(t *testing.T) {
	db := newModelUsersDB(t)
	model := &UserPasskeyModel{DB: db}
	if model.getDB() != db {
		t.Error("getDB() should return the injected DB")
	}

	setModelGlobalDualDB(t, nil, db)
	fallback := &UserPasskeyModel{}
	if fallback.getDB() != db {
		t.Error("getDB() should fall back to database.GetUsersDB() when DB field is nil")
	}
}
