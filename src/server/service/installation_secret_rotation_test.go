package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/secret"
)

// rotationFixture installs a secret store over a scratch database and returns the report store.
func rotationFixture(t *testing.T) *SecurityReportStore {
	t.Helper()
	store := newSecurityTestStore(t)
	secrets := secret.NewStore(store.Store)
	if err := secrets.EnsureAll(); err != nil {
		t.Fatal(err)
	}
	secret.SetDefault(secrets)
	t.Cleanup(func() { secret.SetDefault(nil) })
	return store
}

func TestRotateInstallationSecretRebasesThePrivateKey(t *testing.T) {
	store := rotationFixture(t)
	if _, err := store.PGP.Generate("wthr", "security@example.com", false); err != nil {
		t.Fatal(err)
	}
	before, err := store.PGP.PrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	oldSecret, _ := InstallationSecret(store.Store)

	if err := RotateInstallationSecret(store.Store, store.PGP); err != nil {
		t.Fatalf("rotation failed: %v", err)
	}

	newSecret, _ := InstallationSecret(store.Store)
	if string(newSecret) == string(oldSecret) {
		t.Fatal("the installation secret must change")
	}
	after, err := store.PGP.PrivateKey()
	if err != nil || after != before {
		t.Fatalf("the private key must read back unchanged after rotation, err=%v", err)
	}
	data, _ := os.ReadFile(store.PGP.privPath())
	if _, err := decryptUnder(newSecret, string(data)); err != nil {
		t.Errorf("the file must now open under the NEW secret: %v", err)
	}
	if _, err := decryptUnder(oldSecret, string(data)); err == nil {
		t.Error("the file must no longer open under the old secret")
	}
}

func TestRotateInstallationSecretKeepsInFlightSecurityIDsValid(t *testing.T) {
	store := rotationFixture(t)
	oldID, _ := CurrentSecurityID(store.Store)

	if err := RotateInstallationSecret(store.Store, store.PGP); err != nil {
		t.Fatal(err)
	}
	if !ValidSecurityID(store.Store, oldID) {
		t.Error("a security_id issued before the rotation must keep validating during the grace period")
	}
	newID, _ := CurrentSecurityID(store.Store)
	if newID == oldID || !ValidSecurityID(store.Store, newID) {
		t.Error("the new secret issues a different, valid id")
	}
}

func TestSecurityIDStopsValidatingAfterTheGracePeriod(t *testing.T) {
	store := rotationFixture(t)
	oldID, _ := CurrentSecurityID(store.Store)
	if err := RotateInstallationSecret(store.Store, store.PGP); err != nil {
		t.Fatal(err)
	}

	secrets := secret.Default()
	// Retiring after the grace period removes the previous value, so the old id fails closed
	if _, err := database.ExecContext(context.Background(), store.Store, database.TimeoutWrite, `UPDATE app_secrets SET value = ? WHERE name = 'installation_secret_previous_until'`,
		time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if retired, err := secrets.RetireExpired(); err != nil || len(retired) != 1 {
		t.Fatalf("the expired previous value must be retired, got %v, %v", retired, err)
	}
	if ValidSecurityID(store.Store, oldID) {
		t.Error("an id from before the rotation must stop validating once the previous secret is retired")
	}
}

func TestRebaseRefusesAFileThatOpensUnderNeitherSecret(t *testing.T) {
	store := rotationFixture(t)
	if err := os.MkdirAll(store.PGP.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.PGP.privPath(), []byte("not an encrypted key"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := store.PGP.Rebase([]byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32)))
	if err == nil {
		t.Error("a file that opens under neither secret must be reported, never overwritten")
	}
	if data, _ := os.ReadFile(store.PGP.privPath()); string(data) != "not an encrypted key" {
		t.Error("the file must be left untouched")
	}
}

func TestRebaseWithoutAKeypairIsANoOp(t *testing.T) {
	store := rotationFixture(t)
	if err := store.PGP.Rebase([]byte("old"), []byte("new")); err != nil {
		t.Errorf("no keypair means nothing to rebase, got %v", err)
	}
	if err := RotateInstallationSecret(store.Store, store.PGP); err != nil {
		t.Errorf("rotating without a keypair must work, got %v", err)
	}
}
