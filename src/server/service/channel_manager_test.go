package service

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

// setupChannelManagerTestDB creates an in-memory SQLite database with the
// notification_channels table used by ChannelManager. Follows the same
// file:NAME?mode=memory&cache=shared pattern used elsewhere in this package
// so pooled connections share one database, with a unique name per test.
func setupChannelManagerTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"_chan?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE notification_channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_type TEXT NOT NULL UNIQUE,
			channel_name TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT 0,
			state TEXT NOT NULL DEFAULT 'disabled',
			config TEXT,
			last_test_at DATETIME,
			last_test_result TEXT,
			last_success_at DATETIME,
			last_error TEXT,
			failure_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("failed to create notification_channels table: %v", err)
	}

	return db
}

// fakeChannel is a minimal NotificationChannel test double whose Send/Test
// behavior is controlled per test so we exercise ChannelManager's real
// success/failure branching (not just the mock's own logic).
type fakeChannel struct {
	channelType string
	name        string
	enabled     bool
	testErr     error
}

func (f *fakeChannel) GetType() string { return f.channelType }
func (f *fakeChannel) GetName() string { return f.name }
func (f *fakeChannel) IsEnabled() bool { return f.enabled }
func (f *fakeChannel) Send(recipient, subject, body string, metadata map[string]interface{}) error {
	return nil
}
func (f *fakeChannel) Test(recipient string) error                        { return f.testErr }
func (f *fakeChannel) ValidateConfig(config map[string]interface{}) error { return nil }

// TestChannelManager_NewChannelManager covers construction: the returned
// manager must have empty (not nil) channel registry and a usable smtp
// sub-service wired to the given db.
func TestChannelManager_NewChannelManager(t *testing.T) {
	db := setupChannelManagerTestDB(t)

	cm := NewChannelManager(db)
	if cm == nil {
		t.Fatal("NewChannelManager returned nil")
	}
	if cm.db != db {
		t.Error("db not wired correctly")
	}
	if cm.channels == nil {
		t.Error("channels map should be initialized, not nil")
	}
	if len(cm.channels) != 0 {
		t.Errorf("channels map should start empty, got %d entries", len(cm.channels))
	}
	if cm.smtp == nil {
		t.Error("smtp sub-service should be initialized")
	}

	list := cm.ListChannels()
	if len(list) != 0 {
		t.Errorf("ListChannels() on fresh manager = %v, want empty", list)
	}
}

// TestChannelManager_RegisterAndGetChannel covers registration, lookup of a
// registered channel, lookup of a missing channel (error path), and
// idempotent re-registration overwriting the same slot rather than growing.
func TestChannelManager_RegisterAndGetChannel(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)

	ch := &fakeChannel{channelType: "slack", name: "Slack"}
	cm.RegisterChannel(ch)

	got, err := cm.GetChannel("slack")
	if err != nil {
		t.Fatalf("unexpected error getting registered channel: %v", err)
	}
	if got != ch {
		t.Errorf("GetChannel returned %+v, want the registered instance %+v", got, ch)
	}

	// Missing channel error path.
	if _, err := cm.GetChannel("does-not-exist"); err == nil {
		t.Error("expected error for unregistered channel type")
	}

	// Idempotent re-registration: registering the same type again should
	// replace, not duplicate.
	ch2 := &fakeChannel{channelType: "slack", name: "Slack v2"}
	cm.RegisterChannel(ch2)

	if len(cm.channels) != 1 {
		t.Fatalf("re-registering same type should not grow the map, got %d entries", len(cm.channels))
	}
	got2, err := cm.GetChannel("slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got2 != ch2 {
		t.Error("re-registration should overwrite the previous channel instance")
	}
}

// TestChannelManager_ListChannels covers the empty case, a single
// registration, and multiple registrations without duplicates.
func TestChannelManager_ListChannels(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)

	if got := cm.ListChannels(); len(got) != 0 {
		t.Errorf("empty manager ListChannels() = %v, want empty", got)
	}

	cm.RegisterChannel(&fakeChannel{channelType: "slack", name: "Slack"})
	cm.RegisterChannel(&fakeChannel{channelType: "discord", name: "Discord"})
	cm.RegisterChannel(&fakeChannel{channelType: "discord", name: "Discord dup"})

	list := cm.ListChannels()
	if len(list) != 2 {
		t.Fatalf("ListChannels() = %v, want 2 unique entries", list)
	}

	seen := map[string]bool{}
	for _, ty := range list {
		seen[ty] = true
	}
	if !seen["slack"] || !seen["discord"] {
		t.Errorf("ListChannels() = %v, want to contain slack and discord", list)
	}
}

