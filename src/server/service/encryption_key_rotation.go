package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

const (
	// encryptionKeyPreviousRow and encryptionKeyUntilRow hold the previous server.security.encryption_key,
	// AES-256-GCM wrapped under the new key, and the UTC time its grace period ends
	encryptionKeyPreviousRow = "encryption_key_previous"
	encryptionKeyUntilRow    = "encryption_key_previous_until"
	// EncryptionKeyGrace is how long a previous encryption key stays valid: 30 days
	EncryptionKeyGrace = 30 * 24 * time.Hour
	// rebaseBatchSize is how many rows one transaction re-encrypts
	rebaseBatchSize = 100
	// pgpArmorPrefix starts an ASCII-armored PGP message
	pgpArmorPrefix = "-----BEGIN PGP"
)

// ErrEncryptionRotationPending is returned when a second rotation is requested while rows are still
// readable only with the previous key.
var ErrEncryptionRotationPending = errors.New("rows are still encrypted under the previous key")

// ErrEncryptionKeyUnavailable is returned when no encryption key is configured.
var ErrEncryptionKeyUnavailable = errors.New("no encryption key is configured")

// EncryptionKeyRotator rotates server.security.encryption_key and re-encrypts the rows that
// depend on it: users.two_factor_secret (users.db) and the AES-sealed security report bodies and
// contacts (server.db).
type EncryptionKeyRotator struct {
	ServerDB *sql.DB
	UsersDB  *sql.DB
}

// newEncryptionKey returns a base64-encoded 32-byte key from crypto/rand.
func newEncryptionKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating encryption key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// currentKey returns the configured key and the config that holds it.
func currentEncryptionKey() (string, *config.AppConfig, error) {
	cfg := config.GetGlobalConfig()
	if cfg == nil || cfg.Server.Security.EncryptionKey == "" {
		return "", nil, ErrEncryptionKeyUnavailable
	}
	return cfg.Server.Security.EncryptionKey, cfg, nil
}

// readRow returns one app_secrets value, or sql.ErrNoRows.
func (r *EncryptionKeyRotator) readRow(name string) (string, error) {
	var value string
	err := database.QueryRowContext(context.Background(), r.ServerDB, database.TimeoutSimpleSelect,
		`SELECT value FROM app_secrets WHERE name = ?`, name).Scan(&value)
	return value, err
}

// writeRow stores one app_secrets value.
func (r *EncryptionKeyRotator) writeRow(name, value string) error {
	_, err := database.ExecContext(context.Background(), r.ServerDB, database.TimeoutWrite,
		`INSERT INTO app_secrets (name, value, created_at, rotated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(name) DO UPDATE SET value = excluded.value, rotated_at = CURRENT_TIMESTAMP`, name, value)
	return err
}

// PreviousKey returns the previous encryption key while its grace period lasts, unwrapped with
// the current key, or "" when there is none.
func (r *EncryptionKeyRotator) PreviousKey() string {
	current, _, err := currentEncryptionKey()
	if err != nil {
		return ""
	}
	untilText, err := r.readRow(encryptionKeyUntilRow)
	if err != nil {
		return ""
	}
	if until, parseErr := time.Parse(time.RFC3339, untilText); parseErr != nil || !time.Now().Before(until) {
		return ""
	}
	wrapped, err := r.readRow(encryptionKeyPreviousRow)
	if err != nil {
		return ""
	}
	previous, err := util.DecryptAtRest(current, wrapped)
	if err != nil {
		return ""
	}
	return previous
}

// GraceUntil returns when the previous key stops being valid, if one is still valid.
func (r *EncryptionKeyRotator) GraceUntil() (time.Time, bool) {
	untilText, err := r.readRow(encryptionKeyUntilRow)
	if err != nil {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339, untilText)
	if err != nil || !time.Now().Before(until) {
		return time.Time{}, false
	}
	return until, true
}

// InstallGraceReads makes every at-rest read fall back to the previous key during the grace period.
func (r *EncryptionKeyRotator) InstallGraceReads() {
	util.SetPreviousEncryptionKeyProvider(r.PreviousKey)
}

// openUnder reports whether sealed opens with key.
func openUnder(key, sealed string) bool {
	_, err := util.DecryptAtRest(key, sealed)
	return err == nil
}

// twoFactorRow is one users.two_factor_secret value.
type twoFactorRow struct {
	id     int64
	secret string
}

