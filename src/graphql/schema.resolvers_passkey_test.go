package graphql

import (
	"context"
	"testing"
)

// withGraphQLPasskeyEnvelope attaches a request host/scheme to the context so
// graphQLPasskeyEnvelope(ctx) can build a WebAuthn config from it.
func withGraphQLPasskeyEnvelope(ctx context.Context, host string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyRequestHost, host)
	ctx = context.WithValue(ctx, ctxKeyRequestScheme, "https")
	return ctx
}

// --- BeginUserPasskeyRegistration -----------------------------------------

func TestMutationResolver_BeginUserPasskeyRegistration(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthenticated without a user in context", func(t *testing.T) {
		_, err := m.BeginUserPasskeyRegistration(context.Background(), "laptop", "correctpass1")
		if err == nil || err.Error() != "unauthorized: user not authenticated" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: user not authenticated")
		}
	})

	user := seedGraphQLUser(t, ddb, "passkeyowner", "passkeyowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("missing request host errors before touching passwords", func(t *testing.T) {
		_, err := m.BeginUserPasskeyRegistration(userCtx, "laptop", "correctpass1")
		if err == nil || err.Error() != "missing request host" {
			t.Fatalf("err = %v, want %q", err, "missing request host")
		}
	})

	envCtx := withGraphQLPasskeyEnvelope(userCtx, "example.com")

	t.Run("empty name and password rejected", func(t *testing.T) {
		_, err := m.BeginUserPasskeyRegistration(envCtx, "  ", "  ")
		if err == nil || err.Error() != "passkey name and password are required" {
			t.Fatalf("err = %v, want %q", err, "passkey name and password are required")
		}
	})

	t.Run("wrong password rejected", func(t *testing.T) {
		_, err := m.BeginUserPasskeyRegistration(envCtx, "laptop", "wrongpassword")
		if err == nil || err.Error() != "invalid password" {
			t.Fatalf("err = %v, want %q", err, "invalid password")
		}
	})

	t.Run("happy path generates registration options with no hardware", func(t *testing.T) {
		result, err := m.BeginUserPasskeyRegistration(envCtx, "laptop", "correctpass1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.CeremonyToken == "" {
			t.Fatalf("result = %+v, want a non-empty CeremonyToken", result)
		}
		if result.Options == nil {
			t.Fatal("expected non-nil Options")
		}
	})
}

// --- FinishUserPasskeyRegistration ----------------------------------------

func TestMutationResolver_FinishUserPasskeyRegistration(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthenticated without a user in context", func(t *testing.T) {
		_, err := m.FinishUserPasskeyRegistration(context.Background(), "token", map[string]any{})
		if err == nil || err.Error() != "unauthorized: user not authenticated" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: user not authenticated")
		}
	})

	user := seedGraphQLUser(t, ddb, "finishpasskeyowner", "finishpasskeyowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("missing request host errors", func(t *testing.T) {
		_, err := m.FinishUserPasskeyRegistration(userCtx, "token", map[string]any{})
		if err == nil || err.Error() != "missing request host" {
			t.Fatalf("err = %v, want %q", err, "missing request host")
		}
	})

	envCtx := withGraphQLPasskeyEnvelope(userCtx, "example.com")

	t.Run("unknown ceremony token rejected", func(t *testing.T) {
		_, err := m.FinishUserPasskeyRegistration(envCtx, "does-not-exist", map[string]any{})
		if err == nil || err.Error() != "passkey session expired" {
			t.Fatalf("err = %v, want %q", err, "passkey session expired")
		}
	})

	t.Run("empty ceremony token rejected", func(t *testing.T) {
		_, err := m.FinishUserPasskeyRegistration(envCtx, "", map[string]any{})
		if err == nil || err.Error() != "passkey session not found" {
			t.Fatalf("err = %v, want %q", err, "passkey session not found")
		}
	})

	// A genuine happy path requires a real WebAuthn authenticator response
	// (browser-generated attestation), which cannot be fixtured here, so it
	// is intentionally not covered by this unit test.
}

