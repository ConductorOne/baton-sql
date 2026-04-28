package bsql

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/stretchr/testify/require"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
)

// newEventTestDB creates an in-memory SQLite database for event feed tests.
func newEventTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newEventTestSyncer creates a minimal SQLSyncer backed by a SQLite DB.
// resourceIDExpr is the CEL expression used as the default resource ID (List.Map.Id fallback).
func newEventTestSyncer(t *testing.T, db *sql.DB, resourceIDExpr string) *SQLSyncer {
	t.Helper()
	celEnv, err := bcel.NewEnv(t.Context())
	require.NoError(t, err)
	return &SQLSyncer{
		db:       db,
		dbEngine: database.SQLite,
		config: ResourceType{
			List: &ListQuery{
				Map: &ResourceMapping{Id: resourceIDExpr},
			},
		},
		env: celEnv,
	}
}

// TestGrantRowKey covers all branches of the rowKey extraction helper.
func TestGrantRowKey(t *testing.T) {
	tests := []struct {
		name string
		row  map[string]any
		pag  *Pagination
		want string
	}{
		{"nil pagination", map[string]any{"id": "1"}, nil, ""},
		{"empty PrimaryKey", map[string]any{"id": "1"}, &Pagination{}, ""},
		{"key missing from row", map[string]any{"name": "x"}, &Pagination{PrimaryKey: "id"}, ""},
		{"int64 value", map[string]any{"id": int64(42)}, &Pagination{PrimaryKey: "id"}, "42"},
		{"float64 value", map[string]any{"id": float64(99)}, &Pagination{PrimaryKey: "id"}, "99"},
		{"string value", map[string]any{"id": "abc-123"}, &Pagination{PrimaryKey: "id"}, "abc-123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, grantRowKey(tt.row, tt.pag))
		})
	}
}

// TestGetSources verifies deterministic source enumeration.
func TestGetSources(t *testing.T) {
	resSync := &ResourceIncrementalSync{Query: "SELECT 1", CursorColumn: "updated_at"}
	grantSync := &GrantsIncrementalSync{
		ResourceId:          "cols.id",
		ChangesQuery:        "SELECT 1",
		ChangesCursorColumn: "updated_at",
	}
	grantSyncWithRevokes := &GrantsIncrementalSync{
		ResourceId:          "cols.id",
		ChangesQuery:        "SELECT 1",
		ChangesCursorColumn: "updated_at",
		RevokesQuery:        "SELECT 1",
		RevokesCursorColumn: "deleted_at",
	}

	t.Run("empty config produces no sources", func(t *testing.T) {
		sources := getSources(Config{ResourceTypes: map[string]ResourceType{
			"user": {Name: "User"},
		}})
		require.Empty(t, sources)
	})

	t.Run("resource-only incremental sync", func(t *testing.T) {
		sources := getSources(Config{ResourceTypes: map[string]ResourceType{
			"user": {IncrementalSync: resSync},
		}})
		require.Len(t, sources, 1)
		require.Equal(t, "user:resource", sources[0].Key)
		require.Equal(t, incSyncSourceKindResource, sources[0].Kind)
		require.Equal(t, "user", sources[0].ResourceType)
	})

	t.Run("grant changes without revokes produces one source", func(t *testing.T) {
		sources := getSources(Config{ResourceTypes: map[string]ResourceType{
			"role": {Grants: []*GrantsQuery{{IncrementalSync: grantSync}}},
		}})
		require.Len(t, sources, 1)
		require.Equal(t, "role:grants:0:changes", sources[0].Key)
		require.Equal(t, incSyncSourceKindGrantChanges, sources[0].Kind)
	})

	t.Run("grant changes with revokes produces two sources", func(t *testing.T) {
		sources := getSources(Config{ResourceTypes: map[string]ResourceType{
			"role": {Grants: []*GrantsQuery{{IncrementalSync: grantSyncWithRevokes}}},
		}})
		require.Len(t, sources, 2)
		require.Equal(t, incSyncSourceKindGrantChanges, sources[0].Kind)
		require.Equal(t, "role:grants:0:revokes", sources[1].Key)
		require.Equal(t, incSyncSourceKindGrantRevokes, sources[1].Kind)
	})

	t.Run("multiple resource types sorted lexicographically", func(t *testing.T) {
		sources := getSources(Config{ResourceTypes: map[string]ResourceType{
			"role":  {IncrementalSync: resSync},
			"group": {IncrementalSync: resSync},
			"user":  {IncrementalSync: resSync},
		}})
		require.Len(t, sources, 3)
		require.Equal(t, "group", sources[0].ResourceType)
		require.Equal(t, "role", sources[1].ResourceType)
		require.Equal(t, "user", sources[2].ResourceType)
	})

	t.Run("resource and grants both present", func(t *testing.T) {
		sources := getSources(Config{ResourceTypes: map[string]ResourceType{
			"group": {
				IncrementalSync: resSync,
				Grants:          []*GrantsQuery{{IncrementalSync: grantSyncWithRevokes}},
			},
		}})
		require.Len(t, sources, 3)
		require.Equal(t, incSyncSourceKindResource, sources[0].Kind)
		require.Equal(t, incSyncSourceKindGrantChanges, sources[1].Kind)
		require.Equal(t, incSyncSourceKindGrantRevokes, sources[2].Kind)
	})
}

