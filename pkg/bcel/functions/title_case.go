package functions

import (
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func TitleCase(s string) string {
	caser := cases.Title(language.English)
	return caser.String(s)
}

func TitleCaseFunc() *FunctionDefinition {
	return &FunctionDefinition{
		Name: "titleCase",
		Overloads: []*OverloadDefinition{
			{
				Operator:   "titleCase_string",
				Args:       []*types.Type{types.StringType},
				ResultType: types.StringType,
				Unary: func(v ref.Val) ref.Val {
					if v.Type() != types.StringType {
						return types.NewErr("invalid argument to title case, expected string")
					}
					input := v.Value().(string)
					result := TitleCase(input)
					return types.String(result)
				},
				TestCases: []*ExprTestCase{
					{
						Expr:     "titleCase('hello')",
						Expected: "Hello",
					},
					{
						Expr:     "titleCase('')",
						Expected: "",
					},
					{
						Expr:     "titleCase('Hello')",
						Expected: "Hello",
					},
					{
						Expr:     "'foo' + titleCase('bar')",
						Expected: "fooBar",
					},
					{
						Expr:     "titleCase('foo bar qux baz')",
						Expected: "Foo Bar Qux Baz",
					},
					{
						Expr:     `titleCase(cols["username"])`,
						Expected: "Alice",
						Inputs: map[string]interface{}{
							"cols": map[string]interface{}{
								"username": "alice",
							},
						},
					},
				},
			},
		},
	}
}
