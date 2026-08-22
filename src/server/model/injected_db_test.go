package model

import (
	"database/sql"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// Every test in this file pins the same contract: when a model is constructed
// with an explicit *sql.DB, that handle is the database its methods read and
// write. The process-global handle is deliberately wired to a SECOND, freshly
// created database seeded with contradicting rows, so a model that quietly
// falls back to database.GetUsersDB()/GetServerDB() fails the assertion instead
// of passing by luck. Both fixture databases are built by executing the real
// database.UsersSchema / database.ServerSchema constants.

// newInjectedAndGlobalUsersDBs returns an injected users database plus a
// separate decoy users database that is wired in as the process-global handle
// (alongside a decoy server handle) for the duration of the test.
func newInjectedAndGlobalUsersDBs(t *testing.T) (injected, global *sql.DB) {
	t.Helper()
	injected = newModelUsersDB(t)
	global = newModelUsersDB(t)
	setModelGlobalDualDB(t, newModelServerDB(t), global)
	return injected, global
}

// newInjectedAndGlobalServerDBs returns an injected server database plus a
// separate decoy server database that is wired in as the process-global handle
// (alongside a decoy users handle) for the duration of the test.
func newInjectedAndGlobalServerDBs(t *testing.T) (injected, global *sql.DB) {
	t.Helper()
	injected = newModelServerDB(t)
	global = newModelServerDB(t)
	setModelGlobalDualDB(t, global, newModelUsersDB(t))
	return injected, global
}

// countRows returns the number of rows in table, failing the test on error.
func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestInjectedHandle_UserModel proves UserModel reads and writes the injected
// users handle even though a different users database is installed globally
// with a contradicting row for the same username.
func TestInjectedHandle_UserModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)

	if _, err := (&UserModel{DB: global}).Create("alice", "global@example.com", "GlobalPassw0rd!"); err != nil {
		t.Fatalf("seed global user: %v", err)
	}

	model := &UserModel{DB: injected}
	if _, err := model.Create("alice", "injected@example.com", "InjectedPassw0rd!"); err != nil {
		t.Fatalf("create injected user: %v", err)
	}

	got, err := model.GetByUsername("alice")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got.Email != "injected@example.com" {
		t.Errorf("GetByUsername read the wrong database: email = %q, want %q", got.Email, "injected@example.com")
	}
}

// TestInjectedHandle_UserSessionModel proves UserSessionModel writes the
// injected handle: the session it creates is unknown to the global database.
func TestInjectedHandle_UserSessionModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	userID := insertTestUser(t, injected, "session-user", "session-user@example.com")

	if _, err := (&UserSessionModel{DB: global}).CreateSession(userID, "10.0.0.1", "global-agent", time.Hour); err != nil {
		t.Fatalf("seed global session: %v", err)
	}

	model := &UserSessionModel{DB: injected}
	created, err := model.CreateSession(userID, "10.0.0.2", "injected-agent", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := model.GetSession(created.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.UserAgent != "injected-agent" {
		t.Errorf("GetSession read the wrong database: user_agent = %q, want %q", got.UserAgent, "injected-agent")
	}
	if n := countRows(t, global, "user_sessions"); n != 1 {
		t.Errorf("global users db has %d sessions, want only the seeded 1 - the model wrote the wrong database", n)
	}
}

// TestInjectedHandle_SessionModel proves SessionModel (the cookie-session
// model) writes and reads the injected handle only.
func TestInjectedHandle_SessionModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	userID := insertTestUser(t, injected, "cookie-user", "cookie-user@example.com")

	if _, err := (&SessionModel{DB: global}).Create(userID, 3600); err != nil {
		t.Fatalf("seed global session: %v", err)
	}

	model := &SessionModel{DB: injected}
	created, err := model.Create(userID, 3600)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := model.GetByID(created.ID); err != nil {
		t.Fatalf("GetByID could not find the session it just created: %v", err)
	}
	if n := countRows(t, global, "user_sessions"); n != 1 {
		t.Errorf("global users db has %d sessions, want only the seeded 1 - the model wrote the wrong database", n)
	}
}

