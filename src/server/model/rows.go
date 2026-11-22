package model

import (
	"database/sql"
	"log"
)

func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Printf("WARNING: failed to close database rows: %v", err)
	}
}
