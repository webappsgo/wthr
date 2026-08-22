// Package dbtime is the single source of truth for how this project writes,
// reads and compares SQL DATETIME/TIMESTAMP values.
//
// SQLite's CURRENT_TIMESTAMP and datetime() emit UTC text in the layout
// "YYYY-MM-DD HH:MM:SS", while modernc.org/sqlite serializes a bound Go
// time.Time as time.Time.String() -- "2006-01-02 15:04:05.999999999 -0700 MST",
// in the writer's LOCAL zone. A table written by both producers therefore holds
// two incomparable text encodings, and any SQL-side comparison such as
// "WHERE expires_at < datetime('now')" silently expires or deletes rows early
// or late by the host's UTC offset, or evaluates to NULL and never matches at
// all. Every writer in this project formats through FormatSQLTimestamp, and
// every comparison parses each stored value with ParseStoredTimestamp and acts
// by primary key instead of comparing text in SQL.
//
// This package deliberately imports nothing but the standard library, so it can
// be a leaf dependency of every package that touches timestamps (model,
// scheduler, cluster) without any possibility of an import cycle.
package dbtime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SQLTimestampLayout is the canonical "YYYY-MM-DD HH:MM:SS" layout SQLite's
// CURRENT_TIMESTAMP and datetime() emit and that PostgreSQL and MySQL accept as
// a timestamp literal. A value computed in Go, converted to UTC and formatted
// with this layout can be bound as a plain query parameter on every driver the
// project supports.
const SQLTimestampLayout = "2006-01-02 15:04:05"

// storedTimestampLayouts lists every textual layout a DATETIME/TIMESTAMP column
// in this project can actually hold: the local-zone time.Time.String() form the
// SQLite driver produces for bound times, the canonical UTC form
// CURRENT_TIMESTAMP produces, and the RFC 3339 forms other drivers produce.
// Layouts carrying no zone are parsed as UTC, which is what their producers
// mean.
var storedTimestampLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999 -0700",
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// deleteChunkSize caps how many row IDs one DELETE statement binds, keeping the
// statement well inside every driver's bind-parameter limit.
const deleteChunkSize = 500

// scanTimeout bounds the SELECT that reads candidate rows. It matches
// database.TimeoutComplexSelect, which this package cannot import without
// giving up its stdlib-only guarantee.
const scanTimeout = 15 * time.Second

// bulkDeleteTimeout bounds each chunked DELETE. It matches database.TimeoutBulk.
const bulkDeleteTimeout = 60 * time.Second

// FormatSQLTimestamp renders t as canonical UTC text suitable for binding into
// a query or storing in a DATETIME column.
func FormatSQLTimestamp(t time.Time) string {
	return t.UTC().Format(SQLTimestampLayout)
}

// ParseStoredTimestamp converts a value scanned from a DATETIME/TIMESTAMP
// column into a UTC time.Time. The second return value is false when the value
// is NULL or in a layout the project never writes; callers treat that as "leave
// this row alone", so an unrecognised value can never cause a delete or mark a
// live row expired.
func ParseStoredTimestamp(value interface{}) (time.Time, bool) {
	switch stored := value.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		return stored.UTC(), true
	case []byte:
		return ParseStoredTimestampText(string(stored))
	case string:
		return ParseStoredTimestampText(stored)
	}

	return time.Time{}, false
}

// ParseStoredTimestampText parses the textual timestamp layouts listed in
// storedTimestampLayouts and normalizes the result to UTC.
func ParseStoredTimestampText(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}

	// time.Time.String() appends a monotonic-clock reading ("m=+0.000000001")
	// for times produced by time.Now(), which is not part of any layout - drop
	// it before parsing.
	if monotonic := strings.Index(value, " m="); monotonic > 0 {
		value = strings.TrimSpace(value[:monotonic])
	}

	// A trailing "Z" on an otherwise zone-less layout still means UTC, which is
	// how the zone-less layouts above are already interpreted.
	value = strings.TrimSuffix(value, "Z")

	for _, layout := range storedTimestampLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}

	return time.Time{}, false
}

// NormalizeScannedRowID converts an ID scanned into an interface{} back into a
// type database/sql accepts as a bind parameter. Drivers hand back []byte for
// text columns, which would otherwise be bound as a BLOB and never match a TEXT
// primary key such as a ULID or a session token.
func NormalizeScannedRowID(id interface{}) interface{} {
	if raw, ok := id.([]byte); ok {
		return string(raw)
	}

	return id
}

// DeleteRowsWithTimestampBefore deletes every row of table whose
// timestampColumn holds an instant earlier than cutoff, comparing in Go so that
// mixed on-disk layouts and mixed timezones all resolve to the same absolute
// instant. When includeEqual is true a row whose timestamp equals cutoff is
// deleted too, matching an SQL "<=" test. It returns the number of rows deleted.
//
// table, idColumn and timestampColumn are compile-time constants supplied by
// the calling package - never user input - and only the row IDs are bound
// values, so no untrusted data reaches the statement text. IDs are carried as
// the driver's own scanned values, which keeps both INTEGER and TEXT (ULID,
// session token, rowid) primary keys exact.
func DeleteRowsWithTimestampBefore(db *sql.DB, table, idColumn, timestampColumn string, cutoff time.Time, includeEqual bool) (int64, error) {
	expiredIDs, err := SelectRowIDsWithTimestampBefore(db, table, idColumn, timestampColumn, cutoff, includeEqual)
	if err != nil {
		return 0, err
	}

	var deleted int64
	for start := 0; start < len(expiredIDs); start += deleteChunkSize {
		end := start + deleteChunkSize
		if end > len(expiredIDs) {
			end = len(expiredIDs)
		}

		chunk := expiredIDs[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)", table, idColumn, placeholders)

		ctx, cancel := context.WithTimeout(context.Background(), bulkDeleteTimeout)
		result, execErr := db.ExecContext(ctx, deleteQuery, chunk...)
		cancel()
		if execErr != nil {
			return deleted, execErr
		}

		affected, _ := result.RowsAffected()
		deleted += affected
	}

	return deleted, nil
}

// SelectRowIDsWithTimestampBefore returns the primary keys of every row of
// table whose timestampColumn holds an instant earlier than cutoff (or equal to
// it when includeEqual is true), parsed in Go rather than compared as SQL text.
// Rows holding NULL or an unrecognised value are never returned.
func SelectRowIDsWithTimestampBefore(db *sql.DB, table, idColumn, timestampColumn string, cutoff time.Time, includeEqual bool) ([]interface{}, error) {
	selectQuery := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s IS NOT NULL", idColumn, timestampColumn, table, timestampColumn)

	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, selectQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cutoff = cutoff.UTC()

	var matched []interface{}
	for rows.Next() {
		var id interface{}
		var stored interface{}
		if scanErr := rows.Scan(&id, &stored); scanErr != nil {
			return nil, scanErr
		}

		parsed, ok := ParseStoredTimestamp(stored)
		if !ok {
			continue
		}

		if parsed.Before(cutoff) || (includeEqual && parsed.Equal(cutoff)) {
			matched = append(matched, NormalizeScannedRowID(id))
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	return matched, nil
}

// IsAfter reports whether a value scanned from a DATETIME column is strictly
// later than threshold. An unparseable or NULL value reports false, which keeps
// a row the project cannot interpret out of every "still valid" listing rather
// than granting it unlimited life.
func IsAfter(value interface{}, threshold time.Time) bool {
	parsed, ok := ParseStoredTimestamp(value)
	if !ok {
		return false
	}

	return parsed.After(threshold.UTC())
}
