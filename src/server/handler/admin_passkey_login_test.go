package handler

import (
	"testing"
	"time"
)

func TestGenerateAdminPendingSessionID(t *testing.T) {
	id1, err := generateAdminPendingSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id1) != 64 {
		t.Errorf("expected 64 hex chars (32 bytes), got %d", len(id1))
	}

	id2, err := generateAdminPendingSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 == id2 {
		t.Errorf("expected distinct session ids across calls")
	}
}

func TestCreateAdminPendingSession(t *testing.T) {
	t.Run("invalid admin id errors", func(t *testing.T) {
		if _, err := CreateAdminPendingSession(0, "127.0.0.1", "test-agent"); err == nil {
			t.Fatalf("expected error for zero admin id")
		}
		if _, err := CreateAdminPendingSession(-1, "127.0.0.1", "test-agent"); err == nil {
			t.Fatalf("expected error for negative admin id")
		}
	})

	t.Run("valid admin id round-trips through LoadAdminPendingSession", func(t *testing.T) {
		token, err := CreateAdminPendingSession(42, "203.0.113.7", "test-agent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token == "" {
			t.Fatalf("expected non-empty token")
		}
		defer DeleteAdminPendingSession(token)

		pending, err := LoadAdminPendingSession(token)
		if err != nil {
			t.Fatalf("unexpected error loading pending session: %v", err)
		}
		if pending.AdminID != 42 {
			t.Errorf("AdminID = %d, want 42", pending.AdminID)
		}
		if pending.IPAddress != "203.0.113.7" {
			t.Errorf("IPAddress = %q, want %q", pending.IPAddress, "203.0.113.7")
		}
		if pending.UserAgent != "test-agent" {
			t.Errorf("UserAgent = %q, want %q", pending.UserAgent, "test-agent")
		}
		if pending.CreatedAt.After(time.Now()) {
			t.Errorf("expected CreatedAt to be in the past")
		}
	})
}

func TestLoadAdminPendingSession(t *testing.T) {
	t.Run("empty token errors", func(t *testing.T) {
		if _, err := LoadAdminPendingSession(""); err == nil {
			t.Fatalf("expected error for empty token")
		}
	})

	t.Run("whitespace-only token errors", func(t *testing.T) {
		if _, err := LoadAdminPendingSession("   "); err == nil {
			t.Fatalf("expected error for whitespace-only token")
		}
	})

	t.Run("unknown token errors", func(t *testing.T) {
		if _, err := LoadAdminPendingSession("does-not-exist"); err == nil {
			t.Fatalf("expected error for unknown token")
		}
	})

	t.Run("garbage cache value errors", func(t *testing.T) {
		adminPendingSessionCache.Set("garbage-token", "not-a-pending-session", adminPendingSessionTTL)
		defer adminPendingSessionCache.Delete("garbage-token")

		if _, err := LoadAdminPendingSession("garbage-token"); err == nil {
			t.Fatalf("expected error for malformed cache entry")
		}
	})
}

func TestDeleteAdminPendingSession(t *testing.T) {
	token, err := CreateAdminPendingSession(7, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	DeleteAdminPendingSession(token)

	if _, err := LoadAdminPendingSession(token); err == nil {
		t.Fatalf("expected error loading deleted pending session")
	}
}

func TestAdminHasPasskeys(t *testing.T) {
	serverDB := newTestServerDB(t)
	admin := newAdminPasskeyTestAdmin(t, serverDB, "haspasskeysadmin", "password123")

	has, err := AdminHasPasskeys(serverDB, admin.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Errorf("expected admin with no passkeys to report false")
	}
}

func TestBeginAdminPasskeyLoginToken(t *testing.T) {
	serverDB := newTestServerDB(t)
	admin := newAdminPasskeyTestAdmin(t, serverDB, "beginloginadmin", "password123")
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("invalid pending session token errors", func(t *testing.T) {
		if _, err := BeginAdminPasskeyLoginToken(serverDB, env, "no-such-token"); err == nil {
			t.Fatalf("expected error for invalid pending session token")
		}
	})

	t.Run("unknown admin id errors", func(t *testing.T) {
		token, err := CreateAdminPendingSession(999999, "127.0.0.1", "test-agent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer DeleteAdminPendingSession(token)

		if _, err := BeginAdminPasskeyLoginToken(serverDB, env, token); err == nil {
			t.Fatalf("expected error for unknown admin id")
		}
	})

	t.Run("admin without passkeys errors", func(t *testing.T) {
		token, err := CreateAdminPendingSession(admin.ID, "127.0.0.1", "test-agent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer DeleteAdminPendingSession(token)

		if _, err := BeginAdminPasskeyLoginToken(serverDB, env, token); err == nil {
			t.Fatalf("expected error for admin with no registered passkeys")
		}
	})
}

func TestFinishAdminPasskeyLoginToken(t *testing.T) {
	serverDB := newTestServerDB(t)
	admin := newAdminPasskeyTestAdmin(t, serverDB, "finishloginadmin", "password123")
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("empty body errors", func(t *testing.T) {
		if _, err := FinishAdminPasskeyLoginToken(serverDB, env, "some-token", nil, "127.0.0.1", "test-agent", time.Hour); err == nil {
			t.Fatalf("expected error for empty body")
		}
	})

	t.Run("unknown ceremony token errors", func(t *testing.T) {
		if _, err := FinishAdminPasskeyLoginToken(serverDB, env, "no-such-token", []byte(`{}`), "127.0.0.1", "test-agent", time.Hour); err == nil {
			t.Fatalf("expected error for unknown ceremony token")
		}
	})

	t.Run("wrong ceremony kind errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindLogin, UserID: admin.ID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)

		if _, err := FinishAdminPasskeyLoginToken(serverDB, env, token, []byte(`{}`), "127.0.0.1", "test-agent", time.Hour); err == nil {
			t.Fatalf("expected error for mismatched ceremony kind")
		}
	})

	t.Run("stale pending session errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{
			Kind:                passkeyKindAdminLogin,
			UserID:              admin.ID,
			PendingSessionToken: "no-such-pending-session",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)

		if _, err := FinishAdminPasskeyLoginToken(serverDB, env, token, []byte(`{}`), "127.0.0.1", "test-agent", time.Hour); err == nil {
			t.Fatalf("expected error for stale pending session")
		}
	})

	t.Run("pending session admin mismatch errors", func(t *testing.T) {
		pendingToken, err := CreateAdminPendingSession(admin.ID+999, "127.0.0.1", "test-agent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer DeleteAdminPendingSession(pendingToken)

		ceremonyToken, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{
			Kind:                passkeyKindAdminLogin,
			UserID:              admin.ID,
			PendingSessionToken: pendingToken,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(ceremonyToken)

		if _, err := FinishAdminPasskeyLoginToken(serverDB, env, ceremonyToken, []byte(`{}`), "127.0.0.1", "test-agent", time.Hour); err == nil {
			t.Fatalf("expected error for pending session/admin id mismatch")
		}
	})
}
