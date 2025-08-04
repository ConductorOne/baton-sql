package bcel

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/conductorone/baton-sql/pkg/bcel/functions"
)

func TestTemplateEnv_Evaluate(tt *testing.T) {
	ctx := tt.Context()

	for _, fn := range functions.GetAllFunctions() {
		for _, op := range fn.Overloads {
			for _, tc := range op.TestCases {
				tt.Run(fmt.Sprintf("%s/%s", fn.Name, op.Operator), func(t *testing.T) {
					if tc.Inputs == nil {
						tc.Inputs = map[string]interface{}{}
					}
					env, err := NewEnv(ctx)
					require.NoError(t, err)
					out, err := env.Evaluate(ctx, tc.Expr, tc.Inputs)
					require.NoError(t, err)
					require.Equal(t, tc.Expected, out)
				})
			}
		}
	}
}