// TestCommitAndAdvance exercises cursor state transitions.
func TestCommitAndAdvance(t *testing.T) {
	src := func(key string) incSyncSource { return incSyncSource{Key: key} }

	t.Run("nextPageToken keeps current source with hasMore=true", func(t *testing.T) {
		f := &SQLEventFeed{}
		cursor := &eventFeedCursor{SourceCursors: make(map[string]string)}
		sources := []incSyncSource{src("a"), src("b")}

		state, err := f.commitAndAdvance(cursor, sources, "page2", time.Time{})
		require.NoError(t, err)
		require.True(t, state.HasMore)
		require.Equal(t, 0, cursor.CurrentSourceIdx)
		require.Equal(t, "page2", cursor.CurrentPageToken)
	})

	t.Run("exhausted source advances index and commits maxSeen", func(t *testing.T) {
		f := &SQLEventFeed{}
		cursor := &eventFeedCursor{SourceCursors: make(map[string]string)}
		sources := []incSyncSource{src("src-a"), src("src-b")}
		ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

		state, err := f.commitAndAdvance(cursor, sources, "", ts)
		require.NoError(t, err)
		require.True(t, state.HasMore)
		require.Equal(t, 1, cursor.CurrentSourceIdx)
		require.Equal(t, ts.UTC().Format(time.RFC3339Nano), cursor.SourceCursors["src-a"])
		require.Empty(t, cursor.CurrentSince)
		require.Empty(t, cursor.CurrentMaxSeen)
		require.Empty(t, cursor.CurrentPageToken)
	})

	t.Run("last source exhausted sets hasMore=false and wraps index", func(t *testing.T) {
		f := &SQLEventFeed{}
		cursor := &eventFeedCursor{SourceCursors: make(map[string]string)}
		sources := []incSyncSource{src("only")}

		state, err := f.commitAndAdvance(cursor, sources, "", time.Time{})
		require.NoError(t, err)
		require.False(t, state.HasMore)
		require.Equal(t, 0, cursor.CurrentSourceIdx)
	})

	t.Run("zero maxSeen does not update committed cursor", func(t *testing.T) {
		f := &SQLEventFeed{}
		cursor := &eventFeedCursor{SourceCursors: make(map[string]string)}
		sources := []incSyncSource{src("src-a")}

		_, err := f.commitAndAdvance(cursor, sources, "", time.Time{})
		require.NoError(t, err)
		require.Empty(t, cursor.SourceCursors["src-a"])
	})

	t.Run("maxSeen accumulates across pages and only advances when newer", func(t *testing.T) {
		f := &SQLEventFeed{}
		cursor := &eventFeedCursor{SourceCursors: make(map[string]string)}
		sources := []incSyncSource{src("src-a"), src("src-b")}
		ts1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		ts2 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

		_, err := f.commitAndAdvance(cursor, sources, "p2", ts1)
		require.NoError(t, err)
		require.Equal(t, ts1.UTC().Format(time.RFC3339Nano), cursor.CurrentMaxSeen)

		_, err = f.commitAndAdvance(cursor, sources, "p3", ts2)
		require.NoError(t, err)
		require.Equal(t, ts2.UTC().Format(time.RFC3339Nano), cursor.CurrentMaxSeen)

		// Older timestamp does not regress CurrentMaxSeen; source exhausted here
		_, err = f.commitAndAdvance(cursor, sources, "", ts1)
		require.NoError(t, err)
		require.Equal(t, ts2.UTC().Format(time.RFC3339Nano), cursor.SourceCursors["src-a"])
	})
}

