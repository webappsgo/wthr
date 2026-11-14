package graphql

import (
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// --- Shared test fixtures -------------------------------------------------
// These resolvers exercise real handler/model logic against a real sqlite
// DualDB, since several model methods (UserModel.Create, AdminModel.Create,
// AdminInviteModel.CreateInvite) bypass their own DB field and always hit
// the database.GetServerDB()/GetUsersDB() package globals. Every test below
// therefore sets the global via database.SetGlobalDualDB and resets it in
// t.Cleanup. These tests cannot run t.Parallel with each other.

func newAuthMutationTestDB(t *testing.T) *database.DualDB {
	t.Helper()
	ddb, err := database.InitDualDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDualDB: %v", err)
	}
	database.SetGlobalDualDB(ddb)
	t.Cleanup(func() {
		database.SetGlobalDualDB(nil)
		ddb.Close()
	})
	return ddb
}

func setOpenRegistrationConfig(t *testing.T) {
	t.Helper()
	config.SetGlobalConfig(&config.AppConfig{
		Users: config.UsersConfig{
			Enabled: true,
			Registration: config.RegistrationConfig{
				Mode: "open",
			},
		},
	})
	t.Cleanup(func() { config.SetGlobalConfig(nil) })
}

func seedGraphQLUser(t *testing.T, ddb *database.DualDB, username, email, password string) *models.User {
	t.Helper()
	user, err := (&models.UserModel{DB: ddb.Users}).CreateUserAccount(username, email, password, "user")
	if err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
	return user
}

// seedPendingTwoFactorSession replicates handler.createPendingTwoFactorSession
// (unexported) using only the public SessionModel API, since it isn't
// reachable from the graphql package.
func seedPendingTwoFactorSession(t *testing.T, ddb *database.DualDB, userID int64) string {
	t.Helper()
	sessionModel := &models.SessionModel{DB: ddb.Users}
	session, err := sessionModel.CreateSession(userID, 900)
	if err != nil {
		t.Fatalf("create pending session: %v", err)
	}
	if err := sessionModel.UpdateData(session.ID, map[string]interface{}{
		"auth_stage":        "pending_2fa",
		"requires_2fa":      true,
		"temporary_session": true,
	}); err != nil {
		t.Fatalf("update pending session data: %v", err)
	}
	return session.ID
}
