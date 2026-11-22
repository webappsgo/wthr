package model

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// AdminTOTPStatus contains public status only; it never includes the secret.
type AdminTOTPStatus struct {
	Enabled           bool `json:"enabled"`
	RecoveryKeysCount int  `json:"recovery_keys_count"`
}

// AdminTOTPStatusFor returns the current MFA status for an administrator.
func AdminTOTPStatusFor(db *sql.DB, adminID int64) (AdminTOTPStatus, error) {
	var status AdminTOTPStatus
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT totp_enabled FROM admins WHERE id = ?`, adminID).Scan(&status.Enabled)
	if err != nil || !status.Enabled {
		return status, err
	}
	err = database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT COUNT(*) FROM admin_recovery_keys WHERE admin_id = ? AND used_at IS NULL`, adminID).Scan(&status.RecoveryKeysCount)
	return status, err
}

// StorePendingAdminTOTPSecret encrypts and stores a setup secret without enabling MFA.
func StorePendingAdminTOTPSecret(db *sql.DB, adminID int64, secret string) error {
	cfg := config.GetGlobalConfig()
	if cfg == nil || strings.TrimSpace(cfg.Server.Security.EncryptionKey) == "" {
		return fmt.Errorf("encryption key is not configured")
	}
	sealed, err := util.EncryptAtRest(cfg.Server.Security.EncryptionKey, secret)
	if err != nil {
		return err
	}
	result, err := database.ExecContext(context.Background(), db, database.TimeoutWrite,
		`UPDATE admins SET totp_secret = ? WHERE id = ? AND totp_enabled = 0`, sealed, adminID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// PendingAdminTOTPSecret decrypts the staged setup secret for confirmation.
func PendingAdminTOTPSecret(db *sql.DB, adminID int64) (string, error) {
	var sealed sql.NullString
	if err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT totp_secret FROM admins WHERE id = ? AND totp_enabled = 0`, adminID).Scan(&sealed); err != nil {
		return "", err
	}
	if !sealed.Valid || sealed.String == "" {
		return "", sql.ErrNoRows
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return "", fmt.Errorf("encryption key is not configured")
	}
	return decryptAdminTOTPSecret(cfg.Server.Security.EncryptionKey, sealed.String)
}

// EnableAdminTOTP enables TOTP and replaces recovery keys atomically.
func EnableAdminTOTP(db *sql.DB, adminID int64, secret string) ([]string, error) {
	keys, hashes, err := generateAdminRecoveryKeys()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), database.TimeoutWrite)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM admin_recovery_keys WHERE admin_id = ?`, adminID); err != nil {
		return nil, err
	}
	for _, hash := range hashes {
		if _, err = tx.ExecContext(ctx, `INSERT INTO admin_recovery_keys (admin_id, key_hash, created_at) VALUES (?, ?, ?)`, adminID, hash, time.Now().UTC()); err != nil {
			return nil, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE admins SET totp_enabled = 1 WHERE id = ? AND totp_secret IS NOT NULL`, adminID)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return nil, sql.ErrNoRows
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return keys, nil
}

// VerifyAdminTOTP checks a TOTP code or consumes a one-time recovery key.
func VerifyAdminTOTP(db *sql.DB, adminID int64, code string, useRecoveryKey bool) (bool, error) {
	if useRecoveryKey {
		return consumeAdminRecoveryKey(db, adminID, code)
	}
	var enabled bool
	var sealed sql.NullString
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT totp_enabled, totp_secret FROM admins WHERE id = ?`, adminID).Scan(&enabled, &sealed)
	if err != nil || !enabled || !sealed.Valid {
		return false, err
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return false, fmt.Errorf("encryption key is not configured")
	}
	secret, err := decryptAdminTOTPSecret(cfg.Server.Security.EncryptionKey, sealed.String)
	if err != nil {
		return false, err
	}
	return util.VerifyTOTP(secret, code)
}

func decryptAdminTOTPSecret(currentKey, sealed string) (string, error) {
	secret, _, err := util.DecryptAtRestGrace(currentKey, sealed)
	if err != nil {
		log.Printf("SECURITY event=security.decrypt_failed resource=admin_totp_secret")
	}
	return secret, err
}

// DisableAdminTOTP removes the encrypted secret and invalidates every recovery key.
func DisableAdminTOTP(db *sql.DB, adminID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), database.TimeoutWrite)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM admin_recovery_keys WHERE admin_id = ?`, adminID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE admins SET totp_enabled = 0, totp_secret = NULL WHERE id = ?`, adminID); err != nil {
		return err
	}
	return tx.Commit()
}

// RegenerateAdminRecoveryKeys replaces all existing codes after caller verification.
func RegenerateAdminRecoveryKeys(db *sql.DB, adminID int64) ([]string, error) {
	keys, hashes, err := generateAdminRecoveryKeys()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), database.TimeoutWrite)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var enabled bool
	if err := tx.QueryRowContext(ctx, `SELECT totp_enabled FROM admins WHERE id = ?`, adminID).Scan(&enabled); err != nil || !enabled {
		if err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM admin_recovery_keys WHERE admin_id = ?`, adminID); err != nil {
		return nil, err
	}
	for _, hash := range hashes {
		if _, err = tx.ExecContext(ctx, `INSERT INTO admin_recovery_keys (admin_id, key_hash, created_at) VALUES (?, ?, ?)`, adminID, hash, time.Now().UTC()); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return keys, nil
}

func consumeAdminRecoveryKey(db *sql.DB, adminID int64, code string) (bool, error) {
	code = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	plain := strings.ReplaceAll(code, "-", "")
	if len(plain) != 12 {
		return false, nil
	}
	code = plain[:8] + "-" + plain[8:]
	result, err := database.ExecContext(context.Background(), db, database.TimeoutWrite,
		`UPDATE admin_recovery_keys SET used_at = ? WHERE admin_id = ? AND key_hash = ? AND used_at IS NULL`, time.Now().UTC(), adminID, HashAPIToken(code))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func generateAdminRecoveryKeys() ([]string, []string, error) {
	keys, hashes := make([]string, 10), make([]string, 10)
	for i := range keys {
		raw := make([]byte, 6)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		hexKey := hex.EncodeToString(raw)
		keys[i] = hexKey[:8] + "-" + hexKey[8:]
		hashes[i] = HashAPIToken(keys[i])
	}
	return keys, hashes, nil
}
