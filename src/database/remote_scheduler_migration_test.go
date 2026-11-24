package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
)

type remoteSchedulerMigrationState struct {
	sync.Mutex
	tables     map[string]map[string]string
	statements []string
}

var remoteSchedulerMigrationTestStates sync.Map
var remoteSchedulerMigrationTestDriverOnce sync.Once

type remoteSchedulerMigrationDriver struct{}

func (remoteSchedulerMigrationDriver) Open(name string) (driver.Conn, error) {
	state, ok := remoteSchedulerMigrationTestStates.Load(name)
	if !ok {
		return nil, fmt.Errorf("unknown migration test state %q", name)
	}
	return remoteSchedulerMigrationConn{state: state.(*remoteSchedulerMigrationState)}, nil
}

type remoteSchedulerMigrationConn struct {
	state *remoteSchedulerMigrationState
}

func (remoteSchedulerMigrationConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}
func (remoteSchedulerMigrationConn) Close() error              { return nil }
func (remoteSchedulerMigrationConn) Begin() (driver.Tx, error) { return nil, driver.ErrSkip }

func (c remoteSchedulerMigrationConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("unexpected metadata query arguments")
	}
	table, ok := args[0].Value.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected table argument %T", args[0].Value)
	}
	c.state.Lock()
	defer c.state.Unlock()
	columns := c.state.tables[table]
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	return &remoteSchedulerMigrationRows{columns: columns, names: names}, nil
}

func (c remoteSchedulerMigrationConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.Lock()
	defer c.state.Unlock()
	c.state.statements = append(c.state.statements, query)
	clean := strings.NewReplacer("`", "", `"`, "").Replace(query)
	rename := regexp.MustCompile(`(?i)^ALTER TABLE (\w+) RENAME COLUMN (\w+) TO (\w+)$`)
	if match := rename.FindStringSubmatch(clean); len(match) == 4 {
		columns := c.state.tables[match[1]]
		columns[match[3]] = columns[match[2]]
		delete(columns, match[2])
		return driver.RowsAffected(0), nil
	}
	add := regexp.MustCompile(`(?i)^ALTER TABLE (\w+) ADD COLUMN (\w+) VARCHAR\(16\) NOT NULL DEFAULT 'global'|^ALTER TABLE (\w+) ADD COLUMN (\w+) BIGINT NULL$`)
	if match := add.FindStringSubmatch(clean); len(match) > 0 {
		table, column := match[1], match[2]
		if table == "" {
			table, column = match[3], match[4]
		}
		c.state.tables[table][column] = "bigint"
		if column == "task_type" {
			c.state.tables[table][column] = "varchar"
		}
		return driver.RowsAffected(0), nil
	}
	drop := regexp.MustCompile(`(?i)^ALTER TABLE (\w+) DROP COLUMN (\w+)$`)
	if match := drop.FindStringSubmatch(clean); len(match) == 3 {
		delete(c.state.tables[match[1]], match[2])
		return driver.RowsAffected(0), nil
	}
	convert := regexp.MustCompile(`(?i)^ALTER TABLE (\w+) ALTER COLUMN (\w+) TYPE BIGINT USING`)
	if match := convert.FindStringSubmatch(clean); len(match) == 3 {
		c.state.tables[match[1]][match[2]] = "bigint"
		return driver.RowsAffected(0), nil
	}
	if strings.HasPrefix(strings.ToUpper(clean), "UPDATE ") || strings.Contains(strings.ToUpper(clean), "MODIFY COLUMN") || strings.Contains(strings.ToUpper(clean), "DROP NOT NULL") {
		return driver.RowsAffected(0), nil
	}
	return nil, fmt.Errorf("unexpected migration statement: %s", query)
}

type remoteSchedulerMigrationRows struct {
	columns map[string]string
	names   []string
	index   int
}

func (*remoteSchedulerMigrationRows) Columns() []string { return []string{"column_name", "data_type"} }
func (r *remoteSchedulerMigrationRows) Close() error    { return nil }
func (r *remoteSchedulerMigrationRows) Next(dest []driver.Value) error {
	if r.index >= len(r.names) {
		return io.EOF
	}
	name := r.names[r.index]
	dest[0], dest[1] = name, r.columns[name]
	r.index++
	return nil
}

func TestMigrateRemoteSchedulerSchemaUpgradesPrefixedTables(t *testing.T) {
	remoteSchedulerMigrationTestDriverOnce.Do(func() {
		sql.Register("wthr-remote-scheduler-migration-test", remoteSchedulerMigrationDriver{})
	})
	for _, dialect := range []string{"postgres", "mysql"} {
		t.Run(dialect, func(t *testing.T) {
			state := &remoteSchedulerMigrationState{tables: map[string]map[string]string{
				"srv_scheduler_tasks": {
					"task_id": "text", "task_name": "text", "enabled": "boolean", "schedule": "text",
					"last_run": "timestamp", "next_run": "timestamp", "locked_at": "timestamp",
				},
				"srv_scheduler_history": {
					"id": "bigint", "task_name": "text", "start_time": "timestamp", "end_time": "timestamp",
					"duration_ms": "integer", "created_at": "timestamp",
				},
			}}
			dsn := fmt.Sprintf("%p-%s", state, dialect)
			remoteSchedulerMigrationTestStates.Store(dsn, state)
			defer remoteSchedulerMigrationTestStates.Delete(dsn)
			db, err := sql.Open("wthr-remote-scheduler-migration-test", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()

			if err := migrateRemoteSchedulerSchema(db, dialect); err != nil {
				t.Fatalf("migrateRemoteSchedulerSchema: %v", err)
			}
			for table, want := range map[string][]string{
				"srv_scheduler_tasks":   {"id", "name", "task_type", "last_run", "next_run", "locked_at"},
				"srv_scheduler_history": {"task_id", "started_at", "finished_at", "duration_ms"},
			} {
				for _, column := range want {
					if _, ok := state.tables[table][column]; !ok {
						t.Errorf("%s missing canonical column %s", table, column)
					}
				}
			}
			for _, obsolete := range []string{"task_name", "start_time", "end_time", "created_at"} {
				if _, ok := state.tables["srv_scheduler_history"][obsolete]; ok {
					t.Errorf("legacy history column %s remains", obsolete)
				}
			}
			if got := state.tables["srv_scheduler_tasks"]["last_run"]; got != "bigint" {
				t.Errorf("last_run type = %q, want bigint", got)
			}
			if got := state.tables["srv_scheduler_history"]["started_at"]; got != "bigint" {
				t.Errorf("started_at type = %q, want bigint", got)
			}
			conversion := "EXTRACT(EPOCH FROM"
			if dialect == "mysql" {
				conversion = "UNIX_TIMESTAMP("
			}
			foundConversion := false
			for _, statement := range state.statements {
				if strings.Contains(statement, conversion) {
					foundConversion = true
				}
			}
			if !foundConversion {
				t.Errorf("migration did not use %q for timestamp conversion", conversion)
			}
		})
	}
}
