package database

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFlushHotCounterUpdatesCoalescesWithinTransaction(t *testing.T) {
	db, err := sql.Open("sqlite", "file:hot-counter-queue?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE activity (id TEXT PRIMARY KEY, last_used TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO activity (id) VALUES ('session-1')`); err != nil {
		t.Fatal(err)
	}

	if err := QueueHotCounterUpdate(db, "session-1", `UPDATE activity SET last_used = ? WHERE id = ?`, "first", "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := QueueHotCounterUpdate(db, "session-1", `UPDATE activity SET last_used = ? WHERE id = ?`, "latest", "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := FlushHotCounterUpdates(context.Background()); err != nil {
		t.Fatal(err)
	}

	var lastUsed string
	if err := db.QueryRow(`SELECT last_used FROM activity WHERE id = 'session-1'`).Scan(&lastUsed); err != nil {
		t.Fatal(err)
	}
	if lastUsed != "latest" {
		t.Fatalf("last_used = %q, want latest coalesced value", lastUsed)
	}
	if err := FlushHotCounterUpdates(context.Background()); err != nil {
		t.Fatalf("second empty flush: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_activity_update BEFORE UPDATE ON activity BEGIN SELECT RAISE(ABORT, 'forced update failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := QueueHotCounterUpdate(db, "session-1", `UPDATE activity SET last_used = ? WHERE id = ?`, "retry", "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := FlushHotCounterUpdates(context.Background()); err == nil {
		t.Fatal("FlushHotCounterUpdates succeeded with a rejecting trigger")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_activity_update`); err != nil {
		t.Fatal(err)
	}
	if err := FlushHotCounterUpdates(context.Background()); err != nil {
		t.Fatalf("flush restored hot-counter update: %v", err)
	}
	if err := StopHotCounterFlusher(context.Background()); err != nil {
		t.Fatalf("StopHotCounterFlusher: %v", err)
	}
}

func TestQueueHotCounterUpdateRejectsInvalidInput(t *testing.T) {
	if err := QueueHotCounterUpdate(nil, "row", "UPDATE activity SET last_used = ?", "now"); err == nil {
		t.Fatal("QueueHotCounterUpdate accepted a nil database")
	}
	if err := QueueHotCounterUpdate(&sql.DB{}, "", "UPDATE activity SET last_used = ?", "now"); err == nil {
		t.Fatal("QueueHotCounterUpdate accepted an empty key")
	}
}
