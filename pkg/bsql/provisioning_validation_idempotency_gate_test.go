package bsql

import (
	"testing"

	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
)

// validationNoRowsMeansIdempotent gates validation "no rows" onto idempotency (not a failed
// precondition) only on the two DDL engines whose already-applied GRANT/REVOKE raises an error
// instead of affecting rows. Db2 ships behind a build tag, so it stays on by engine regardless of
// the opt-in; Oracle ships in every binary, so it reinterprets no-rows only when the entitlement
// opts in. Every non-DDL engine fails loudly in both cases. This guards against re-enabling any
// engine by accident, flipping Db2's default-on behavior, or dropping Oracle's opt-in requirement.
func TestValidationNoRowsMeansIdempotent_EngineGate(t *testing.T) {
	cases := map[database.DbEngine]struct {
		optInOff bool
		optInOn  bool
	}{
		database.DB2:        {optInOff: true, optInOn: true},
		database.Oracle:     {optInOff: false, optInOn: true},
		database.SQLite:     {optInOff: false, optInOn: false},
		database.MySQL:      {optInOff: false, optInOn: false},
		database.PostgreSQL: {optInOff: false, optInOn: false},
		database.MSSQL:      {optInOff: false, optInOn: false},
		database.HDB:        {optInOff: false, optInOn: false},
		database.Vertica:    {optInOff: false, optInOn: false},
	}
	for engine, want := range cases {
		s := &SQLSyncer{dbEngine: engine}
		require.Equal(t, want.optInOff, s.validationNoRowsMeansIdempotent(false), "opt-in off, engine=%v", engine)
		require.Equal(t, want.optInOn, s.validationNoRowsMeansIdempotent(true), "opt-in on, engine=%v", engine)
	}
}
