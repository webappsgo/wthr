package database

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
)

// newSchemaDB returns a *DB backed by an in-memory SQLite database with the
// real production ServerSchema applied, matching what InitDualDB builds at
// startup.
func newSchemaDB(t *testing.T, name string) *DB {
	t.Helper()
	raw := openMemDB(t, name)
	if _, err := raw.Exec(ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	return &DB{raw}
}

func TestDB_HealthCheck(t *testing.T) {
	t.Run("connected", func(t *testing.T) {
		db := newSchemaDB(t, "healthcheck_ok")
		status, latency, err := db.HealthCheck()
		if err != nil {
			t.Fatalf("HealthCheck: %v", err)
		}
		if status != "connected" {
			t.Errorf("status = %q, want connected", status)
		}
		if latency < 0 {
			t.Errorf("latency = %d, want >= 0", latency)
		}
	})

	t.Run("disconnected reports error", func(t *testing.T) {
		db := newSchemaDB(t, "healthcheck_closed")
		db.Close()
		status, _, err := db.HealthCheck()
		if err == nil {
			t.Fatal("HealthCheck on closed db = nil error, want error")
		}
		if status != "disconnected" {
			t.Errorf("status = %q, want disconnected", status)
		}
	})
}

// newUsersSchemaDB returns an in-memory SQLite handle with the real production
// UsersSchema applied. newSchemaDB above applies only ServerSchema, and a
// session count that spans both databases needs a users-side handle too.
func newUsersSchemaDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	raw := openMemDB(t, name)
	if _, err := raw.Exec(UsersSchema); err != nil {
		t.Fatalf("apply UsersSchema: %v", err)
	}
	return raw
}

// sessionZoneWest is a fixed offset far enough west of UTC that a genuinely
// future instant rendered in it produces text sorting BELOW the canonical UTC
// "now" text.
const sessionZoneWest = -11 * 60 * 60

// sessionZoneEast is the mirror image: a genuinely past instant rendered in it
// produces text sorting ABOVE the canonical UTC "now" text.
const sessionZoneEast = 13 * 60 * 60

// offsetTimestamp renders t in a fixed zone using a layout that carries the
// offset, so the absolute instant survives the round trip even though the
// leading wall-clock digits deliberately misrepresent it.
func offsetTimestamp(t time.Time, offsetSeconds int) string {
	return t.In(time.FixedZone("fixture", offsetSeconds)).Format("2006-01-02 15:04:05-07:00")
}

// insertUserSession seeds one row in the users database's user_sessions table
// with a caller-supplied raw expires_at text, so a fixture can store a value no
// well-behaved writer would produce.
func insertUserSession(t *testing.T, db *sql.DB, tokenHash, expiresAt string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO user_sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)",
		1, tokenHash, expiresAt,
	)
	if err != nil {
		t.Fatalf("seed user_sessions row %q: %v", tokenHash, err)
	}
}

// insertAdminSession seeds one row in the server database's
// server_admin_sessions table with a caller-supplied raw expires_at text.
func insertAdminSession(t *testing.T, db *sql.DB, id, expiresAt string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO server_admin_sessions (id, admin_id, expires_at) VALUES (?, ?, ?)",
		id, 1, expiresAt,
	)
	if err != nil {
		t.Fatalf("seed server_admin_sessions row %q: %v", id, err)
	}
}

