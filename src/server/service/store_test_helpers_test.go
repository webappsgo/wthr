package service

import (
	"database/sql"

	"github.com/webappsgo/wthr/src/database"
)

func serviceStoreForDB(db *sql.DB) *database.Store {
	if db == nil {
		return nil
	}
	return database.NewStore(db)
}
