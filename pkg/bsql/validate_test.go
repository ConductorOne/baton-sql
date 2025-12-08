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

			err := tc.validator.StaticValidate(ctx, syncer)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
