package model

import (
	"database/sql"
	"testing"
	"time"
)

// testZone is a fixed non-UTC zone used to build fixtures in the exact layout
// modernc.org/sqlite produces when a Go time.Time is bound directly
// ("2006-01-02 15:04:05.999999999 -0700 MST", in the writer's LOCAL zone).
var testZone = time.FixedZone("MST", -7*60*60)

// cleanupCutoff is the reference instant every fixture in this file is
// positioned around.
var cleanupCutoff = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

// localLayout mirrors the layout time.Time.String() emits.
const localLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

func TestParseStoredTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		stored interface{}
		want   time.Time
		wantOK bool
	}{
		{
			name:   "canonical utc text",
			stored: "2025-01-01 12:00:00",
			want:   cleanupCutoff,
			wantOK: true,
		},
		{
			name:   "canonical utc bytes",
			stored: []byte("2025-01-01 12:00:00"),
			want:   cleanupCutoff,
			wantOK: true,
		},
		{
			name:   "local zone string layout",
			stored: "2025-01-01 05:00:00 -0700 MST",
			want:   cleanupCutoff,
			wantOK: true,
		},
		{
			name:   "local zone string layout with monotonic suffix",
			stored: "2025-01-01 05:00:00 -0700 MST m=+0.000000001",
			want:   cleanupCutoff,
			wantOK: true,
		},
		{
			name:   "rfc3339",
			stored: "2025-01-01T12:00:00Z",
			want:   cleanupCutoff,
			wantOK: true,
		},
		{
			name:   "driver returned time",
			stored: cleanupCutoff.In(testZone),
			want:   cleanupCutoff,
			wantOK: true,
		},
		{
			name:   "null",
			stored: nil,
			wantOK: false,
		},
		{
			name:   "empty",
			stored: "   ",
			wantOK: false,
		},
		{
			name:   "unparseable",
			stored: "not-a-timestamp",
			wantOK: false,
		},
		{
			name:   "unsupported type",
			stored: 12345,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseStoredTimestamp(tt.stored)
			if ok != tt.wantOK {
				t.Fatalf("parseStoredTimestamp(%v) ok = %v, want %v", tt.stored, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if !got.Equal(tt.want) {
				t.Fatalf("parseStoredTimestamp(%v) = %s, want %s", tt.stored, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("parseStoredTimestamp(%v) location = %s, want UTC", tt.stored, got.Location())
			}
		})
	}
}

// sessionFixture is one user_sessions row planted with a hand-written
// expires_at value so the on-disk layout is controlled by the test rather than
// by the driver.
type sessionFixture struct {
	name      string
	expiresAt string
}

// insertSessionFixtures plants each fixture and returns the assigned row IDs,
// keyed by fixture name.
func insertSessionFixtures(t *testing.T, db *sql.DB, userID int64, fixtures []sessionFixture) map[string]int64 {
	t.Helper()

	ids := make(map[string]int64, len(fixtures))
	for _, fixture := range fixtures {
		res, err := db.Exec(`
			INSERT INTO user_sessions (user_id, token_hash, expires_at)
			VALUES (?, ?, ?)
		`, userID, fixture.name, fixture.expiresAt)
		if err != nil {
			t.Fatalf("insert session fixture %q: %v", fixture.name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("session fixture %q id: %v", fixture.name, err)
		}
		ids[fixture.name] = id
	}

	return ids
}

// survivingSessionNames returns the token_hash of every remaining session row.
func survivingSessionNames(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()

	rows, err := db.Query("SELECT token_hash FROM user_sessions")
	if err != nil {
		t.Fatalf("select surviving sessions: %v", err)
	}
	defer rows.Close()

	surviving := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan surviving session: %v", err)
		}
		surviving[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate surviving sessions: %v", err)
	}

	return surviving
}

func TestDeleteRowsWithTimestampBeforeMixedLayouts(t *testing.T) {
	// Every fixture is described by the absolute instant it means, then written
	// in a different on-disk layout, so a lexicographic SQL comparison and a
	// correct instant comparison disagree about which rows are expired.
	fixtures := []sessionFixture{
		// One hour before the cutoff, canonical UTC layout.
		{name: "utc-expired", expiresAt: "2024-12-31 23:00:00"},
		// One day after the cutoff, canonical UTC layout.
		{name: "utc-live", expiresAt: "2025-01-02 00:00:00"},
		// 2025-01-01T13:00:00Z, written in the -0700 local layout. It sorts as
		// "2025-01-01 06:00:00..." so a text comparison calls it expired even
		// though it is an hour in the future.
		{name: "local-live", expiresAt: cleanupCutoff.Add(time.Hour).In(testZone).Format(localLayout)},
		// Same trap, plus the monotonic-clock suffix time.Time.String() appends.
		{name: "local-live-monotonic", expiresAt: cleanupCutoff.Add(2*time.Hour).In(testZone).Format(localLayout) + " m=+0.000000001"},
		// 2025-01-01T05:00:00Z in the -0700 local layout: genuinely expired.
		{name: "local-expired", expiresAt: cleanupCutoff.Add(-7 * time.Hour).In(testZone).Format(localLayout)},
		// Exactly the cutoff instant.
		{name: "exact-cutoff", expiresAt: "2025-01-01 12:00:00"},
		// Junk the project never writes: must never be deleted.
		{name: "unparseable", expiresAt: "not-a-timestamp"},
	}

	tests := []struct {
		name         string
		includeEqual bool
		wantDeleted  int64
		wantSurvivor []string
	}{
		{
			name:         "strictly before cutoff",
			includeEqual: false,
			wantDeleted:  2,
			wantSurvivor: []string{"utc-live", "local-live", "local-live-monotonic", "exact-cutoff", "unparseable"},
		},
		{
			name:         "at or before cutoff",
			includeEqual: true,
			wantDeleted:  3,
			wantSurvivor: []string{"utc-live", "local-live", "local-live-monotonic", "unparseable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newModelUsersDB(t)
			userID := insertTestUser(t, db, "cleanup-user", "cleanup@example.com")
			insertSessionFixtures(t, db, userID, fixtures)

			deleted, err := deleteRowsWithTimestampBefore(db, "user_sessions", "id", "expires_at", cleanupCutoff, tt.includeEqual)
			if err != nil {
				t.Fatalf("deleteRowsWithTimestampBefore: %v", err)
			}
			if deleted != tt.wantDeleted {
				t.Fatalf("deleted = %d, want %d", deleted, tt.wantDeleted)
			}

			surviving := survivingSessionNames(t, db)
			if len(surviving) != len(tt.wantSurvivor) {
				t.Fatalf("surviving = %v, want exactly %v", surviving, tt.wantSurvivor)
			}
			for _, name := range tt.wantSurvivor {
				if !surviving[name] {
					t.Errorf("row %q was deleted but should have survived", name)
				}
			}
		})
	}
}

func TestSessionModelCleanupExpiredKeepsLocalZoneFutureRows(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, newModelServerDB(t), db)

	userID := insertTestUser(t, db, "session-user", "session@example.com")
	now := time.Now().UTC()

	fixtures := []sessionFixture{
		// An hour in the future but written in a -0700 local zone: the text
		// comparison this replaced deleted it because its date/hour text sorts
		// below the UTC "now" text.
		{name: "future-local", expiresAt: now.Add(time.Hour).In(testZone).Format(localLayout)},
		// An hour in the future, canonical UTC.
		{name: "future-utc", expiresAt: sqlTimestamp(now.Add(time.Hour))},
		// An hour in the past, canonical UTC.
		{name: "past-utc", expiresAt: sqlTimestamp(now.Add(-time.Hour))},
		// An hour in the past, local zone.
		{name: "past-local", expiresAt: now.Add(-time.Hour).In(testZone).Format(localLayout)},
	}
	insertSessionFixtures(t, db, userID, fixtures)

	sessions := &SessionModel{DB: db}
	if err := sessions.CleanupExpired(); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}

	surviving := survivingSessionNames(t, db)
	want := []string{"future-local", "future-utc"}
	if len(surviving) != len(want) {
		t.Fatalf("surviving = %v, want exactly %v", surviving, want)
	}
	for _, name := range want {
		if !surviving[name] {
			t.Errorf("session %q was deleted but has not expired", name)
		}
	}
}