// --- DeleteUserPasskey -----------------------------------------------------

func TestMutationResolver_DeleteUserPasskey(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthenticated without a user in context", func(t *testing.T) {
		_, err := m.DeleteUserPasskey(context.Background(), "1")
		if err == nil || err.Error() != "unauthorized: user not authenticated" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: user not authenticated")
		}
	})

	user := seedGraphQLUser(t, ddb, "deletepasskeyowner", "deletepasskeyowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("invalid passkey id rejected", func(t *testing.T) {
		_, err := m.DeleteUserPasskey(userCtx, "not-a-number")
		if err == nil || err.Error() != "invalid passkey id" {
			t.Fatalf("err = %v, want %q", err, "invalid passkey id")
		}
	})

	t.Run("zero or negative passkey id rejected", func(t *testing.T) {
		_, err := m.DeleteUserPasskey(userCtx, "0")
		if err == nil || err.Error() != "invalid passkey id" {
			t.Fatalf("err = %v, want %q", err, "invalid passkey id")
		}
	})

	t.Run("deleting a nonexistent passkey errors", func(t *testing.T) {
		_, err := m.DeleteUserPasskey(userCtx, "999999")
		if err == nil {
			t.Fatal("expected an error deleting a passkey that does not exist")
		}
	})
}

// --- BeginUserPasskeyChallenge (public mutation) ---------------------------

func TestMutationResolver_BeginUserPasskeyChallenge(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("missing request host errors even with no session token", func(t *testing.T) {
		_, err := m.BeginUserPasskeyChallenge(context.Background(), nil)
		if err == nil || err.Error() != "missing request host" {
			t.Fatalf("err = %v, want %q", err, "missing request host")
		}
	})

	envCtx := withGraphQLPasskeyEnvelope(context.Background(), "example.com")

	t.Run("invalid pending session token rejected", func(t *testing.T) {
		bogus := "does-not-exist"
		_, err := m.BeginUserPasskeyChallenge(envCtx, &bogus)
		if err == nil {
			t.Fatal("expected an error for an unknown pending session token")
		}
	})

	t.Run("happy path discoverable login needs no prior session token or hardware", func(t *testing.T) {
		result, err := m.BeginUserPasskeyChallenge(envCtx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.CeremonyToken == "" {
			t.Fatalf("result = %+v, want a non-empty CeremonyToken", result)
		}
		if result.Options == nil {
			t.Fatal("expected non-nil Options")
		}
	})

	t.Run("blank pending session token behaves like no session token", func(t *testing.T) {
		blank := "   "
		result, err := m.BeginUserPasskeyChallenge(envCtx, &blank)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.CeremonyToken == "" {
			t.Fatalf("result = %+v, want a non-empty CeremonyToken", result)
		}
	})
}

// --- FinishUserPasskeyChallenge (public mutation) ---------------------------

func TestMutationResolver_FinishUserPasskeyChallenge(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("missing request host errors", func(t *testing.T) {
		_, err := m.FinishUserPasskeyChallenge(context.Background(), "token", map[string]any{})
		if err == nil || err.Error() != "missing request host" {
			t.Fatalf("err = %v, want %q", err, "missing request host")
		}
	})

	envCtx := withGraphQLPasskeyEnvelope(context.Background(), "example.com")

	// Note: json.Marshal(nil) yields the 4-byte literal "null", not an empty
	// slice, so the handler's len(body)==0 "invalid request body" guard is
	// unreachable from this resolver (credential is always JSON-encoded
	// before being passed down) and is not exercised here.

	t.Run("unknown ceremony token rejected", func(t *testing.T) {
		_, err := m.FinishUserPasskeyChallenge(envCtx, "does-not-exist", map[string]any{"id": "abc"})
		if err == nil || err.Error() != "passkey session expired" {
			t.Fatalf("err = %v, want %q", err, "passkey session expired")
		}
	})

	// A genuine happy path requires a real WebAuthn authenticator assertion,
	// which cannot be fixtured here, so it is intentionally not covered by
	// this unit test.
}
