package model

import (
	"strings"
	"testing"
	"time"
)

// TestGenerateTokenWithPrefix covers format and uniqueness for each prefix.
func TestGenerateTokenWithPrefix(t *testing.T) {
	for _, prefix := range []string{PrefixAdmin, PrefixUser, PrefixOrg} {
		t.Run(prefix, func(t *testing.T) {
			token, err := GenerateTokenWithPrefix(prefix)
			if err != nil {
				t.Fatalf("GenerateTokenWithPrefix() error = %v", err)
			}
			if !strings.HasPrefix(token, prefix) {
				t.Errorf("GenerateTokenWithPrefix() = %q, want prefix %q", token, prefix)
			}
			if err := ValidateTokenFormat(token); err != nil {
				t.Errorf("generated token failed ValidateTokenFormat(): %v", err)
			}
		})
	}
}

// TestHashToken_Deterministic verifies the SHA-256 hash never stores the
// plaintext and is stable across calls.
func TestHashToken_Deterministic(t *testing.T) {
	token := "usr_deadbeef"
	h1 := HashToken(token)
	h2 := HashToken(token)
	if h1 != h2 {
		t.Error("HashToken() should be deterministic")
	}
	if h1 == token {
		t.Error("HashToken() must not return the plaintext token")
	}
}

