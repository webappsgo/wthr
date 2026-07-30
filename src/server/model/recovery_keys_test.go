package models

import (
	"strings"
	"testing"
)

// TestRecoveryKeyModel_GenerateRecoveryKeys covers key generation (count,
// format, uniqueness) and that regenerating invalidates the previous set.
func TestRecoveryKeyModel_GenerateRecoveryKeys(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "rk-user", "rk-user@example.com")
	model := &RecoveryKeyModel{DB: db}

	first, err := model.GenerateRecoveryKeys(int(userID))
	if err != nil {
		t.Fatalf("GenerateRecoveryKeys() error = %v", err)
	}
	if len(first) != 10 {
		t.Fatalf("GenerateRecoveryKeys() returned %d keys, want 10", len(first))
	}

	seen := make(map[string]bool)
	for _, k := range first {
		if len(k) != 13 || k[8] != '-' {
			t.Errorf("key %q does not match {8}-{4} format", k)
		}
		if seen[k] {
			t.Errorf("duplicate key generated: %q", k)
		}
		seen[k] = true
	}

	count, err := model.GetUnusedKeysCount(int(userID))
	if err != nil {
		t.Fatalf("GetUnusedKeysCount() error = %v", err)
	}
	if count != 10 {
		t.Errorf("GetUnusedKeysCount() = %d, want 10", count)
	}

	t.Run("regeneration invalidates old keys", func(t *testing.T) {
		second, err := model.GenerateRecoveryKeys(int(userID))
		if err != nil {
			t.Fatalf("GenerateRecoveryKeys() second call error = %v", err)
		}
		ok, err := model.VerifyAndUseRecoveryKey(int(userID), first[0])
		if err != nil {
			t.Fatalf("VerifyAndUseRecoveryKey() error = %v", err)
		}
		if ok {
			t.Error("old key from before regeneration should no longer verify")
		}
		ok, err = model.VerifyAndUseRecoveryKey(int(userID), second[0])
		if err != nil {
			t.Fatalf("VerifyAndUseRecoveryKey() error = %v", err)
		}
		if !ok {
			t.Error("newly generated key should verify")
		}
	})
}

// TestRecoveryKeyModel_VerifyAndUseRecoveryKey covers case-insensitivity,
// dash-optional input, one-time use, and rejection of unknown/used keys.
func TestRecoveryKeyModel_VerifyAndUseRecoveryKey(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "rk-verify", "rk-verify@example.com")
	model := &RecoveryKeyModel{DB: db}

	tests := []struct {
		name string
		// transform builds the verification input from a freshly generated,
		// still-unused key so each success case gets its own one-time key.
		transform func(key string) string
		wantOK    bool
	}{
		{name: "uppercase with dash", transform: func(k string) string { return strings.ToUpper(k) }, wantOK: true},
		{name: "no dash", transform: func(k string) string { return strings.ReplaceAll(k, "-", "") }, wantOK: true},
		{name: "with stray spaces", transform: func(k string) string { return " " + k + " " }, wantOK: true},
		{name: "unknown key", transform: func(string) string { return "ffffffff-ffff" }, wantOK: false},
		{name: "wrong length after strip", transform: func(string) string { return "abcd" }, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, err := model.GenerateRecoveryKeys(int(userID))
			if err != nil {
				t.Fatalf("GenerateRecoveryKeys() error = %v", err)
			}
			input := tt.transform(keys[0])
			ok, err := model.VerifyAndUseRecoveryKey(int(userID), input)
			if err != nil {
				t.Fatalf("VerifyAndUseRecoveryKey() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Errorf("VerifyAndUseRecoveryKey() = %v, want %v", ok, tt.wantOK)
			}
		})
	}

	t.Run("used key cannot be reused", func(t *testing.T) {
		keys, err := model.GenerateRecoveryKeys(int(userID))
		if err != nil {
			t.Fatalf("GenerateRecoveryKeys() error = %v", err)
		}
		key := keys[0]
		ok, err := model.VerifyAndUseRecoveryKey(int(userID), key)
		if err != nil || !ok {
			t.Fatalf("first use: ok=%v err=%v, want ok=true err=nil", ok, err)
		}
		ok, err = model.VerifyAndUseRecoveryKey(int(userID), key)
		if err != nil {
			t.Fatalf("second use error = %v", err)
		}
		if ok {
			t.Error("recovery key should not verify a second time")
		}
	})
}

// TestRecoveryKeyModel_GetAllKeysForUserAndDelete covers listing and bulk
// deletion, including the empty-set case.
func TestRecoveryKeyModel_GetAllKeysForUserAndDelete(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, nil, db)
	userID := insertTestUser(t, db, "rk-list", "rk-list@example.com")
	model := &RecoveryKeyModel{DB: db}

	t.Run("empty before generation", func(t *testing.T) {
		keys, err := model.GetAllKeysForUser(int(userID))
		if err != nil {
			t.Fatalf("GetAllKeysForUser() error = %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("GetAllKeysForUser() = %d keys, want 0", len(keys))
		}
	})

	generated, err := model.GenerateRecoveryKeys(int(userID))
	if err != nil {
		t.Fatalf("GenerateRecoveryKeys() error = %v", err)
	}
	if _, err := model.VerifyAndUseRecoveryKey(int(userID), generated[0]); err != nil {
		t.Fatalf("VerifyAndUseRecoveryKey() error = %v", err)
	}

	t.Run("lists all with used state", func(t *testing.T) {
		keys, err := model.GetAllKeysForUser(int(userID))
		if err != nil {
			t.Fatalf("GetAllKeysForUser() error = %v", err)
		}
		if len(keys) != 10 {
			t.Fatalf("GetAllKeysForUser() = %d keys, want 10", len(keys))
		}
		usedCount := 0
		for _, k := range keys {
			if k.UsedAt != nil {
				usedCount++
			}
		}
		if usedCount != 1 {
			t.Errorf("used key count = %d, want 1", usedCount)
		}
	})

	t.Run("DeleteAllForUser", func(t *testing.T) {
		if err := model.DeleteAllForUser(int(userID)); err != nil {
			t.Fatalf("DeleteAllForUser() error = %v", err)
		}
		keys, err := model.GetAllKeysForUser(int(userID))
		if err != nil {
			t.Fatalf("GetAllKeysForUser() error = %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("GetAllKeysForUser() after delete = %d, want 0", len(keys))
		}
	})
}
