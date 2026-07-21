//go:build db2

package db2

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/ibmdb/go_ibm_db"
)

// Connect establishes a connection to DB2 database.
func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	// Convert URL format to DB2 DSN format if needed
	db2DSN, err := convertToDB2DSN(dsn)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("go_ibm_db", db2DSN)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(time.Minute * 5)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
