package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// legacyRename maps a table name used before AI.md PART 12 naming to its
// canonical name.
type legacyRename struct {
	oldName string
	newName string
}

// serverLegacyRenames are the server.db tables whose canonical names dropped the
// server_ prefix. None of them carries a foreign key, so a plain rename keeps the
// data and every index.
var serverLegacyRenames = []legacyRename{
	{"server_rate_limits", "rate_limits"},
	{"server_audit_log", "audit_log"},
	{"server_scheduler_state", "scheduler_tasks"},
	{"server_scheduler_history", "scheduler_history"},
	{"server_backup_history", "backups"},
}

// usersLegacyRenames are the users.db tables whose canonical names dropped the
// user_ prefix (user_tokens is not here: it becomes the unified tokens table by
// copy, see migrateLegacyTokens). SQLite rewrites the foreign keys of child tables when a parent is
// renamed, so the whole family keeps its references.
var usersLegacyRenames = []legacyRename{
	{"user_accounts", "users"},
	{"user_password_resets", "password_resets"},
	{"user_email_verifications", "email_verifications"},
}

// sqliteTableExists reports whether name is a table in db.
func sqliteTableExists(db *sql.DB, name string) (bool, error) {
	var count int
	err := QueryRowContext(context.Background(), db, TimeoutSimpleSelect,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check table %s: %w", name, err)
	}
	return count > 0, nil
}

// renameLegacyTables renames every legacy table that still exists while its
// canonical name is free. It runs before the base schema so the schema's
// CREATE TABLE IF NOT EXISTS finds the renamed table and leaves it alone. It is
// idempotent: once a table is renamed the legacy name no longer exists.
func renameLegacyTables(db *sql.DB, renames []legacyRename) error {
	for _, rename := range renames {
		oldExists, err := sqliteTableExists(db, rename.oldName)
		if err != nil {
			return err
		}
		if !oldExists {
			continue
		}
		newExists, err := sqliteTableExists(db, rename.newName)
		if err != nil {
			return err
		}
		if newExists {
			continue
		}
		if _, err := ExecContext(context.Background(), db, TimeoutMigration,
			fmt.Sprintf("ALTER TABLE %s RENAME TO %s", rename.oldName, rename.newName)); err != nil {
			return fmt.Errorf("rename %s to %s: %w", rename.oldName, rename.newName, err)
		}
	}
	return nil
}