// TestInjectedHandle_TokenModel proves TokenModel lists only the tokens stored
// in the injected handle.
func TestInjectedHandle_TokenModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	userID := insertTestUser(t, injected, "token-user", "token-user@example.com")

	if _, err := (&TokenModel{DB: global}).Create(int(userID), "global-token"); err != nil {
		t.Fatalf("seed global token: %v", err)
	}

	model := &TokenModel{DB: injected}
	if _, err := model.Create(int(userID), "injected-token"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tokens, err := model.GetByUserID(int(userID))
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Name != "injected-token" {
		t.Errorf("GetByUserID read the wrong database: got %d tokens, first name %q", len(tokens), tokenName(tokens))
	}
}

// tokenName returns the first token's name, or an empty string, for error text.
func tokenName(tokens []*APIToken) string {
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0].Name
}

// TestInjectedHandle_RecoveryKeyModel proves RecoveryKeyModel counts the keys
// in the injected handle: the global database deliberately holds a different
// number of unused keys for the same user.
func TestInjectedHandle_RecoveryKeyModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	userID := insertTestUser(t, injected, "recovery-user", "recovery-user@example.com")

	if _, err := (&RecoveryKeyModel{DB: global}).GenerateRecoveryKeys(int(userID)); err != nil {
		t.Fatalf("seed global recovery keys: %v", err)
	}

	model := &RecoveryKeyModel{DB: injected}
	keys, err := model.GenerateRecoveryKeys(int(userID))
	if err != nil {
		t.Fatalf("GenerateRecoveryKeys: %v", err)
	}

	// Burn four keys in the injected database only, so the two databases now
	// disagree: 6 unused here versus 10 unused in the global decoy.
	for i := 0; i < 4; i++ {
		ok, err := model.VerifyAndUseRecoveryKey(int(userID), keys[i])
		if err != nil || !ok {
			t.Fatalf("VerifyAndUseRecoveryKey(%d) = %v, %v", i, ok, err)
		}
	}

	count, err := model.GetUnusedKeysCount(int(userID))
	if err != nil {
		t.Fatalf("GetUnusedKeysCount: %v", err)
	}
	if count != 6 {
		t.Errorf("GetUnusedKeysCount = %d, want 6 - the model read the wrong database", count)
	}
}

// TestInjectedHandle_UserEmailVerificationModel proves the verification lookup
// resolves against the injected handle.
func TestInjectedHandle_UserEmailVerificationModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	userID := insertTestUser(t, injected, "verify-user", "verify-user@example.com")

	if _, err := (&UserEmailVerificationModel{DB: global}).CreateVerification(userID, "global@example.com"); err != nil {
		t.Fatalf("seed global verification: %v", err)
	}

	model := &UserEmailVerificationModel{DB: injected}
	created, err := model.CreateVerification(userID, "injected@example.com")
	if err != nil {
		t.Fatalf("CreateVerification: %v", err)
	}

	got, err := model.GetVerification(created.Token)
	if err != nil {
		t.Fatalf("GetVerification: %v", err)
	}
	if got.Email != "injected@example.com" {
		t.Errorf("GetVerification read the wrong database: email = %q", got.Email)
	}
}

// TestInjectedHandle_UserPasswordResetModel proves the reset lookup resolves
// against the injected handle.
func TestInjectedHandle_UserPasswordResetModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	injectedUserID := insertTestUser(t, injected, "reset-user", "reset-user@example.com")
	globalUserID := insertTestUser(t, global, "reset-user", "reset-user@example.com")

	if _, err := (&UserPasswordResetModel{DB: global}).CreateReset(globalUserID); err != nil {
		t.Fatalf("seed global reset: %v", err)
	}

	model := &UserPasswordResetModel{DB: injected}
	created, err := model.CreateReset(injectedUserID)
	if err != nil {
		t.Fatalf("CreateReset: %v", err)
	}

	got, err := model.GetReset(created.Token)
	if err != nil {
		t.Fatalf("GetReset could not find the reset it just created: %v", err)
	}
	if got.UserID != injectedUserID {
		t.Errorf("GetReset read the wrong database: user_id = %d, want %d", got.UserID, injectedUserID)
	}
	if n := countRows(t, global, "user_password_resets"); n != 1 {
		t.Errorf("global users db has %d resets, want only the seeded 1 - the model wrote the wrong database", n)
	}
}

