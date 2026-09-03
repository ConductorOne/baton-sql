package bsql

import (
	"testing"

	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
)

// validationNoRowsMeansIdempotent is the DDL-engine gate: it must be true only for
// engines whose already-applied GRANT/REVOKE raises an error instead of affecting rows,
// so validation "no rows" means idempotency rather than a failed precondition.
//
// The behavioral grant/revoke wiring is covered by the Db2 tests in
// provisioning_validation_idempotency_test.go; Oracle can't reuse them because the
// Oracle driver rewrites ?<name> placeholders to bind syntax the sqlite test backend
// rejects. Oracle's end-to-end behavior was verified live against Oracle XE 21c on
// 2026-09-03 (grant/re-grant -> GrantAlreadyExists, revoke/re-revoke -> GrantAlreadyRevoked,
// ORA-01951 no longer surfaced).
func TestValidationNoRowsMeansIdempotent_EngineGate(t *testing.T) {
	ddl := map[database.DbEngine]bool{
		database.DB2:        true,
		database.Oracle:     true,
		database.SQLite:     false,
		database.MySQL:      false,
		database.PostgreSQL: false,
		database.MSSQL:      false,
		database.HDB:        false,
		database.Vertica:    false,
	}
	for engine, want := range ddl {
		s := &SQLSyncer{dbEngine: engine}
		require.Equal(t, want, s.validationNoRowsMeansIdempotent(), "engine=%v", engine)
	}
}
