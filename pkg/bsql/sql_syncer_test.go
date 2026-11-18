package bsql

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

func TestSQLSyncerGet(t *testing.T) {
	newSyncer := func(t *testing.T, get *GetQuery) *SQLSyncer {
		t.Helper()
		db := newEventTestDB(t)
		_, err := db.ExecContext(t.Context(), `CREATE TABLE users (id TEXT NOT NULL, name TEXT NOT NULL)`)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), `INSERT INTO users VALUES ('u1', 'User One')`)
		require.NoError(t, err)

		s := newEventTestSyncer(t, db, ".id")
		s.resourceType = &v2.ResourceType{Id: "user"}
		s.config.List.Map.DisplayName = ".name"
		s.config.Get = get
		return s
	}

	t.Run("returns Unimplemented when no get query is configured", func(t *testing.T) {
		s := newSyncer(t, nil)
		_, _, err := s.Get(t.Context(), &v2.ResourceId{ResourceType: "user", Resource: "u1"}, nil)
		require.Error(t, err)
		require.Equal(t, codes.Unimplemented, status.Code(err))
	})

	getQuery := &GetQuery{Query: "SELECT id, name FROM users WHERE id = ?<resource_id>"}

	t.Run("returns the resource matching ?<resource_id>", func(t *testing.T) {
		s := newSyncer(t, getQuery)
		r, _, err := s.Get(t.Context(), &v2.ResourceId{ResourceType: "user", Resource: "u1"}, nil)
		require.NoError(t, err)
		require.Equal(t, "u1", r.GetId().GetResource())
		require.Equal(t, "User One", r.GetDisplayName())
	})

	t.Run("returns NotFound when no row matches", func(t *testing.T) {
		s := newSyncer(t, getQuery)
		_, _, err := s.Get(t.Context(), &v2.ResourceId{ResourceType: "user", Resource: "missing"}, nil)
		require.Equal(t, codes.NotFound, status.Code(err))
	})
}