// TestChannelManager_InitializeChannels covers the full DB-backed happy path
// and, critically, idempotency: running InitializeChannels twice must not
// error or duplicate rows (INSERT only happens when a row doesn't exist).
func TestChannelManager_InitializeChannels(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)

	if err := cm.InitializeChannels(); err != nil {
		t.Fatalf("InitializeChannels() unexpected error: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM notification_channels").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != len(ChannelRegistry) {
		t.Fatalf("row count = %d, want %d (len(ChannelRegistry))", count, len(ChannelRegistry))
	}

	// All channels must be inserted disabled per spec.
	var enabledCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM notification_channels WHERE enabled = 1").Scan(&enabledCount); err != nil {
		t.Fatalf("failed to count enabled rows: %v", err)
	}
	if enabledCount != 0 {
		t.Errorf("enabled row count = %d, want 0 (all channels disabled by default)", enabledCount)
	}

	// Idempotency: calling again must not error or duplicate.
	if err := cm.InitializeChannels(); err != nil {
		t.Fatalf("second InitializeChannels() call unexpected error: %v", err)
	}
	var count2 int
	if err := db.QueryRow("SELECT COUNT(*) FROM notification_channels").Scan(&count2); err != nil {
		t.Fatalf("failed to count rows after second call: %v", err)
	}
	if count2 != count {
		t.Errorf("row count after re-running InitializeChannels = %d, want unchanged %d", count2, count)
	}
}

// TestChannelManager_InitializeChannels_QueryError covers the error path
// when the underlying table does not exist — the query itself must fail and
// InitializeChannels must propagate a wrapped error rather than panicking.
func TestChannelManager_InitializeChannels_QueryError(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.Name()+"_noschema?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	cm := NewChannelManager(db)
	if err := cm.InitializeChannels(); err == nil {
		t.Fatal("expected error when notification_channels table does not exist")
	}
}

// TestChannelManager_EnableDisableChannel covers the enable/disable state
// transitions, including idempotency of repeating the same transition.
func TestChannelManager_EnableDisableChannel(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)
	seedChannelRow(t, db, "webhook", "Generic Webhook")

	if err := cm.EnableChannel("webhook"); err != nil {
		t.Fatalf("EnableChannel unexpected error: %v", err)
	}
	state, err := cm.GetChannelState("webhook")
	if err != nil {
		t.Fatalf("GetChannelState unexpected error: %v", err)
	}
	if state != "enabled" {
		t.Errorf("state after EnableChannel = %q, want %q", state, "enabled")
	}

	// Idempotent repeat.
	if err := cm.EnableChannel("webhook"); err != nil {
		t.Fatalf("second EnableChannel unexpected error: %v", err)
	}
	if state, _ := cm.GetChannelState("webhook"); state != "enabled" {
		t.Errorf("state after repeated EnableChannel = %q, want %q", state, "enabled")
	}

	if err := cm.DisableChannel("webhook"); err != nil {
		t.Fatalf("DisableChannel unexpected error: %v", err)
	}
	state, err = cm.GetChannelState("webhook")
	if err != nil {
		t.Fatalf("GetChannelState unexpected error: %v", err)
	}
	if state != "disabled" {
		t.Errorf("state after DisableChannel = %q, want %q", state, "disabled")
	}
}

// TestChannelManager_EnableChannel_UnknownType covers the boundary where the
// channel_type does not exist in the DB: UPDATE affects zero rows but
// sql.Exec does not error in that case, so EnableChannel must return nil
// (matching the real driver semantics) rather than a fabricated error.
func TestChannelManager_EnableChannel_UnknownType(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)

	if err := cm.EnableChannel("does-not-exist"); err != nil {
		t.Errorf("EnableChannel on unknown type should not error (zero rows affected), got: %v", err)
	}
}

// TestChannelManager_GetChannelState_MissingRow covers the error path when
// querying state for a channel_type with no row.
func TestChannelManager_GetChannelState_MissingRow(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)

	_, err := cm.GetChannelState("missing")
	if err == nil {
		t.Fatal("expected error (sql.ErrNoRows) for missing channel_type")
	}
}

// TestChannelManager_UpdateChannelState covers direct state mutation and
// idempotency of setting the same state twice.
func TestChannelManager_UpdateChannelState(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)
	seedChannelRow(t, db, "pushover", "Pushover")

	for i := 0; i < 2; i++ {
		if err := cm.UpdateChannelState("pushover", "testing"); err != nil {
			t.Fatalf("iteration %d: UpdateChannelState unexpected error: %v", i, err)
		}
		state, err := cm.GetChannelState("pushover")
		if err != nil {
			t.Fatalf("iteration %d: GetChannelState unexpected error: %v", i, err)
		}
		if state != "testing" {
			t.Errorf("iteration %d: state = %q, want %q", i, state, "testing")
		}
	}
}

