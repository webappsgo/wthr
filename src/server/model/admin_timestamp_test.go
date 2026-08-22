package model

import (
	"database/sql"
	"sort"
	"testing"
	"time"
)

// adminZoneWest and adminZoneEast are deliberately extreme fixed offsets. A
// timestamp rendered in adminZoneWest reads eleven hours EARLIER as wall-clock
// text than the instant it represents, and one rendered in adminZoneEast reads
// thirteen hours LATER. Wall-clock text ordering and true instant ordering
// therefore disagree for both fixtures no matter what timezone the host running
// the test is set to. Both zone names are short, all-uppercase and digit-free so
// that the "MST" element of the local time.Time.String() layout can read them
// back: a name carrying digits or more than six letters would make every such
// fixture unparseable, quietly turning these cases into duplicates of the
// deliberate "unparseable" case instead of real zone comparisons.
var (
	adminZoneWest = time.FixedZone("WST", -11*60*60)
	adminZoneEast = time.FixedZone("EAT", 13*60*60)
)

// adminSessionFixture describes one row planted in server_admin_sessions.
type adminSessionFixture struct {
	name string
	// storedExpiresAt is written verbatim, so each case controls the exact
	// on-disk layout rather than relying on the driver's serialization.
	storedExpiresAt string
	// wantSurvives records whether the row is still valid at the moment the
	// test runs, judged by the true instant its timestamp represents.
	wantSurvives bool
}

// adminSessionFixtures builds the shared fixture set relative to now.
// Each row is one hour either side of now, so the outcome never depends on how
// long the test takes to run.
func adminSessionFixtures(now time.Time) []adminSessionFixture {
	return []adminSessionFixture{
		{
			name:            "live-utc",
			storedExpiresAt: sqlTimestamp(now.Add(time.Hour)),
			wantSurvives:    true,
		},
		{
			name:            "expired-utc",
			storedExpiresAt: sqlTimestamp(now.Add(-time.Hour)),
			wantSurvives:    false,
		},
		{
			name:            "live-west-zone-text-reads-as-past",
			storedExpiresAt: now.Add(time.Hour).In(adminZoneWest).Format(localLayout),
			wantSurvives:    true,
		},
		{
			name:            "expired-east-zone-text-reads-as-future",
			storedExpiresAt: now.Add(-time.Hour).In(adminZoneEast).Format(localLayout),
			wantSurvives:    false,
		},
		{
			name:            "unparseable-is-left-alone",
			storedExpiresAt: "not-a-timestamp",
			wantSurvives:    true,
		},
	}
}

// insertAdminSessionFixtures plants the fixture rows for one admin, writing
// expires_at as literal text so the stored layout is exactly what the case
// under test intends.
func insertAdminSessionFixtures(t *testing.T, db *sql.DB, adminID int64, fixtures []adminSessionFixture) {
	t.Helper()

	for _, fixture := range fixtures {
		_, err := db.Exec(`
			INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, created_at, expires_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
		`, fixture.name, adminID, "203.0.113.10", "go-test", fixture.storedExpiresAt)
		if err != nil {
			t.Fatalf("insert admin session fixture %q: %v", fixture.name, err)
		}
	}
}

