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
