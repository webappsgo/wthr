package models

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/webappsgo/wthr/src/database"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type UserPasskey struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	CredentialID    string     `json:"credential_id"`
	PublicKey       string     `json:"public_key"`
	AAGUID          string     `json:"aaguid,omitempty"`
	SignCount       uint32     `json:"sign_count"`
	Name            string     `json:"name"`
	TransportJSON   string     `json:"-"`
	AttestationType string     `json:"attestation_type,omitempty"`
	BackupEligible  bool       `json:"backup_eligible"`
	BackupState     bool       `json:"backup_state"`
	CreatedAt       time.Time  `json:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

type UserPasskeyModel struct {
	DB *sql.DB
}

func (m *UserPasskeyModel) getDB() *sql.DB {
	if m.DB != nil {
		return m.DB
	}

	return database.GetUsersDB()
}

func (m *UserPasskeyModel) ensurePasskeySchema() error {
	rows, err := m.getDB().Query(`PRAGMA table_info(user_passkeys)`)
	if err != nil {
		return fmt.Errorf("failed to inspect user_passkeys schema: %w", err)
	}
	defer rows.Close()

	hasTransport := false
	hasAttestationType := false
	hasBackupEligible := false
	hasBackupState := false

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("failed to scan user_passkeys schema: %w", err)
		}

		switch name {
		case "transport":
			hasTransport = true
		case "attestation_type":
			hasAttestationType = true
		case "backup_eligible":
			hasBackupEligible = true
		case "backup_state":
			hasBackupState = true
		}
	}

	if !hasTransport {
		if _, err := m.getDB().Exec(`ALTER TABLE user_passkeys ADD COLUMN transport TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("failed to add user_passkeys.transport: %w", err)
		}
	}

	if !hasAttestationType {
		if _, err := m.getDB().Exec(`ALTER TABLE user_passkeys ADD COLUMN attestation_type TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("failed to add user_passkeys.attestation_type: %w", err)
		}
	}

	if !hasBackupEligible {
		if _, err := m.getDB().Exec(`ALTER TABLE user_passkeys ADD COLUMN backup_eligible BOOLEAN NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("failed to add user_passkeys.backup_eligible: %w", err)
		}
	}

	if !hasBackupState {
		if _, err := m.getDB().Exec(`ALTER TABLE user_passkeys ADD COLUMN backup_state BOOLEAN NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("failed to add user_passkeys.backup_state: %w", err)
		}
	}

	return nil
}

func encodePasskeyBytes(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodePasskeyBytes(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("failed to decode stored passkey bytes: %w", err)
	}

	return decoded, nil
}

func marshalPasskeyTransport(transports []protocol.AuthenticatorTransport) (string, error) {
	if len(transports) == 0 {
		return "[]", nil
	}

	encoded := make([]string, 0, len(transports))
	for _, transport := range transports {
		encoded = append(encoded, string(transport))
	}

	payload, err := json.Marshal(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to encode passkey transports: %w", err)
	}

	return string(payload), nil
}

func unmarshalPasskeyTransport(raw string) ([]protocol.AuthenticatorTransport, error) {
	if raw == "" {
		return nil, nil
	}

	var encoded []string
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil {
		return nil, fmt.Errorf("failed to decode passkey transports: %w", err)
	}

	transports := make([]protocol.AuthenticatorTransport, 0, len(encoded))
	for _, transport := range encoded {
		if transport == "" {
			continue
		}
		transports = append(transports, protocol.AuthenticatorTransport(transport))
	}

	return transports, nil
}

func (m *UserPasskeyModel) scanPasskey(rowScanner interface {
	Scan(dest ...interface{}) error
}) (*UserPasskey, error) {
	var (
		passkey      UserPasskey
		lastUsedAt   sql.NullTime
		transportRaw sql.NullString
		signCount    int64
	)

	if err := rowScanner.Scan(
		&passkey.ID,
		&passkey.UserID,
		&passkey.CredentialID,
		&passkey.PublicKey,
		&passkey.AAGUID,
		&signCount,
		&passkey.Name,
		&transportRaw,
		&passkey.AttestationType,
		&passkey.BackupEligible,
		&passkey.BackupState,
		&passkey.CreatedAt,
		&lastUsedAt,
	); err != nil {
		return nil, err
	}

	if transportRaw.Valid {
		passkey.TransportJSON = transportRaw.String
	}
	passkey.SignCount = uint32(signCount)
	if lastUsedAt.Valid {
		passkey.LastUsedAt = &lastUsedAt.Time
	}

	return &passkey, nil
}

func (m *UserPasskeyModel) ListByUserID(userID int64) ([]*UserPasskey, error) {
	if err := m.ensurePasskeySchema(); err != nil {
		return nil, err
	}

	rows, err := m.getDB().Query(`
		SELECT id, user_id, credential_id, public_key, COALESCE(aaguid, ''), sign_count, name,
		       COALESCE(transport, '[]'), COALESCE(attestation_type, ''), backup_eligible, backup_state,
		       created_at, last_used_at
		FROM user_passkeys
		WHERE user_id = ?
		ORDER BY created_at ASC, id ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query passkeys: %w", err)
	}
	defer rows.Close()

	passkeys := make([]*UserPasskey, 0)
	for rows.Next() {
		passkey, err := m.scanPasskey(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan passkey: %w", err)
		}
		passkeys = append(passkeys, passkey)
	}

	return passkeys, nil
}

