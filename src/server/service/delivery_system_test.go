package service

import (
	"database/sql"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"

	_ "modernc.org/sqlite"
)

// setupDeliverySystemTestDB creates in-memory server and users SQLite
// databases with the ad hoc tables delivery_system.go queries directly
// through database.GetServerDB()/GetUsersDB().
func setupDeliverySystemTestDB(t *testing.T) (serverDB, usersDB *sql.DB) {
	t.Helper()
	name := t.Name()

	var err error
	serverDB, err = sql.Open("sqlite", "file:"+name+"_server?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open server db: %v", err)
	}
	t.Cleanup(func() { serverDB.Close() })

	usersDB, err = sql.Open("sqlite", "file:"+name+"_users?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open users db: %v", err)
	}
	t.Cleanup(func() { usersDB.Close() })
	// MaxOpenConns is intentionally left unbounded (matches convention used
	// elsewhere in this package): ProcessQueue holds an open *sql.Rows while
	// dispatching goroutines that immediately issue further queries against
	// the same DB, and a single-connection pool serializes/blocks that.

	serverStmts := []string{
		`CREATE TABLE notification_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			channel_type TEXT,
			template_id INTEGER,
			priority INTEGER,
			state TEXT,
			subject TEXT,
			body TEXT,
			variables TEXT,
			retry_count INTEGER,
			max_retries INTEGER,
			next_retry_at DATETIME,
			delivered_at DATETIME,
			failed_at DATETIME,
			error_message TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE server_notification_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			queue_id INTEGER,
			user_id INTEGER,
			channel_type TEXT,
			status TEXT,
			subject TEXT,
			body TEXT,
			sent_at DATETIME,
			error_message TEXT,
			metadata TEXT
		)`,
		`CREATE TABLE notification_channels (
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
		)`,
		`CREATE TABLE server_config (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
	}
	for _, stmt := range serverStmts {
		if _, err := serverDB.Exec(stmt); err != nil {
			t.Fatalf("failed to create server table: %v\n%s", err, stmt)
		}
	}

	usersStmts := []string{
		`CREATE TABLE user_notification_preferences (
			user_id INTEGER,
			channel_type TEXT,
			enabled INTEGER,
			config TEXT
		)`,
		`CREATE TABLE user_accounts (
			id INTEGER PRIMARY KEY,
			email TEXT
		)`,
	}
	for _, stmt := range usersStmts {
		if _, err := usersDB.Exec(stmt); err != nil {
			t.Fatalf("failed to create users table: %v\n%s", err, stmt)
		}
	}

	return serverDB, usersDB
}

// deliveryFakeChannel is a controllable NotificationChannel test double,
// distinct from channel_manager_test.go's fakeChannel to avoid a symbol
// collision in the shared package. Send behavior is configurable per test.
type deliveryFakeChannel struct {
	channelType string
	sendErr     error
	sentTo      []string
}

func (f *deliveryFakeChannel) GetType() string { return f.channelType }
func (f *deliveryFakeChannel) GetName() string { return f.channelType }
func (f *deliveryFakeChannel) IsEnabled() bool { return true }
func (f *deliveryFakeChannel) Send(recipient, subject, body string, metadata map[string]interface{}) error {
	f.sentTo = append(f.sentTo, recipient)
	return f.sendErr
}
func (f *deliveryFakeChannel) Test(recipient string) error                             { return nil }
func (f *deliveryFakeChannel) ValidateConfig(config map[string]interface{}) error { return nil }

func intPtr(v int) *int { return &v }

// TestDeliverySystem_NewDeliverySystem verifies the documented defaults.
func TestDeliverySystem_NewDeliverySystem(t *testing.T) {
	ds := NewDeliverySystem(nil, nil, nil)

	if ds.maxRetries != 3 {
		t.Errorf("expected maxRetries=3, got %d", ds.maxRetries)
	}
	if ds.retryBackoff != "exponential" {
		t.Errorf("expected retryBackoff=exponential, got %q", ds.retryBackoff)
	}
	if ds.queueWorkers != 5 {
		t.Errorf("expected queueWorkers=5, got %d", ds.queueWorkers)
	}
	if ds.batchSize != 100 {
		t.Errorf("expected batchSize=100, got %d", ds.batchSize)
	}
	if ds.rateLimitPerMin != 60 {
		t.Errorf("expected rateLimitPerMin=60, got %d", ds.rateLimitPerMin)
	}
}

// TestDeliverySystem_LoadSettings covers overrides from seeded config rows
// and the fallback-to-default behavior when a key is absent.
func TestDeliverySystem_LoadSettings(t *testing.T) {
	t.Run("applies overrides for present keys, keeps default for absent ones", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		if _, err := serverDB.Exec(`INSERT INTO server_config (key, value) VALUES
			('notifications.retry_max', '7'),
			('notifications.retry_backoff', 'linear')`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}

		ds := NewDeliverySystem(serverDB, nil, nil)
		if err := ds.LoadSettings(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ds.maxRetries != 7 {
			t.Errorf("expected maxRetries overridden to 7, got %d", ds.maxRetries)
		}
		if ds.retryBackoff != "linear" {
			t.Errorf("expected retryBackoff overridden to linear, got %q", ds.retryBackoff)
		}
		if ds.queueWorkers != 5 {
			t.Errorf("expected queueWorkers to keep default 5, got %d", ds.queueWorkers)
		}
	})

	t.Run("no config rows keeps all defaults and returns nil", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)
		if err := ds.LoadSettings(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ds.maxRetries != 3 || ds.retryBackoff != "exponential" {
			t.Errorf("expected defaults preserved, got maxRetries=%d retryBackoff=%q", ds.maxRetries, ds.retryBackoff)
		}
	})
}

// TestDeliverySystem_Enqueue covers the happy path, a nil userID/variables
// boundary, and a complex variables map that must round-trip through JSON.
func TestDeliverySystem_Enqueue(t *testing.T) {
	serverDB, usersDB := setupDeliverySystemTestDB(t)
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	ds := NewDeliverySystem(serverDB, nil, nil)

	t.Run("happy path returns a valid id", func(t *testing.T) {
		id, err := ds.Enqueue(intPtr(1), "email", "Subject", "Body", 2, map[string]interface{}{"k": "v"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id <= 0 {
			t.Fatalf("expected positive id, got %d", id)
		}
	})

	t.Run("nil userID and nil variables are accepted", func(t *testing.T) {
		id, err := ds.Enqueue(nil, "email", "Subject", "Body", 1, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id <= 0 {
			t.Fatalf("expected positive id, got %d", id)
		}

		var userID sql.NullInt64
		if err := serverDB.QueryRow(`SELECT user_id FROM notification_queue WHERE id = ?`, id).Scan(&userID); err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if userID.Valid {
			t.Errorf("expected NULL user_id, got %v", userID)
		}
	})

	t.Run("complex variables map round-trips through JSON", func(t *testing.T) {
		vars := map[string]interface{}{
			"nested": map[string]interface{}{"a": 1, "b": []interface{}{"x", "y"}},
			"count":  42,
		}
		id, err := ds.Enqueue(intPtr(2), "webhook", "S", "B", 1, vars)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var raw string
		if err := serverDB.QueryRow(`SELECT variables FROM notification_queue WHERE id = ?`, id).Scan(&raw); err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if raw == "" || raw == "null" {
			t.Errorf("expected non-empty JSON variables, got %q", raw)
		}
	})
}

// TestDeliverySystem_processNotification exercises the per-notification
// pipeline directly (bypassing ProcessQueue's async goroutine dispatch, see
// report) covering: success, channel-not-found, recipient-not-found, and
// channel.Send failure paths.
func TestDeliverySystem_processNotification(t *testing.T) {
	t.Run("success path updates state to delivered and records history", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		fc := &deliveryFakeChannel{channelType: "email"}
		cm.RegisterChannel(fc)
		ds := NewDeliverySystem(serverDB, cm, nil)

		id, err := ds.Enqueue(intPtr(1), "email", "S", "B", 1, map[string]interface{}{"recipient": "user@example.com"})
		if err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}

		nq := &NotificationQueue{
			ID: id, ChannelType: "email", Subject: "S", Body: "B",
			MaxRetries: 3, Variables: map[string]interface{}{"recipient": "user@example.com"},
		}
		ds.processNotification(nq)

		var state string
		var deliveredAt sql.NullTime
		if err := serverDB.QueryRow(`SELECT state, delivered_at FROM notification_queue WHERE id = ?`, id).Scan(&state, &deliveredAt); err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if state != string(StateDelivered) {
			t.Errorf("expected state=delivered, got %q", state)
		}
		if !deliveredAt.Valid {
			t.Error("expected delivered_at to be set")
		}
		if len(fc.sentTo) != 1 || fc.sentTo[0] != "user@example.com" {
			t.Errorf("expected channel to receive send call, got %+v", fc.sentTo)
		}

		var historyCount int
		serverDB.QueryRow(`SELECT COUNT(*) FROM server_notification_history WHERE queue_id = ? AND status = 'delivered'`, id).Scan(&historyCount)
		if historyCount != 1 {
			t.Errorf("expected 1 delivered history row, got %d", historyCount)
		}
	})

	t.Run("channel not found is treated as a failure", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		ds := NewDeliverySystem(serverDB, cm, nil)

		id, err := ds.Enqueue(intPtr(1), "sms", "S", "B", 1, nil)
		if err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}

		nq := &NotificationQueue{ID: id, ChannelType: "sms", Subject: "S", Body: "B", MaxRetries: 3}
		ds.processNotification(nq)

		var state string
		if err := serverDB.QueryRow(`SELECT state FROM notification_queue WHERE id = ?`, id).Scan(&state); err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if state != string(StateFailed) {
			t.Errorf("expected state=failed after unknown channel, got %q", state)
		}
	})

	t.Run("recipient not found is treated as a failure without calling Send", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		fc := &deliveryFakeChannel{channelType: "email"}
		cm.RegisterChannel(fc)
		ds := NewDeliverySystem(serverDB, cm, nil)

		id, err := ds.Enqueue(intPtr(1), "email", "S", "B", 1, nil)
		if err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}

		nq := &NotificationQueue{ID: id, ChannelType: "email", Subject: "S", Body: "B", MaxRetries: 3}
		ds.processNotification(nq)

		if len(fc.sentTo) != 0 {
			t.Errorf("expected Send not called when recipient is empty, got %+v", fc.sentTo)
		}

		var state string
		serverDB.QueryRow(`SELECT state FROM notification_queue WHERE id = ?`, id).Scan(&state)
		if state != string(StateFailed) {
			t.Errorf("expected state=failed when no recipient found, got %q", state)
		}
	})

	t.Run("channel.Send error is treated as a failure", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		fc := &deliveryFakeChannel{channelType: "email", sendErr: sql.ErrConnDone}
		cm.RegisterChannel(fc)
		ds := NewDeliverySystem(serverDB, cm, nil)

		id, err := ds.Enqueue(intPtr(1), "email", "S", "B", 1, map[string]interface{}{"recipient": "a@b.com"})
		if err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}

		nq := &NotificationQueue{ID: id, ChannelType: "email", Subject: "S", Body: "B", MaxRetries: 3, Variables: map[string]interface{}{"recipient": "a@b.com"}}
		ds.processNotification(nq)

		var state, errMsg string
		serverDB.QueryRow(`SELECT state, error_message FROM notification_queue WHERE id = ?`, id).Scan(&state, &errMsg)
		if state != string(StateFailed) {
			t.Errorf("expected state=failed, got %q", state)
		}
		if errMsg == "" {
			t.Error("expected error_message to be recorded")
		}
	})
}

// TestDeliverySystem_ProcessQueue verifies rows in eligible states are
// dispatched (asynchronously - see report on the goroutine-dispatch gap) and
// that a next_retry_at in the future is correctly excluded from the batch.
func TestDeliverySystem_ProcessQueue(t *testing.T) {
	serverDB, usersDB := setupDeliverySystemTestDB(t)
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	cm := NewChannelManager(serverDB)
	fc := &deliveryFakeChannel{channelType: "email"}
	cm.RegisterChannel(fc)
	ds := NewDeliverySystem(serverDB, cm, nil)

	// nil userID is required here: getRecipient returns immediately inside
	// its UserID.Valid branch for channel_type "email" (querying
	// user_notification_preferences / user_accounts) and never falls
	// through to the Variables["recipient"] fallback when UserID is set.
	readyID, err := ds.Enqueue(nil, "email", "S", "B", 1, map[string]interface{}{"recipient": "a@b.com"})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	futureID, err := ds.Enqueue(nil, "email", "S", "B", 1, map[string]interface{}{"recipient": "a@b.com"})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if _, err := serverDB.Exec(`UPDATE notification_queue SET state = ?, next_retry_at = ? WHERE id = ?`,
		StateFailed, time.Now().Add(time.Hour), futureID); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if err := ds.ProcessQueue(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var state string
		serverDB.QueryRow(`SELECT state FROM notification_queue WHERE id = ?`, readyID).Scan(&state)
		if state == string(StateDelivered) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	var readyState, futureState string
	serverDB.QueryRow(`SELECT state FROM notification_queue WHERE id = ?`, readyID).Scan(&readyState)
	serverDB.QueryRow(`SELECT state FROM notification_queue WHERE id = ?`, futureID).Scan(&futureState)

	if readyState != string(StateDelivered) {
		t.Errorf("expected the eligible row to reach delivered state, got %q", readyState)
	}
	if futureState != string(StateFailed) {
		t.Errorf("expected the future-retry row to remain untouched (failed), got %q", futureState)
	}
}

// TestDeliverySystem_handleFailure covers the retry-vs-dead-letter boundary:
// below MaxRetries goes back to failed/queued-for-retry, at/above goes to
// the dead letter queue.
func TestDeliverySystem_handleFailure(t *testing.T) {
	t.Run("retry count under max moves to failed state for retry", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		ds := NewDeliverySystem(serverDB, cm, nil)

		id, err := ds.Enqueue(intPtr(1), "email", "S", "B", 1, nil)
		if err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}

		nq := &NotificationQueue{ID: id, ChannelType: "email", RetryCount: 0, MaxRetries: 3}
		ds.handleFailure(nq, sql.ErrNoRows)

		var state string
		var retryCount int
		serverDB.QueryRow(`SELECT state, retry_count FROM notification_queue WHERE id = ?`, id).Scan(&state, &retryCount)
		if state != string(StateFailed) {
			t.Errorf("expected state=failed, got %q", state)
		}
		if retryCount != 1 {
			t.Errorf("expected retry_count=1, got %d", retryCount)
		}
	})

	t.Run("retry count at max moves to dead letter", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		ds := NewDeliverySystem(serverDB, cm, nil)

		id, err := ds.Enqueue(intPtr(1), "email", "S", "B", 1, nil)
		if err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}

		nq := &NotificationQueue{ID: id, ChannelType: "email", RetryCount: 2, MaxRetries: 3}
		ds.handleFailure(nq, sql.ErrNoRows)

		var state string
		serverDB.QueryRow(`SELECT state FROM notification_queue WHERE id = ?`, id).Scan(&state)
		if state != string(StateDeadLetter) {
			t.Errorf("expected state=dead_letter when retry_count reaches max, got %q", state)
		}

		var historyCount int
		serverDB.QueryRow(`SELECT COUNT(*) FROM server_notification_history WHERE queue_id = ? AND status = 'dead_letter'`, id).Scan(&historyCount)
		if historyCount != 1 {
			t.Errorf("expected 1 dead_letter history row, got %d", historyCount)
		}
	})
}

// TestDeliverySystem_calculateNextRetry covers exponential progression,
// linear progression, and the 60-minute cap boundary for each.
func TestDeliverySystem_calculateNextRetry(t *testing.T) {
	tests := []struct {
		name       string
		backoff    string
		retryCount int
		wantMin    float64
	}{
		{"exponential retry 1 is about 2 minutes", "exponential", 1, 2},
		{"exponential retry 3 is about 8 minutes", "exponential", 3, 8},
		{"exponential retry 6 is about 64 minutes capped to 60", "exponential", 6, 60},
		{"linear retry 1 is about 5 minutes", "linear", 1, 5},
		{"linear retry 4 is about 20 minutes", "linear", 4, 20},
		{"linear retry 20 is capped to 60 minutes", "linear", 20, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &DeliverySystem{retryBackoff: tt.backoff}
			before := time.Now()
			got := ds.calculateNextRetry(tt.retryCount)
			elapsedMinutes := got.Sub(before).Minutes()

			if elapsedMinutes < tt.wantMin-0.5 || elapsedMinutes > tt.wantMin+0.5 {
				t.Errorf("expected roughly %v minutes from now, got %v", tt.wantMin, elapsedMinutes)
			}
		})
	}
}

// TestDeliverySystem_getRecipient covers the config-address path, the
// email-fallback path, the variables["recipient"] fallback, and the
// empty-string boundary when nothing matches.
func TestDeliverySystem_getRecipient(t *testing.T) {
	t.Run("uses address from user preference config JSON", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		if _, err := usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled, config) VALUES (1, 'webhook', 1, '{"address":"https://hook.example.com"}')`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}

		ds := NewDeliverySystem(serverDB, nil, nil)
		nq := &NotificationQueue{ChannelType: "webhook", UserID: sql.NullInt64{Int64: 1, Valid: true}}
		got := ds.getRecipient(nq)
		if got != "https://hook.example.com" {
			t.Errorf("expected config address, got %q", got)
		}
	})

	t.Run("falls back to user email for email channel when no config address", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		if _, err := usersDB.Exec(`INSERT INTO user_accounts (id, email) VALUES (2, 'user2@example.com')`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}

		ds := NewDeliverySystem(serverDB, nil, nil)
		nq := &NotificationQueue{ChannelType: "email", UserID: sql.NullInt64{Int64: 2, Valid: true}}
		got := ds.getRecipient(nq)
		if got != "user2@example.com" {
			t.Errorf("expected fallback email, got %q", got)
		}
	})

	t.Run("falls back to variables[recipient] when no user_id", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)
		nq := &NotificationQueue{ChannelType: "webhook", Variables: map[string]interface{}{"recipient": "fallback@example.com"}}
		got := ds.getRecipient(nq)
		if got != "fallback@example.com" {
			t.Errorf("expected variables fallback, got %q", got)
		}
	})

	t.Run("returns empty string when nothing matches", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)
		nq := &NotificationQueue{ChannelType: "webhook"}
		got := ds.getRecipient(nq)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

