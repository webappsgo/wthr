package handler

import (
	"database/sql"

	"github.com/webappsgo/wthr/src/database"
)

func storeForDB(db any) *database.Store {
	switch value := db.(type) {
	case *database.Store:
		return value
	case *sql.DB:
		return database.NewStore(value)
	default:
		return nil
	}
}