// TestChannelManager_TestChannel_Success covers the full success path of
// TestChannel: it must call the channel's Test method, then flip state to
// 'enabled' and set enabled=1 with failure_count reset.
func TestChannelManager_TestChannel_Success(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)
	seedChannelRow(t, db, "gotify", "Gotify")
	cm.RegisterChannel(&fakeChannel{channelType: "gotify", name: "Gotify", testErr: nil})

	if err := cm.TestChannel("gotify", "recipient@example.com"); err != nil {
		t.Fatalf("TestChannel unexpected error: %v", err)
	}

	stats, err := cm.GetChannelStats("gotify")
	if err != nil {
		t.Fatalf("GetChannelStats unexpected error: %v", err)
	}
	if stats["state"] != "enabled" {
		t.Errorf("state after successful test = %v, want %q", stats["state"], "enabled")
	}
	if stats["enabled"] != true {
		t.Errorf("enabled after successful test = %v, want true", stats["enabled"])
	}
	if stats["failure_count"] != 0 {
		t.Errorf("failure_count after successful test = %v, want 0", stats["failure_count"])
	}
}

// TestChannelManager_TestChannel_Failure covers the failure path: the
// channel's Test error must propagate back to the caller AND be recorded
// in the DB (state=failed, last_error set, failure_count incremented).
func TestChannelManager_TestChannel_Failure(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)
	seedChannelRow(t, db, "twilio", "Twilio SMS")
	wantErr := fmt.Errorf("boom: connection refused")
	cm.RegisterChannel(&fakeChannel{channelType: "twilio", name: "Twilio SMS", testErr: wantErr})

	err := cm.TestChannel("twilio", "+15555550100")
	if err == nil {
		t.Fatal("expected error to propagate from failing channel Test()")
	}
	if err.Error() != wantErr.Error() {
		t.Errorf("TestChannel error = %v, want %v", err, wantErr)
	}

	stats, statErr := cm.GetChannelStats("twilio")
	if statErr != nil {
		t.Fatalf("GetChannelStats unexpected error: %v", statErr)
	}
	if stats["state"] != "failed" {
		t.Errorf("state after failed test = %v, want %q", stats["state"], "failed")
	}
	if stats["last_error"] != wantErr.Error() {
		t.Errorf("last_error = %v, want %q", stats["last_error"], wantErr.Error())
	}
	if stats["failure_count"] != 1 {
		t.Errorf("failure_count after one failure = %v, want 1", stats["failure_count"])
	}
}

// TestChannelManager_TestChannel_UnregisteredChannel covers the error path
// where TestChannel is called for a type never registered in-memory (even if
// a DB row exists) — GetChannel's error must short-circuit before any DB
// mutation.
func TestChannelManager_TestChannel_UnregisteredChannel(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)
	seedChannelRow(t, db, "matrix", "Matrix")

	if err := cm.TestChannel("matrix", "someone"); err == nil {
		t.Fatal("expected error for channel not registered in-memory")
	}

	// State should remain untouched (still 'disabled' from seed).
	state, err := cm.GetChannelState("matrix")
	if err != nil {
		t.Fatalf("GetChannelState unexpected error: %v", err)
	}
	if state != "disabled" {
		t.Errorf("state after failed TestChannel lookup = %q, want unchanged %q", state, "disabled")
	}
}

