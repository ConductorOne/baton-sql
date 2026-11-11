package sqlserver

import (
	"context"
	"database/sql"

	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/microsoft/go-mssqldb/integratedauth/krb5" // registers the "krb5" integrated-auth provider
)

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, err
	}
	return db, nil
}