// TestSessionWritersAgreeOnLayout verifies the two independent writers of
// user_sessions.expires_at (SessionModel and UserSessionModel) store the same
// canonical UTC layout. They used to disagree, which left one table holding two
// incomparable text encodings of the same kind of instant.
func TestSessionWritersAgreeOnLayout(t *testing.T) {
	db := newModelUsersDB(t)
	setModelGlobalDualDB(t, newModelServerDB(t), db)

	userID := insertTestUser(t, db, "layout-user", "layout@example.com")

	if _, err := (&SessionModel{DB: db}).Create(userID, 3600); err != nil {
		t.Fatalf("SessionModel.Create: %v", err)
	}
	if _, err := (&UserSessionModel{DB: db}).CreateSession(userID, "127.0.0.1", "test-agent", time.Hour); err != nil {
		t.Fatalf("UserSessionModel.CreateSession: %v", err)
	}

	// CAST(... AS TEXT) keeps the driver from converting the column to a
	// time.Time on the way out, so the assertion sees the bytes on disk.
	rows, err := db.Query("SELECT CAST(expires_at AS TEXT) FROM user_sessions")
	if err != nil {
		t.Fatalf("select expires_at: %v", err)
	}
	defer rows.Close()

	var stored []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan expires_at: %v", err)
		}
		stored = append(stored, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate expires_at: %v", err)
	}

	if len(stored) != 2 {
		t.Fatalf("got %d session rows, want 2", len(stored))
	}
	for _, value := range stored {
		if _, err := time.Parse(sqlTimestampLayout, value); err != nil {
			t.Errorf("expires_at %q is not in the canonical layout: %v", value, err)
		}
	}
}