// sqliteColumns returns the column names of table in declaration order.
func sqliteColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := QueryContext(context.Background(), db, TimeoutSimpleSelect, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan %s columns: %w", table, err)
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

// tableRowCount returns the number of rows in table.
func tableRowCount(db *sql.DB, table string) (int, error) {
	var count int
	err := QueryRowContext(context.Background(), db, TimeoutSimpleSelect, "SELECT COUNT(*) FROM "+table).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return count, nil
}

// copyLegacyRows copies the rows of srcTable in src into dstTable in dst. Source
// columns are mapped to destination columns through columnMap (a source column
// absent from the map keeps its name); a source column without a destination
// column is skipped, and fixed supplies constant values for destination columns
// the source lacks. skipColumns names source columns that must not be copied.
// With onlyIfEmpty the copy is skipped when the destination already holds rows,
// so it only ever seeds an empty table and is safe to run on every startup;
// without it rows are appended and INSERT OR IGNORE absorbs duplicates. where,
// when not empty, restricts the source rows.
func copyLegacyRows(src, dst *sql.DB, srcTable, dstTable string, columnMap map[string]string, fixed map[string]interface{}, skipColumns map[string]bool, where string, onlyIfEmpty bool) (int, error) {
	srcExists, err := sqliteTableExists(src, srcTable)
	if err != nil || !srcExists {
		return 0, err
	}

	if onlyIfEmpty {
		existing, err := tableRowCount(dst, dstTable)
		if err != nil || existing > 0 {
			return 0, err
		}
	}

	srcColumns, err := sqliteColumns(src, srcTable)
	if err != nil {
		return 0, err
	}
	dstColumns, err := sqliteColumns(dst, dstTable)
	if err != nil {
		return 0, err
	}
	dstSet := make(map[string]bool, len(dstColumns))
	for _, column := range dstColumns {
		dstSet[column] = true
	}

	var selectColumns, insertColumns []string
	for _, column := range srcColumns {
		if skipColumns[column] {
			continue
		}
		target := column
		if mapped, ok := columnMap[column]; ok {
			target = mapped
		}
		if !dstSet[target] {
			continue
		}
		selectColumns = append(selectColumns, column)
		insertColumns = append(insertColumns, target)
	}
	if len(selectColumns) == 0 {
		return 0, nil
	}

	fixedNames := make([]string, 0, len(fixed))
	fixedValues := make([]interface{}, 0, len(fixed))
	for name, value := range fixed {
		if dstSet[name] {
			fixedNames = append(fixedNames, name)
			fixedValues = append(fixedValues, value)
		}
	}

	query := "SELECT " + strings.Join(selectColumns, ", ") + " FROM " + srcTable
	if where != "" {
		query += " WHERE " + where
	}
	rows, err := QueryContext(context.Background(), src, TimeoutBulk, query)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcTable, err)
	}
	defer rows.Close()

	allColumns := append(append([]string{}, insertColumns...), fixedNames...)
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(allColumns)), ", ")
	insert := "INSERT OR IGNORE INTO " + dstTable + " (" + strings.Join(allColumns, ", ") + ") VALUES (" + placeholders + ")"

	copied := 0
	for rows.Next() {
		values := make([]interface{}, len(selectColumns))
		pointers := make([]interface{}, len(selectColumns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return copied, fmt.Errorf("scan %s: %w", srcTable, err)
		}
		args := append(append([]interface{}{}, values...), fixedValues...)
		if _, err := ExecContext(context.Background(), dst, TimeoutWrite, insert, args...); err != nil {
			return copied, fmt.Errorf("copy %s row into %s: %w", srcTable, dstTable, err)
		}
		copied++
	}
	if err := rows.Err(); err != nil {
		return copied, fmt.Errorf("iterate %s: %w", srcTable, err)
	}
	return copied, nil
}

// migrateLegacyAdminTables carries Server Admin data forward from server.db to
// users.db, where AI.md PART 12 places it, and merges the former per-account-type
// passkey tables into the single passkeys table. Nothing is dropped: the legacy
// tables stay in place, and once the admin credentials have been copied their
// password and API token hashes are blanked so no stale secret remains. The copy
// only seeds empty destination tables, so it is idempotent.
func migrateLegacyAdminTables(serverDB, usersDB *sql.DB) error {
	const legacyCredentials = "server_admin_credentials"

	credentialsExist, err := sqliteTableExists(serverDB, legacyCredentials)
	if err != nil {
		return err
	}

	if credentialsExist {
		copied, err := copyLegacyRows(serverDB, usersDB, legacyCredentials, "admins", nil, nil, nil, "password_hash != ''", true)
		if err != nil {
			return err
		}
		if copied > 0 {
			if err := copyLegacyAdminTokens(serverDB, usersDB, legacyCredentials); err != nil {
				return err
			}
			if _, err := ExecContext(context.Background(), serverDB, TimeoutWrite,
				"UPDATE "+legacyCredentials+" SET password_hash = '', api_token_hash = NULL, api_token_prefix = NULL"); err != nil {
				return fmt.Errorf("blank legacy admin credentials: %w", err)
			}
		}

		sameShape := []string{"server_admin_preferences:admin_preferences", "server_admin_invites:admin_invites",
			"server_admin_notifications:admin_notifications", "server_admin_notification_preferences:admin_notification_preferences"}
		for _, pair := range sameShape {
			names := strings.SplitN(pair, ":", 2)
			if _, err := copyLegacyRows(serverDB, usersDB, names[0], names[1], nil, nil, nil, "", true); err != nil {
				return err
			}
		}
	}

	if err := migrateLegacyUserTokens(usersDB); err != nil {
		return err
	}

	passkeysCount, err := tableRowCount(usersDB, "passkeys")
	if err != nil {
		return err
	}
	if passkeysCount == 0 {
		skipID := map[string]bool{"id": true}
		if _, err := copyLegacyRows(serverDB, usersDB, "server_admin_passkeys", "passkeys",
			map[string]string{"admin_id": "user_id"}, map[string]interface{}{"user_type": "admin"}, skipID, "", false); err != nil {
			return err
		}
		if _, err := copyLegacyRows(usersDB, usersDB, "user_passkeys", "passkeys",
			nil, map[string]interface{}{"user_type": "user"}, skipID, "", false); err != nil {
			return err
		}
	}

	return nil
}

