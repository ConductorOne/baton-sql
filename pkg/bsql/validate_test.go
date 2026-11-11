package bsql

import (
	"testing"

	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
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
		{
			name: "valid cluster scope",
			validator: &ListQuery{
				Query: "SELECT 1",
				Scope: "cluster",
			},
			expectErr: false,
		},
		{
			name: "invalid scope typo",
			validator: &ListQuery{
				Query: "SELECT 1",
				Scope: "clustr",
			},
			expectErr: true,
		},
		{
			name: "action with singular query",
			validator: &ActionConfig{
				Query: "UPDATE users SET disabled = 1 WHERE id = ?<userid>",
				Arguments: map[string]ArgumentConfig{
					"userid": {Type: "string"},
				},
			},
			expectErr: false,
		},
		{
			// Actions may define `queries` instead of `query`; validating only the
			// singular field rejected every multi-statement action outright.
			name: "action with queries",
			validator: &ActionConfig{
				Queries: []string{
					"UPDATE users SET disabled = 1 WHERE id = ?<userid>",
					"DELETE FROM user_sessions WHERE user_id = ?<userid>",
				},
				Arguments: map[string]ArgumentConfig{
					"userid": {Type: "string"},
				},
			},
			expectErr: false,
		},
		{
			name: "action with queries referencing undefined var",
			validator: &ActionConfig{
				Queries: []string{
					"UPDATE users SET disabled = 1 WHERE id = ?<userid>",
					"DELETE FROM user_sessions WHERE user_id = ?<unknown>",
				},
				Arguments: map[string]ArgumentConfig{
					"userid": {Type: "string"},
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
			name:      "resource feed with cursor strategy",
			validator: &ResourceEventFeed{Query: resourceQuery, CursorColumn: "updated_at", Pagination: &Pagination{Strategy: "cursor", PrimaryKey: "id"}},
			errSubstr: `event_feed.query.pagination.strategy must be "offset" for event feed queries; "cursor" cannot resume a timestamp-ordered query`,
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
			name: "grant feed with cursor strategy",
			validator: &GrantsEventFeed{
				ResourceId: ".user_id", ChangesQuery: grantQuery, ChangesCursorColumn: "updated_at",
				RevokesQuery: grantQuery, RevokesCursorColumn: "updated_at", Pagination: &Pagination{Strategy: "cursor", PrimaryKey: "id"},
			},
			errSubstr: `event_feed.changes_query.pagination.strategy must be "offset" for event feed queries`,
		},
		{
			name: "list query with cursor strategy",
			validator: &ListQuery{
				Query:      "SELECT id FROM users WHERE id > ?<Cursor> ORDER BY id LIMIT ?<Limit>",
				Pagination: &Pagination{Strategy: "cursor", PrimaryKey: "id"},
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
		{
			name: "grant feed with invalid resource_id CEL expression",
			validator: &GrantsEventFeed{
				ResourceId: "cols.user_id +", ChangesQuery: grantQuery, ChangesCursorColumn: "updated_at", Pagination: offset,
			},
			errSubstr: "event_feed.resource_id: invalid CEL expression",
		},
	}

	celEnv, err := bcel.NewEnv(t.Context())
	require.NoError(t, err)

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.validator.staticValidate(t.Context(), &SQLSyncer{env: celEnv})
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

func TestValidateEventFeedConfig(t *testing.T) {
	require.NoError(t, (&EventFeedConfig{DefaultLookback: "3h", Overlap: "30s"}).staticValidate(t.Context(), nil))
	require.NoError(t, (&EventFeedConfig{Overlap: "0s"}).staticValidate(t.Context(), nil))
	require.ErrorContains(t, (&EventFeedConfig{Overlap: "soon"}).staticValidate(t.Context(), nil), "invalid event_feed.overlap")
	require.ErrorContains(t, (&EventFeedConfig{Overlap: "-1m"}).staticValidate(t.Context(), nil), "must not be negative")
}

// TestValidateProvisioningIdempotency covers the shared validation_queries_signal_idempotency
// checks: validation queries are always vars-checked, the opt-in requires at least one validation
// query, and on a DDL engine it requires no_transaction. The grant and revoke helpers both run it.
func TestValidateProvisioningIdempotency(t *testing.T) {
	vars := map[string]string{"p": "principal.ID"}
	oneValidation := []string{"SELECT 1 FROM dual WHERE x = ?<p>"}

	tests := []struct {
		name    string
		engine  database.DbEngine
		pq      EntitlementProvisioningQueries
		wantErr bool
	}{
		{
			name:    "signal on without validation queries fails",
			engine:  database.Oracle,
			pq:      EntitlementProvisioningQueries{ValidationQueriesSignalIdempotency: true, NoTransaction: true},
			wantErr: true,
		},
		{
			name:    "signal on DDL engine without no_transaction fails",
			engine:  database.Oracle,
			pq:      EntitlementProvisioningQueries{ValidationQueriesSignalIdempotency: true, ValidationQueries: oneValidation},
			wantErr: true,
		},
		{
			name:    "signal on DDL engine with no_transaction and a validation query ok",
			engine:  database.Oracle,
			pq:      EntitlementProvisioningQueries{ValidationQueriesSignalIdempotency: true, NoTransaction: true, ValidationQueries: oneValidation},
			wantErr: false,
		},
		{
			name:    "signal on non-DDL engine is rejected",
			engine:  database.PostgreSQL,
			pq:      EntitlementProvisioningQueries{ValidationQueriesSignalIdempotency: true, NoTransaction: true, ValidationQueries: oneValidation},
			wantErr: true,
		},
		{
			name:    "Db2 default-on validation without no_transaction fails",
			engine:  database.DB2,
			pq:      EntitlementProvisioningQueries{ValidationQueries: oneValidation},
			wantErr: true,
		},
		{
			name:    "Db2 default-on validation with no_transaction ok",
			engine:  database.DB2,
			pq:      EntitlementProvisioningQueries{NoTransaction: true, ValidationQueries: oneValidation},
			wantErr: false,
		},
		{
			name:    "validation query with undefined var fails",
			engine:  database.Oracle,
			pq:      EntitlementProvisioningQueries{NoTransaction: true, ValidationQueries: []string{"SELECT 1 WHERE x = ?<missing>"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SQLSyncer{dbEngine: tt.engine}

			gErr := validateGrantProvisioningQueries(s, &GrantEntitlementProvisioningQueries{EntitlementProvisioningQueries: tt.pq}, vars)
			rErr := validateRevokeProvisioningQueries(s, &RevokeEntitlementProvisioningQueries{EntitlementProvisioningQueries: tt.pq}, vars)
			if tt.wantErr {
				require.Error(t, gErr)
				require.Error(t, rErr)
			} else {
				require.NoError(t, gErr)
				require.NoError(t, rErr)
			}
		})
	}
}
