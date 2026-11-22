package model

import (
	"bytes"
	"encoding/base64"
	"log"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/util"
)

func TestAdminTOTPEnrollmentRecoveryAndDisable(t *testing.T) {
	db := newModelUsersDB(t)
	if _, err := db.Exec(`INSERT INTO admins (username,email,password_hash) VALUES ('totp-admin','totp@example.test','hash')`); err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	old := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Security.EncryptionKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(old) })
	const adminID = int64(1)
	secret, err := totp.Generate(totp.GenerateOpts{Issuer: "Test", AccountName: "admin"})
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	if err := StorePendingAdminTOTPSecret(db, adminID, secret.Secret()); err != nil {
		t.Fatalf("store secret: %v", err)
	}
	var stored string
	if err := db.QueryRow(`SELECT totp_secret FROM admins WHERE id = ?`, adminID).Scan(&stored); err != nil {
		t.Fatalf("read encrypted secret: %v", err)
	}
	if stored == secret.Secret() {
		t.Fatal("TOTP secret was stored in plaintext")
	}
	keys, err := EnableAdminTOTP(db, adminID, secret.Secret())
	if err != nil {
		t.Fatalf("enable TOTP: %v", err)
	}
	if len(keys) != 10 {
		t.Fatalf("expected 10 recovery keys, got %d", len(keys))
	}
	code, err := totp.GenerateCode(secret.Secret(), time.Now())
	if err != nil {
		t.Fatalf("generate TOTP code: %v", err)
	}
	valid, err := VerifyAdminTOTP(db, adminID, code, false)
	if err != nil || !valid {
		t.Fatalf("verify TOTP: valid=%t err=%v", valid, err)
	}
	valid, err = VerifyAdminTOTP(db, adminID, keys[0], true)
	if err != nil || !valid {
		t.Fatalf("consume recovery key: valid=%t err=%v", valid, err)
	}
	valid, err = VerifyAdminTOTP(db, adminID, keys[0], true)
	if err != nil || valid {
		t.Fatalf("reused recovery key accepted: valid=%t err=%v", valid, err)
	}
	newKeys, err := RegenerateAdminRecoveryKeys(db, adminID)
	if err != nil || len(newKeys) != 10 {
		t.Fatalf("regenerate keys: count=%d err=%v", len(newKeys), err)
	}
	valid, err = VerifyAdminTOTP(db, adminID, code, false)
	if err != nil || !valid {
		t.Fatalf("verify after regeneration: valid=%t err=%v", valid, err)
	}
	if err := DisableAdminTOTP(db, adminID); err != nil {
		t.Fatalf("disable TOTP: %v", err)
	}
	status, err := AdminTOTPStatusFor(db, adminID)
	if err != nil || status.Enabled {
		t.Fatalf("expected disabled status: %+v err=%v", status, err)
	}
}

func TestAdminTOTPReadsPreviousEncryptionKeyDuringGrace(t *testing.T) {
	db := newModelUsersDB(t)
	for _, admin := range []struct{ username, email string }{{"pending-grace", "pending-grace@example.test"}, {"active-grace", "active-grace@example.test"}} {
		if _, err := db.Exec(`INSERT INTO admins (username,email,password_hash) VALUES (?,?, 'hash')`, admin.username, admin.email); err != nil {
			t.Fatalf("insert admin %q: %v", admin.username, err)
		}
	}
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	newKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	oldConfig := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Security.EncryptionKey = oldKey
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() {
		config.SetGlobalConfig(oldConfig)
		util.SetPreviousEncryptionKeyProvider(nil)
	})

	pendingSecret, err := totp.Generate(totp.GenerateOpts{Issuer: "Test", AccountName: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if err := StorePendingAdminTOTPSecret(db, 1, pendingSecret.Secret()); err != nil {
		t.Fatalf("store pending secret: %v", err)
	}
	activeSecret, err := totp.Generate(totp.GenerateOpts{Issuer: "Test", AccountName: "active"})
	if err != nil {
		t.Fatal(err)
	}
	sealedActive, err := util.EncryptAtRest(oldKey, activeSecret.Secret())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE admins SET totp_enabled = 1, totp_secret = ? WHERE id = 2`, sealedActive); err != nil {
		t.Fatal(err)
	}

	config.GetGlobalConfig().Server.Security.EncryptionKey = newKey
	util.SetPreviousEncryptionKeyProvider(func() string { return oldKey })
	if got, err := PendingAdminTOTPSecret(db, 1); err != nil || got != pendingSecret.Secret() {
		t.Fatalf("pending TOTP secret after key rotation = %q, %v", got, err)
	}
	code, err := totp.GenerateCode(activeSecret.Secret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	valid, err := VerifyAdminTOTP(db, 2, code, false)
	if err != nil || !valid {
		t.Fatalf("verify TOTP with previous encryption key = %t, %v", valid, err)
	}
}

func TestAdminTOTPDecryptFailureIsLogged(t *testing.T) {
	db := newModelUsersDB(t)
	if _, err := db.Exec(`INSERT INTO admins (username,email,password_hash,totp_secret) VALUES ('broken-totp','broken@example.test','hash','invalid-ciphertext')`); err != nil {
		t.Fatal(err)
	}
	oldConfig := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Security.EncryptionKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(oldConfig) })

	var output bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousWriter) })
	if _, err := PendingAdminTOTPSecret(db, 1); err == nil {
		t.Fatal("invalid TOTP ciphertext unexpectedly decrypted")
	}
	if !bytes.Contains(output.Bytes(), []byte("security.decrypt_failed")) {
		t.Fatalf("decrypt failure log = %q, want security.decrypt_failed", output.String())
	}
}
