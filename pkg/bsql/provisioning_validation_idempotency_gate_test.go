package bsql

import (
	"testing"

	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
)

// validationNoRowsMeansIdempotent is the DDL-engine gate: validation "no rows" is only
// treated as idempotency (not a failed precondition) for engines whose already-applied
// GRANT/REVOKE raises an error instead of affecting rows. Only Db2 qualifies today, and it
// ships opt-in behind the db2 build tag. Oracle and the other DDL engines stay false: they
// ship default-on, so flipping the gate would silently reinterpret existing configs that use
// validation_queries as loud existence preconditions. Adding one back needs a per-config
// opt-in first, so this test guards against re-enabling any of them by accident.
func TestValidationNoRowsMeansIdempotent_EngineGate(t *testing.T) {
	ddl := map[database.DbEngine]bool{
		database.DB2:        true,
		database.Oracle:     false,
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
