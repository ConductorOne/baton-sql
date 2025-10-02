package hdb

import (
	"context"
	"database/sql"

	_ "github.com/SAP/go-hdb/driver"
)

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("hdb", dsn)
	if err != nil {
		return nil, err
	}

	return db, nil
}
