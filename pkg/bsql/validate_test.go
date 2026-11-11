package bsql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tcases := []struct {
		name      string
		validator staticValidator
		expectErr bool
	}{
		{
			name: "valid list query",
			validator: &ListQuery{
				Query: "SELECT * FROM users WHERE id = ?<userid> LIMIT ?<Limit> OFFSET ?<Offset>",
				Vars: map[string]string{
					"userid": "string",
				},
			},
			expectErr: false,
		},
		{
			name: "invalid list query",
			validator: &ListQuery{
				Query: "SELECT * FROM users WHERE id = ?<unknown> LIMIT ?<Limit> OFFSET ?<Offset>",
				Vars: map[string]string{
					"userid": "string",
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

			syncer := &SQLSyncer{}

			err := tc.validator.staticValidate(ctx, syncer)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEventFeedPagination(t *testing.T) {
	offset := &Pagination{Strategy: "offset", PrimaryKey: "id"}
	resourceQuery := "SELECT id, updated_at FROM t WHERE updated_at >= ?<since> ORDER BY updated_at, id LIMIT ?<Limit> OFFSET ?<Offset>"
	grantQuery := "SELECT id, user_id, updated_at FROM g WHERE updated_at >= ?<since> ORDER BY updated_at, id LIMIT ?<Limit> OFFSET ?<Offset>"

	tcases := []struct {
		name      string
		validator staticValidator
		errSubstr string
	}{
		{
			name:      "resource feed with pagination",
			validator: &ResourceEventFeed{Query: resourceQuery, CursorColumn: "updated_at", Pagination: offset},
		},
		{
			name:      "resource feed without pagination",
			validator: &ResourceEventFeed{Query: resourceQuery, CursorColumn: "updated_at"},
			errSubstr: "event_feed.query.pagination is required",
		},
		{
			name:      "resource feed without primary key",
			validator: &ResourceEventFeed{Query: resourceQuery, CursorColumn: "updated_at", Pagination: &Pagination{Strategy: "offset"}},
			errSubstr: "primary_key is required",
		},
		{
			name:      "resource feed with unknown strategy",
			validator: &ResourceEventFeed{Query: resourceQuery, CursorColumn: "updated_at", Pagination: &Pagination{Strategy: "page", PrimaryKey: "id"}},
			errSubstr: "strategy must be",
		},
		{
			name:      "resource feed without limit placeholder",
			validator: &ResourceEventFeed{Query: "SELECT id, updated_at FROM t WHERE updated_at >= ?<since>", CursorColumn: "updated_at", Pagination: offset},
			errSubstr: "must contain ?<Limit>",
		},
		{
			name: "grant feed with pagination",
			validator: &GrantsEventFeed{
				ResourceId: ".user_id", ChangesQuery: grantQuery, ChangesCursorColumn: "updated_at",
				RevokesQuery: grantQuery, RevokesCursorColumn: "updated_at", Pagination: offset,
			},
		},
		{
			name:      "grant feed without pagination",
			validator: &GrantsEventFeed{ResourceId: ".user_id", ChangesQuery: grantQuery, ChangesCursorColumn: "updated_at"},
			errSubstr: "event_feed.changes_query.pagination is required",
		},
		{
			name: "grant feed revokes query without limit placeholder",
			validator: &GrantsEventFeed{
				ResourceId: ".user_id", ChangesQuery: grantQuery, ChangesCursorColumn: "updated_at",
				RevokesQuery:        "SELECT id, user_id, deleted_at FROM g WHERE deleted_at >= ?<since>",
				RevokesCursorColumn: "deleted_at", Pagination: offset,
			},
			errSubstr: "event_feed.revokes_query must contain ?<Limit>",
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.validator.staticValidate(t.Context(), &SQLSyncer{})
			if tc.errSubstr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.errSubstr)
		})
	}
}

func TestParseQueryOptsPaginationVarsWithoutPagination(t *testing.T) {
	s := &SQLSyncer{}
	for _, q := range []string{
		"SELECT * FROM t LIMIT ?<Limit>",
		"SELECT * FROM t OFFSET ?<Offset>",
		"SELECT * FROM t WHERE id > ?<Cursor>",
	} {
		_, _, _, err := s.parseQueryOpts(nil, q, nil)
		require.ErrorContains(t, err, "no pagination is configured", q)
	}
}