// insertNotificationFixture plants one notification row with a hand-written
// expires_at value.
func insertNotificationFixture(t *testing.T, db *sql.DB, table, id string, ownerColumn string, ownerID int64, expiresAt string) {
	t.Helper()

	query := "INSERT INTO " + table + " (id, " + ownerColumn + ", type, display, title, message, expires_at) VALUES (?, ?, 'info', 'toast', 'title', 'message', ?)"
	if _, err := db.Exec(query, id, ownerID, expiresAt); err != nil {
		t.Fatalf("insert %s fixture %q: %v", table, id, err)
	}
}

// survivingNotificationIDs returns the id of every remaining notification row.
func survivingNotificationIDs(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()

	rows, err := db.Query("SELECT id FROM " + table)
	if err != nil {
		t.Fatalf("select surviving %s: %v", table, err)
	}
	defer rows.Close()

	surviving := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan surviving %s: %v", table, err)
		}
		surviving[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate surviving %s: %v", table, err)
	}

	return surviving
}

func TestNotificationCleanupExpiredMixedLayouts(t *testing.T) {
	usersDB := newModelUsersDB(t)
	serverDB := newModelServerDB(t)
	userID := insertTestUser(t, usersDB, "notify-user", "notify@example.com")

	now := time.Now().UTC()

	type fixture struct {
		id        string
		expiresAt string
	}

	fixtures := []fixture{
		{id: "future-local", expiresAt: now.Add(time.Hour).In(testZone).Format(localLayout)},
		{id: "future-local-monotonic", expiresAt: now.Add(2*time.Hour).In(testZone).Format(localLayout) + " m=+0.000000001"},
		{id: "future-utc", expiresAt: sqlTimestamp(now.Add(time.Hour))},
		{id: "past-utc", expiresAt: sqlTimestamp(now.Add(-time.Hour))},
		{id: "past-local", expiresAt: now.Add(-time.Hour).In(testZone).Format(localLayout)},
		{id: "unparseable", expiresAt: "not-a-timestamp"},
	}

	tests := []struct {
		name        string
		table       string
		ownerColumn string
		ownerID     int64
		db          *sql.DB
		cleanup     func() (int64, error)
	}{
		{
			name:        "user notifications",
			table:       "user_notifications",
			ownerColumn: "user_id",
			ownerID:     userID,
			db:          usersDB,
			cleanup: func() (int64, error) {
				return (&UserNotificationModel{DB: usersDB}).CleanupExpired()
			},
		},
		{
			name:        "admin notifications",
			table:       "server_admin_notifications",
			ownerColumn: "admin_id",
			ownerID:     1,
			db:          serverDB,
			cleanup: func() (int64, error) {
				return (&AdminNotificationModel{DB: serverDB}).CleanupExpired()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, f := range fixtures {
				insertNotificationFixture(t, tt.db, tt.table, f.id, tt.ownerColumn, tt.ownerID, f.expiresAt)
			}
			// A NULL expires_at means "never expires" and must always survive.
			nullQuery := "INSERT INTO " + tt.table + " (id, " + tt.ownerColumn + ", type, display, title, message) VALUES ('no-expiry', ?, 'info', 'toast', 'title', 'message')"
			if _, err := tt.db.Exec(nullQuery, tt.ownerID); err != nil {
				t.Fatalf("insert null-expiry fixture: %v", err)
			}

			deleted, err := tt.cleanup()
			if err != nil {
				t.Fatalf("CleanupExpired: %v", err)
			}
			if deleted != 2 {
				t.Fatalf("deleted = %d, want 2", deleted)
			}

			surviving := survivingNotificationIDs(t, tt.db, tt.table)
			want := []string{"future-local", "future-local-monotonic", "future-utc", "unparseable", "no-expiry"}
			if len(surviving) != len(want) {
				t.Fatalf("surviving = %v, want exactly %v", surviving, want)
			}
			for _, id := range want {
				if !surviving[id] {
					t.Errorf("notification %q was deleted but should have survived", id)
				}
			}
		})
	}
}