// TestSinceForSource verifies committed-cursor lookup and default-lookback fallback.
func TestSinceForSource(t *testing.T) {
	t.Run("returns committed cursor when present", func(t *testing.T) {
		ts := time.Date(2025, 3, 1, 8, 0, 0, 0, time.UTC)
		f := &SQLEventFeed{config: Config{}}
		cursor := &eventFeedCursor{
			SourceCursors: map[string]string{"user:resource": ts.UTC().Format(time.RFC3339Nano)},
		}
		got := f.sinceForSource(cursor, "user:resource")
		require.True(t, got.Equal(ts))
	})

	t.Run("falls back to default lookback when no committed cursor", func(t *testing.T) {
		f := &SQLEventFeed{config: Config{}}
		cursor := &eventFeedCursor{SourceCursors: map[string]string{}}
		before := time.Now().UTC().Add(-defaultLookbackDuration)
		got := f.sinceForSource(cursor, "user:resource")
		require.False(t, got.Before(before.Add(-time.Second)))
		require.False(t, got.After(time.Now().UTC()))
	})

	t.Run("uses configured DefaultLookback duration", func(t *testing.T) {
		f := &SQLEventFeed{config: Config{
			IncrementalSync: &IncrementalSyncConfig{DefaultLookback: "30m"},
		}}
		cursor := &eventFeedCursor{SourceCursors: map[string]string{}}
		before := time.Now().UTC().Add(-30 * time.Minute)
		got := f.sinceForSource(cursor, "user:resource")
		require.False(t, got.Before(before.Add(-time.Second)))
		require.False(t, got.After(time.Now().UTC()))
	})
}

// --- SQLite integration tests ---

// since is a fixed reference point used across integration tests.
var testSince = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// ts returns a UTC timestamp t days after testSince.
func ts(days int) time.Time {
	return testSince.AddDate(0, 0, days)
}

// tsStr returns the MySQL-format string ("2006-01-02 15:04:05") for t days after testSince.
// Insert these into SQLite TEXT columns: glebarez parses this format back to time.Time on read,
// and parseTime also handles this format — so both the happy path and error detection work.
func tsStr(days int) string {
	return ts(days).UTC().Format("2006-01-02 15:04:05")
}