// TestGetTokenPrefix covers the short-input edge case (len < 8).
func TestGetTokenPrefix(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "long token truncates to 8", token: "usr_abcdef0123456789", want: "usr_abcd"},
		{name: "short token returned as-is", token: "abc", want: "abc"},
		{name: "empty token", token: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetTokenPrefix(tt.token); got != tt.want {
				t.Errorf("GetTokenPrefix(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

// TestValidateTokenFormat covers standard prefixes, agent-compound
// prefixes, and every rejection path.
func TestValidateTokenFormat(t *testing.T) {
	random32 := strings.Repeat("a", 32)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{name: "valid admin token", token: PrefixAdmin + random32, wantErr: false},
		{name: "valid user token", token: PrefixUser + random32, wantErr: false},
		{name: "valid org token", token: PrefixOrg + random32, wantErr: false},
		{name: "valid admin agent token", token: PrefixAdminAgt + random32, wantErr: false},
		{name: "valid user agent token", token: PrefixUserAgt + random32, wantErr: false},
		{name: "valid org agent token", token: PrefixOrgAgt + random32, wantErr: false},
		{name: "admin agent wrong length", token: PrefixAdminAgt + "short", wantErr: true},
		{name: "user agent wrong length", token: PrefixUserAgt + "short", wantErr: true},
		{name: "org agent wrong length", token: PrefixOrgAgt + "short", wantErr: true},
		{name: "no underscore", token: "notoken", wantErr: true},
		{name: "unknown prefix", token: "bad_" + random32, wantErr: true},
		{name: "random part too short", token: PrefixUser + "short", wantErr: true},
		{name: "random part too long", token: PrefixUser + random32 + "extra", wantErr: true},
		{name: "empty string", token: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTokenFormat(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTokenFormat(%q) error = %v, wantErr %v", tt.token, err, tt.wantErr)
			}
		})
	}
}

// TestTokenModelV2_CreateToken covers owner/scope validation, prefix
// selection per owner type, and expiration calculation.
func TestTokenModelV2_CreateToken(t *testing.T) {
	db := newModelLegacyDB(t)
	model := &TokenModelV2{DB: db}

	t.Run("invalid owner type", func(t *testing.T) {
		if _, err := model.CreateToken("bogus", 1, "n", ScopeRead, 0); err == nil {
			t.Error("CreateToken() expected error for invalid owner type")
		}
	})

	t.Run("invalid scope", func(t *testing.T) {
		if _, err := model.CreateToken(OwnerTypeUser, 1, "n", "bogus-scope", 0); err == nil {
			t.Error("CreateToken() expected error for invalid scope")
		}
	})

	t.Run("never expires", func(t *testing.T) {
		tok, err := model.CreateToken(OwnerTypeUser, 1, "Never", ScopeGlobal, 0)
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		if !strings.HasPrefix(tok.Token, PrefixUser) {
			t.Errorf("CreateToken() token = %q, want %s prefix", tok.Token, PrefixUser)
		}
		if tok.ExpiresAt != nil {
			t.Error("CreateToken() with 0 duration should not set an expiry")
		}
	})

	t.Run("admin token uses admin prefix", func(t *testing.T) {
		tok, err := model.CreateToken(OwnerTypeAdmin, 2, "Admin", ScopeGlobal, 0)
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		if !strings.HasPrefix(tok.Token, PrefixAdmin) {
			t.Errorf("CreateToken() token = %q, want %s prefix", tok.Token, PrefixAdmin)
		}
	})

	t.Run("org token uses org prefix and sets expiry", func(t *testing.T) {
		tok, err := model.CreateToken(OwnerTypeOrg, 3, "Org", ScopeReadWrite, 24*time.Hour)
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		if !strings.HasPrefix(tok.Token, PrefixOrg) {
			t.Errorf("CreateToken() token = %q, want %s prefix", tok.Token, PrefixOrg)
		}
		if tok.ExpiresAt == nil {
			t.Fatal("CreateToken() with positive duration should set an expiry")
		}
		if !tok.ExpiresAt.After(time.Now()) {
			t.Error("CreateToken() expiry should be in the future")
		}
	})
}

// TestTokenModelV2_ValidateToken covers format rejection, not-found,
// expired, and the happy path.
func TestTokenModelV2_ValidateToken(t *testing.T) {
	db := newModelLegacyDB(t)
	model := &TokenModelV2{DB: db}

	t.Run("invalid format rejected before DB lookup", func(t *testing.T) {
		if _, err := model.ValidateToken("not-a-token"); err == nil {
			t.Error("ValidateToken() expected error for malformed token")
		}
	})

	t.Run("well-formed but unknown token", func(t *testing.T) {
		unknown := PrefixUser + strings.Repeat("f", 32)
		if _, err := model.ValidateToken(unknown); err == nil {
			t.Error("ValidateToken() expected error for unknown token")
		}
	})

	t.Run("valid token", func(t *testing.T) {
		created, err := model.CreateToken(OwnerTypeUser, 10, "Key", ScopeRead, 0)
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		got, err := model.ValidateToken(created.Token)
		if err != nil {
			t.Fatalf("ValidateToken() error = %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("ValidateToken() ID = %d, want %d", got.ID, created.ID)
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		created, err := model.CreateToken(OwnerTypeUser, 11, "Expired", ScopeRead, time.Nanosecond)
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		time.Sleep(2 * time.Millisecond)
		if _, err := model.ValidateToken(created.Token); err == nil {
			t.Error("ValidateToken() expected error for expired token")
		}
	})
}

// TestTokenModelV2_UpdateLastUsedListDeleteRotate covers the remaining
// lifecycle methods, including ownership-scoped delete failures.
func TestTokenModelV2_UpdateLastUsedListDeleteRotate(t *testing.T) {
	db := newModelLegacyDB(t)
	model := &TokenModelV2{DB: db}

	created, err := model.CreateToken(OwnerTypeUser, 20, "Key", ScopeGlobal, 0)
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	t.Run("UpdateLastUsed", func(t *testing.T) {
		if err := model.UpdateLastUsed(created.ID); err != nil {
			t.Fatalf("UpdateLastUsed() error = %v", err)
		}
		got, err := model.ValidateToken(created.Token)
		if err != nil {
			t.Fatalf("ValidateToken() error = %v", err)
		}
		if got.LastUsedAt == nil {
			t.Error("LastUsedAt should be set after UpdateLastUsed()")
		}
	})

	t.Run("ListTokens", func(t *testing.T) {
		if _, err := model.CreateToken(OwnerTypeUser, 20, "Second", ScopeRead, 0); err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		list, err := model.ListTokens(OwnerTypeUser, 20)
		if err != nil {
			t.Fatalf("ListTokens() error = %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("ListTokens() = %d tokens, want 2", len(list))
		}
	})

	t.Run("ListTokens empty for other owner", func(t *testing.T) {
		list, err := model.ListTokens(OwnerTypeUser, 999)
		if err != nil {
			t.Fatalf("ListTokens() error = %v", err)
		}
		if len(list) != 0 {
			t.Errorf("ListTokens() = %d tokens, want 0", len(list))
		}
	})

	t.Run("DeleteToken wrong owner is rejected", func(t *testing.T) {
		if err := model.DeleteToken(created.ID, OwnerTypeUser, 999); err == nil {
			t.Error("DeleteToken() expected error when owner does not match")
		}
	})

	t.Run("RotateToken changes token value but keeps id/scope", func(t *testing.T) {
		rotated, err := model.RotateToken(created.ID, OwnerTypeUser, 20)
		if err != nil {
			t.Fatalf("RotateToken() error = %v", err)
		}
		if rotated.ID != created.ID {
			t.Errorf("RotateToken() ID = %d, want %d", rotated.ID, created.ID)
		}
		if rotated.Token == created.Token {
			t.Error("RotateToken() should produce a new token value")
		}
		if _, err := model.ValidateToken(created.Token); err == nil {
			t.Error("old token should no longer validate after rotation")
		}
		if _, err := model.ValidateToken(rotated.Token); err != nil {
			t.Errorf("rotated token should validate, got error: %v", err)
		}
	})

	t.Run("RotateToken not found", func(t *testing.T) {
		if _, err := model.RotateToken(999999, OwnerTypeUser, 20); err == nil {
			t.Error("RotateToken() expected error for missing token")
		}
	})

	t.Run("DeleteToken happy path", func(t *testing.T) {
		if err := model.DeleteToken(created.ID, OwnerTypeUser, 20); err != nil {
			t.Fatalf("DeleteToken() error = %v", err)
		}
		if _, err := model.ValidateToken(created.Token); err == nil {
			t.Error("token should not validate after DeleteToken()")
		}
	})
}