// TestChannelManager_RecordSuccess_RecordFailure_ThresholdBehavior covers
// RecordSuccess resetting failure_count, RecordFailure incrementing it, and
// the SQL CASE boundary that flips state to 'failed' only once failure_count
// reaches 5 (and not before).
func TestChannelManager_RecordSuccess_RecordFailure_ThresholdBehavior(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)
	seedChannelRow(t, db, "opsgenie", "Opsgenie")

	// Four failures: state must stay whatever it was (still 'disabled' from seed).
	for i := 1; i <= 4; i++ {
		if err := cm.RecordFailure("opsgenie", fmt.Sprintf("error #%d", i)); err != nil {
			t.Fatalf("RecordFailure #%d unexpected error: %v", i, err)
		}
	}
	stats, err := cm.GetChannelStats("opsgenie")
	if err != nil {
		t.Fatalf("GetChannelStats unexpected error: %v", err)
	}
	if stats["failure_count"] != 4 {
		t.Errorf("failure_count after 4 failures = %v, want 4", stats["failure_count"])
	}
	if stats["state"] != "disabled" {
		t.Errorf("state after 4 failures (below threshold) = %v, want unchanged %q", stats["state"], "disabled")
	}

	// 5th failure crosses the threshold.
	if err := cm.RecordFailure("opsgenie", "error #5"); err != nil {
		t.Fatalf("RecordFailure #5 unexpected error: %v", err)
	}
	stats, err = cm.GetChannelStats("opsgenie")
	if err != nil {
		t.Fatalf("GetChannelStats unexpected error: %v", err)
	}
	if stats["failure_count"] != 5 {
		t.Errorf("failure_count after 5 failures = %v, want 5", stats["failure_count"])
	}
	if stats["state"] != "failed" {
		t.Errorf("state after reaching 5 failures = %v, want %q", stats["state"], "failed")
	}

	// RecordSuccess must reset failure_count (though it does not itself reset state).
	if err := cm.RecordSuccess("opsgenie"); err != nil {
		t.Fatalf("RecordSuccess unexpected error: %v", err)
	}
	stats, err = cm.GetChannelStats("opsgenie")
	if err != nil {
		t.Fatalf("GetChannelStats unexpected error: %v", err)
	}
	if stats["failure_count"] != 0 {
		t.Errorf("failure_count after RecordSuccess = %v, want 0", stats["failure_count"])
	}
	if stats["last_success_at"] == nil {
		t.Error("last_success_at should be set after RecordSuccess")
	}
}

// TestChannelManager_GetChannelStats_MissingRow covers the error path for a
// channel_type with no row in the database.
func TestChannelManager_GetChannelStats_MissingRow(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)

	if _, err := cm.GetChannelStats("nonexistent"); err == nil {
		t.Fatal("expected error for channel_type with no row")
	}
}

// TestChannelManager_GetChannelStats_NullFieldsHandled covers the boundary
// of a freshly-inserted channel row where last_test_at/last_success_at/
// last_error are all NULL — GetChannelStats must surface Go nil, not panic
// on the nullable scan targets.
func TestChannelManager_GetChannelStats_NullFieldsHandled(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)
	seedChannelRow(t, db, "fcm", "Firebase Cloud Messaging")

	stats, err := cm.GetChannelStats("fcm")
	if err != nil {
		t.Fatalf("GetChannelStats unexpected error: %v", err)
	}
	if stats["last_test_at"] != nil {
		t.Errorf("last_test_at = %v, want nil for unseen channel", stats["last_test_at"])
	}
	if stats["last_success_at"] != nil {
		t.Errorf("last_success_at = %v, want nil for unseen channel", stats["last_success_at"])
	}
	if stats["last_error"] != nil {
		t.Errorf("last_error = %v, want nil for unseen channel", stats["last_error"])
	}
}

// TestChannelManager_ListEnabledChannels covers empty result, a mix of
// enabled/disabled rows, and the enabled=1 AND state='enabled' filter
// boundary (a row can be enabled=1 but state != 'enabled', e.g. 'testing').
func TestChannelManager_ListEnabledChannels(t *testing.T) {
	db := setupChannelManagerTestDB(t)
	cm := NewChannelManager(db)

	// No rows at all yet.
	list, err := cm.ListEnabledChannels()
	if err != nil {
		t.Fatalf("ListEnabledChannels on empty table unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListEnabledChannels on empty table = %v, want empty", list)
	}

	seedChannelRow(t, db, "slack", "Slack")
	seedChannelRow(t, db, "discord", "Discord")
	seedChannelRow(t, db, "telegram", "Telegram")

	mustExec(t, db, "UPDATE notification_channels SET enabled=1, state='enabled' WHERE channel_type='slack'")
	mustExec(t, db, "UPDATE notification_channels SET enabled=1, state='testing' WHERE channel_type='discord'")
	// telegram left disabled/'disabled'.

	list, err = cm.ListEnabledChannels()
	if err != nil {
		t.Fatalf("ListEnabledChannels unexpected error: %v", err)
	}
	if len(list) != 1 || list[0] != "slack" {
		t.Errorf("ListEnabledChannels() = %v, want [slack] (enabled=1 AND state='enabled' only)", list)
	}
}

// seedChannelRow inserts a minimal disabled channel row, mirroring what
// InitializeChannels would produce, for tests that operate on a single
// channel_type without needing the full registry seeded.
func seedChannelRow(t *testing.T, db *sql.DB, channelType, name string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO notification_channels
		(channel_type, channel_name, enabled, state, config, created_at, updated_at)
		VALUES (?, ?, 0, 'disabled', '{}', datetime('now'), datetime('now'))
	`, channelType, name)
	if err != nil {
		t.Fatalf("failed to seed channel row %s: %v", channelType, err)
	}
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("failed to exec %q: %v", query, err)
	}
}
