package oracle

import (
	"context"
	"database/sql"

	_ "github.com/sijms/go-ora/v2"
)

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, err
	}
	return db, nil
}
