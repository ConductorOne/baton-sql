package bsql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func newSyncerForIterTest(dbNames []string) *SQLSyncer {
	// Synthetic *sql.DB stand-ins. iterateDBs never dereferences them so a non-nil sentinel
	// is enough; query execution is exercised by higher-level tests with real connections.
	dbs := make(map[string]*sql.DB, len(dbNames))
	for _, name := range dbNames {
		dbs[name] = &sql.DB{}
	}
	primary := ""
	if len(dbNames) > 0 {
		primary = dbNames[0]
	}
	return &SQLSyncer{
		dbs:           dbs,
		dbNames:       append([]string(nil), dbNames...),
		primaryDBName: primary,
	}
}

func TestIterateDBs_SingleDatabasePathIsTransparent(t *testing.T) {
	s := newSyncerForIterTest([]string{"only"})

	var calls int
	npt, err := s.iterateDBs(context.Background(), "", &pagination.Token{Token: "inner-token"}, func(_ context.Context, dbName string, innerToken *pagination.Token) (string, error) {
		calls++
		require.Equal(t, "only", dbName)
		require.Equal(t, "inner-token", innerToken.Token)
		return "next-inner", nil
	})

	require.NoError(t, err)
	require.Equal(t, 1, calls)
	// Single-database path returns the work fn's token verbatim — wire-compatible with the
	// pre-multi-database token format.
	require.Equal(t, "next-inner", npt)
}

func TestIterateDBs_ClusterScopeRunsOnceAgainstPrimary(t *testing.T) {
	s := newSyncerForIterTest([]string{"a", "b", "c"})

	var calls int
	npt, err := s.iterateDBs(context.Background(), scopeCluster, &pagination.Token{Token: "carry"}, func(_ context.Context, dbName string, innerToken *pagination.Token) (string, error) {
		calls++
		require.Equal(t, "a", dbName, "cluster scope must use primary (sorted first)")
		require.Equal(t, "carry", innerToken.Token)
		return "", nil
	})

	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "", npt)
	require.Equal(t, "a", s.currentDBName)
}

func TestIterateDBs_MultiDatabaseVisitsEveryDBInOrder(t *testing.T) {
	s := newSyncerForIterTest([]string{"db1", "db2", "db3"})

	var visited []string
	token := ""
	for i := 0; i < 5; i++ {
		next, err := s.iterateDBs(context.Background(), "", &pagination.Token{Token: token}, func(_ context.Context, dbName string, _ *pagination.Token) (string, error) {
			visited = append(visited, dbName)
			return "", nil // each DB completes in one page
		})
		require.NoError(t, err)
		token = next
		if next == "" {
			break
		}
	}

	require.Equal(t, []string{"db1", "db2", "db3"}, visited)
	require.Equal(t, "", token, "exhausted bag must marshal to empty")
}

func TestIterateDBs_MultiDatabasePreservesInnerPagination(t *testing.T) {
	s := newSyncerForIterTest([]string{"db1", "db2"})

	type call struct {
		db    string
		inner string
	}
	var calls []call

	// db1: two pages. db2: one page. Total: 3 SDK calls to iterateDBs.
	step := map[string]string{
		"db1-":   "p1",
		"db1-p1": "",
		"db2-":   "",
	}

	token := ""
	for i := 0; i < 10; i++ {
		next, err := s.iterateDBs(context.Background(), "", &pagination.Token{Token: token}, func(_ context.Context, dbName string, innerToken *pagination.Token) (string, error) {
			calls = append(calls, call{db: dbName, inner: innerToken.Token})
			return step[dbName+"-"+innerToken.Token], nil
		})
		require.NoError(t, err)
		token = next
		if next == "" {
			break
		}
	}

	require.Equal(t, []call{
		{db: "db1", inner: ""},
		{db: "db1", inner: "p1"},
		{db: "db2", inner: ""},
	}, calls)
}

func TestIterateDBs_PropagatesWorkErrors(t *testing.T) {
	s := newSyncerForIterTest([]string{"db1", "db2"})
	sentinel := errors.New("boom")

	_, err := s.iterateDBs(context.Background(), "", &pagination.Token{}, func(_ context.Context, _ string, _ *pagination.Token) (string, error) {
		return "", sentinel
	})

	require.ErrorIs(t, err, sentinel)
}

func TestSetCurrentDB_UnknownNameErrors(t *testing.T) {
	s := newSyncerForIterTest([]string{"a", "b"})
	require.NoError(t, s.setCurrentDB("a"))
	require.Same(t, s.dbs["a"], s.db)

	err := s.setCurrentDB("nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown database")
	// State must not have mutated to the unknown handle.
	require.Same(t, s.dbs["a"], s.db)
}

func TestSortedDBNames_IsSorted(t *testing.T) {
	dbs := map[string]*sql.DB{
		"zeta":  {},
		"alpha": {},
		"mu":    {},
	}
	require.Equal(t, []string{"alpha", "mu", "zeta"}, sortedDBNames(dbs))
}

func TestDatabasesConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DatabasesConfig
		wantErr bool
	}{
		{"static only", DatabasesConfig{Static: []string{"a"}}, false},
		{"discovery only", DatabasesConfig{DiscoveryQuery: "SELECT 1"}, false},
		{"both set", DatabasesConfig{Static: []string{"a"}, DiscoveryQuery: "SELECT 1"}, true},
		{"neither set", DatabasesConfig{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolveProvisioningDB_RoutesByVars(t *testing.T) {
	// dbNames are sorted, so analytics is primary even though we point s.db at dev to
	// simulate sync iteration landing on a non-primary handle.
	s := newSyncerForIterTest([]string{"analytics", "dev"})
	s.db = s.dbs["dev"]

	got, err := s.resolveProvisioningDB(map[string]any{rowColDatabase: "dev"})
	require.NoError(t, err)
	require.Same(t, s.dbs["dev"], got)

	// No "database" var → primary, NOT s.db. Decouples provisioning from sync state.
	got, err = s.resolveProvisioningDB(map[string]any{})
	require.NoError(t, err)
	require.Same(t, s.dbs["analytics"], got)

	got, err = s.resolveProvisioningDB(map[string]any{rowColDatabase: ""})
	require.NoError(t, err)
	require.Same(t, s.dbs["analytics"], got)

	_, err = s.resolveProvisioningDB(map[string]any{rowColDatabase: "nope"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown database")
}
