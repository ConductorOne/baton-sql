package functions

import (
	"strings"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

func ToUpper(s string) string {
	return strings.ToUpper(s)
}

func ToUpperFunc() *FunctionDefinition {
	return &FunctionDefinition{
		Name: "toUpper",
		Overloads: []*OverloadDefinition{
			{
				Operator:   "toUpper_string",
				Args:       []*types.Type{types.StringType},
				ResultType: types.StringType,
				Unary: func(v ref.Val) ref.Val {
					if v.Type() != types.StringType {
						return types.NewErr("invalid argument to uppercase, expected string")
					}
					input := v.Value().(string)
					result := ToUpper(input)
					return types.String(result)
				},
				TestCases: []*ExprTestCase{
					{
						Expr:     "toUpper('hello')",
						Expected: "HELLO",
					},
					{
						Expr:     "toUpper('')",
						Expected: "",
					},
					{
						Expr:     "toUpper('Hello')",
						Expected: "HELLO",
					},
					{
						Expr:     "'foo' + toUpper('bar')",
						Expected: "fooBAR",
					},
					{
						Expr:     `toUpper(cols["username"])`,
						Expected: "ALICE",
						Inputs: map[string]interface{}{
							"cols": map[string]interface{}{
								"username": "Alice",
							},
						},
					},
				},
			},
		},
	}
}
