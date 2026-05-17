package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/casapps/wthr/src/database"

	"github.com/go-webauthn/webauthn/webauthn"
)

// AdminPasskey is the server-admin counterpart to UserPasskey. The on-disk
// shape is identical so the same WebAuthn ceremony helpers can be reused;
// only the storage location (server.db vs users.db) differs because admins
// live in server.db per AI.md PART 22.
type AdminPasskey struct {
	ID              int64      `json:"id"`
	AdminID         int64      `json:"admin_id"`
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

type AdminPasskeyModel struct {
	DB *sql.DB
}

func (m *AdminPasskeyModel) getDB() *sql.DB {
	if m.DB != nil {
		return m.DB
	}
	return database.GetServerDB()
}

func (m *AdminPasskeyModel) ensurePasskeySchema() error {
	rows, err := m.getDB().Query(`PRAGMA table_info(server_admin_passkeys)`)
	if err != nil {
		return fmt.Errorf("failed to inspect server_admin_passkeys schema: %w", err)
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
			return fmt.Errorf("failed to scan server_admin_passkeys schema: %w", err)
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
		if _, err := m.getDB().Exec(`ALTER TABLE server_admin_passkeys ADD COLUMN transport TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("failed to add server_admin_passkeys.transport: %w", err)
		}
	}
	if !hasAttestationType {
		if _, err := m.getDB().Exec(`ALTER TABLE server_admin_passkeys ADD COLUMN attestation_type TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("failed to add server_admin_passkeys.attestation_type: %w", err)
		}
	}
	if !hasBackupEligible {
		if _, err := m.getDB().Exec(`ALTER TABLE server_admin_passkeys ADD COLUMN backup_eligible BOOLEAN NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("failed to add server_admin_passkeys.backup_eligible: %w", err)
		}
	}
	if !hasBackupState {
		if _, err := m.getDB().Exec(`ALTER TABLE server_admin_passkeys ADD COLUMN backup_state BOOLEAN NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("failed to add server_admin_passkeys.backup_state: %w", err)
		}
	}

	return nil
}

func (m *AdminPasskeyModel) scanPasskey(rowScanner interface {
	Scan(dest ...interface{}) error
}) (*AdminPasskey, error) {
	var (
		passkey      AdminPasskey
		lastUsedAt   sql.NullTime
		transportRaw sql.NullString
		signCount    int64
	)

	if err := rowScanner.Scan(
		&passkey.ID,
		&passkey.AdminID,
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

func (m *AdminPasskeyModel) ListByAdminID(adminID int64) ([]*AdminPasskey, error) {
	if err := m.ensurePasskeySchema(); err != nil {
		return nil, err
	}

	rows, err := m.getDB().Query(`
		SELECT id, admin_id, credential_id, public_key, COALESCE(aaguid, ''), sign_count, name,
		       COALESCE(transport, '[]'), COALESCE(attestation_type, ''), backup_eligible, backup_state,
		       created_at, last_used_at
		FROM server_admin_passkeys
		WHERE admin_id = ?
		ORDER BY created_at ASC, id ASC
	`, adminID)
	if err != nil {
		return nil, fmt.Errorf("failed to query admin passkeys: %w", err)
	}
	defer rows.Close()

	passkeys := make([]*AdminPasskey, 0)
	for rows.Next() {
		passkey, err := m.scanPasskey(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan admin passkey: %w", err)
		}
		passkeys = append(passkeys, passkey)
	}

	return passkeys, nil
}

func (m *AdminPasskeyModel) CountByAdminID(adminID int64) (int, error) {
	if err := m.ensurePasskeySchema(); err != nil {
		return 0, err
	}

	var count int
	if err := m.getDB().QueryRow(`SELECT COUNT(*) FROM server_admin_passkeys WHERE admin_id = ?`, adminID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count admin passkeys: %w", err)
	}

	return count, nil
}

func (m *AdminPasskeyModel) HasPasskeys(adminID int64) (bool, error) {
	count, err := m.CountByAdminID(adminID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (m *AdminPasskeyModel) ListCredentialsByAdminID(adminID int64) ([]webauthn.Credential, error) {
	passkeys, err := m.ListByAdminID(adminID)
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

func (m *AdminPasskeyModel) Create(adminID int64, name string, credential *webauthn.Credential) (*AdminPasskey, error) {
	if err := m.ensurePasskeySchema(); err != nil {
		return nil, err
	}

	transportJSON, err := marshalPasskeyTransport(credential.Transport)
	if err != nil {
		return nil, err
	}

	result, err := m.getDB().Exec(`
		INSERT INTO server_admin_passkeys (
			admin_id, credential_id, public_key, aaguid, sign_count, name, transport,
			attestation_type, backup_eligible, backup_state, created_at, last_used_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, NULL)
	`,
		adminID,
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
		return nil, fmt.Errorf("failed to store admin passkey: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to load admin passkey id: %w", err)
	}

	row := m.getDB().QueryRow(`
		SELECT id, admin_id, credential_id, public_key, COALESCE(aaguid, ''), sign_count, name,
		       COALESCE(transport, '[]'), COALESCE(attestation_type, ''), backup_eligible, backup_state,
		       created_at, last_used_at
		FROM server_admin_passkeys
		WHERE id = ?
	`, id)

	passkey, err := m.scanPasskey(row)
	if err != nil {
		return nil, fmt.Errorf("failed to reload admin passkey: %w", err)
	}

	return passkey, nil
}

func (m *AdminPasskeyModel) UpdateCredential(adminID int64, credential *webauthn.Credential) error {
	if err := m.ensurePasskeySchema(); err != nil {
		return err
	}

	transportJSON, err := marshalPasskeyTransport(credential.Transport)
	if err != nil {
		return err
	}

	result, err := m.getDB().Exec(`
		UPDATE server_admin_passkeys
		SET sign_count = ?, last_used_at = CURRENT_TIMESTAMP, transport = ?, attestation_type = ?,
		    backup_eligible = ?, backup_state = ?, aaguid = ?
		WHERE admin_id = ? AND credential_id = ?
	`,
		credential.Authenticator.SignCount,
		transportJSON,
		credential.AttestationType,
		credential.Flags.BackupEligible,
		credential.Flags.BackupState,
		encodePasskeyBytes(credential.Authenticator.AAGUID),
		adminID,
		encodePasskeyBytes(credential.ID),
	)
	if err != nil {
		return fmt.Errorf("failed to update admin passkey: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect admin passkey update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("passkey not found")
	}

	return nil
}

func (m *AdminPasskeyModel) DeleteByID(adminID int64, passkeyID int64) error {
	if err := m.ensurePasskeySchema(); err != nil {
		return err
	}

	result, err := m.getDB().Exec(`DELETE FROM server_admin_passkeys WHERE id = ? AND admin_id = ?`, passkeyID, adminID)
	if err != nil {
		return fmt.Errorf("failed to delete admin passkey: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect admin passkey delete: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("passkey not found")
	}

	return nil
}

// AdminPasskeyUserHandlePrefix is prepended to the admin ID to form the
// WebAuthn user handle for admin passkeys, mirroring the `usr:` prefix used
// for regular users so the two sides cannot be confused at verify time.
const AdminPasskeyUserHandlePrefix = "adm:"
