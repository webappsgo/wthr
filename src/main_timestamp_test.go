// Timestamp regression tests for main.go per AI.md PART 10 (Database) and
// PART 29 (Testing).
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
)

// mainZoneWest and mainZoneEast are deliberately extreme fixed offsets. A
// timestamp rendered in mainZoneWest reads eleven hours EARLIER as wall-clock
// text than the instant it represents, and one rendered in mainZoneEast reads
// thirteen hours LATER. Wall-clock text ordering therefore contradicts true
// instant ordering for both fixtures no matter which timezone the host running
// the test is set to.
//
// The abbreviations are plain three-letter names on purpose: Go's "MST" layout
// element only consumes uppercase letters, so a name containing digits would
// leave trailing characters and make the fixture unparseable for reasons that
// have nothing to do with the behaviour under test.
var (
	mainZoneWest = time.FixedZone("WST", -11*60*60)
	mainZoneEast = time.FixedZone("EAT", 13*60*60)
)

// mainLocalLayout is the layout modernc.org/sqlite produces when a raw
// time.Time is bound as a query parameter — time.Time.String() in the writer's
// local zone. Rows written before this project bound canonical UTC text are
// still on disk in this layout.
const mainLocalLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// mainDBCounter guarantees a unique in-memory SQLite DSN per test.
var mainDBCounter int64

