package bsql

import (
	"testing"

	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
)

// validationNoRowsMeansIdempotent gates validation "no rows" onto idempotency (not a failed
// precondition) only on DDL engines whose already-applied GRANT/REVOKE raises an error instead
// of affecting rows (Db2, Oracle), AND only when the entitlement opts in. With the opt-in off,
// every engine fails loudly; with it on, only the two DDL engines reinterpret no-rows. This
// guards against re-enabling any engine by accident or dropping the opt-in requirement.
func TestValidationNoRowsMeansIdempotent_EngineGate(t *testing.T) {
	ddlEngines := map[database.DbEngine]bool{
		database.DB2:        true,
		database.Oracle:     true,
		database.SQLite:     false,
		database.MySQL:      false,
		database.PostgreSQL: false,
		database.MSSQL:      false,
		database.HDB:        false,
		database.Vertica:    false,
	}
	for engine, ddlOptIn := range ddlEngines {
		s := &SQLSyncer{dbEngine: engine}
		require.False(t, s.validationNoRowsMeansIdempotent(false), "opt-in off must never signal idempotency, engine=%v", engine)
		require.Equal(t, ddlOptIn, s.validationNoRowsMeansIdempotent(true), "opt-in on, engine=%v", engine)
	}
}
