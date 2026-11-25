package graphql

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

func setupGraphQLTestConfig(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "server.yml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("create test config: %v", err)
	}
	t.Setenv("CONFIG_DIR", configDir)
}

// --- Shared test fixtures -------------------------------------------------
// These resolvers exercise real handler/model logic against a real sqlite
// DualDB, since several model methods (UserModel.Create, AdminModel.Create,
// AdminInviteModel.CreateInvite) bypass their own DB field and always hit
// the database.GetServerDB()/GetUsersDB() package globals. Every test below
// therefore sets the global via database.SetGlobalDualDB and resets it in
// t.Cleanup. These tests cannot run t.Parallel with each other.

func newAuthMutationTestDB(t *testing.T) *database.DualDB {
	t.Helper()
	setupGraphQLTestConfig(t)
	ddb, err := database.InitDualDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDualDB: %v", err)
	}
	database.SetGlobalDualDB(ddb)
	t.Cleanup(func() {
		database.SetGlobalDualDB(nil)
		if err := ddb.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return ddb
}

func seedGraphQLUser(t *testing.T, ddb *database.DualDB, username, email, password string) *models.User {
	t.Helper()
	user, err := (&models.UserModel{Store: ddb.GetUsersStore()}).CreateUserAccount(username, email, password, "user")
	if err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
	return user
}