// newMainUsersDB opens a fresh in-memory SQLite database with the real
// production UsersSchema applied, so fixtures exercise the shipped column
// definitions rather than a hand-rolled CREATE TABLE.
func newMainUsersDB(t *testing.T) *sql.DB {
	t.Helper()

	n := atomic.AddInt64(&mainDBCounter, 1)
	dsn := fmt.Sprintf("file:main_users_%d?mode=memory&cache=shared", n)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open users db: %v", err)
	}
	if _, err := db.Exec(database.UsersSchema); err != nil {
		db.Close()
		t.Fatalf("apply UsersSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

// newMainServerDB opens a fresh in-memory SQLite database with the real
// production ServerSchema applied.
func newMainServerDB(t *testing.T) *sql.DB {
	t.Helper()

	n := atomic.AddInt64(&mainDBCounter, 1)
	dsn := fmt.Sprintf("file:main_server_%d?mode=memory&cache=shared", n)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open server db: %v", err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		db.Close()
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

// insertVerificationUser creates the user_accounts row the verification
// fixtures reference and returns its id.
func insertVerificationUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	result, err := db.Exec(`
		INSERT INTO user_accounts (username, email, password_hash)
		VALUES (?, ?, ?)
	`, "verifyuser", "verify@example.com", "argon2id$placeholder")
	if err != nil {
		t.Fatalf("insert user_accounts: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("user_accounts last insert id: %v", err)
	}

	return id
}

// TestLookupEmailVerificationHonorsStoredZone proves the verification lookup
// judges expiry by absolute instant rather than by stored text.
//
// Against the pre-fix statement
// "SELECT id, user_id, expires_at FROM user_email_verifications
//
//	WHERE token = ? AND expires_at > ?"
//
// bound with a raw time.Now(), this test fails on two rows. The bound parameter
// itself was serialized by modernc.org/sqlite as time.Time.String() in the host's
// LOCAL zone, and both sides of ">" were then compared as text: the
// "live-west-zone" row (a token valid for another hour, written at -11:00) reads
// eleven hours in the past and was rejected as expired, while the
// "expired-east-zone" row (a token that died an hour ago, written at +13:00)
// reads thirteen hours in the future and was accepted, verifying an email
// address from a dead token. The "unparseable" row additionally used to abort
// the whole query with a scan error into time.Time; it must now simply fail
// closed.
func TestLookupEmailVerificationHonorsStoredZone(t *testing.T) {
	usersDB := newMainUsersDB(t)
	userID := insertVerificationUser(t, usersDB)

	now := time.Now().UTC()

	tests := []struct {
		name string
		// storedExpiresAt is written verbatim so each case controls the exact
		// on-disk layout instead of relying on the driver's serialization.
		storedExpiresAt string
		wantValid       bool
	}{
		{
			name:            "live-utc",
			storedExpiresAt: dbtime.FormatSQLTimestamp(now.Add(time.Hour)),
			wantValid:       true,
		},
		{
			name:            "expired-utc",
			storedExpiresAt: dbtime.FormatSQLTimestamp(now.Add(-time.Hour)),
			wantValid:       false,
		},
		{
			name:            "live-west-zone-text-reads-as-past",
			storedExpiresAt: now.Add(time.Hour).In(mainZoneWest).Format(mainLocalLayout),
			wantValid:       true,
		},
		{
			name:            "expired-east-zone-text-reads-as-future",
			storedExpiresAt: now.Add(-time.Hour).In(mainZoneEast).Format(mainLocalLayout),
			wantValid:       false,
		},
		{
			name:            "unparseable-fails-closed",
			storedExpiresAt: "not-a-timestamp",
			wantValid:       false,
		},
	}

	for _, tt := range tests {
		// The token column holds model.HashAPIToken(raw), never the raw value
		// — that is what every writer of this table stores, so a fixture that
		// seeded the raw token would only match a lookup that had the same bug.
		_, err := usersDB.Exec(`
			INSERT INTO user_email_verifications (user_id, email, token, expires_at)
			VALUES (?, ?, ?, ?)
		`, userID, "verify@example.com", model.HashAPIToken(tt.name), tt.storedExpiresAt)
		if err != nil {
			t.Fatalf("insert verification fixture %q: %v", tt.name, err)
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotUserID, gotValid := lookupEmailVerification(usersDB, tt.name, now)
			if gotValid != tt.wantValid {
				t.Fatalf("lookupEmailVerification(%q) valid = %v, want %v (stored %q)", tt.name, gotValid, tt.wantValid, tt.storedExpiresAt)
			}
			if !tt.wantValid {
				if gotID != 0 || gotUserID != 0 {
					t.Errorf("rejected token returned id=%d user_id=%d, want zeroes", gotID, gotUserID)
				}
				return
			}
			if gotID == 0 {
				t.Error("accepted token returned id = 0, want the row id")
			}
			if gotUserID != userID {
				t.Errorf("accepted token returned user_id = %d, want %d", gotUserID, userID)
			}
		})
	}
}

// TestLookupEmailVerificationUnknownToken confirms an absent token is rejected
// without touching the expiry path.
func TestLookupEmailVerificationUnknownToken(t *testing.T) {
	usersDB := newMainUsersDB(t)

	if _, _, valid := lookupEmailVerification(usersDB, "no-such-token", time.Now()); valid {
		t.Error("lookupEmailVerification() accepted a token that does not exist")
	}
}

// insertPreferencesAdmin creates the server_admin_credentials row that
// server_admin_preferences references and returns its id.
func insertPreferencesAdmin(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	result, err := db.Exec(`
		INSERT INTO server_admin_credentials (username, email, password_hash, is_super_admin, is_active)
		VALUES (?, ?, ?, 1, 1)
	`, "prefsadmin", "prefsadmin@example.com", "argon2id$placeholder")
	if err != nil {
		t.Fatalf("insert server_admin_credentials: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("server_admin_credentials last insert id: %v", err)
	}

	return id
}

// TestLoadAdminPreferencesRowBindOrder pins the parameter order of the
// INSERT ... SELECT that seeds a new admin's preferences.
//
// Replacing the projected CURRENT_TIMESTAMP literal with a placeholder grows
// that statement from three bound values to four, and the new one sits in the
// middle of the projection rather than at the end. Getting the order wrong is
// silent: the preferences blob and the timestamp simply swap columns, so this
// test asserts both that updated_at is canonical UTC text and that the
// preferences column still round-trips as the default JSON document.
func TestLoadAdminPreferencesRowBindOrder(t *testing.T) {
	serverDB := newMainServerDB(t)
	adminID := insertPreferencesAdmin(t, serverDB)

	prefs, err := loadAdminPreferencesRow(serverDB, adminID)
	if err != nil {
		t.Fatalf("loadAdminPreferencesRow() error = %v", err)
	}
	if prefs.AdminID != adminID {
		t.Errorf("prefs.AdminID = %d, want %d", prefs.AdminID, adminID)
	}
	if prefs.Theme != "auto" || prefs.Language != "en" || prefs.Timezone != "UTC" {
		t.Errorf("prefs = %+v, want the encoded defaults (auto/en/UTC)", prefs)
	}

	// CAST(... AS TEXT) keeps the driver from re-rendering the column on the way
	// out, so the assertion sees the bytes actually on disk.
	var storedPrefs, storedUpdatedAt string
	err = serverDB.QueryRow(`
		SELECT preferences, CAST(updated_at AS TEXT)
		FROM server_admin_preferences
		WHERE admin_id = ?
	`, adminID).Scan(&storedPrefs, &storedUpdatedAt)
	if err != nil {
		t.Fatalf("read back preferences row: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(storedPrefs), &decoded); err != nil {
		t.Fatalf("preferences column %q is not the JSON blob: %v", storedPrefs, err)
	}

	parsed, err := time.Parse(dbtime.SQLTimestampLayout, storedUpdatedAt)
	if err != nil {
		t.Fatalf("updated_at %q is not in the canonical layout: %v", storedUpdatedAt, err)
	}
	if drift := time.Since(parsed); drift < -time.Minute || drift > time.Minute {
		t.Errorf("updated_at is %s away from now, want within a minute", drift)
	}
}

// TestLoadAdminPreferencesRowReadsLegacyLayouts proves a preferences row left
// behind by an older writer still loads.
//
// The pre-fix code scanned updated_at straight into a time.Time. A row holding
// the driver's local-zone time.Time.String() form, or any other value the
// driver cannot convert, failed that scan and turned every admin preferences
// read into a 500 for that admin — with no way to recover, since the seeding
// INSERT is skipped once a row exists. The value must now be parsed through
// dbtime, and an uninterpretable one must leave UpdatedAt zero rather than
// failing the load.
func TestLoadAdminPreferencesRowReadsLegacyLayouts(t *testing.T) {
	want := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	tests := []struct {
		name          string
		storedValue   string
		wantUpdatedAt time.Time
	}{
		{
			name:          "legacy-west-zone",
			storedValue:   want.In(mainZoneWest).Format(mainLocalLayout),
			wantUpdatedAt: want,
		},
		{
			name:          "legacy-east-zone",
			storedValue:   want.In(mainZoneEast).Format(mainLocalLayout),
			wantUpdatedAt: want,
		},
		{
			name:          "unparseable-leaves-zero",
			storedValue:   "not-a-timestamp",
			wantUpdatedAt: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverDB := newMainServerDB(t)
			adminID := insertPreferencesAdmin(t, serverDB)

			defaultJSON, err := defaultAdminPreferencesJSON(adminID)
			if err != nil {
				t.Fatalf("defaultAdminPreferencesJSON() error = %v", err)
			}

			_, err = serverDB.Exec(`
				INSERT INTO server_admin_preferences (admin_id, preferences, updated_at)
				VALUES (?, ?, ?)
			`, adminID, defaultJSON, tt.storedValue)
			if err != nil {
				t.Fatalf("insert legacy preferences row: %v", err)
			}

			prefs, err := loadAdminPreferencesRow(serverDB, adminID)
			if err != nil {
				t.Fatalf("loadAdminPreferencesRow() error = %v, want the row to load", err)
			}
			if !prefs.UpdatedAt.Equal(tt.wantUpdatedAt) {
				t.Errorf("prefs.UpdatedAt = %s, want %s", prefs.UpdatedAt, tt.wantUpdatedAt)
			}
		})
	}
}
