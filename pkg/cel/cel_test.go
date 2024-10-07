package cel

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/conductorone/baton-sql/pkg/cel/functions"
)

func TestTemplateEnv_Evaluate(tt *testing.T) {
	ctx := context.Background()

	env, err := NewTemplateEnv(ctx)
	require.NoError(tt, err)

	for _, fn := range functions.GetAllFunctions() {
		for _, op := range fn.Overloads {
			for _, tc := range op.TestCases {
				tt.Run(fmt.Sprintf("%s/%s", fn.Name, op.Operator), func(t *testing.T) {
					out, err := env.Evaluate(ctx, tc.Expr, tc.Inputs)
					require.NoError(t, err)
					require.Equal(t, tc.Expected, out)
				})
			}
		}
	}
}