// TestDB_GetSessionCount is a regression test for a real production bug:
// GetSessionCount queried a "sessions" table that neither ServerSchema nor
// UsersSchema ever creates, so the call failed on every live deployment, and it
// compared expires_at SQL-side against a bound time.Time whose driver rendering
// never matches the canonical stored layout.
//
// The count now sums unexpired rows from the two tables the live schema really
// creates - user_sessions (users DB) and server_admin_sessions (server DB) -
// and decides expiry in Go.
func TestDB_GetSessionCount(t *testing.T) {
	now := time.Now()
	future := dbtime.FormatSQLTimestamp(now.Add(24 * time.Hour))
	past := dbtime.FormatSQLTimestamp(now.Add(-24 * time.Hour))

	t.Run("sums both session tables and excludes expired rows", func(t *testing.T) {
		resetGlobalDualDB(t)
		serverDB := newSchemaDB(t, "sessioncount_sum_server")
		usersDB := newUsersSchemaDB(t, "sessioncount_sum_users")
		SetGlobalDualDB(&DualDB{Server: serverDB.DB, Users: usersDB})

		insertAdminSession(t, serverDB.DB, "admin-live", future)
		insertAdminSession(t, serverDB.DB, "admin-expired", past)
		insertUserSession(t, usersDB, "user-live-a", future)
		insertUserSession(t, usersDB, "user-live-b", future)
		insertUserSession(t, usersDB, "user-expired", past)

		count, err := serverDB.GetSessionCount()
		if err != nil {
			t.Fatalf("GetSessionCount: %v", err)
		}
		if count != 3 {
			t.Errorf("GetSessionCount() = %d, want 3 (1 admin + 2 user sessions still active); "+
				"a count that omits either table under-reports every live deployment", count)
		}
	})

	t.Run("orders by instant, not by stored wall-clock text", func(t *testing.T) {
		resetGlobalDualDB(t)
		serverDB := newSchemaDB(t, "sessioncount_zone_server")
		usersDB := newUsersSchemaDB(t, "sessioncount_zone_users")
		SetGlobalDualDB(&DualDB{Server: serverDB.DB, Users: usersDB})

		// Genuinely active, but its leading digits sort below the canonical
		// UTC "now" text - a text comparison would drop it.
		insertAdminSession(t, serverDB.DB, "admin-live-west-text", offsetTimestamp(now.Add(24*time.Hour), sessionZoneWest))
		// Genuinely expired, but its leading digits sort above the canonical
		// UTC "now" text - a text comparison would keep it.
		insertUserSession(t, usersDB, "user-expired-east-text", offsetTimestamp(now.Add(-24*time.Hour), sessionZoneEast))

		count, err := serverDB.GetSessionCount()
		if err != nil {
			t.Fatalf("GetSessionCount: %v", err)
		}
		if count != 1 {
			t.Errorf("GetSessionCount() = %d, want 1 - only the genuinely future session counts; "+
				"if this fails the comparison is ordering by wall-clock text instead of by instant, "+
				"and the result flips with the host timezone", count)
		}
	})

	t.Run("unparseable expires_at counts as expired", func(t *testing.T) {
		resetGlobalDualDB(t)
		serverDB := newSchemaDB(t, "sessioncount_garbage_server")
		usersDB := newUsersSchemaDB(t, "sessioncount_garbage_users")
		SetGlobalDualDB(&DualDB{Server: serverDB.DB, Users: usersDB})

		insertAdminSession(t, serverDB.DB, "admin-garbage", "not-a-timestamp")
		insertUserSession(t, usersDB, "user-garbage", "")

		count, err := serverDB.GetSessionCount()
		if err != nil {
			t.Fatalf("GetSessionCount: %v", err)
		}
		if count != 0 {
			t.Errorf("GetSessionCount() = %d, want 0 - a value that cannot be parsed must fail "+
				"closed; under a raw text comparison %q sorts above every real timestamp and would "+
				"be reported as a session that never expires", count, "not-a-timestamp")
		}
	})

	t.Run("missing users handle contributes zero rather than erroring", func(t *testing.T) {
		resetGlobalDualDB(t)
		serverDB := newSchemaDB(t, "sessioncount_nousers_server")
		SetGlobalDualDB(&DualDB{Server: serverDB.DB})

		insertAdminSession(t, serverDB.DB, "admin-live", future)

		count, err := serverDB.GetSessionCount()
		if err != nil {
			t.Fatalf("GetSessionCount with no users DB: %v", err)
		}
		if count != 1 {
			t.Errorf("GetSessionCount() = %d, want 1 - a single-database deployment must still "+
				"report the sessions it does have", count)
		}
	})

	t.Run("no databases configured returns zero", func(t *testing.T) {
		resetGlobalDualDB(t)
		db := newSchemaDB(t, "sessioncount_unset")

		count, err := db.GetSessionCount()
		if err != nil {
			t.Fatalf("GetSessionCount with no global DBs: %v", err)
		}
		if count != 0 {
			t.Errorf("GetSessionCount() = %d, want 0", count)
		}
	})

	// Documents the exact tables the count is required to read, so a future
	// schema rename cannot silently reintroduce the original bug.
	t.Run("reads only tables the live schema creates", func(t *testing.T) {
		for _, table := range []string{"user_sessions", "server_admin_sessions"} {
			schema := UsersSchema
			if table == "server_admin_sessions" {
				schema = ServerSchema
			}
			needle := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (", table)
			if !strings.Contains(schema, needle) {
				t.Errorf("%s is not created by the live schema; GetSessionCount must never "+
					"query a table production does not have", table)
			}
		}
		if strings.Contains(UsersSchema, "CREATE TABLE IF NOT EXISTS sessions (") ||
			strings.Contains(ServerSchema, "CREATE TABLE IF NOT EXISTS sessions (") {
			t.Error("a bare \"sessions\" table now exists; revisit GetSessionCount, which was " +
				"fixed precisely because that table never existed")
		}
	})
}

func TestDB_IsFirstRun(t *testing.T) {
	resetGlobalDualDB(t)

	t.Run("no global server db configured", func(t *testing.T) {
		resetGlobalDualDB(t)
		db := newSchemaDB(t, "isfirstrun_unset")
		_, err := db.IsFirstRun()
		if err == nil {
			t.Error("IsFirstRun() with no global server DB = nil error, want error")
		}
	})

	t.Run("true when no admins exist", func(t *testing.T) {
		resetGlobalDualDB(t)
		db := newSchemaDB(t, "isfirstrun_true")
		SetGlobalDualDB(&DualDB{Server: db.DB})

		first, err := db.IsFirstRun()
		if err != nil {
			t.Fatalf("IsFirstRun: %v", err)
		}
		if !first {
			t.Error("IsFirstRun() = false with zero admins, want true")
		}
	})

	t.Run("false once an admin exists", func(t *testing.T) {
		resetGlobalDualDB(t)
		db := newSchemaDB(t, "isfirstrun_false")
		if _, err := db.Exec("INSERT INTO server_admin_credentials (username, email, password_hash) VALUES ('admin', 'admin@example.com', 'hash')"); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
		SetGlobalDualDB(&DualDB{Server: db.DB})

		first, err := db.IsFirstRun()
		if err != nil {
			t.Fatalf("IsFirstRun: %v", err)
		}
		if first {
			t.Error("IsFirstRun() = true with an admin present, want false")
		}
	})
}
