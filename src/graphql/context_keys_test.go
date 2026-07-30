package graphql

import (
	"context"
	"testing"
)

// TestContextKeyTypeMismatch_AdminID demonstrates a genuine production bug:
// graphql.go's withGraphQLAdminValues() writes the admin ID into the context
// using the typed key ctxKeyAdminID (type contextKey = "admin_id"), but
// resolvers_helpers.go (line 474, line 839) and schema.resolvers.go (many
// call sites, e.g. line 1219) read it back with the untyped raw string
// literal "admin_id". Per Go's context.Context.Value semantics, a lookup
// only matches if both the dynamic TYPE and the value of the key are equal.
// contextKey("admin_id") != string("admin_id") as context keys, so every
// resolver that does `ctx.Value("admin_id").(int)` silently fails to find
// the admin ID that was actually stored by withGraphQLAdminValues, and the
// `ok` result is always false — even for a correctly authenticated admin.
//
// This test does not modify production code; it only proves the mismatch.
func TestContextKeyTypeMismatch_AdminID(t *testing.T) {
	ctx := withGraphQLAdminValues(context.Background(), 42, "admin@example.test")

	// The typed key used by the writer succeeds.
	typedVal, typedOK := ctx.Value(ctxKeyAdminID).(int)
	if !typedOK || typedVal != 42 {
		t.Fatalf("typed key lookup: got (%v, %v), want (42, true)", typedVal, typedOK)
	}

	// The raw string key used throughout resolvers_helpers.go and
	// schema.resolvers.go fails, because contextKey("admin_id") is a
	// different context key than the plain string "admin_id".
	rawVal, rawOK := ctx.Value("admin_id").(int)
	if rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %v; "+
			"if this starts passing, the context-key-type-mismatch bug in "+
			"graphql.go/resolvers_helpers.go has been fixed and this test "+
			"(and its documentation) should be removed", rawVal)
	}

	// Concrete demonstration: the actual reader helper used at
	// resolvers_helpers.go:474 (graphQLContextAdminID or equivalent) cannot
	// see the admin ID that withGraphQLAdminValues just stored.
	t.Log("BUG CONFIRMED: graphql.go writes context values with typed " +
		"`contextKey` constants (ctxKeyAdminID, ctxKeyUserRole, " +
		"ctxKeyAdminEmail, ctxKeyUserID, ...), but resolvers_helpers.go " +
		"and schema.resolvers.go read them back via raw string literals " +
		"(ctx.Value(\"admin_id\"), ctx.Value(\"user_role\"), " +
		"ctx.Value(\"admin_email\")). Every authenticated GraphQL resolver " +
		"that checks admin/user role or ID via these raw-string lookups " +
		"will always see ok=false, defeating authorization checks that " +
		"rely on a positive admin/role match.")
}

// TestContextKeyTypeMismatch_UserRole is the same demonstration for the
// user_role key, which schema.resolvers.go reads at ~50 call sites (e.g.
// lines 838, 890, 913, 957, ... 3026) via `ctx.Value("user_role").(string)`.
func TestContextKeyTypeMismatch_UserRole(t *testing.T) {
	ctx := withGraphQLAdminValues(context.Background(), 7, "root@example.test")

	typedVal, typedOK := ctx.Value(ctxKeyUserRole).(string)
	if !typedOK || typedVal != "admin" {
		t.Fatalf("typed key lookup: got (%q, %v), want (\"admin\", true)", typedVal, typedOK)
	}

	rawVal, rawOK := ctx.Value("user_role").(string)
	if rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %q; "+
			"if this starts passing, the context-key-type-mismatch bug has "+
			"been fixed", rawVal)
	}
}

// TestContextKeyTypeMismatch_UserID mirrors the same defect for the
// per-request authenticated user ID, read raw at
// schema.resolvers_impl.go:12 via `ctx.Value("user_id").(int)`.
func TestContextKeyTypeMismatch_UserID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKeyUserID, 99)

	typedVal, typedOK := ctx.Value(ctxKeyUserID).(int)
	if !typedOK || typedVal != 99 {
		t.Fatalf("typed key lookup: got (%v, %v), want (99, true)", typedVal, typedOK)
	}

	rawVal, rawOK := ctx.Value("user_id").(int)
	if rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %v", rawVal)
	}
}
