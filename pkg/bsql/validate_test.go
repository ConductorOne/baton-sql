package bsql

import (
	"testing"

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
			name:    "signal on non-DDL engine does not require no_transaction",
			engine:  database.PostgreSQL,
			pq:      EntitlementProvisioningQueries{ValidationQueriesSignalIdempotency: true, ValidationQueries: oneValidation},
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