func TestProcessResourceChangePage(t *testing.T) {
	const schema = `CREATE TABLE resources (id TEXT NOT NULL, updated_at TEXT NOT NULL)`

	t.Run("emits ResourceChangeEvent per matching row", func(t *testing.T) {
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),schema)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(),`INSERT INTO resources VALUES (?,?),(?,?)`, "user-1", tsStr(1), "user-2", tsStr(2))
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindResource,
			ResourceType: "user",
			ResConfig: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM resources WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
			},
		}

		events, npt, maxSeen, err := f.processResourceChangePage(t.Context(), s, source, testSince, 0, "")
		require.NoError(t, err)
		require.Len(t, events, 2)
		require.Empty(t, npt)
		require.True(t, maxSeen.Equal(ts(2)))

		for _, ev := range events {
			require.NotNil(t, ev.GetResourceChangeEvent(), "expected ResourceChangeEvent, got %v", ev)
			require.True(t, strings.HasPrefix(ev.Id, "resource:user:"), "unexpected event ID prefix: %s", ev.Id)
		}
	})

	t.Run("event ID includes rowKey when PrimaryKey is configured", func(t *testing.T) {
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),schema)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(),`INSERT INTO resources VALUES (?,?)`, "user-42", tsStr(1))
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindResource,
			ResourceType: "user",
			ResConfig: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM resources WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
				Pagination:   &Pagination{Strategy: "offset", PrimaryKey: "id"},
			},
		}

		events, _, _, err := f.processResourceChangePage(t.Context(), s, source, testSince, 0, "")
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.True(t, strings.HasSuffix(events[0].Id, ":user-42"),
			"expected ID ending in :user-42, got %s", events[0].Id)
	})

	t.Run("custom ResourceId expression overrides List.Map.Id", func(t *testing.T) {
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),`CREATE TABLE things (ext_id TEXT, row_id TEXT, updated_at TEXT)`)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(),`INSERT INTO things VALUES (?,?,?)`, "ext-99", "row-1", tsStr(1))
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.row_id") // default would pick row_id
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindResource,
			ResourceType: "thing",
			ResConfig: &ResourceIncrementalSync{
				Query:        "SELECT ext_id, row_id, updated_at FROM things WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
				ResourceId:   "cols.ext_id", // override: use ext_id instead
			},
		}

		events, _, _, err := f.processResourceChangePage(t.Context(), s, source, testSince, 0, "")
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, "ext-99", events[0].GetResourceChangeEvent().GetResourceId().GetResource())
		require.Contains(t, events[0].Id, "ext-99")
	})

	t.Run("no rows returns empty events and zero maxSeen", func(t *testing.T) {
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),schema)
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindResource,
			ResourceType: "user",
			ResConfig: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM resources WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
			},
		}

		events, npt, maxSeen, err := f.processResourceChangePage(t.Context(), s, source, testSince, 0, "")
		require.NoError(t, err)
		require.Empty(t, events)
		require.Empty(t, npt)
		require.True(t, maxSeen.IsZero())
	})

	t.Run("unparseable cursor column value returns error", func(t *testing.T) {
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),schema)
		require.NoError(t, err)
		// "not-a-timestamp" lexicographically > "2025-..." so the WHERE clause passes it through.
		_, err = db.ExecContext(t.Context(),`INSERT INTO resources VALUES (?,?)`, "user-bad", "not-a-timestamp")
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindResource,
			ResourceType: "user",
			ResConfig: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM resources WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
			},
		}

		_, _, _, err = f.processResourceChangePage(t.Context(), s, source, testSince, 0, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "updated_at")
	})
}