// loadTwoFactorRows reads every stored two-factor secret.
func (r *EncryptionKeyRotator) loadTwoFactorRows() ([]twoFactorRow, error) {
	if r.UsersDB == nil {
		return nil, nil
	}
	rows, err := database.QueryContext(context.Background(), r.UsersDB, database.TimeoutBulk,
		`SELECT id, two_factor_secret FROM users WHERE two_factor_secret IS NOT NULL AND two_factor_secret != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var found []twoFactorRow
	for rows.Next() {
		var row twoFactorRow
		if err := rows.Scan(&row.id, &row.secret); err != nil {
			return nil, err
		}
		found = append(found, row)
	}
	return found, rows.Err()
}

// reportRow is the AES-sealed part of one security report.
type reportRow struct {
	trackingID string
	body       []byte
	bodyAlg    string
	contact    []byte
}

// loadReportRows reads every report that has an AES-sealed body or contact.
func (r *EncryptionKeyRotator) loadReportRows() ([]reportRow, error) {
	rows, err := database.QueryContext(context.Background(), r.ServerDB, database.TimeoutBulk,
		`SELECT tracking_id, body_enc, body_alg, contact_enc FROM security_reports`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var found []reportRow
	for rows.Next() {
		var row reportRow
		if err := rows.Scan(&row.trackingID, &row.body, &row.bodyAlg, &row.contact); err != nil {
			return nil, err
		}
		found = append(found, row)
	}
	return found, rows.Err()
}

// aesSealedContact reports whether a contact blob is AES-sealed rather than PGP.
func aesSealedContact(contact []byte) bool {
	return len(contact) > 0 && !strings.HasPrefix(string(contact), pgpArmorPrefix)
}

// PendingRows counts the rows that open only with the previous key.
func (r *EncryptionKeyRotator) PendingRows() (int, error) {
	current, _, err := currentEncryptionKey()
	if err != nil {
		return 0, err
	}
	previous := r.PreviousKey()
	if previous == "" {
		return 0, nil
	}

	pending := 0
	twoFactor, err := r.loadTwoFactorRows()
	if err != nil {
		return 0, err
	}
	for _, row := range twoFactor {
		if !openUnder(current, row.secret) && openUnder(previous, row.secret) {
			pending++
		}
	}
	reports, err := r.loadReportRows()
	if err != nil {
		return 0, err
	}
	for _, row := range reports {
		if row.bodyAlg == "aes-256-gcm" && !openUnder(current, string(row.body)) && openUnder(previous, string(row.body)) {
			pending++
		}
		if aesSealedContact(row.contact) && !openUnder(current, string(row.contact)) && openUnder(previous, string(row.contact)) {
			pending++
		}
	}
	return pending, nil
}

// reencrypt returns sealed re-encrypted from oldKey to newKey, and changed=false when it already
// opens under newKey or opens under neither (a legacy plaintext value is left for its own migration).
func reencrypt(oldKey, newKey, sealed string) (result string, changed bool, err error) {
	if openUnder(newKey, sealed) {
		return sealed, false, nil
	}
	plaintext, decryptErr := util.DecryptAtRest(oldKey, sealed)
	if decryptErr != nil {
		return sealed, false, nil
	}
	result, err = util.EncryptAtRest(newKey, plaintext)
	return result, err == nil, err
}

// Rebase re-encrypts, eagerly and in batches, every dependent row from oldKey to newKey. A row
// already readable under newKey is skipped, so a re-run finishes unfinished work.
func (r *EncryptionKeyRotator) Rebase(oldKey, newKey string) error {
	twoFactor, err := r.loadTwoFactorRows()
	if err != nil {
		return err
	}
	for start := 0; start < len(twoFactor); start += rebaseBatchSize {
		end := min(start+rebaseBatchSize, len(twoFactor))
		if err := r.rebaseTwoFactorBatch(twoFactor[start:end], oldKey, newKey); err != nil {
			return err
		}
	}

	reports, err := r.loadReportRows()
	if err != nil {
		return err
	}
	for start := 0; start < len(reports); start += rebaseBatchSize {
		end := min(start+rebaseBatchSize, len(reports))
		if err := r.rebaseReportBatch(reports[start:end], oldKey, newKey); err != nil {
			return err
		}
	}
	return nil
}

// rebaseTwoFactorBatch re-encrypts one batch of two-factor secrets in one transaction.
func (r *EncryptionKeyRotator) rebaseTwoFactorBatch(batch []twoFactorRow, oldKey, newKey string) error {
	tx, err := r.UsersDB.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	for _, row := range batch {
		sealed, changed, err := reencrypt(oldKey, newKey, row.secret)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if !changed {
			continue
		}
		if _, err := tx.Exec(`UPDATE users SET two_factor_secret = ? WHERE id = ?`, sealed, row.id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// rebaseReportBatch re-encrypts one batch of report bodies and contacts in one transaction.
func (r *EncryptionKeyRotator) rebaseReportBatch(batch []reportRow, oldKey, newKey string) error {
	tx, err := r.ServerDB.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	for _, row := range batch {
		if row.bodyAlg == "aes-256-gcm" {
			sealed, changed, err := reencrypt(oldKey, newKey, string(row.body))
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			if changed {
				if _, err := tx.Exec(`UPDATE security_reports SET body_enc = ? WHERE tracking_id = ?`, []byte(sealed), row.trackingID); err != nil {
					_ = tx.Rollback()
					return err
				}
			}
		}
		if aesSealedContact(row.contact) {
			sealed, changed, err := reencrypt(oldKey, newKey, string(row.contact))
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			if changed {
				if _, err := tx.Exec(`UPDATE security_reports SET contact_enc = ? WHERE tracking_id = ?`, []byte(sealed), row.trackingID); err != nil {
					_ = tx.Rollback()
					return err
				}
			}
		}
	}
	return tx.Commit()
}

// Rotate replaces server.security.encryption_key following the PART 11 "Rotation contract":
// (1) write the previous-value rows (the old key wrapped under the new key), (2) persist the new key
// and its incremented version in server.yml atomically, (3) re-encrypt every dependent row eagerly.
// The previous key is stored before the new one is used, so a crash or a failed step loses nothing.
// A second rotation is refused while any row is still readable only with the previous key.
func (r *EncryptionKeyRotator) Rotate() error {
	oldKey, cfg, err := currentEncryptionKey()
	if err != nil {
		return err
	}

	if _, readErr := r.readRow(encryptionKeyPreviousRow); readErr == nil {
		pending, err := r.PendingRows()
		if err != nil {
			return err
		}
		if pending > 0 {
			return ErrEncryptionRotationPending
		}
	}

	newKey, err := newEncryptionKey()
	if err != nil {
		return err
	}
	wrapped, err := util.EncryptAtRest(newKey, oldKey)
	if err != nil {
		return err
	}
	if err := r.writeRow(encryptionKeyPreviousRow, wrapped); err != nil {
		return err
	}
	if err := r.writeRow(encryptionKeyUntilRow, time.Now().UTC().Add(EncryptionKeyGrace).Format(time.RFC3339)); err != nil {
		return err
	}

	previousVersion := cfg.Server.Security.EncryptionKeyVersion
	cfg.Server.Security.EncryptionKey = newKey
	cfg.Server.Security.EncryptionKeyVersion = previousVersion + 1
	if err := config.SaveConfig(cfg); err != nil {
		cfg.Server.Security.EncryptionKey = oldKey
		cfg.Server.Security.EncryptionKeyVersion = previousVersion
		return fmt.Errorf("saving the new encryption key: %w", err)
	}

	return r.Rebase(oldKey, newKey)
}

// FinishUnfinishedRebase completes a re-encryption that a crash interrupted: at start, while a
// previous key exists, every row readable only with it is re-encrypted under the current key.
func (r *EncryptionKeyRotator) FinishUnfinishedRebase() error {
	current, _, err := currentEncryptionKey()
	if err != nil {
		return nil
	}
	previous := r.PreviousKey()
	if previous == "" {
		return nil
	}
	return r.Rebase(previous, current)
}

// RetireExpired deletes the previous-key rows once their grace period has passed. A row that
// still needs the retired key then fails closed (the caller logs security.decrypt_failed).
func (r *EncryptionKeyRotator) RetireExpired() (bool, error) {
	untilText, err := r.readRow(encryptionKeyUntilRow)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if until, parseErr := time.Parse(time.RFC3339, untilText); parseErr == nil && time.Now().Before(until) {
		return false, nil
	}
	for _, name := range []string{encryptionKeyPreviousRow, encryptionKeyUntilRow} {
		if _, err := database.ExecContext(context.Background(), r.ServerDB, database.TimeoutWrite,
			`DELETE FROM app_secrets WHERE name = ?`, name); err != nil {
			return false, err
		}
	}
	return true, nil
}
