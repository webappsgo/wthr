package model

import (
	"database/sql"
	"testing"
	"time"
)

// The fixed zones adminZoneWest (-11:00) and adminZoneEast (+13:00) and the
// localLayout constant are shared with admin_timestamp_test.go and
// timestamp_cleanup_test.go. They are reused here so the same guarantee holds:
// a timestamp rendered in adminZoneWest reads eleven hours EARLIER as
// wall-clock text than the instant it represents, and one rendered in
// adminZoneEast reads thirteen hours LATER, so text ordering and true instant
// ordering disagree on every host timezone.

// userSessionFixture describes one row planted in user_sessions. Both
// timestamp columns are written verbatim so each case controls the exact
// on-disk layout rather than relying on the driver's serialization.
type userSessionFixture struct {
	rawToken string
	// storedExpiresAt is the literal text written to expires_at.
	storedExpiresAt string
	// storedLastUsedAt is the literal text written to last_used_at.
	storedLastUsedAt string
	// wantActive records whether the row is genuinely live at the moment the
	// test runs, judged by the true instant its expires_at represents.
	wantActive bool
}

// insertUserSessionFixtures plants the fixture rows for one user.
func insertUserSessionFixtures(t *testing.T, db *sql.DB, userID int64, fixtures []userSessionFixture) {
	t.Helper()

	for _, fixture := range fixtures {
		_, err := db.Exec(`
			INSERT INTO user_sessions (user_id, token_hash, ip_address, user_agent, created_at, expires_at, last_used_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`,
			userID,
			hashUserToken(fixture.rawToken),
			"203.0.113.10",
			"go-test",
			sqlTimestamp(time.Now().Add(-time.Hour)),
			fixture.storedExpiresAt,
			fixture.storedLastUsedAt,
		)
		if err != nil {
			t.Fatalf("insert user session fixture %q: %v", fixture.rawToken, err)
		}
	}
}

// userSessionExpiryFixtures builds the expiry fixture set relative to now.
// Each row sits one hour either side of now, so the outcome never depends on
// how long the test takes to run.
func userSessionExpiryFixtures(now time.Time) []userSessionFixture {
	lastUsed := sqlTimestamp(now.Add(-time.Minute))
	return []userSessionFixture{
		{
			rawToken:         "live-utc",
			storedExpiresAt:  sqlTimestamp(now.Add(time.Hour)),
			storedLastUsedAt: lastUsed,
			wantActive:       true,
		},
		{
			rawToken:         "expired-utc",
			storedExpiresAt:  sqlTimestamp(now.Add(-time.Hour)),
			storedLastUsedAt: lastUsed,
			wantActive:       false,
		},
		{
			rawToken:         "live-west-zone-text-reads-as-past",
			storedExpiresAt:  now.Add(time.Hour).In(adminZoneWest).Format(localLayout),
			storedLastUsedAt: lastUsed,
			wantActive:       true,
		},
		{
			rawToken:         "expired-east-zone-text-reads-as-future",
			storedExpiresAt:  now.Add(-time.Hour).In(adminZoneEast).Format(localLayout),
			storedLastUsedAt: lastUsed,
			wantActive:       false,
		},
		{
			rawToken:         "unparseable-fails-closed",
			storedExpiresAt:  "not-a-timestamp",
			storedLastUsedAt: lastUsed,
			wantActive:       false,
		},
	}
}

