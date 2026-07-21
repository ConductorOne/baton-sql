//go:build !db2

package db2

import (
	"context"
	"database/sql"
	"errors"
)

// Connect is a stub used when DB2 support is not compiled in. The go_ibm_db
// driver requires cgo and the IBM clidriver at build time, so it is only
// included when building with -tags db2.
func Connect(_ context.Context, _ string) (*sql.DB, error) {
	return nil, errors.New("baton-sql: DB2 support not compiled into this binary; rebuild with -tags db2 (see docs/db2.md)")
}
