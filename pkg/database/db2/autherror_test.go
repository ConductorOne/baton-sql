//go:build db2

package db2

import (
	"fmt"
	"testing"

	"github.com/ibmdb/go_ibm_db"
	"github.com/stretchr/testify/require"
)

func TestIsAuthError(t *testing.T) {
	badCreds := &go_ibm_db.Error{Diag: []go_ibm_db.DiagRecord{{State: "28000"}}}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"class 28 bad credentials", badCreds, true},
		{"class 28 wrapped", fmt.Errorf("connect: %w", badCreds), true},
		{"non-auth sqlstate", &go_ibm_db.Error{Diag: []go_ibm_db.DiagRecord{{State: "42501"}}}, false},
		{"no diag records", &go_ibm_db.Error{}, false},
		{"unrelated error", fmt.Errorf("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsAuthError(tt.err))
		})
	}
}