// copyLegacyAdminSessions carries the rows of the former server_admin_sessions
// table into admin_sessions. The legacy table keeps a foreign key to the legacy
// admin credentials, which would reject sessions of admins created after the
// move, so its rows are copied instead of the table being renamed.
func copyLegacyAdminSessions(serverDB *sql.DB) error {
	_, err := copyLegacyRows(serverDB, serverDB, "server_admin_sessions", "admin_sessions", nil, nil, nil, "", true)
	return err
}

// copyLegacyAdminTokens moves the single API token each legacy admin row carried
// into the unified tokens table as the admin's "default" token, keeping the stored
// hash and display prefix so existing tokens keep working.
func copyLegacyAdminTokens(serverDB, usersDB *sql.DB, legacyCredentials string) error {
	rows, err := QueryContext(context.Background(), serverDB, TimeoutBulk, `
		SELECT id, api_token_hash, COALESCE(NULLIF(api_token_prefix, ''), 'adm_'), created_at
		FROM `+legacyCredentials+`
		WHERE api_token_hash IS NOT NULL AND api_token_hash != '' AND password_hash != ''
	`)
	if err != nil {
		return fmt.Errorf("read legacy admin tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var adminID int64
		var tokenHash, tokenPrefix string
		var createdAt interface{}
		if err := rows.Scan(&adminID, &tokenHash, &tokenPrefix, &createdAt); err != nil {
			return fmt.Errorf("scan legacy admin token: %w", err)
		}
		if _, err := ExecContext(context.Background(), usersDB, TimeoutWrite, `
			INSERT OR IGNORE INTO tokens (owner_type, owner_id, name, token_hash, token_prefix, scope, created_at)
			VALUES ('admin', ?, 'default', ?, ?, 'global', COALESCE(?, CURRENT_TIMESTAMP))
		`, adminID, tokenHash, tokenPrefix, createdAt); err != nil {
			return fmt.Errorf("copy legacy admin token: %w", err)
		}
	}
	return rows.Err()
}

// migrateLegacyUserTokens copies the former user_tokens table into the unified
// tokens table as user-owned tokens, then marks each legacy hash as migrated so
// the legacy table no longer holds a usable secret and a later start never copies
// it again. Rows without a name get a stable generated one.
func migrateLegacyUserTokens(usersDB *sql.DB) error {
	exists, err := sqliteTableExists(usersDB, "user_tokens")
	if err != nil || !exists {
		return err
	}

	if _, err := ExecContext(context.Background(), usersDB, TimeoutBulk, `
		INSERT OR IGNORE INTO tokens (owner_type, owner_id, name, token_hash, token_prefix, scope, expires_at, last_used_at, last_used_ip, created_at)
		SELECT 'user', user_id, COALESCE(NULLIF(name, ''), 'token-' || id), token_hash, token_prefix,
			COALESCE(NULLIF(scopes, ''), 'global'), expires_at, last_used_at, last_used_ip, created_at
		FROM user_tokens
		WHERE token_hash NOT LIKE 'migrated:%'
	`); err != nil {
		return fmt.Errorf("copy legacy user tokens: %w", err)
	}

	if _, err := ExecContext(context.Background(), usersDB, TimeoutBulk,
		"UPDATE user_tokens SET token_hash = 'migrated:' || id WHERE token_hash NOT LIKE 'migrated:%'"); err != nil {
		return fmt.Errorf("blank legacy user tokens: %w", err)
	}
	return nil
}
