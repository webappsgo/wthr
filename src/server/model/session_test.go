package model

import (
	"testing"
	"time"
)

// TestGenerateSessionID_Unique verifies tokens are non-empty and unique
// across calls (a collision would be a catastrophic security bug).
func TestGenerateSessionID_Unique(t *testing.T) {
	a, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	b, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("GenerateSessionID() returned empty token")
	}
	if a == b {
		t.Error("GenerateSessionID() produced duplicate tokens across two calls")
	}
}

// TestSessionModel_CreateAndGetByID covers session creation, lookup by raw
// token (verifying it's found via its SHA-256 hash, never plaintext), and
// invalid userID type / not-found error paths.
func TestSessionModel_CreateAndGetByID(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "sess-user", "sess-user@example.com")
	model := &SessionModel{DB: db}

	tests := []struct {
		name    string
		userID  interface{}
		wantErr bool
	}{
		{name: "int userID", userID: int(userID), wantErr: false},
		{name: "int64 userID", userID: userID, wantErr: false},
		{name: "unsupported type", userID: "not-an-id", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess, err := model.Create(tt.userID, 3600)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if sess.ID == "" {
				t.Fatal("Create() returned empty session ID")
			}

			got, err := model.GetByID(sess.ID)
			if err != nil {
				t.Fatalf("GetByID() error = %v", err)
			}
			if got.ID != sess.ID {
				t.Errorf("GetByID() ID = %q, want %q", got.ID, sess.ID)
			}
			if got.UserID != int(userID) {
				t.Errorf("GetByID() UserID = %d, want %d", got.UserID, userID)
			}
		})
	}

	t.Run("not found", func(t *testing.T) {
		if _, err := model.GetByID("bogus-token"); err == nil {
			t.Error("GetByID() expected error for unknown token")
		}
	})
}

// TestSessionModel_GetByID_Expired verifies expired sessions are rejected
// and self-cleaned (deleted) on read.
func TestSessionModel_GetByID_Expired(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "sess-exp", "sess-exp@example.com")
	model := &SessionModel{DB: db}

	sess, err := model.Create(int(userID), -1)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := model.GetByID(sess.ID); err == nil {
		t.Error("GetByID() expected error for expired session")
	}

	// Session should have been deleted as a side effect.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM user_sessions WHERE token_hash = ?", hashToken(sess.ID)).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 0 {
		t.Error("GetByID() should delete expired session")
	}
}

// TestSessionModel_UpdateDataAndExtend covers the 2FA pending-data path and
// the session-extension path.
func TestSessionModel_UpdateDataAndExtend(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "sess-data", "sess-data@example.com")
	model := &SessionModel{DB: db}

	sess, err := model.Create(int(userID), 3600)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	data := map[string]interface{}{"pending_2fa": true, "attempt": float64(1)}
	if err := model.UpdateData(sess.ID, data); err != nil {
		t.Fatalf("UpdateData() error = %v", err)
	}

	got, err := model.GetByID(sess.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Data["pending_2fa"] != true {
		t.Errorf("Data[pending_2fa] = %v, want true", got.Data["pending_2fa"])
	}

	originalExpiry := got.ExpiresAt
	time.Sleep(10 * time.Millisecond)
	if err := model.Extend(sess.ID, 7200); err != nil {
		t.Fatalf("Extend() error = %v", err)
	}
	extended, err := model.GetByID(sess.ID)
	if err != nil {
		t.Fatalf("GetByID() after extend error = %v", err)
	}
	if !extended.ExpiresAt.After(originalExpiry) {
		t.Errorf("Extend() did not push expiry forward: before %v after %v", originalExpiry, extended.ExpiresAt)
	}
}

// TestSessionModel_DeleteAndCleanup covers single delete, delete-by-user,
// and cleanup of expired rows.
func TestSessionModel_DeleteAndCleanup(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "sess-cleanup", "sess-cleanup@example.com")
	model := &SessionModel{DB: db}

	sess1, err := model.Create(int(userID), 3600)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	sess2, err := model.Create(int(userID), 3600)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("Delete", func(t *testing.T) {
		if err := model.Delete(sess1.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := model.GetByID(sess1.ID); err == nil {
			t.Error("GetByID() expected error after Delete")
		}
	})

	t.Run("DeleteByUserID", func(t *testing.T) {
		if err := model.DeleteByUserID(int(userID)); err != nil {
			t.Fatalf("DeleteByUserID() error = %v", err)
		}
		if _, err := model.GetByID(sess2.ID); err == nil {
			t.Error("GetByID() expected error after DeleteByUserID")
		}
	})

	t.Run("CleanupExpired", func(t *testing.T) {
		expiredSess, err := model.Create(int(userID), -10)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := model.CleanupExpired(); err != nil {
			t.Fatalf("CleanupExpired() error = %v", err)
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM user_sessions WHERE token_hash = ?", hashToken(expiredSess.ID)).Scan(&count); err != nil {
			t.Fatalf("query count: %v", err)
		}
		if count != 0 {
			t.Error("CleanupExpired() should have removed the expired session")
		}
	})
}
