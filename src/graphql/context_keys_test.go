package graphql

import (
	"context"
	"testing"
)

// TestContextKeyTypeMatch_AdminID is a regression test for a previously
// real production bug: graphql.go's withGraphQLAdminValues() writes the
// admin ID into the context using the typed key ctxKeyAdminID, but
// resolvers_helpers.go and schema.resolvers.go used to read it back with
// the untyped raw string literal "admin_id". Per Go's context.Context.Value
// semantics, a lookup only matches if both the dynamic TYPE and the value
// of the key are equal, so contextKey("admin_id") != string("admin_id") as
// context keys — every resolver doing `ctx.Value("admin_id").(int)` always
// found ok=false, even for a correctly authenticated admin. All call sites
// now read via the typed ctxKeyAdminID constant; this test guards against
// the mismatch being reintroduced.
func TestContextKeyTypeMatch_AdminID(t *testing.T) {
	ctx := withGraphQLAdminValues(context.Background(), 42, "admin@example.test")

	typedVal, typedOK := ctx.Value(ctxKeyAdminID).(int)
	if !typedOK || typedVal != 42 {
		t.Fatalf("typed key lookup: got (%v, %v), want (42, true)", typedVal, typedOK)
	}

	if rawVal, rawOK := ctx.Value("admin_id").(int); rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %v; "+
			"production code must read admin_id via ctxKeyAdminID only", rawVal)
	}
}

// TestContextKeyTypeMatch_UserRole is the same regression guard for the
// user_role key, which schema.resolvers.go reads via ctxKeyUserRole at
// every authorization check.
func TestContextKeyTypeMatch_UserRole(t *testing.T) {
	ctx := withGraphQLAdminValues(context.Background(), 7, "root@example.test")

	typedVal, typedOK := ctx.Value(ctxKeyUserRole).(string)
	if !typedOK || typedVal != "admin" {
		t.Fatalf("typed key lookup: got (%q, %v), want (\"admin\", true)", typedVal, typedOK)
	}

	if rawVal, rawOK := ctx.Value("user_role").(string); rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %q; "+
			"production code must read user_role via ctxKeyUserRole only", rawVal)
	}
}

// TestContextKeyTypeMatch_UserID is the same regression guard for the
// per-request authenticated user ID, read via ctxKeyUserID in
// schema.resolvers_impl.go's getUserIDFromContext.
func TestContextKeyTypeMatch_UserID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKeyUserID, 99)

	typedVal, typedOK := ctx.Value(ctxKeyUserID).(int)
	if !typedOK || typedVal != 99 {
		t.Fatalf("typed key lookup: got (%v, %v), want (99, true)", typedVal, typedOK)
	}

	if rawVal, rawOK := ctx.Value("user_id").(int); rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %v", rawVal)
	}
}

// TestContextKeyTypeMatch_ClientIP is the same regression guard for the
// client IP, read via ctxKeyClientIP in getIPFromContext and directly in
// schema.resolvers.go.
func TestContextKeyTypeMatch_ClientIP(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKeyClientIP, "203.0.113.5")

	typedVal, typedOK := ctx.Value(ctxKeyClientIP).(string)
	if !typedOK || typedVal != "203.0.113.5" {
		t.Fatalf("typed key lookup: got (%q, %v), want (\"203.0.113.5\", true)", typedVal, typedOK)
	}

	if rawVal, rawOK := ctx.Value("client_ip").(string); rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %q", rawVal)
	}
}

// TestContextKeyTypeMatch_AdminEmail is the same regression guard for the
// admin email, read via ctxKeyAdminEmail in resolvers_helpers.go.
func TestContextKeyTypeMatch_AdminEmail(t *testing.T) {
	ctx := withGraphQLAdminValues(context.Background(), 7, "root@example.test")

	typedVal, typedOK := ctx.Value(ctxKeyAdminEmail).(string)
	if !typedOK || typedVal != "root@example.test" {
		t.Fatalf("typed key lookup: got (%q, %v), want (\"root@example.test\", true)", typedVal, typedOK)
	}

	if rawVal, rawOK := ctx.Value("admin_email").(string); rawOK {
		t.Fatalf("raw string key lookup unexpectedly succeeded with value %q", rawVal)
	}
}
