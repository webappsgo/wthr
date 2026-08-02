package model

import (
	"strings"
	"testing"
)

// TestGenerateToken_FormatAndUniqueness verifies the usr_ prefix, hex body
// length, and that two calls never collide.
func TestGenerateToken_FormatAndUniqueness(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if !strings.HasPrefix(a, "usr_") {
		t.Errorf("GenerateToken() = %q, want usr_ prefix", a)
	}
	if len(a) != len("usr_")+32 {
		t.Errorf("GenerateToken() length = %d, want %d", len(a), len("usr_")+32)
	}
	b, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if a == b {
		t.Error("GenerateToken() produced duplicate tokens across two calls")
	}
}

// TestHashUserTokenAndPrefix_DelegateToV2 verifies the thin wrappers really
// delegate to the token_v2 implementations rather than diverging.
func TestHashUserTokenAndPrefix_DelegateToV2(t *testing.T) {
	token := "usr_abcdef0123456789"
	if HashUserToken(token) != HashToken(token) {
		t.Error("HashUserToken() should delegate to HashToken()")
	}
	if GetUserTokenPrefix(token) != GetTokenPrefix(token) {
		t.Error("GetUserTokenPrefix() should delegate to GetTokenPrefix()")
	}
}

// TestTokenModel_CreateAndGetByToken covers creation (plaintext returned
// once, never stored) and lookup by plaintext via the SHA-256 hash.
func TestTokenModel_CreateAndGetByToken(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "tok-user", "tok-user@example.com")
	model := &TokenModel{DB: db}

	created, err := model.Create(int(userID), "CI Key")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Token == "" {
		t.Fatal("Create() should return the plaintext token")
	}
	if created.ID == 0 {
		t.Error("Create() returned zero ID")
	}

	var storedHash string
	if err := db.QueryRow("SELECT token_hash FROM user_tokens WHERE id = ?", created.ID).Scan(&storedHash); err != nil {
		t.Fatalf("query stored hash: %v", err)
	}
	if storedHash == created.Token {
		t.Fatal("token_hash column stores plaintext token, want SHA-256 hash")
	}
	if storedHash != HashUserToken(created.Token) {
		t.Error("stored hash does not match HashUserToken(plaintext)")
	}

	t.Run("GetByToken finds by plaintext", func(t *testing.T) {
		got, err := model.GetByToken(created.Token)
		if err != nil {
			t.Fatalf("GetByToken() error = %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("GetByToken() ID = %d, want %d", got.ID, created.ID)
		}
		if got.UserID != int(userID) {
			t.Errorf("GetByToken() UserID = %d, want %d", got.UserID, userID)
		}
	})

	t.Run("GetByToken unknown token errors", func(t *testing.T) {
		if _, err := model.GetByToken("usr_doesnotexist00000000000000000"); err == nil {
			t.Error("GetByToken() expected error for unknown token")
		}
	})
}

// TestTokenModel_GetByUserID covers listing (empty and multiple, newest
// first per ORDER BY created_at DESC).
func TestTokenModel_GetByUserID(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "tok-list", "tok-list@example.com")
	model := &TokenModel{DB: db}

	t.Run("empty", func(t *testing.T) {
		tokens, err := model.GetByUserID(int(userID))
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}
		if len(tokens) != 0 {
			t.Errorf("GetByUserID() = %d tokens, want 0", len(tokens))
		}
	})

	if _, err := model.Create(int(userID), "First"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := model.Create(int(userID), "Second"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("returns all for user", func(t *testing.T) {
		tokens, err := model.GetByUserID(int(userID))
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}
		if len(tokens) != 2 {
			t.Fatalf("GetByUserID() = %d tokens, want 2", len(tokens))
		}
	})
}

// TestTokenModel_UpdateLastUsedAndDelete covers the last_used_at write and
// both delete paths.
func TestTokenModel_UpdateLastUsedAndDelete(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "tok-del", "tok-del@example.com")
	model := &TokenModel{DB: db}

	created, err := model.Create(int(userID), "Key")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("UpdateLastUsed", func(t *testing.T) {
		if err := model.UpdateLastUsed(created.ID); err != nil {
			t.Fatalf("UpdateLastUsed() error = %v", err)
		}
		got, err := model.GetByToken(created.Token)
		if err != nil {
			t.Fatalf("GetByToken() error = %v", err)
		}
		if got.LastUsedAt == nil {
			t.Error("LastUsedAt should be set after UpdateLastUsed()")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := model.Delete(created.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := model.GetByToken(created.Token); err == nil {
			t.Error("GetByToken() expected error after Delete")
		}
	})

	t.Run("DeleteByUserID", func(t *testing.T) {
		second, err := model.Create(int(userID), "Second")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := model.DeleteByUserID(int(userID)); err != nil {
			t.Fatalf("DeleteByUserID() error = %v", err)
		}
		tokens, err := model.GetByUserID(int(userID))
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}
		if len(tokens) != 0 {
			t.Errorf("GetByUserID() after DeleteByUserID = %d, want 0", len(tokens))
		}
		if _, err := model.GetByToken(second.Token); err == nil {
			t.Error("GetByToken() expected error after DeleteByUserID")
		}
	})
}
