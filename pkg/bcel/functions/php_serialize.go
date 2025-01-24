package functions

import (
	"github.com/elliotchance/phpserialize"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// PHPSerializeStringArray serializes a slice of strings into a PHP serialized string.
func PHPSerializeStringArray(input []string) (string, error) {
	// Create a map where each string is a key with value true
	// This matches PHP's array serialization format
	data := make(map[interface{}]interface{}, len(input))
	for _, str := range input {
		data[str] = true
	}

	// Serialize the map using phpserialize
	serialized, err := phpserialize.Marshal(data, nil)
	if err != nil {
		return "", err
	}

	return string(serialized), nil
}

func PHPSerializeStringArrayFunc() *FunctionDefinition {
	return &FunctionDefinition{
		Name: "phpSerializeStringArray",
		Overloads: []*OverloadDefinition{
			{
				Operator:   "phpSerializeStringArray_list_string",
				Args:       []*types.Type{types.NewListType(types.StringType)},
				ResultType: types.StringType,
				Unary: func(v ref.Val) ref.Val {
					if v.Type() != types.ListType {
						return types.NewErr("invalid argument to phpSerializeStringArray, expected string list 1")
					}

					input, err := v.ConvertToNative(stringListType)
					if err != nil {
						return types.NewErr("invalid argument to phpSerializeStringArray, expected string list 2")
					}

					result, err := PHPSerializeStringArray(input.([]string))
					if err != nil {
						return types.NewErr("error while serializing string array: %w", err)
					}
					return types.String(result)
				},
				TestCases: []*ExprTestCase{
					{
						Expr:     `phpSerializeStringArray(['administrator'])`,
						Expected: `a:1:{s:13:"administrator";b:1;}`,
					},
					{
						Expr:     `phpSerializeStringArray(['subscriber'])`,
						Expected: `a:1:{s:10:"subscriber";b:1;}`,
					},
				},
			},
		},
	}
}
