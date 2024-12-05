package functions

import (
	"regexp"
	"strings"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

var slugifyAlphaNumRegex = regexp.MustCompile(`[^a-z0-9-]+`)
var slugifyHyphenRegex = regexp.MustCompile(`-+`)

func Slugify(s string) string {
	slug := strings.ToLower(s)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = slugifyAlphaNumRegex.ReplaceAllString(slug, "")
	slug = slugifyHyphenRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	return slug
}

func SlugifyFunc() *FunctionDefinition {
	return &FunctionDefinition{
		Name: "slugify",
		Overloads: []*OverloadDefinition{
			{
				Operator:   "slugify_string",
				Args:       []*types.Type{types.StringType},
				ResultType: types.StringType,
				Unary: func(v ref.Val) ref.Val {
					if v.Type() != types.StringType {
						return types.NewErr("invalid argument to slugify, expected string")
					}
					input := v.Value().(string)
					result := Slugify(input)
					return types.String(result)
				},
				TestCases: []*ExprTestCase{
					{
						Expr:     "slugify('Hello, World!')",
						Expected: "hello-world",
					},
					{
						Expr:     "slugify('GoLang_is Awesome')",
						Expected: "golang-is-awesome",
					},
					{
						Expr:     "slugify(' This--is !a Test ')",
						Expected: "this-is-a-test",
					},
					{
						Expr:     "slugify('Complex_Example_42')",
						Expected: "complex-example-42",
					},
					{
						Expr:     "slugify('Multiple   Spaces')",
						Expected: "multiple-spaces",
					},
					{
						Expr:     "slugify('____leading_and_trailing____')",
						Expected: "leading-and-trailing",
					},
					{
						Expr:     "slugify('special@#$_characters!!')",
						Expected: "special-characters",
					},
					{
						Expr:     "slugify('MiXeD CaSe')",
						Expected: "mixed-case",
					},
					{
						Expr:     "slugify('123 Numbers')",
						Expected: "123-numbers",
					},
					{
						Expr:     "slugify('Already--slugified')",
						Expected: "already-slugified",
					},
				},
			},
		},
	}
}
