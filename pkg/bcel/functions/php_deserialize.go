package functions

import (
	"sort"

	"github.com/elliotchance/phpserialize"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// PHPDeserializeStringArray deserializes a PHP serialized string into a list of sorted strings.
func PHPDeserializeStringArray(s string) ([]string, error) {
	results := make(map[any]any)
	err := phpserialize.Unmarshal([]byte(s), &results)
	if err != nil {
		return nil, err
	}

	ret := make([]string, 0, len(results))
	for k := range results {
		if kStr, ok := k.(string); ok {
			ret = append(ret, kStr)
		}
	}

	sort.Strings(ret)

	return ret, nil
}

func PHPDeserializeStringArrayFunc() *FunctionDefinition {
	return &FunctionDefinition{
		Name: "phpDeserializeStringArray",
		Overloads: []*OverloadDefinition{
			{
				Operator:   "phpDeserializeStringArray_string",
				Args:       []*types.Type{types.StringType},
				ResultType: types.NewListType(types.StringType),
				Unary: func(v ref.Val) ref.Val {
					if v.Type() != types.StringType {
						return types.NewErr("invalid argument to phpDeserializeStringArray, expected string")
					}
					input := v.Value().(string)
					result, err := PHPDeserializeStringArray(input)
					if err != nil {
						return types.NewErr("error while deserializing PHP string: %w", err)
					}
					return types.NewStringList(types.DefaultTypeAdapter, result)
				},
				TestCases: []*ExprTestCase{
					{
						Expr:     `phpDeserializeStringArray('a:1:{s:13:"administrator";b:1;}')`,
						Expected: []string{"administrator"},
					},
					{
						Expr:     `phpDeserializeStringArray('a:1:{s:13:"administrator";b:1;}')[0]`,
						Expected: `administrator`,
					},
					{
						Expr:     `phpDeserializeStringArray('a:2:{s:3:"foo";s:3:"bar";s:3:"baz";s:3:"qux";}')[0]`,
						Expected: `baz`,
					},
					{
						Expr:     `phpDeserializeStringArray('a:2:{s:3:"foo";s:3:"bar";s:3:"baz";s:3:"qux";}')[1]`,
						Expected: `foo`,
					},
				},
			},
		},
	}
}
