package graphql

import (
	"context"
	"testing"
)

// --- CreateUserToken ---------------------------------------------------------

func TestMutationResolver_CreateUserToken(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.CreateUserToken(context.Background(), "my token", nil, nil)
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	user := seedGraphQLUser(t, ddb, "tokenuser", "tokenuser@example.com", "correctpass1")
	ctx := withGraphQLUserContext(context.Background(), user)

	t.Run("happy path returns the plaintext token once", func(t *testing.T) {
		result, err := m.CreateUserToken(ctx, "my first token", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.Token == nil || *result.Token == "" {
			t.Fatal("expected a non-empty plaintext token")
		}
		if result.Name == nil || *result.Name != "my first token" {
			t.Fatalf("Name = %v, want %q", result.Name, "my first token")
		}
		if result.ExpiresAt != nil {
			t.Fatal("expected ExpiresAt to be nil when expiresIn is not set")
		}
	})

	t.Run("scopes and expiresIn are applied", func(t *testing.T) {
		scopes := "read,write"
		expiresIn := 30
		result, err := m.CreateUserToken(ctx, "scoped token", &scopes, &expiresIn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Scopes == nil || *result.Scopes != scopes {
			t.Fatalf("Scopes = %v, want %q", result.Scopes, scopes)
		}
		if result.ExpiresAt == nil {
			t.Fatal("expected a non-nil ExpiresAt when expiresIn is set")
		}
	})

	t.Run("rejects a 6th token for the same user", func(t *testing.T) {
		limitUser := seedGraphQLUser(t, ddb, "limituser", "limituser@example.com", "correctpass1")
		limitCtx := withGraphQLUserContext(context.Background(), limitUser)
		for i := 0; i < 5; i++ {
			if _, err := m.CreateUserToken(limitCtx, "token", nil, nil); err != nil {
				t.Fatalf("unexpected error seeding token %d: %v", i, err)
			}
		}
		_, err := m.CreateUserToken(limitCtx, "one too many", nil, nil)
		if err == nil || err.Error() != "maximum 5 tokens per user" {
			t.Fatalf("err = %v, want %q", err, "maximum 5 tokens per user")
		}
	})
}

// --- RevokeUserToken ---------------------------------------------------------

func TestMutationResolver_RevokeUserToken(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.RevokeUserToken(context.Background(), "1")
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	user := seedGraphQLUser(t, ddb, "revokeuser", "revokeuser@example.com", "correctpass1")
	ctx := withGraphQLUserContext(context.Background(), user)

	t.Run("invalid id returns an error", func(t *testing.T) {
		_, err := m.RevokeUserToken(ctx, "not-a-number")
		if err == nil || err.Error() != "invalid token id" {
			t.Fatalf("err = %v, want %q", err, "invalid token id")
		}
	})

	t.Run("unknown token id returns a not-found response, not an error", func(t *testing.T) {
		result, err := m.RevokeUserToken(ctx, "999999")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false for an unknown token id")
		}
		if result.Message != "Token not found" {
			t.Fatalf("Message = %q, want %q", result.Message, "Token not found")
		}
	})

	t.Run("owned token is revoked", func(t *testing.T) {
		created, err := m.CreateUserToken(ctx, "to be revoked", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error creating token: %v", err)
		}
		result, err := m.RevokeUserToken(ctx, created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}
		if result.Message != "Token revoked" {
			t.Fatalf("Message = %q, want %q", result.Message, "Token revoked")
		}
	})

	t.Run("cannot revoke another user's token", func(t *testing.T) {
		otherUser := seedGraphQLUser(t, ddb, "otheruser", "otheruser@example.com", "correctpass1")
		otherCtx := withGraphQLUserContext(context.Background(), otherUser)
		created, err := m.CreateUserToken(otherCtx, "other's token", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error creating token: %v", err)
		}
		result, err := m.RevokeUserToken(ctx, created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false when revoking another user's token")
		}
	})
}
