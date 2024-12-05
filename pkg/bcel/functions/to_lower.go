package functions

import (
	"strings"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

func ToLower(s string) string {
	return strings.ToLower(s)
}

func ToLowerFunc() *FunctionDefinition {
	return &FunctionDefinition{
		Name: "toLower",
		Overloads: []*OverloadDefinition{
			{
				Operator:   "toLower_string",
				Args:       []*types.Type{types.StringType},
				ResultType: types.StringType,
				Unary: func(v ref.Val) ref.Val {
					if v.Type() != types.StringType {
						return types.NewErr("invalid argument to lowercase, expected string")
					}
					input := v.Value().(string)
					result := ToLower(input)
					return types.String(result)
				},
				TestCases: []*ExprTestCase{
					{
						Expr:     "toLower('hello')",
						Expected: "hello",
					},
					{
						Expr:     "toLower('')",
						Expected: "",
					},
					{
						Expr:     "toLower('Hello')",
						Expected: "hello",
					},
					{
						Expr:     "'FOO' + toLower('BAR')",
						Expected: "FOObar",
					},
					{
						Expr:     `toLower(cols["username"])`,
						Expected: "alice",
						Inputs: map[string]interface{}{
							"cols": map[string]interface{}{
								"username": "ALICE",
							},
						},
					},
				},
			},
		},
	}
}
