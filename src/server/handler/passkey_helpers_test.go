package handler

import (
	"database/sql"
	"io"
	"testing"
	"time"

	models "github.com/webappsgo/wthr/src/server/model"
)

// newPasskeyTestUser creates a real user row in an isolated in-memory users
// DB so passkey helper functions that load/verify users have something to
// operate against.
func newPasskeyTestUser(t *testing.T, usersDB *sql.DB, username, password string) *models.User {
	t.Helper()
	// UserModel.Create (and GetByID, which it calls internally) read through
	// the package-level database.GetUsersDB() accessor rather than the
	// struct's injected DB field, so the global dual-DB must be wired for
	// these calls to hit this in-memory database instead of nil-panicking.
	setGlobalTestDualDB(t, usersDB, usersDB)
	user, err := (&models.UserModel{DB: usersDB}).Create(username, username+"@example.com", password, "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

func TestBuildWebAuthnFromEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		env     PasskeyEnvelope
		wantErr bool
	}{
		{"empty host errors", PasskeyEnvelope{Host: ""}, true},
		{"whitespace host errors", PasskeyEnvelope{Host: "   "}, true},
		{"plain host http", PasskeyEnvelope{Host: "example.com"}, false},
		{"host with port strips port for rpid", PasskeyEnvelope{Host: "example.com:8443", HTTPS: true}, false},
		// The webauthn library validates RPID as a URI and rejects a bare
		// IPv6 literal ("::1" after bracket/port stripping) with "not a
		// valid URI... missing protocol scheme" — a library limitation,
		// not something buildWebAuthnFromEnvelope itself can fix.
		{"ipv6 host brackets stripped", PasskeyEnvelope{Host: "[::1]:8080"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wa, err := buildWebAuthnFromEnvelope(tt.env)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wa == nil {
				t.Fatalf("expected non-nil webauthn instance")
			}
		})
	}
}

func TestBuildHTTPRequestFromBody(t *testing.T) {
	body := []byte(`{"foo":"bar"}`)
	req := buildHTTPRequestFromBody(body)
	if req == nil {
		t.Fatalf("expected non-nil request")
	}
	if req.Method != "POST" {
		t.Errorf("expected POST method, got %s", req.Method)
	}
	if req.ContentLength != int64(len(body)) {
		t.Errorf("expected content length %d, got %d", len(body), req.ContentLength)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("expected application/json content type, got %s", got)
	}
	got, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed reading body: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("expected body %q, got %q", body, got)
	}
}

func TestIssueAndLoadPasskeyCeremonyToken(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		state := &passkeyCeremonyState{Kind: passkeyKindLogin, UserID: 42}
		token, err := issuePasskeyCeremonyToken(state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token == "" {
			t.Fatalf("expected non-empty token")
		}
		loaded, err := loadPasskeyCeremonyByToken(token)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loaded.UserID != 42 || loaded.Kind != passkeyKindLogin {
			t.Errorf("loaded state mismatch: %+v", loaded)
		}
		passkeyCeremonyCache.Delete(token)
	})

	t.Run("empty token errors", func(t *testing.T) {
		if _, err := loadPasskeyCeremonyByToken(""); err == nil {
			t.Fatalf("expected error for empty token")
		}
	})

	t.Run("whitespace token errors", func(t *testing.T) {
		if _, err := loadPasskeyCeremonyByToken("   "); err == nil {
			t.Fatalf("expected error for whitespace token")
		}
	})

	t.Run("unknown token errors", func(t *testing.T) {
		if _, err := loadPasskeyCeremonyByToken("does-not-exist-token"); err == nil {
			t.Fatalf("expected error for unknown token")
		}
	})

	t.Run("garbage cache value errors", func(t *testing.T) {
		passkeyCeremonyCache.Set("garbage-token", "not-a-state", passkeyCeremonyTTL)
		defer passkeyCeremonyCache.Delete("garbage-token")
		if _, err := loadPasskeyCeremonyByToken("garbage-token"); err == nil {
			t.Fatalf("expected error for non-state cache value")
		}
	})
}