// remainingAdminSessionIDs returns the sorted ids still present in the table.
func remainingAdminSessionIDs(t *testing.T, db *sql.DB) []string {
	t.Helper()

	rows, err := db.Query("SELECT id FROM server_admin_sessions ORDER BY id")
	if err != nil {
		t.Fatalf("query remaining admin sessions: %v", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan remaining admin session: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate remaining admin sessions: %v", err)
	}

	sort.Strings(ids)
	return ids
}

// assertSameStrings compares two id sets that have both already been sorted.
func assertSameStrings(t *testing.T, what string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

// TestAdminSessionDeleteExpiredHonorsStoredZone proves DeleteExpiredSessions
// judges expiry by absolute instant.
//
// Against the pre-fix statement
// "DELETE FROM server_admin_sessions WHERE datetime(expires_at) < datetime('now')"
// this test fails on the "expired-east-zone-text-reads-as-future" row: its
// stored text is the driver's local-zone time.Time.String() form, which
// SQLite's datetime() cannot parse and therefore evaluates to NULL, so the
// comparison never matched and a session that expired an hour ago was kept
// alive indefinitely. Had SQLite instead compared the text lexicographically,
// the same row would still have been kept (its +13h wall clock reads twelve
// hours into the future) and "live-west-zone-text-reads-as-past" would have
// been deleted ten hours early.
func TestAdminSessionDeleteExpiredHonorsStoredZone(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	now := time.Now()
	fixtures := adminSessionFixtures(now)
	insertAdminSessionFixtures(t, serverDB, 1, fixtures)

	model := &AdminSessionModel{DB: serverDB}
	if err := model.DeleteExpiredSessions(); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}

	var want []string
	for _, fixture := range fixtures {
		if fixture.wantSurvives {
			want = append(want, fixture.name)
		}
	}
	sort.Strings(want)

	assertSameStrings(t, "surviving admin sessions", remainingAdminSessionIDs(t, serverDB), want)
}

// TestAdminSessionGetActiveHonorsStoredZone proves GetActiveSessions lists a
// session by the instant it really expires at.
//
// Against the pre-fix predicate
// "WHERE admin_id = ? AND datetime(expires_at) > datetime('now')" this test
// fails twice over: "live-west-zone-text-reads-as-past" was omitted from the
// admin's own session list (datetime() returns NULL for its layout) while any
// backend that compared the text instead would have listed the already-expired
// "expired-east-zone-text-reads-as-future" row as active.
func TestAdminSessionGetActiveHonorsStoredZone(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	now := time.Now()
	fixtures := adminSessionFixtures(now)
	insertAdminSessionFixtures(t, serverDB, 1, fixtures)

	// A second admin's row must never appear in the first admin's listing.
	insertAdminSessionFixtures(t, serverDB, 2, []adminSessionFixture{
		{
			name:            "other-admin-live",
			storedExpiresAt: sqlTimestamp(now.Add(time.Hour)),
			wantSurvives:    true,
		},
	})

	model := &AdminSessionModel{DB: serverDB}
	sessions, err := model.GetActiveSessions(1)
	if err != nil {
		t.Fatalf("GetActiveSessions: %v", err)
	}

	var got []string
	for _, session := range sessions {
		got = append(got, session.SessionID)
	}
	sort.Strings(got)

	// The unparseable row is deliberately absent here: it survives cleanup, but
	// a value the project cannot interpret is never reported as active.
	want := []string{"live-utc", "live-west-zone-text-reads-as-past"}

	assertSameStrings(t, "active admin sessions", got, want)
}

// TestAdminSessionCreateReadRoundTrip proves the writer and the reader agree on
// one layout: CreateSession stores canonical UTC text, and GetSession returns
// the same instant it was handed.
func TestAdminSessionCreateReadRoundTrip(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	model := &AdminSessionModel{DB: serverDB}
	created, err := model.CreateSession(1, "203.0.113.10", "go-test", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var stored string
	err = serverDB.QueryRow("SELECT CAST(expires_at AS TEXT) FROM server_admin_sessions WHERE id = ?", created.SessionID).Scan(&stored)
	if err != nil {
		t.Fatalf("read stored expires_at: %v", err)
	}

	// Parsing with the canonical layout alone (not the full tolerant layout
	// list) is what makes this a real assertion: a local-zone value fails here.
	parsed, err := time.Parse(sqlTimestampLayout, stored)
	if err != nil {
		t.Fatalf("stored expires_at %q is not canonical UTC text: %v", stored, err)
	}
	if want := created.ExpiresAt.UTC().Truncate(time.Second); !parsed.Equal(want) {
		t.Fatalf("stored expires_at = %s, want %s", parsed, want)
	}

	fetched, err := model.GetSession(created.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !fetched.ExpiresAt.Equal(parsed) {
		t.Fatalf("GetSession expires_at = %s, want %s", fetched.ExpiresAt, parsed)
	}
}

// adminInviteFixture describes one row planted in server_admin_invites.
type adminInviteFixture struct {
	token string
	// storedExpiresAt is written verbatim so each case pins its on-disk layout.
	storedExpiresAt string
	used            bool
	wantSurvives    bool
	wantPending     bool
}

// adminInviteFixtures mirrors the session fixture set, adding the used-invite
// case that DeleteExpiredInvites handles with a separate statement.
func adminInviteFixtures(now time.Time) []adminInviteFixture {
	return []adminInviteFixture{
		{
			token:           "invite-live-utc",
			storedExpiresAt: sqlTimestamp(now.Add(time.Hour)),
			wantSurvives:    true,
			wantPending:     true,
		},
		{
			token:           "invite-expired-utc",
			storedExpiresAt: sqlTimestamp(now.Add(-time.Hour)),
			wantSurvives:    false,
		},
		{
			token:           "invite-live-west-zone-text-reads-as-past",
			storedExpiresAt: now.Add(time.Hour).In(adminZoneWest).Format(localLayout),
			wantSurvives:    true,
			wantPending:     true,
		},
		{
			token:           "invite-expired-east-zone-text-reads-as-future",
			storedExpiresAt: now.Add(-time.Hour).In(adminZoneEast).Format(localLayout),
			wantSurvives:    false,
		},
		{
			token:           "invite-used",
			storedExpiresAt: sqlTimestamp(now.Add(time.Hour)),
			used:            true,
			wantSurvives:    false,
		},
		{
			token:           "invite-unparseable",
			storedExpiresAt: "not-a-timestamp",
			wantSurvives:    true,
		},
	}
}

// insertAdminInviteFixtures plants the invite rows, writing expires_at as
// literal text so the stored layout is exactly what the case intends.
func insertAdminInviteFixtures(t *testing.T, db *sql.DB, fixtures []adminInviteFixture) {
	t.Helper()

	for _, fixture := range fixtures {
		var usedAt interface{}
		var usedBy interface{}
		if fixture.used {
			usedAt = sqlTimestamp(time.Now())
			usedBy = int64(2)
		}

		_, err := db.Exec(`
			INSERT INTO server_admin_invites (token, invited_email, invited_by, created_at, expires_at, used_by, used_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?)
		`, fixture.token, fixture.token+"@example.com", int64(1), fixture.storedExpiresAt, usedBy, usedAt)
		if err != nil {
			t.Fatalf("insert admin invite fixture %q: %v", fixture.token, err)
		}
	}
}

// remainingAdminInviteTokens returns the sorted tokens still present.
func remainingAdminInviteTokens(t *testing.T, db *sql.DB) []string {
	t.Helper()

	rows, err := db.Query("SELECT token FROM server_admin_invites ORDER BY token")
	if err != nil {
		t.Fatalf("query remaining admin invites: %v", err)
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			t.Fatalf("scan remaining admin invite: %v", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate remaining admin invites: %v", err)
	}

	sort.Strings(tokens)
	return tokens
}

// TestAdminInviteDeleteExpiredHonorsStoredZone proves DeleteExpiredInvites
// prunes by absolute instant while still removing used invites.
//
// Against the pre-fix statement
// "DELETE FROM server_admin_invites WHERE datetime(expires_at) < datetime('now') OR used_at IS NOT NULL"
// this test fails on "invite-expired-east-zone-text-reads-as-future": SQLite's
// datetime() returns NULL for the driver's local-zone layout, so an invite that
// expired an hour ago was never pruned and stayed redeemable forever.
func TestAdminInviteDeleteExpiredHonorsStoredZone(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	fixtures := adminInviteFixtures(time.Now())
	insertAdminInviteFixtures(t, serverDB, fixtures)

	model := &AdminInviteModel{DB: serverDB}
	if err := model.DeleteExpiredInvites(); err != nil {
		t.Fatalf("DeleteExpiredInvites: %v", err)
	}

	var want []string
	for _, fixture := range fixtures {
		if fixture.wantSurvives {
			want = append(want, fixture.token)
		}
	}
	sort.Strings(want)

	assertSameStrings(t, "surviving admin invites", remainingAdminInviteTokens(t, serverDB), want)
}

// TestAdminInviteGetPendingHonorsStoredZone proves GetPendingInvites lists an
// invite by the instant it really expires at.
//
// Against the pre-fix predicate
// "WHERE used_at IS NULL AND datetime(expires_at) > datetime('now')" this test
// fails on "invite-live-west-zone-text-reads-as-past": the invite is valid for
// another hour, but datetime() cannot parse its stored layout, so the admin
// panel showed no pending invitation at all for it.
func TestAdminInviteGetPendingHonorsStoredZone(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	fixtures := adminInviteFixtures(time.Now())
	insertAdminInviteFixtures(t, serverDB, fixtures)

	model := &AdminInviteModel{DB: serverDB}
	invites, err := model.GetPendingInvites()
	if err != nil {
		t.Fatalf("GetPendingInvites: %v", err)
	}

	var got []string
	for _, invite := range invites {
		got = append(got, invite.Token)
	}
	sort.Strings(got)

	var want []string
	for _, fixture := range fixtures {
		if fixture.wantPending {
			want = append(want, fixture.token)
		}
	}
	sort.Strings(want)

	assertSameStrings(t, "pending admin invites", got, want)
}

// TestAdminInviteCreateStoresCanonicalLayout proves CreateInvite writes
// expires_at in the one layout every reader in the project expects.
func TestAdminInviteCreateStoresCanonicalLayout(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	model := &AdminInviteModel{DB: serverDB}
	invite, err := model.CreateInvite("invitee@example.com", 1, 15*time.Minute)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	var stored string
	err = serverDB.QueryRow("SELECT CAST(expires_at AS TEXT) FROM server_admin_invites WHERE rowid = ?", invite.ID).Scan(&stored)
	if err != nil {
		t.Fatalf("read stored expires_at: %v", err)
	}

	parsed, err := time.Parse(sqlTimestampLayout, stored)
	if err != nil {
		t.Fatalf("stored expires_at %q is not canonical UTC text: %v", stored, err)
	}
	if want := invite.ExpiresAt.UTC().Truncate(time.Second); !parsed.Equal(want) {
		t.Fatalf("stored expires_at = %s, want %s", parsed, want)
	}

	// The freshly created invite must be visible as pending, which only holds
	// if the writer's layout and the reader's parser agree.
	pending, err := model.GetPendingInvites()
	if err != nil {
		t.Fatalf("GetPendingInvites: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending invites = %d, want 1", len(pending))
	}
}