func (m *UserPasskeyModel) CountByUserID(userID int64) (int, error) {
	if err := m.ensurePasskeySchema(); err != nil {
		return 0, err
	}

	var count int
	if err := m.getDB().QueryRow(`SELECT COUNT(*) FROM user_passkeys WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count passkeys: %w", err)
	}

	return count, nil
}

func (m *UserPasskeyModel) HasPasskeys(userID int64) (bool, error) {
	count, err := m.CountByUserID(userID)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (m *UserPasskeyModel) ListCredentialsByUserID(userID int64) ([]webauthn.Credential, error) {
	passkeys, err := m.ListByUserID(userID)
	if err != nil {
		return nil, err
	}

	credentials := make([]webauthn.Credential, 0, len(passkeys))
	for _, passkey := range passkeys {
		credentialID, err := decodePasskeyBytes(passkey.CredentialID)
		if err != nil {
			return nil, err
		}
		publicKey, err := decodePasskeyBytes(passkey.PublicKey)
		if err != nil {
			return nil, err
		}
		aaguid, err := decodePasskeyBytes(passkey.AAGUID)
		if err != nil {
			return nil, err
		}
		transports, err := unmarshalPasskeyTransport(passkey.TransportJSON)
		if err != nil {
			return nil, err
		}

		credentials = append(credentials, webauthn.Credential{
			ID:              credentialID,
			PublicKey:       publicKey,
			AttestationType: passkey.AttestationType,
			Transport:       transports,
			Flags: webauthn.CredentialFlags{
				BackupEligible: passkey.BackupEligible,
				BackupState:    passkey.BackupState,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    aaguid,
				SignCount: passkey.SignCount,
			},
		})
	}

	return credentials, nil
}

func (m *UserPasskeyModel) Create(userID int64, name string, credential *webauthn.Credential) (*UserPasskey, error) {
	if err := m.ensurePasskeySchema(); err != nil {
		return nil, err
	}

	transportJSON, err := marshalPasskeyTransport(credential.Transport)
	if err != nil {
		return nil, err
	}

	result, err := m.getDB().Exec(`
		INSERT INTO user_passkeys (
			user_id, credential_id, public_key, aaguid, sign_count, name, transport,
			attestation_type, backup_eligible, backup_state, created_at, last_used_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, NULL)
	`,
		userID,
		encodePasskeyBytes(credential.ID),
		encodePasskeyBytes(credential.PublicKey),
		encodePasskeyBytes(credential.Authenticator.AAGUID),
		credential.Authenticator.SignCount,
		name,
		transportJSON,
		credential.AttestationType,
		credential.Flags.BackupEligible,
		credential.Flags.BackupState,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store passkey: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to load passkey id: %w", err)
	}

	row := m.getDB().QueryRow(`
		SELECT id, user_id, credential_id, public_key, COALESCE(aaguid, ''), sign_count, name,
		       COALESCE(transport, '[]'), COALESCE(attestation_type, ''), backup_eligible, backup_state,
		       created_at, last_used_at
		FROM user_passkeys
		WHERE id = ?
	`, id)

	passkey, err := m.scanPasskey(row)
	if err != nil {
		return nil, fmt.Errorf("failed to reload passkey: %w", err)
	}

	return passkey, nil
}

func (m *UserPasskeyModel) UpdateCredential(userID int64, credential *webauthn.Credential) error {
	if err := m.ensurePasskeySchema(); err != nil {
		return err
	}

	transportJSON, err := marshalPasskeyTransport(credential.Transport)
	if err != nil {
		return err
	}

	result, err := m.getDB().Exec(`
		UPDATE user_passkeys
		SET sign_count = ?, last_used_at = CURRENT_TIMESTAMP, transport = ?, attestation_type = ?,
		    backup_eligible = ?, backup_state = ?, aaguid = ?
		WHERE user_id = ? AND credential_id = ?
	`,
		credential.Authenticator.SignCount,
		transportJSON,
		credential.AttestationType,
		credential.Flags.BackupEligible,
		credential.Flags.BackupState,
		encodePasskeyBytes(credential.Authenticator.AAGUID),
		userID,
		encodePasskeyBytes(credential.ID),
	)
	if err != nil {
		return fmt.Errorf("failed to update passkey: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect passkey update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("passkey not found")
	}

	return nil
}

func (m *UserPasskeyModel) DeleteByID(userID int64, passkeyID int64) error {
	if err := m.ensurePasskeySchema(); err != nil {
		return err
	}

	result, err := m.getDB().Exec(`DELETE FROM user_passkeys WHERE id = ? AND user_id = ?`, passkeyID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete passkey: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect passkey delete: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("passkey not found")
	}

	return nil
}