func TestSummarizePasskey(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := summarizePasskey(nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("without last used", func(t *testing.T) {
		pk := &models.UserPasskey{ID: 7, Name: "yubikey"}
		got := summarizePasskey(pk)
		if got == nil {
			t.Fatalf("expected non-nil summary")
		}
		if got.ID != 7 || got.Name != "yubikey" {
			t.Errorf("unexpected summary: %+v", got)
		}
		if got.LastUsedAt != nil {
			t.Errorf("expected nil LastUsedAt, got %v", *got.LastUsedAt)
		}
		if got.Raw != pk {
			t.Errorf("expected Raw to reference original passkey")
		}
	})

	t.Run("with last used", func(t *testing.T) {
		now := time.Now()
		pk := &models.UserPasskey{ID: 8, Name: "phone", LastUsedAt: &now}
		got := summarizePasskey(pk)
		if got.LastUsedAt == nil {
			t.Fatalf("expected non-nil LastUsedAt")
		}
	})
}

func TestListUserPasskeys(t *testing.T) {
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, usersDB, usersDB)
	user := newPasskeyTestUser(t, usersDB, "listuser", "password123")

	t.Run("no passkeys returns empty slice", func(t *testing.T) {
		summaries, err := ListUserPasskeys(usersDB, user.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(summaries) != 0 {
			t.Errorf("expected 0 summaries, got %d", len(summaries))
		}
	})

	t.Run("db error surfaces", func(t *testing.T) {
		badDB := newTestUsersDB(t)
		if _, err := badDB.Exec("DROP TABLE user_passkeys"); err != nil {
			t.Fatalf("failed to drop table: %v", err)
		}
		if _, err := ListUserPasskeys(badDB, user.ID); err == nil {
			t.Fatalf("expected error from missing table")
		}
	})
}

func TestDeleteUserPasskey(t *testing.T) {
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, usersDB, usersDB)
	user := newPasskeyTestUser(t, usersDB, "deluser", "password123")

	t.Run("deleting non-existent passkey errors", func(t *testing.T) {
		if err := DeleteUserPasskey(usersDB, user.ID, 999); err == nil {
			t.Fatalf("expected error deleting non-existent passkey")
		}
	})
}

func TestBeginPasskeyRegistrationToken(t *testing.T) {
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, usersDB, usersDB)
	user := newPasskeyTestUser(t, usersDB, "reguser", "password123")
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("empty name errors", func(t *testing.T) {
		if _, err := BeginPasskeyRegistrationToken(usersDB, user, env, "", "password123"); err == nil {
			t.Fatalf("expected error for empty name")
		}
	})

	t.Run("empty password errors", func(t *testing.T) {
		if _, err := BeginPasskeyRegistrationToken(usersDB, user, env, "my key", ""); err == nil {
			t.Fatalf("expected error for empty password")
		}
	})

	t.Run("wrong password errors", func(t *testing.T) {
		if _, err := BeginPasskeyRegistrationToken(usersDB, user, env, "my key", "wrong-password"); err == nil {
			t.Fatalf("expected error for wrong password")
		}
	})

	t.Run("invalid envelope host errors", func(t *testing.T) {
		if _, err := BeginPasskeyRegistrationToken(usersDB, user, PasskeyEnvelope{}, "my key", "password123"); err == nil {
			t.Fatalf("expected error for missing host")
		}
	})

	t.Run("valid inputs succeed", func(t *testing.T) {
		result, err := BeginPasskeyRegistrationToken(usersDB, user, env, "my key", "password123")
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

func TestFinishPasskeyRegistrationToken(t *testing.T) {
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, usersDB, usersDB)
	user := newPasskeyTestUser(t, usersDB, "finreguser", "password123")
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("unknown ceremony token errors", func(t *testing.T) {
		if _, err := FinishPasskeyRegistrationToken(usersDB, user, env, "no-such-token", []byte(`{}`)); err == nil {
			t.Fatalf("expected error for unknown ceremony token")
		}
	})

	t.Run("wrong ceremony kind errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindLogin, UserID: user.ID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishPasskeyRegistrationToken(usersDB, user, env, token, []byte(`{}`)); err == nil {
			t.Fatalf("expected error for mismatched ceremony kind")
		}
	})

	t.Run("mismatched user id errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindRegistration, UserID: user.ID + 999})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishPasskeyRegistrationToken(usersDB, user, env, token, []byte(`{}`)); err == nil {
			t.Fatalf("expected error for mismatched user id")
		}
	})

	t.Run("malformed body fails ceremony", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindRegistration, UserID: user.ID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishPasskeyRegistrationToken(usersDB, user, env, token, []byte(`not json`)); err == nil {
			t.Fatalf("expected error for malformed credential body")
		}
	})
}