// TestInjectedHandle_UserActivityLogModel proves activity rows are written to
// and read from the injected handle.
func TestInjectedHandle_UserActivityLogModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	userID := insertTestUser(t, injected, "activity-user", "activity-user@example.com")

	if err := (&UserActivityLogModel{DB: global}).LogActivity(userID, "login", "10.0.0.1", "global-agent", "global"); err != nil {
		t.Fatalf("seed global activity: %v", err)
	}

	model := &UserActivityLogModel{DB: injected}
	if err := model.LogActivity(userID, "login", "10.0.0.2", "injected-agent", "injected"); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}

	activities, err := model.GetActivities(userID, 0, 10)
	if err != nil {
		t.Fatalf("GetActivities: %v", err)
	}
	if len(activities) != 1 || activities[0].UserAgent != "injected-agent" {
		t.Errorf("GetActivities read the wrong database: got %d rows", len(activities))
	}
}

// TestInjectedHandle_UserInviteModel proves invite lookups resolve against the
// injected handle.
func TestInjectedHandle_UserInviteModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)

	if _, err := (&UserInviteModel{DB: global}).CreateInvite("bob", "global@example.com", "user", 7); err != nil {
		t.Fatalf("seed global invite: %v", err)
	}

	model := &UserInviteModel{DB: injected}
	created, err := model.CreateInvite("bob", "injected@example.com", "user", 7)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	got, err := model.GetByToken(created.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got.Email != "injected@example.com" {
		t.Errorf("GetByToken read the wrong database: email = %q", got.Email)
	}
}