// TestUserSessionGetRowIDByTokenHonorsStoredZone proves GetRowIDByToken judges
// a session live or dead by the absolute instant its expires_at represents.
//
// BEHAVIOUR CHANGE COVERAGE. The pre-fix query was
// "SELECT id FROM user_sessions WHERE token_hash = ? AND expires_at > ?" with a
// raw time.Time bound as the second parameter. That is a lexicographic TEXT
// comparison across two incompatible layouts:
//   - "live-west-zone-text-reads-as-past" stores "...-1100 WST" text whose
//     wall-clock reading is eleven hours behind the instant it means, so the
//     comparison rejected a session that is still live for another hour and the
//     user was logged out early.
//   - "expired-east-zone-text-reads-as-future" stores "...+1300 EST13" text
//     reading thirteen hours ahead, so a session that expired an hour ago
//     compared as still valid and kept granting access.
//   - "unparseable-fails-closed" compared as a plain string against the bound
//     value with an arbitrary result; it must now always fail closed.
func TestUserSessionGetRowIDByTokenHonorsStoredZone(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	userID := insertTestUser(t, usersDB, "rowidzone", "rowidzone@example.com")

	now := time.Now()
	fixtures := userSessionExpiryFixtures(now)
	insertUserSessionFixtures(t, usersDB, userID, fixtures)

	model := &UserSessionModel{DB: usersDB}
	for _, fixture := range fixtures {
		rowID, err := model.GetRowIDByToken(fixture.rawToken)
		if err != nil {
			t.Fatalf("GetRowIDByToken(%q): %v", fixture.rawToken, err)
		}
		if fixture.wantActive && rowID == 0 {
			t.Errorf("GetRowIDByToken(%q) = 0, want a live session row ID", fixture.rawToken)
		}
		if !fixture.wantActive && rowID != 0 {
			t.Errorf("GetRowIDByToken(%q) = %d, want 0 (session is not live)", fixture.rawToken, rowID)
		}
	}

	// A token that was never issued must resolve to no row and no error.
	rowID, err := model.GetRowIDByToken("never-issued")
	if err != nil {
		t.Fatalf("GetRowIDByToken(unknown): %v", err)
	}
	if rowID != 0 {
		t.Errorf("GetRowIDByToken(unknown) = %d, want 0", rowID)
	}
}

// TestUserSessionGetActiveSessionsHonorsStoredZone proves GetActiveSessions
// lists exactly the sessions that are live as absolute instants.
//
// BEHAVIOUR CHANGE COVERAGE. The pre-fix predicate was
// "WHERE user_id = ? AND expires_at > ?" with a raw time.Time bound. It hid
// "live-west-zone-text-reads-as-past" from the user's own session list and
// listed the already-dead "expired-east-zone-text-reads-as-future" as active,
// for the same two-layout reason described above.
func TestUserSessionGetActiveSessionsHonorsStoredZone(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	userID := insertTestUser(t, usersDB, "activezone", "activezone@example.com")
	otherID := insertTestUser(t, usersDB, "otherzone", "otherzone@example.com")

	now := time.Now()
	fixtures := userSessionExpiryFixtures(now)
	insertUserSessionFixtures(t, usersDB, userID, fixtures)

	// A second user's live row must never appear in the first user's listing.
	insertUserSessionFixtures(t, usersDB, otherID, []userSessionFixture{
		{
			rawToken:         "other-user-live",
			storedExpiresAt:  sqlTimestamp(now.Add(time.Hour)),
			storedLastUsedAt: sqlTimestamp(now),
			wantActive:       true,
		},
	})

	model := &UserSessionModel{DB: usersDB}
	sessions, err := model.GetActiveSessions(userID)
	if err != nil {
		t.Fatalf("GetActiveSessions: %v", err)
	}

	got := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		got[session.SessionID] = true
	}

	for _, fixture := range fixtures {
		hash := hashUserToken(fixture.rawToken)
		if fixture.wantActive && !got[hash] {
			t.Errorf("GetActiveSessions omitted live session %q", fixture.rawToken)
		}
		if !fixture.wantActive && got[hash] {
			t.Errorf("GetActiveSessions listed non-live session %q", fixture.rawToken)
		}
	}

	if got[hashUserToken("other-user-live")] {
		t.Error("GetActiveSessions leaked another user's session")
	}
}