func TestBeginPasskeyChallengeToken(t *testing.T) {
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, usersDB, usersDB)
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("invalid envelope host errors", func(t *testing.T) {
		if _, err := BeginPasskeyChallengeToken(usersDB, PasskeyEnvelope{}, ""); err == nil {
			t.Fatalf("expected error for missing host")
		}
	})

	t.Run("discoverable login succeeds without pending session", func(t *testing.T) {
		result, err := BeginPasskeyChallengeToken(usersDB, env, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CeremonyToken == "" {
			t.Errorf("expected non-empty ceremony token")
		}
		passkeyCeremonyCache.Delete(result.CeremonyToken)
	})

	t.Run("unknown pending session token errors", func(t *testing.T) {
		if _, err := BeginPasskeyChallengeToken(usersDB, env, "no-such-pending-session"); err == nil {
			t.Fatalf("expected error for unknown pending session token")
		}
	})

	t.Run("user without passkeys errors", func(t *testing.T) {
		user := newPasskeyTestUser(t, usersDB, "nopasskeyuser", "password123")
		pending, err := createPendingTwoFactorSession(usersDB, user.ID)
		if err != nil {
			t.Fatalf("unexpected error creating pending session: %v", err)
		}
		if _, err := BeginPasskeyChallengeToken(usersDB, env, pending.ID); err == nil {
			t.Fatalf("expected error for user with no registered passkeys")
		}
	})
}

func TestFinishPasskeyChallengeToken(t *testing.T) {
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, usersDB, usersDB)
	env := PasskeyEnvelope{Host: "example.com", HTTPS: true}

	t.Run("empty body errors", func(t *testing.T) {
		if _, err := FinishPasskeyChallengeToken(usersDB, env, "any-token", nil, "127.0.0.1"); err == nil {
			t.Fatalf("expected error for empty body")
		}
	})

	t.Run("unknown ceremony token errors", func(t *testing.T) {
		if _, err := FinishPasskeyChallengeToken(usersDB, env, "no-such-token", []byte(`{}`), "127.0.0.1"); err == nil {
			t.Fatalf("expected error for unknown ceremony token")
		}
	})

	t.Run("invalid envelope host errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindLogin})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishPasskeyChallengeToken(usersDB, PasskeyEnvelope{}, token, []byte(`{}`), "127.0.0.1"); err == nil {
			t.Fatalf("expected error for missing host")
		}
	})

	t.Run("malformed credential response errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindLogin})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishPasskeyChallengeToken(usersDB, env, token, []byte(`not json`), "127.0.0.1"); err == nil {
			t.Fatalf("expected error for malformed credential response")
		}
	})

	t.Run("unknown ceremony kind errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: "bogus-kind"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishPasskeyChallengeToken(usersDB, env, token, []byte(`{}`), "127.0.0.1"); err == nil {
			t.Fatalf("expected error for unknown ceremony kind")
		}
	})

	t.Run("two factor kind with invalid pending session errors", func(t *testing.T) {
		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{Kind: passkeyKindTwoFactor, PendingSessionToken: "no-such-session"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer passkeyCeremonyCache.Delete(token)
		if _, err := FinishPasskeyChallengeToken(usersDB, env, token, []byte(`{}`), "127.0.0.1"); err == nil {
			t.Fatalf("expected error for invalid pending session")
		}
	})
}