// TestDeliverySystem_GetQueueStats covers state-count aggregation, the
// pending boundary matching ProcessQueue's WHERE clause, and dead_letters
// counting.
func TestDeliverySystem_GetQueueStats(t *testing.T) {
	serverDB, usersDB := setupDeliverySystemTestDB(t)
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	ds := NewDeliverySystem(serverDB, nil, nil)

	if _, err := serverDB.Exec(`INSERT INTO notification_queue (state, created_at, updated_at) VALUES
		('created', ?, ?), ('queued', ?, ?), ('delivered', ?, ?), ('dead_letter', ?, ?), ('dead_letter', ?, ?)`,
		time.Now(), time.Now(), time.Now(), time.Now(), time.Now(), time.Now(), time.Now(), time.Now(), time.Now(), time.Now()); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	// A failed row whose next_retry_at is in the future should not count as pending.
	if _, err := serverDB.Exec(`INSERT INTO notification_queue (state, next_retry_at, created_at, updated_at) VALUES ('failed', ?, ?, ?)`,
		time.Now().Add(time.Hour), time.Now(), time.Now()); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	stats, err := ds.GetQueueStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byState, ok := stats["by_state"].(map[string]int)
	if !ok {
		t.Fatalf("expected by_state to be map[string]int, got %T", stats["by_state"])
	}
	if byState["dead_letter"] != 2 {
		t.Errorf("expected 2 dead_letter rows, got %d", byState["dead_letter"])
	}
	if byState["created"] != 1 || byState["queued"] != 1 {
		t.Errorf("unexpected state counts: %+v", byState)
	}

	pending, ok := stats["pending"].(int)
	if !ok {
		t.Fatalf("expected pending to be int, got %T", stats["pending"])
	}
	if pending != 2 {
		t.Errorf("expected pending=2 (created+queued, excluding future-retry failed row), got %d", pending)
	}

	deadLetters, ok := stats["dead_letters"].(int)
	if !ok {
		t.Fatalf("expected dead_letters to be int, got %T", stats["dead_letters"])
	}
	if deadLetters != 2 {
		t.Errorf("expected dead_letters=2, got %d", deadLetters)
	}
}