// TestUserSessionGetActiveSessionsOrdersByInstant proves the "most recently
// used first" ordering is computed from real instants.
//
// BEHAVIOUR CHANGE COVERAGE. The pre-fix statement ended in
// "ORDER BY last_used_at DESC", a lexicographic TEXT sort. The three rows below
// are laid out so that text ordering and instant ordering are exact opposites:
// the oldest session (three hours ago) is stored in the +13:00 zone so its text
// reads ten hours into the future and sorted FIRST, while the middle session
// (two hours ago) is stored in the -11:00 zone so its text reads thirteen hours
// in the past and sorted LAST. The user's "current sessions" screen therefore
// presented its rows in almost exactly reverse order.
func TestUserSessionGetActiveSessionsOrdersByInstant(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	userID := insertTestUser(t, usersDB, "orderzone", "orderzone@example.com")

	now := time.Now()
	liveExpiry := sqlTimestamp(now.Add(time.Hour))

	fixtures := []userSessionFixture{
		{
			rawToken:         "oldest-east-zone-text-reads-first",
			storedExpiresAt:  liveExpiry,
			storedLastUsedAt: now.Add(-3 * time.Hour).In(adminZoneEast).Format(localLayout),
			wantActive:       true,
		},
		{
			rawToken:         "newest-utc",
			storedExpiresAt:  liveExpiry,
			storedLastUsedAt: sqlTimestamp(now.Add(-time.Minute)),
			wantActive:       true,
		},
		{
			rawToken:         "middle-west-zone-text-reads-last",
			storedExpiresAt:  liveExpiry,
			storedLastUsedAt: now.Add(-2 * time.Hour).In(adminZoneWest).Format(localLayout),
			wantActive:       true,
		},
	}
	insertUserSessionFixtures(t, usersDB, userID, fixtures)

	model := &UserSessionModel{DB: usersDB}
	sessions, err := model.GetActiveSessions(userID)
	if err != nil {
		t.Fatalf("GetActiveSessions: %v", err)
	}
	if len(sessions) != len(fixtures) {
		t.Fatalf("GetActiveSessions returned %d sessions, want %d", len(sessions), len(fixtures))
	}

	want := []string{
		hashUserToken("newest-utc"),
		hashUserToken("middle-west-zone-text-reads-last"),
		hashUserToken("oldest-east-zone-text-reads-first"),
	}
	for i, wantHash := range want {
		if sessions[i].SessionID != wantHash {
			t.Fatalf("session at position %d is not the expected one; ordering is not by absolute instant", i)
		}
	}

	// The parsed instants must themselves be strictly descending, which proves
	// the ordering came from the timestamps rather than from insertion order.
	for i := 1; i < len(sessions); i++ {
		if !sessions[i-1].LastUsedAt.After(sessions[i].LastUsedAt) {
			t.Fatalf("session %d last_used_at %v is not after session %d last_used_at %v",
				i-1, sessions[i-1].LastUsedAt, i, sessions[i].LastUsedAt)
		}
	}
}

// TestSessionModelGetByIDFailsClosedOnStoredZone proves SessionModel.GetByID
// (the cookie-session reader in session.go) resolves expiry from the true
// instant and fails closed on a value it cannot interpret.
//
// BEHAVIOUR CHANGE COVERAGE. The pre-fix reader scanned expires_at straight
// into a time.Time, so the "-1100 WST" and "+1300 EST13" rows failed the scan
// outright and every lookup returned a driver conversion error instead of a
// session; the unparseable row did the same. The reader now parses the value
// itself, honours the real instant, and treats an uninterpretable expires_at as
// expired rather than granting the session unlimited life.
func TestSessionModelGetByIDFailsClosedOnStoredZone(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	userID := insertTestUser(t, usersDB, "cookiezone", "cookiezone@example.com")

	now := time.Now()
	fixtures := userSessionExpiryFixtures(now)
	insertUserSessionFixtures(t, usersDB, userID, fixtures)

	model := &SessionModel{DB: usersDB}
	for _, fixture := range fixtures {
		session, err := model.GetByID(fixture.rawToken)
		if fixture.wantActive {
			if err != nil {
				t.Errorf("GetByID(%q) = error %v, want a live session", fixture.rawToken, err)
				continue
			}
			if session.ID != fixture.rawToken {
				t.Errorf("GetByID(%q).ID = %q, want the raw token back", fixture.rawToken, session.ID)
			}
			if !session.ExpiresAt.After(now.UTC()) {
				t.Errorf("GetByID(%q).ExpiresAt = %v, want an instant in the future", fixture.rawToken, session.ExpiresAt)
			}
			continue
		}

		if err == nil {
			t.Errorf("GetByID(%q) returned a session, want it rejected", fixture.rawToken)
			continue
		}

		// A rejected session is also removed, so a second lookup must report it
		// missing rather than expired.
		if _, err := model.GetByID(fixture.rawToken); err == nil {
			t.Errorf("GetByID(%q) still resolves after rejection, want the row deleted", fixture.rawToken)
		}
	}
}
