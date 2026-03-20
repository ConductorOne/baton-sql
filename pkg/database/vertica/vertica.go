package vertica

import (
	"context"
	"database/sql"

	_ "github.com/vertica/vertica-sql-go"
)

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("vertica", dsn)
	if err != nil {
		return nil, err
	}

	return db, nil
}
