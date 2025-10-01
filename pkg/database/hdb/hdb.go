package hdb

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/SAP/go-hdb/driver"
)

const (
	MaxIdleConns    = 10
	MaxOpenConns    = 10
	MaxConnLifetime = 5 * time.Minute
)

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("hdb", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(MaxOpenConns)
	db.SetMaxIdleConns(MaxIdleConns)
	db.SetConnMaxLifetime(MaxConnLifetime)

	return db, nil
}
