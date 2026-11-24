package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/webappsgo/wthr/src/config"
)

type remoteSchedulerColumn struct {
	dataType string
}

func migrateRemoteSchedulerSchema(db *sql.DB, dialect string) error {
	if dialect != "postgres" && dialect != "mysql" && dialect != "mariadb" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), TimeoutMigration)
	defer cancel()
	if err := migrateRemoteSchedulerTable(ctx, db, dialect, "srv_scheduler_tasks", true); err != nil {
		return err
	}
	return migrateRemoteSchedulerTable(ctx, db, dialect, "srv_scheduler_history", false)
}

func migrateRemoteSchedulerTable(ctx context.Context, db *sql.DB, dialect, table string, tasks bool) error {
	columns, exists, err := remoteSchedulerColumns(ctx, db, dialect, table)
	if err != nil || !exists {
		return err
	}
	pairs := [][2]string{{"task_id", "id"}, {"task_name", "name"}}
	if !tasks {
		pairs = [][2]string{{"task_name", "task_id"}, {"start_time", "started_at"}, {"end_time", "finished_at"}}
	}
	for _, pair := range pairs {
		if _, hasOld := columns[pair[0]]; !hasOld {
			continue
		}
		if _, hasNew := columns[pair[1]]; hasNew {
			continue
		}
		query := "ALTER TABLE " + remoteSchedulerQuote(dialect, table) + " RENAME COLUMN " + remoteSchedulerQuote(dialect, pair[0]) + " TO " + remoteSchedulerQuote(dialect, pair[1])
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("rename remote scheduler column %s.%s: %w", table, pair[0], err)
		}
		columns, _, err = remoteSchedulerColumns(ctx, db, dialect, table)
		if err != nil {
			return err
		}
	}

	if tasks {
		if _, exists := columns["task_type"]; !exists {
			query := "ALTER TABLE " + remoteSchedulerQuote(dialect, table) + " ADD COLUMN task_type VARCHAR(16) NOT NULL DEFAULT 'global'"
			if _, err := db.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("add remote scheduler task type: %w", err)
			}
		}
		for _, name := range []string{"last_run", "next_run", "locked_at"} {
			columns, _, err = remoteSchedulerColumns(ctx, db, dialect, table)
			if err != nil {
				return err
			}
			if err := convertRemoteSchedulerTimestamp(ctx, db, dialect, table, name, columns); err != nil {
				return fmt.Errorf("convert remote scheduler timestamp %s.%s: %w", table, name, err)
			}
		}
		nameColumn := "name"
		if _, exists := columns[nameColumn]; !exists {
			nameColumn = "task_name"
		}
		for _, def := range config.SchedulerTaskDefs() {
			if !def.Global {
				continue
			}
			taskType := "local"
			if def.Global {
				taskType = "global"
			}
			query := "UPDATE " + remoteSchedulerQuote(dialect, table) + " SET task_type = '" + taskType + "' WHERE " + remoteSchedulerQuote(dialect, nameColumn) + " = '" + def.Name + "'"
			if _, err := db.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("set remote scheduler task type for %s: %w", def.Name, err)
			}
		}
		return nil
	}

	for _, name := range []string{"started_at", "finished_at"} {
		columns, _, err = remoteSchedulerColumns(ctx, db, dialect, table)
		if err != nil {
			return err
		}
		if err := convertRemoteSchedulerTimestamp(ctx, db, dialect, table, name, columns); err != nil {
			return fmt.Errorf("convert remote scheduler timestamp %s.%s: %w", table, name, err)
		}
	}
	query := "ALTER TABLE " + remoteSchedulerQuote(dialect, table)
	if dialect == "mysql" || dialect == "mariadb" {
		query += " MODIFY COLUMN finished_at BIGINT NULL"
	} else {
		query += " ALTER COLUMN finished_at DROP NOT NULL"
	}
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("make remote scheduler finish time nullable: %w", err)
	}
	if columns["duration_ms"].dataType != "" {
		query = "ALTER TABLE " + remoteSchedulerQuote(dialect, table)
		if dialect == "mysql" || dialect == "mariadb" {
			query += " MODIFY COLUMN duration_ms BIGINT NULL"
		} else {
			query += " ALTER COLUMN duration_ms DROP NOT NULL"
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("make remote scheduler duration nullable: %w", err)
		}
	}
	if _, exists := columns["created_at"]; exists {
		query := "ALTER TABLE " + remoteSchedulerQuote(dialect, table) + " DROP COLUMN created_at"
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remove obsolete remote scheduler history created_at: %w", err)
		}
	}
	return nil
}

func remoteSchedulerColumns(ctx context.Context, db *sql.DB, dialect, table string) (map[string]remoteSchedulerColumn, bool, error) {
	query := `SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1`
	if dialect == "mysql" || dialect == "mariadb" {
		query = `SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?`
	}
	rows, err := db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, false, fmt.Errorf("inspect remote scheduler table %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	columns := make(map[string]remoteSchedulerColumn)
	for rows.Next() {
		var name, dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			return nil, false, err
		}
		columns[strings.ToLower(name)] = remoteSchedulerColumn{dataType: strings.ToLower(dataType)}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return columns, len(columns) > 0, nil
}

func convertRemoteSchedulerTimestamp(ctx context.Context, db *sql.DB, dialect, table, column string, columns map[string]remoteSchedulerColumn) error {
	current, exists := columns[column]
	if exists && isRemoteIntegerType(current.dataType) {
		return nil
	}
	quotedTable := remoteSchedulerQuote(dialect, table)
	quotedColumn := remoteSchedulerQuote(dialect, column)
	if dialect == "postgres" {
		if !exists {
			return nil
		}
		query := "ALTER TABLE " + quotedTable + " ALTER COLUMN " + quotedColumn + " TYPE BIGINT USING CASE WHEN " + quotedColumn + " IS NULL THEN NULL ELSE EXTRACT(EPOCH FROM " + quotedColumn + ")::BIGINT END"
		_, err := db.ExecContext(ctx, query)
		return err
	}
	temporary := column + "_unix_migration"
	quotedTemporary := remoteSchedulerQuote(dialect, temporary)
	if !exists {
		if _, hasTemporary := columns[temporary]; !hasTemporary {
			return nil
		}
		_, err := db.ExecContext(ctx, "ALTER TABLE "+quotedTable+" RENAME COLUMN "+quotedTemporary+" TO "+quotedColumn)
		return err
	}
	if _, hasTemporary := columns[temporary]; !hasTemporary {
		if _, err := db.ExecContext(ctx, "ALTER TABLE "+quotedTable+" ADD COLUMN "+quotedTemporary+" BIGINT NULL"); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE "+quotedTable+" SET "+quotedTemporary+" = UNIX_TIMESTAMP("+quotedColumn+")"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE "+quotedTable+" DROP COLUMN "+quotedColumn); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "ALTER TABLE "+quotedTable+" RENAME COLUMN "+quotedTemporary+" TO "+quotedColumn)
	return err
}

func isRemoteIntegerType(dataType string) bool {
	switch strings.ToLower(dataType) {
	case "bigint", "integer", "int", "int2", "int4", "int8", "smallint", "mediumint":
		return true
	default:
		return false
	}
}

func remoteSchedulerQuote(dialect, name string) string {
	if dialect == "mysql" || dialect == "mariadb" {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