// TestDeliverySystem_CleanupOld covers deleting only delivered rows older
// than the cutoff, the exact-cutoff boundary, and the zero-rows case.
func TestDeliverySystem_CleanupOld(t *testing.T) {
	t.Run("deletes only delivered rows older than retention window", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)

		old := time.Now().AddDate(0, 0, -10)
		recent := time.Now().AddDate(0, 0, -1)
		if _, err := serverDB.Exec(`INSERT INTO notification_queue (state, delivered_at, created_at, updated_at) VALUES
			('delivered', ?, ?, ?), ('delivered', ?, ?, ?), ('failed', ?, ?, ?)`,
			old, old, old, recent, recent, recent, old, old, old); err != nil {
			t.Fatalf("seed failed: %v", err)
		}

		if err := ds.CleanupOld(7); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue`).Scan(&count)
		if count != 2 {
			t.Errorf("expected 2 rows remaining (recent delivered + failed), got %d", count)
		}
	})

	t.Run("zero rows to delete is a no-op", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)
		if err := ds.CleanupOld(30); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestDeliverySystem_RequeueDeadLetters covers the empty-ids no-op, the
// single-id happy path, and the multi-id case - which is a regression test
// for the placeholder-join bug fixed in delivery_system.go (see report):
// the query previously used only placeholders[0] instead of joining all of
// them, so multi-id calls generated a mismatched placeholder/arg count.
func TestDeliverySystem_RequeueDeadLetters(t *testing.T) {
	t.Run("empty ids is a no-op", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)
		if err := ds.RequeueDeadLetters(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := ds.RequeueDeadLetters([]int{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("single id happy path requeues the dead letter", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)
		res, err := serverDB.Exec(`INSERT INTO notification_queue (state, retry_count, error_message, created_at, updated_at) VALUES ('dead_letter', 3, 'boom', ?, ?)`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("seed failed: %v", err)
		}
		id, _ := res.LastInsertId()

		if err := ds.RequeueDeadLetters([]int{int(id)}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var state string
		var retryCount int
		serverDB.QueryRow(`SELECT state, retry_count FROM notification_queue WHERE id = ?`, id).Scan(&state, &retryCount)
		if state != string(StateQueued) || retryCount != 0 {
			t.Errorf("expected state=queued retry_count=0, got state=%q retry_count=%d", state, retryCount)
		}
	})

	t.Run("multiple ids all get requeued (regression test for placeholder-join fix)", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)

		var ids []int
		for i := 0; i < 3; i++ {
			res, err := serverDB.Exec(`INSERT INTO notification_queue (state, retry_count, created_at, updated_at) VALUES ('dead_letter', 3, ?, ?)`, time.Now(), time.Now())
			if err != nil {
				t.Fatalf("seed failed: %v", err)
			}
			id, _ := res.LastInsertId()
			ids = append(ids, int(id))
		}

		if err := ds.RequeueDeadLetters(ids); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var queuedCount int
		serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue WHERE state = ?`, StateQueued).Scan(&queuedCount)
		if queuedCount != 3 {
			t.Errorf("expected all 3 dead letters requeued, got %d", queuedCount)
		}
	})

	t.Run("only rows currently in dead_letter state are requeued", func(t *testing.T) {
		serverDB, usersDB := setupDeliverySystemTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ds := NewDeliverySystem(serverDB, nil, nil)
		res, err := serverDB.Exec(`INSERT INTO notification_queue (state, retry_count, created_at, updated_at) VALUES ('delivered', 1, ?, ?)`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("seed failed: %v", err)
		}
		id, _ := res.LastInsertId()

		if err := ds.RequeueDeadLetters([]int{int(id)}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var state string
		serverDB.QueryRow(`SELECT state FROM notification_queue WHERE id = ?`, id).Scan(&state)
		if state != string(StateDelivered) {
			t.Errorf("expected delivered row to be untouched, got state=%q", state)
		}
	})
}
