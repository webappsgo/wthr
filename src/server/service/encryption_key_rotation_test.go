package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// encryptionRotationFixture builds both databases, a config holding a key and a CONFIG_DIR scratch directory.
func encryptionRotationFixture(t *testing.T) (*EncryptionKeyRotator, *SecurityReportStore, string) {
	t.Helper()
	t.Setenv("CONFIG_DIR", t.TempDir())
	ddb, err := database.InitDualDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ddb.Close() })

	key, err := newEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Security.EncryptionKey = key
	cfg.Server.Security.EncryptionKeyVersion = 1
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() {
		config.SetGlobalConfig(previous)
		util.SetPreviousEncryptionKeyProvider(nil)
	})

	rotator := &EncryptionKeyRotator{ServerDB: ddb.Server, UsersDB: ddb.Users}
	rotator.InstallGraceReads()
	reports := &SecurityReportStore{DB: ddb.Server, EncryptionKey: key}
	return rotator, reports, key
}

func TestRotateEncryptionKeyReencryptsEveryDependentRow(t *testing.T) {
	rotator, reports, oldKey := encryptionRotationFixture(t)
	backupPassword, err := util.EncryptAtRest(oldKey, "backup-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WriteSecret("server.backup.encryption_password", backupPassword); err != nil {
		t.Fatal(err)
	}

	sealedSecret, err := util.EncryptAtRest(oldKey, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotator.UsersDB.Exec(`INSERT INTO users (username, email, password_hash, two_factor_enabled, two_factor_secret) VALUES ('alice', 'alice@example.com', 'x', 1, ?)`, sealedSecret); err != nil {
		t.Fatal(err)
	}
	adminTOTPSecret, err := util.EncryptAtRest(oldKey, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotator.UsersDB.Exec(`INSERT INTO admins (username, email, password_hash, totp_enabled, totp_secret) VALUES ('admin', 'admin@example.com', 'x', 1, ?)`, adminTOTPSecret); err != nil {
		t.Fatal(err)
	}
	trackingID, _, err := reports.Create("a1b2c3d4e5f60718", sampleReport(), ReportContact{Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	if err := rotator.Rotate(); err != nil {
		t.Fatalf("rotation failed: %v", err)
	}

	cfg := config.GetGlobalConfig()
	newKey := cfg.Server.Security.EncryptionKey
	if newKey == oldKey || cfg.Server.Security.EncryptionKeyVersion != 2 {
		t.Fatalf("the key must change and its version increment, got version %d", cfg.Server.Security.EncryptionKeyVersion)
	}

	var storedSecret string
	if err := rotator.UsersDB.QueryRow(`SELECT two_factor_secret FROM users WHERE username = 'alice'`).Scan(&storedSecret); err != nil {
		t.Fatal(err)
	}
	if plain, err := util.DecryptAtRest(newKey, storedSecret); err != nil || plain != "JBSWY3DPEHPK3PXP" {
		t.Errorf("the 2FA secret must now open under the new key, got %q, %v", plain, err)
	}
	if _, err := util.DecryptAtRest(oldKey, storedSecret); err == nil {
		t.Error("the 2FA secret must no longer open under the old key")
	}
	var storedAdminTOTPSecret string
	if err := rotator.UsersDB.QueryRow(`SELECT totp_secret FROM admins WHERE username = 'admin'`).Scan(&storedAdminTOTPSecret); err != nil {
		t.Fatal(err)
	}
	if plain, err := util.DecryptAtRest(newKey, storedAdminTOTPSecret); err != nil || plain != "JBSWY3DPEHPK3PXP" {
		t.Errorf("the admin TOTP secret must now open under the new key, got %q, %v", plain, err)
	}
	if _, err := util.DecryptAtRest(oldKey, storedAdminTOTPSecret); err == nil {
		t.Error("the admin TOTP secret must no longer open under the old key")
	}

	reports.EncryptionKey = newKey
	report, contact, err := reports.Body(trackingID)
	if err != nil || report == nil || contact == nil || contact.Email != "ada@example.com" {
		t.Errorf("the report body and contact must read back under the new key, got %v, %v, %v", report, contact, err)
	}

	if pending, err := rotator.PendingRows(); err != nil || pending != 0 {
		t.Errorf("nothing may be left to re-encrypt, got %d, %v", pending, err)
	}
	if rotator.PreviousKey() != oldKey {
		t.Error("the previous key must stay readable during the grace period")
	}
	storedBackupPassword, err := config.ReadSecret("server.backup.encryption_password")
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := util.DecryptAtRest(newKey, storedBackupPassword); err != nil || plain != "backup-password" {
		t.Errorf("the scheduled-backup password must be re-encrypted under the new key, got %q, %v", plain, err)
	}
}

func TestGraceReadsOpenARowThatStillUsesThePreviousKey(t *testing.T) {
	rotator, _, oldKey := encryptionRotationFixture(t)
	sealed, _ := util.EncryptAtRest(oldKey, "legacy")
	if err := rotator.Rotate(); err != nil {
		t.Fatal(err)
	}
	newKey := config.GetGlobalConfig().Server.Security.EncryptionKey

	plain, usedPrevious, err := util.DecryptAtRestGrace(newKey, sealed)
	if err != nil || plain != "legacy" || !usedPrevious {
		t.Errorf("a value sealed before the rotation must open through the grace read, got %q %v %v", plain, usedPrevious, err)
	}
}

func TestSecondRotationIsRefusedWhileRowsAreStillOnThePreviousKey(t *testing.T) {
	rotator, _, oldKey := encryptionRotationFixture(t)
	if err := rotator.Rotate(); err != nil {
		t.Fatal(err)
	}

	// A row sealed under the previous key and not yet re-encrypted blocks a second rotation
	stale, _ := util.EncryptAtRest(oldKey, "still-old")
	if _, err := rotator.UsersDB.Exec(`INSERT INTO users (username, email, password_hash, two_factor_enabled, two_factor_secret) VALUES ('bob', 'bob@example.com', 'x', 1, ?)`, stale); err != nil {
		t.Fatal(err)
	}
	staleAdminTOTP, err := util.EncryptAtRest(oldKey, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotator.UsersDB.Exec(`INSERT INTO admins (username, email, password_hash, totp_enabled, totp_secret) VALUES ('stale-admin', 'stale-admin@example.com', 'x', 1, ?)`, staleAdminTOTP); err != nil {
		t.Fatal(err)
	}
	if pending, err := rotator.PendingRows(); err != nil || pending != 2 {
		t.Fatalf("pending user and admin secrets = %d, %v; want 2", pending, err)
	}
	if err := rotator.Rotate(); err != ErrEncryptionRotationPending {
		t.Fatalf("err = %v, want ErrEncryptionRotationPending", err)
	}

	if err := rotator.FinishUnfinishedRebase(); err != nil {
		t.Fatal(err)
	}
	if err := rotator.Rotate(); err != nil {
		t.Errorf("once every row is on the current key a rotation must work, got %v", err)
	}
}

func TestRebaseAdminTOTPRejectsUndecryptableSecret(t *testing.T) {
	rotator, _, oldKey := encryptionRotationFixture(t)
	newKey, err := newEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotator.UsersDB.Exec(`INSERT INTO admins (username, email, password_hash, totp_enabled, totp_secret) VALUES ('broken-admin', 'broken-admin@example.com', 'x', 1, 'not-an-encrypted-secret')`); err != nil {
		t.Fatal(err)
	}
	if err := rotator.Rebase(oldKey, newKey); err == nil {
		t.Fatal("Rebase accepted an admin TOTP secret that neither key can decrypt")
	}
	var stored string
	if err := rotator.UsersDB.QueryRow(`SELECT totp_secret FROM admins WHERE username = 'broken-admin'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "not-an-encrypted-secret" {
		t.Fatalf("failed rebase changed stored admin TOTP secret to %q", stored)
	}
}

func TestRetireExpiredRemovesThePreviousKeyAfterTheGracePeriod(t *testing.T) {
	rotator, _, _ := encryptionRotationFixture(t)
	if retired, err := rotator.RetireExpired(); err != nil || retired {
		t.Fatalf("nothing to retire before any rotation, got %v, %v", retired, err)
	}
	if err := rotator.Rotate(); err != nil {
		t.Fatal(err)
	}
	if retired, _ := rotator.RetireExpired(); retired {
		t.Fatal("a previous key inside its grace period must stay")
	}

	if err := rotator.writeRow(encryptionKeyUntilRow, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if retired, err := rotator.RetireExpired(); err != nil || !retired {
		t.Fatalf("an expired previous key must be retired, got %v, %v", retired, err)
	}
	if rotator.PreviousKey() != "" {
		t.Error("a retired previous key must no longer be returned")
	}
}

func TestRotateEncryptionKeyNeedsAConfiguredKey(t *testing.T) {
	rotator, _, _ := encryptionRotationFixture(t)
	config.GetGlobalConfig().Server.Security.EncryptionKey = ""
	if err := rotator.Rotate(); err != ErrEncryptionKeyUnavailable {
		t.Errorf("err = %v, want ErrEncryptionKeyUnavailable", err)
	}
}

func TestRotateEncryptionKeyReencryptsDNSCredentials(t *testing.T) {
	rotator, _, oldKey := encryptionRotationFixture(t)
	if err := os.WriteFile(filepath.Join(os.Getenv("CONFIG_DIR"), "server.yml"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sealed, err := util.SealDNSCredential(oldKey, "dns-token")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.GetGlobalConfig()
	cfg.Server.SSL.LetsEncrypt.DNSProvider = "cloudflare"
	cfg.Server.SSL.LetsEncrypt.DNSCredentials = map[string]string{"CF_DNS_API_TOKEN": sealed}

	if err := rotator.Rotate(); err != nil {
		t.Fatalf("rotation failed: %v", err)
	}

	newKey := cfg.Server.Security.EncryptionKey
	stored := cfg.Server.SSL.LetsEncrypt.DNSCredentials["CF_DNS_API_TOKEN"]
	plain, _, err := util.OpenDNSCredential(newKey, stored)
	if err != nil || plain != "dns-token" {
		t.Errorf("the credential must open under the new key, got %q, %v", plain, err)
	}
	if _, _, err := util.OpenDNSCredential(oldKey, stored); err == nil {
		t.Error("the credential must no longer open under the old key")
	}
	file, err := os.ReadFile(filepath.Join(os.Getenv("CONFIG_DIR"), "secrets.yml"))
	if err != nil || !strings.Contains(string(file), stored) {
		t.Errorf("secrets.yml must carry the re-encrypted value: %v", err)
	}
	if pending, err := rotator.PendingRows(); err != nil || pending != 0 {
		t.Errorf("nothing may be left to re-encrypt, got %d, %v", pending, err)
	}
}