// TestInjectedHandle_UserPasskeyModel proves passkey counts come from the
// injected handle, which holds one passkey while the global decoy holds two.
func TestInjectedHandle_UserPasskeyModel(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	userID := insertTestUser(t, injected, "passkey-user", "passkey-user@example.com")

	globalModel := &UserPasskeyModel{DB: global}
	if _, err := globalModel.Create(userID, "global-1", testCredential(1)); err != nil {
		t.Fatalf("seed global passkey 1: %v", err)
	}
	if _, err := globalModel.Create(userID, "global-2", testCredential(2)); err != nil {
		t.Fatalf("seed global passkey 2: %v", err)
	}

	model := &UserPasskeyModel{DB: injected}
	if _, err := model.Create(userID, "injected-1", testCredential(3)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	count, err := model.CountByUserID(userID)
	if err != nil {
		t.Fatalf("CountByUserID: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByUserID = %d, want 1 - the model read the wrong database", count)
	}
}

// TestInjectedHandle_AdminModel proves AdminModel resolves admins from the
// injected server handle rather than the globally installed decoy.
func TestInjectedHandle_AdminModel(t *testing.T) {
	injected, global := newInjectedAndGlobalServerDBs(t)

	if _, err := (&AdminModel{DB: global}).Create("root", "global@example.com", "GlobalPassw0rd!", true); err != nil {
		t.Fatalf("seed global admin: %v", err)
	}

	model := &AdminModel{DB: injected}
	if _, err := model.Create("root", "injected@example.com", "InjectedPassw0rd!", true); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := model.GetByEmail("injected@example.com"); err != nil {
		t.Fatalf("GetByEmail could not find the admin it just created: %v", err)
	}
	if _, err := model.GetByEmail("global@example.com"); err == nil {
		t.Error("GetByEmail resolved an admin that exists only in the global decoy database")
	}
}

// TestInjectedHandle_AdminInviteModel proves admin invite lookups resolve
// against the injected server handle.
func TestInjectedHandle_AdminInviteModel(t *testing.T) {
	injected, global := newInjectedAndGlobalServerDBs(t)

	if _, err := (&AdminInviteModel{DB: global}).CreateInvite("global@example.com", 1, time.Hour); err != nil {
		t.Fatalf("seed global invite: %v", err)
	}

	model := &AdminInviteModel{DB: injected}
	created, err := model.CreateInvite("injected@example.com", 1, time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	got, err := model.GetInvite(created.Token)
	if err != nil {
		t.Fatalf("GetInvite: %v", err)
	}
	if got.InvitedEmail != "injected@example.com" {
		t.Errorf("GetInvite read the wrong database: invited_email = %q", got.InvitedEmail)
	}
}

// TestInjectedHandle_AdminSessionModel proves admin sessions are written to and
// read from the injected server handle.
func TestInjectedHandle_AdminSessionModel(t *testing.T) {
	injected, global := newInjectedAndGlobalServerDBs(t)

	if _, err := (&AdminSessionModel{DB: global}).CreateSession(1, "10.0.0.1", "global-agent", time.Hour); err != nil {
		t.Fatalf("seed global admin session: %v", err)
	}

	model := &AdminSessionModel{DB: injected}
	created, err := model.CreateSession(1, "10.0.0.2", "injected-agent", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := model.GetSession(created.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.UserAgent != "injected-agent" {
		t.Errorf("GetSession read the wrong database: user_agent = %q", got.UserAgent)
	}
	if n := countRows(t, global, "server_admin_sessions"); n != 1 {
		t.Errorf("global server db has %d admin sessions, want only the seeded 1", n)
	}
}

// TestInjectedHandle_AdminPasskeyModel proves admin passkey counts come from
// the injected server handle.
func TestInjectedHandle_AdminPasskeyModel(t *testing.T) {
	injected, global := newInjectedAndGlobalServerDBs(t)

	globalModel := &AdminPasskeyModel{DB: global}
	if _, err := globalModel.Create(1, "global-1", testCredential(1)); err != nil {
		t.Fatalf("seed global admin passkey 1: %v", err)
	}
	if _, err := globalModel.Create(1, "global-2", testCredential(2)); err != nil {
		t.Fatalf("seed global admin passkey 2: %v", err)
	}

	model := &AdminPasskeyModel{DB: injected}
	if _, err := model.Create(1, "injected-1", testCredential(3)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	count, err := model.CountByAdminID(1)
	if err != nil {
		t.Fatalf("CountByAdminID: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByAdminID = %d, want 1 - the model read the wrong database", count)
	}
}

// TestModelHandleAccessors pins both halves of the documented accessor
// contract for every converted model type: an injected handle is returned
// verbatim, and a nil field falls back to the process-global handle.
func TestModelHandleAccessors(t *testing.T) {
	usersDB := newModelUsersDB(t)
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	usersCases := []struct {
		name    string
		resolve func(db *sql.DB) *sql.DB
	}{
		{"UserModel", func(db *sql.DB) *sql.DB { return (&UserModel{DB: db}).getDB() }},
		{"UserSessionModel", func(db *sql.DB) *sql.DB { return (&UserSessionModel{DB: db}).getDB() }},
		{"UserEmailVerificationModel", func(db *sql.DB) *sql.DB { return (&UserEmailVerificationModel{DB: db}).getDB() }},
		{"UserPasswordResetModel", func(db *sql.DB) *sql.DB { return (&UserPasswordResetModel{DB: db}).getDB() }},
		{"UserActivityLogModel", func(db *sql.DB) *sql.DB { return (&UserActivityLogModel{DB: db}).getDB() }},
		{"UserInviteModel", func(db *sql.DB) *sql.DB { return (&UserInviteModel{DB: db}).getDB() }},
		{"SessionModel", func(db *sql.DB) *sql.DB { return (&SessionModel{DB: db}).getDB() }},
		{"TokenModel", func(db *sql.DB) *sql.DB { return (&TokenModel{DB: db}).getDB() }},
		{"RecoveryKeyModel", func(db *sql.DB) *sql.DB { return (&RecoveryKeyModel{DB: db}).getDB() }},
		{"UserPasskeyModel", func(db *sql.DB) *sql.DB { return (&UserPasskeyModel{DB: db}).getDB() }},
	}

	serverCases := []struct {
		name    string
		resolve func(db *sql.DB) *sql.DB
	}{
		{"AdminModel", func(db *sql.DB) *sql.DB { return (&AdminModel{DB: db}).getDB() }},
		{"AdminInviteModel", func(db *sql.DB) *sql.DB { return (&AdminInviteModel{DB: db}).getDB() }},
		{"AdminSessionModel", func(db *sql.DB) *sql.DB { return (&AdminSessionModel{DB: db}).getDB() }},
		{"AdminPasskeyModel", func(db *sql.DB) *sql.DB { return (&AdminPasskeyModel{DB: db}).getDB() }},
	}

	injectedUsers := newModelUsersDB(t)
	for _, tc := range usersCases {
		t.Run(tc.name+"/injected", func(t *testing.T) {
			if got := tc.resolve(injectedUsers); got != injectedUsers {
				t.Error("accessor did not return the injected handle")
			}
		})
		t.Run(tc.name+"/nil falls back", func(t *testing.T) {
			if got := tc.resolve(nil); got != usersDB {
				t.Error("accessor did not fall back to the global users handle")
			}
		})
	}

	injectedServer := newModelServerDB(t)
	for _, tc := range serverCases {
		t.Run(tc.name+"/injected", func(t *testing.T) {
			if got := tc.resolve(injectedServer); got != injectedServer {
				t.Error("accessor did not return the injected handle")
			}
		})
		t.Run(tc.name+"/nil falls back", func(t *testing.T) {
			if got := tc.resolve(nil); got != serverDB {
				t.Error("accessor did not fall back to the global server handle")
			}
		})
	}
}

// TestSettingsModelAlwaysUsesGlobalServerDB pins the deliberate exception
// documented on SettingsModel.serverDB: server_config lives in server.db, but
// callers systematically construct SettingsModel with the users handle, so the
// injected field is ignored and the global server handle is used instead.
func TestSettingsModelAlwaysUsesGlobalServerDB(t *testing.T) {
	serverDB := newModelServerDB(t)
	usersDB := newModelUsersDB(t)
	setModelGlobalDualDB(t, serverDB, usersDB)

	// The users handle is what production injects here; it has no server_config
	// table at all, so honoring it would break every settings lookup.
	model := &SettingsModel{DB: usersDB}
	if got := model.serverDB(); got != serverDB {
		t.Fatal("serverDB() did not resolve the global server handle")
	}

	if err := model.Set("unit.test.key", "from-server-db", "string"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var stored string
	if err := serverDB.QueryRow("SELECT value FROM server_config WHERE key = ?", "unit.test.key").Scan(&stored); err != nil {
		t.Fatalf("settings row was not written to server.db: %v", err)
	}
	if stored != "from-server-db" {
		t.Errorf("stored value = %q, want %q", stored, "from-server-db")
	}

	setting, err := model.Get("unit.test.key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if setting.Value != "from-server-db" {
		t.Errorf("Get value = %q, want %q", setting.Value, "from-server-db")
	}
}

// TestGlobalHandlesAreDistinctFromInjected is a guard on the fixtures
// themselves: if newModelUsersDB ever returned a shared handle, every test in
// this file would pass vacuously.
func TestGlobalHandlesAreDistinctFromInjected(t *testing.T) {
	injected, global := newInjectedAndGlobalUsersDBs(t)
	if injected == global {
		t.Fatal("fixture returned the same handle twice")
	}
	if database.GetUsersDB() != global {
		t.Fatal("global users handle is not the decoy database")
	}
	if database.GetUsersDB() == injected {
		t.Fatal("global users handle is the injected database")
	}
}