func TestNotificationPreferencesRoundTripUpdatedAt(t *testing.T) {
	usersDB := newModelUsersDB(t)
	serverDB := newModelServerDB(t)
	userID := insertTestUser(t, usersDB, "prefs-user", "prefs@example.com")

	prefsModel := &NotificationPreferencesModel{UserDB: usersDB, ServerDB: serverDB}

	before := time.Now().UTC().Add(-time.Second)
	if err := prefsModel.UpdateUserPreferences(int(userID), &NotificationPreferences{
		EnableToast:          true,
		EnableBanner:         true,
		EnableCenter:         true,
		EnableSound:          false,
		ToastDurationSuccess: 5,
		ToastDurationInfo:    5,
		ToastDurationWarning: 10,
	}); err != nil {
		t.Fatalf("UpdateUserPreferences: %v", err)
	}

	got, err := prefsModel.GetUserPreferences(int(userID))
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	if got.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt = %s, want at or after %s", got.UpdatedAt, before)
	}

	// A row written in the pre-normalization local-zone layout must still read
	// back as the correct absolute instant.
	planted := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	if _, err := usersDB.Exec("UPDATE user_notification_preferences SET updated_at = ? WHERE user_id = ?", planted.In(testZone).Format(localLayout), userID); err != nil {
		t.Fatalf("plant local-zone updated_at: %v", err)
	}

	got, err = prefsModel.GetUserPreferences(int(userID))
	if err != nil {
		t.Fatalf("GetUserPreferences after plant: %v", err)
	}
	if !got.UpdatedAt.Equal(planted) {
		t.Fatalf("UpdatedAt = %s, want %s", got.UpdatedAt, planted)
	}
}
