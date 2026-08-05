package handler

import (
	"testing"
)

// TestNewAdminPasskeyHandler verifies the constructor wires the DB field
// as passed.
func TestNewAdminPasskeyHandler(t *testing.T) {
	db := newTestServerDB(t)
	h := NewAdminPasskeyHandler(db)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.DB != db {
		t.Error("expected DB field to be the passed *sql.DB")
	}
}

// TestLogAdminPasskeyAudit_InsertsRow verifies a successful audit write
// inserts a row into server_audit_log with the expected action, actor, and
// resource fields.
func TestLogAdminPasskeyAudit_InsertsRow(t *testing.T) {
	db := newTestServerDB(t)

	logAdminPasskeyAudit(db, "admin.passkey_added", 7, 42, "yubikey-1", "203.0.113.5", "test-agent")

	var action, actorID, resourceID, ipAddress string
	err := db.QueryRow(`
		SELECT action, actor_id, resource_id, ip_address
		FROM server_audit_log
		WHERE resource_type = 'admin_passkey'
	`).Scan(&action, &actorID, &resourceID, &ipAddress)
	if err != nil {
		t.Fatalf("expected an inserted audit row, query failed: %v", err)
	}

	if action != "admin.passkey_added" {
		t.Errorf("action = %q, want %q", action, "admin.passkey_added")
	}
	if actorID != "7" {
		t.Errorf("actor_id = %q, want %q", actorID, "7")
	}
	if resourceID != "42" {
		t.Errorf("resource_id = %q, want %q", resourceID, "42")
	}
	if ipAddress != "203.0.113.5" {
		t.Errorf("ip_address = %q, want %q", ipAddress, "203.0.113.5")
	}
}