func TestProcessGrantPage(t *testing.T) {
	const schema = `
CREATE TABLE memberships (
	id        INTEGER PRIMARY KEY,
	group_id  TEXT NOT NULL,
	user_id   TEXT NOT NULL,
	role      TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	deleted_at TEXT
)`

	setupDB := func(t *testing.T) *sql.DB {
		t.Helper()
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),schema)
		require.NoError(t, err)
		return db
	}

	grantMap := []*GrantMapping{
		{PrincipalId: "cols.user_id", PrincipalType: "user", Entitlement: "cols.role"},
	}
	changesConfig := &GrantsIncrementalSync{
		ResourceId:          "cols.group_id",
		ChangesQuery:        "SELECT id, group_id, user_id, role, updated_at FROM memberships WHERE updated_at > ?<since>",
		ChangesCursorColumn: "updated_at",
	}

	t.Run("emits CreateGrantEvent per row", func(t *testing.T) {
		db := setupDB(t)
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO memberships(group_id, user_id, role, updated_at) VALUES (?,?,?,?),(?,?,?,?)`,
			"group-1", "user-a", "member", tsStr(1),
			"group-1", "user-b", "member", tsStr(2),
		)
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindGrantChanges,
			ResourceType: "group",
			GrantConfig:  changesConfig,
			GrantMap:     grantMap,
		}

		events, npt, maxSeen, err := f.processGrantPage(t.Context(), s, source, testSince, 0, "", false)
		require.NoError(t, err)
		require.Len(t, events, 2)
		require.Empty(t, npt)
		require.True(t, maxSeen.Equal(ts(2)))

		for _, ev := range events {
			require.NotNil(t, ev.GetCreateGrantEvent(), "expected CreateGrantEvent, got %v", ev)
			require.True(t, strings.HasPrefix(ev.Id, "grant:group:group-1:"), "unexpected event ID: %s", ev.Id)
		}
	})

	t.Run("emits CreateRevokeEvent when isRevoke=true", func(t *testing.T) {
		db := setupDB(t)
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO memberships(group_id, user_id, role, updated_at, deleted_at) VALUES (?,?,?,?,?)`,
			"group-1", "user-a", "member", tsStr(1), tsStr(1),
		)
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindGrantRevokes,
			ResourceType: "group",
			GrantConfig: &GrantsIncrementalSync{
				ResourceId:          "cols.group_id",
				RevokesQuery:        "SELECT id, group_id, user_id, role, deleted_at, updated_at FROM memberships WHERE deleted_at > ?<since>",
				RevokesCursorColumn: "deleted_at",
			},
			GrantMap: grantMap,
		}

		events, _, _, err := f.processGrantPage(t.Context(), s, source, testSince, 0, "", true)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.NotNil(t, events[0].GetCreateRevokeEvent())
		require.True(t, strings.HasPrefix(events[0].Id, "revoke:group:"), "unexpected event ID: %s", events[0].Id)
	})

	t.Run("event ID format is grant:rtType:resourceID:principalID:timestamp:rowKey", func(t *testing.T) {
		db := setupDB(t)
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO memberships(id, group_id, user_id, role, updated_at) VALUES (?,?,?,?,?)`,
			int64(7), "grp-1", "usr-1", "admin", tsStr(166), // well after testSince
		)
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindGrantChanges,
			ResourceType: "group",
			GrantConfig: &GrantsIncrementalSync{
				ResourceId:          "cols.group_id",
				ChangesQuery:        "SELECT id, group_id, user_id, role, updated_at FROM memberships WHERE updated_at > ?<since>",
				ChangesCursorColumn: "updated_at",
				Pagination:          &Pagination{Strategy: "offset", PrimaryKey: "id"},
			},
			GrantMap: grantMap,
		}

		events, _, _, err := f.processGrantPage(t.Context(), s, source, testSince, 0, "", false)
		require.NoError(t, err)
		require.Len(t, events, 1)

		parts := strings.Split(events[0].Id, ":")
		// Format: grant:group:grp-1:usr-1:<timestamp>:<rowKey>
		require.Equal(t, "grant", parts[0])
		require.Equal(t, "group", parts[1])
		require.Equal(t, "grp-1", parts[2])
		require.Equal(t, "usr-1", parts[3])
		require.NotEmpty(t, parts[4]) // timestamp
		require.Equal(t, "7", parts[len(parts)-1]) // rowKey = primary key value
	})

	t.Run("SkipIf expression skips matching rows", func(t *testing.T) {
		db := setupDB(t)
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO memberships(group_id, user_id, role, updated_at) VALUES (?,?,?,?),(?,?,?,?)`,
			"group-1", "skip-me", "member", tsStr(1),
			"group-1", "keep-me", "member", tsStr(2),
		)
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindGrantChanges,
			ResourceType: "group",
			GrantConfig:  changesConfig,
			GrantMap: []*GrantMapping{
				{
					PrincipalId:   "cols.user_id",
					PrincipalType: "user",
					Entitlement:   "cols.role",
					SkipIf:        "cols.user_id == 'skip-me'",
				},
			},
		}

		events, _, _, err := f.processGrantPage(t.Context(), s, source, testSince, 0, "", false)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, "keep-me", events[0].GetCreateGrantEvent().GetPrincipal().GetId().GetResource())
	})

	t.Run("unparseable cursor column returns error", func(t *testing.T) {
		db := setupDB(t)
		// "not-a-timestamp" passes the WHERE clause due to string ordering.
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO memberships(group_id, user_id, role, updated_at) VALUES (?,?,?,?)`,
			"group-1", "user-a", "member", "not-a-timestamp",
		)
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, "cols.id")
		f := &SQLEventFeed{}
		source := incSyncSource{
			Kind:         incSyncSourceKindGrantChanges,
			ResourceType: "group",
			GrantConfig:  changesConfig,
			GrantMap:     grantMap,
		}

		_, _, _, err = f.processGrantPage(t.Context(), s, source, testSince, 0, "", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "updated_at")
	})
}

// newListEventsSyncer builds a SQLSyncer for use in ListEvents integration tests.
func newListEventsSyncer(t *testing.T, db *sql.DB, rt ResourceType) *SQLSyncer {
	t.Helper()
	celEnv, err := bcel.NewEnv(t.Context())
	require.NoError(t, err)
	return &SQLSyncer{
		db:       db,
		dbEngine: database.SQLite,
		config:   rt,
		env:      celEnv,
	}
}

// TestListEvents covers the main ListEvents entry point end-to-end.
func TestListEvents(t *testing.T) {
	t.Run("no incremental sync sources returns hasMore=false", func(t *testing.T) {
		f := &SQLEventFeed{config: Config{ResourceTypes: map[string]ResourceType{
			"user": {Name: "User"},
		}}}
		events, state, _, err := f.ListEvents(t.Context(), nil, &pagination.StreamToken{})
		require.NoError(t, err)
		require.Empty(t, events)
		require.False(t, state.HasMore)
	})

	t.Run("missing syncer skips source and returns hasMore=false", func(t *testing.T) {
		config := Config{ResourceTypes: map[string]ResourceType{
			"user": {IncrementalSync: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM t WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
			}},
		}}
		f := &SQLEventFeed{config: config, syncers: map[string]*SQLSyncer{}}

		events, state, _, err := f.ListEvents(t.Context(), nil, &pagination.StreamToken{})
		require.NoError(t, err)
		require.Empty(t, events)
		require.False(t, state.HasMore)
	})

	t.Run("returns resource change events from database", func(t *testing.T) {
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),`CREATE TABLE res (id TEXT NOT NULL, updated_at TEXT NOT NULL)`)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(),`INSERT INTO res VALUES (?,?),(?,?)`, "r1", tsStr(1), "r2", tsStr(2))
		require.NoError(t, err)

		rt := ResourceType{
			List: &ListQuery{Map: &ResourceMapping{Id: "cols.id"}},
			IncrementalSync: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM res WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
			},
		}
		syncer := newListEventsSyncer(t, db, rt)
		config := Config{ResourceTypes: map[string]ResourceType{"res": rt}}
		f := &SQLEventFeed{config: config, syncers: map[string]*SQLSyncer{"res": syncer}}

		// Pre-set the committed cursor so `since` is before our test data.
		initCursor, err := marshalCursor(&eventFeedCursor{
			SourceCursors: map[string]string{"res:resource": testSince.UTC().Format(time.RFC3339Nano)},
		})
		require.NoError(t, err)

		events, state, _, err := f.ListEvents(t.Context(), nil, &pagination.StreamToken{Cursor: initCursor})
		require.NoError(t, err)
		require.Len(t, events, 2)
		require.False(t, state.HasMore)
		for _, ev := range events {
			require.NotNil(t, ev.GetResourceChangeEvent())
		}
	})

	t.Run("CurrentSince is pinned across pages of the same scan cycle", func(t *testing.T) {
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(),`CREATE TABLE items (id TEXT NOT NULL, updated_at TEXT NOT NULL)`)
		require.NoError(t, err)
		for i := 1; i <= 3; i++ {
			_, err = db.ExecContext(t.Context(),`INSERT INTO items VALUES (?,?)`, fmt.Sprintf("i%d", i), tsStr(i))
			require.NoError(t, err)
		}

		rt := ResourceType{
			List: &ListQuery{Map: &ResourceMapping{Id: "cols.id"}},
			IncrementalSync: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM items WHERE updated_at > ?<since> ORDER BY id LIMIT ?<limit> OFFSET ?<offset>",
				CursorColumn: "updated_at",
				Pagination:   &Pagination{Strategy: "offset", PrimaryKey: "id", PageSize: 1},
			},
		}
		syncer := newListEventsSyncer(t, db, rt)
		config := Config{ResourceTypes: map[string]ResourceType{"item": rt}}
		f := &SQLEventFeed{config: config, syncers: map[string]*SQLSyncer{"item": syncer}}

		initCursor, err := marshalCursor(&eventFeedCursor{
			SourceCursors: map[string]string{"item:resource": testSince.UTC().Format(time.RFC3339Nano)},
		})
		require.NoError(t, err)

		// Page 1.
		_, state1, _, err := f.ListEvents(t.Context(), nil, &pagination.StreamToken{Cursor: initCursor})
		require.NoError(t, err)
		require.True(t, state1.HasMore)

		cursor1, err := unmarshalCursor(state1.Cursor)
		require.NoError(t, err)
		require.NotEmpty(t, cursor1.CurrentSince)

		// Page 2: CurrentSince must stay constant within the cycle.
		_, state2, _, err := f.ListEvents(t.Context(), nil, &pagination.StreamToken{Cursor: state1.Cursor})
		require.NoError(t, err)

		cursor2, err := unmarshalCursor(state2.Cursor)
		require.NoError(t, err)
		require.Equal(t, cursor1.CurrentSince, cursor2.CurrentSince)
	})

	t.Run("out-of-bounds CurrentSourceIdx is clamped to 0", func(t *testing.T) {
		config := Config{ResourceTypes: map[string]ResourceType{
			"user": {IncrementalSync: &ResourceIncrementalSync{
				Query:        "SELECT id, updated_at FROM t WHERE updated_at > ?<since>",
				CursorColumn: "updated_at",
			}},
		}}
		f := &SQLEventFeed{config: config, syncers: map[string]*SQLSyncer{}}

		cursorStr, err := marshalCursor(&eventFeedCursor{
			SourceCursors:    map[string]string{},
			CurrentSourceIdx: 999,
		})
		require.NoError(t, err)

		_, state, _, err := f.ListEvents(t.Context(), nil, &pagination.StreamToken{Cursor: cursorStr})
		require.NoError(t, err)
		// Clamped to 0 → single source with no syncer → skip → wrap → hasMore=false.
		require.False(t, state.HasMore)
	})
}

// TestMarshalUnmarshalCursor covers cursor serialization round-trips and error paths.
func TestMarshalUnmarshalCursor(t *testing.T) {
	t.Run("roundtrip preserves all fields", func(t *testing.T) {
		c := &eventFeedCursor{
			SourceCursors:    map[string]string{"user:resource": "2025-01-01T00:00:00Z"},
			CurrentSourceIdx: 2,
			CurrentPageToken: "tok",
			CurrentSince:     "2025-01-01T00:00:00Z",
			CurrentMaxSeen:   "2025-01-02T00:00:00Z",
		}
		s, err := marshalCursor(c)
		require.NoError(t, err)
		require.NotEmpty(t, s)

		got, err := unmarshalCursor(s)
		require.NoError(t, err)
		require.Equal(t, c.CurrentSourceIdx, got.CurrentSourceIdx)
		require.Equal(t, c.CurrentPageToken, got.CurrentPageToken)
		require.Equal(t, c.CurrentSince, got.CurrentSince)
		require.Equal(t, c.CurrentMaxSeen, got.CurrentMaxSeen)
		require.Equal(t, c.SourceCursors, got.SourceCursors)
	})

	t.Run("empty string returns initialized empty cursor", func(t *testing.T) {
		got, err := unmarshalCursor("")
		require.NoError(t, err)
		require.NotNil(t, got.SourceCursors)
		require.Equal(t, 0, got.CurrentSourceIdx)
	})

	t.Run("malformed JSON returns error with baton-sql prefix", func(t *testing.T) {
		_, err := unmarshalCursor("{not-json")
		require.Error(t, err)
		require.Contains(t, err.Error(), "baton-sql")
	})

	t.Run("nil cursor marshals to empty string", func(t *testing.T) {
		s, err := marshalCursor(nil)
		require.NoError(t, err)
		require.Empty(t, s)
	})

	t.Run("null SourceCursors is repaired to empty map on unmarshal", func(t *testing.T) {
		s := `{"source_cursors":null,"current_source_idx":0,"current_page_token":""}`
		got, err := unmarshalCursor(s)
		require.NoError(t, err)
		require.NotNil(t, got.SourceCursors)
	})
}

// TestToTime covers all type branches of the toTime converter.
func TestToTime(t *testing.T) {
	ref := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("time.Time passthrough", func(t *testing.T) {
		got, err := toTime(ref)
		require.NoError(t, err)
		require.True(t, got.Equal(ref))
	})

	t.Run("string in MySQL format", func(t *testing.T) {
		got, err := toTime(ref.UTC().Format("2006-01-02 15:04:05"))
		require.NoError(t, err)
		require.True(t, got.Equal(ref))
	})

	t.Run("[]byte in MySQL format", func(t *testing.T) {
		got, err := toTime([]byte(ref.UTC().Format("2006-01-02 15:04:05")))
		require.NoError(t, err)
		require.True(t, got.Equal(ref))
	})

	t.Run("unsupported type returns error", func(t *testing.T) {
		_, err := toTime(12345)
		require.Error(t, err)
	})
}

// TestDefaultLookback covers all branches of the defaultLookback helper.
func TestDefaultLookback(t *testing.T) {
	t.Run("nil config returns default duration", func(t *testing.T) {
		require.Equal(t, defaultLookbackDuration, defaultLookback(nil))
	})

	t.Run("empty DefaultLookback returns default duration", func(t *testing.T) {
		require.Equal(t, defaultLookbackDuration, defaultLookback(&IncrementalSyncConfig{}))
	})

	t.Run("invalid duration string returns default duration", func(t *testing.T) {
		require.Equal(t, defaultLookbackDuration, defaultLookback(&IncrementalSyncConfig{DefaultLookback: "not-a-duration"}))
	})

	t.Run("valid duration string is parsed", func(t *testing.T) {
		require.Equal(t, 30*time.Minute, defaultLookback(&IncrementalSyncConfig{DefaultLookback: "30m"}))
	})
}

// TestMapGrantFromRow covers the skip branches of the grant-row mapping helper.
func TestMapGrantFromRow(t *testing.T) {
	celEnv, err := bcel.NewEnv(t.Context())
	require.NoError(t, err)

	s := &SQLSyncer{env: celEnv}
	f := &SQLEventFeed{}
	resource := &v2.Resource{
		Id: &v2.ResourceId{ResourceType: "group", Resource: "grp-1"},
	}

	t.Run("empty principalID expression skips row", func(t *testing.T) {
		mapping := &GrantMapping{
			PrincipalId:   `""`,
			PrincipalType: "user",
			Entitlement:   `"member"`,
		}
		grant, ok, err := f.mapGrantFromRow(t.Context(), s, resource, mapping, map[string]any{})
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, grant)
	})

	t.Run("empty entitlementID expression skips row", func(t *testing.T) {
		mapping := &GrantMapping{
			PrincipalId:   `"user-1"`,
			PrincipalType: "user",
			Entitlement:   `""`,
		}
		grant, ok, err := f.mapGrantFromRow(t.Context(), s, resource, mapping, map[string]any{})
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, grant)
	})

	t.Run("SkipIf=true skips row", func(t *testing.T) {
		mapping := &GrantMapping{
			SkipIf:        "true",
			PrincipalId:   `"user-1"`,
			PrincipalType: "user",
			Entitlement:   `"member"`,
		}
		grant, ok, err := f.mapGrantFromRow(t.Context(), s, resource, mapping, map[string]any{})
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, grant)
	})

	t.Run("valid mapping returns grant with correct principal", func(t *testing.T) {
		mapping := &GrantMapping{
			PrincipalId:   `"user-42"`,
			PrincipalType: "user",
			Entitlement:   `"member"`,
		}
		grant, ok, err := f.mapGrantFromRow(t.Context(), s, resource, mapping, map[string]any{})
		require.NoError(t, err)
		require.True(t, ok)
		require.NotNil(t, grant)
		require.Equal(t, "user-42", grant.GetPrincipal().GetId().GetResource())
		require.Equal(t, "user", grant.GetPrincipal().GetId().GetResourceType())
	})
}
