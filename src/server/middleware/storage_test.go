package middleware

import (
	"database/sql"

	"github.com/webappsgo/wthr/src/database"
)

func storeForDB(db *sql.DB) *database.Store {
	return database.NewStore(db)
}
