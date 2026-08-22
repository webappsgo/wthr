package scheduler

import (
	"database/sql"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
)

// The timestamp helpers below are thin aliases over src/common/dbtime, the
// project's single source of truth for SQL timestamp formatting, parsing and
// comparison. They stay package-local so the existing callers in this package
// keep reading naturally; dbtime itself imports only the standard library, so
// packages that cannot import scheduler (model, cluster) share the same code
// without any import cycle.

// sqlTimestampLayout is the canonical "YYYY-MM-DD HH:MM:SS" layout SQLite's
// CURRENT_TIMESTAMP and datetime() emit and that PostgreSQL/MySQL accept as a
// timestamp literal.
const sqlTimestampLayout = dbtime.SQLTimestampLayout

// parseStoredTimestamp converts a value scanned from a DATETIME column into a
// UTC time.Time, reporting false for NULL and for layouts the project never
// writes so an unrecognised value can never cause a delete.
func parseStoredTimestamp(value interface{}) (time.Time, bool) {
	return dbtime.ParseStoredTimestamp(value)
}

// deleteRowsWithTimestampBefore deletes every row of table whose
// timestampColumn holds an instant strictly earlier than cutoff, comparing in
// Go so that mixed on-disk layouts and mixed timezones all resolve to the same
// absolute instant. It returns the number of rows deleted.
func deleteRowsWithTimestampBefore(db *sql.DB, table, idColumn, timestampColumn string, cutoff time.Time) (int64, error) {
	return dbtime.DeleteRowsWithTimestampBefore(db, table, idColumn, timestampColumn, cutoff, false)
}
