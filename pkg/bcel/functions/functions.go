package functions

import (
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/functions"
)

var (
	stringListType = reflect.TypeOf([]string{})
)

type FunctionDefinition struct {
	Name      string
	Overloads []*OverloadDefinition
}

type OverloadDefinition struct {
	Operator   string
	Args       []*cel.Type
	ResultType *cel.Type
	Unary      functions.UnaryOp
	Binary     functions.BinaryOp
	TestCases  []*ExprTestCase
}

type ExprTestCase struct {
	Expr     string
	Expected interface{}
	Inputs   map[string]any
}

func (fd *FunctionDefinition) GetOptions() []cel.EnvOption {
	var opts []cel.EnvOption

	for _, overload := range fd.Overloads {
		var fn cel.EnvOption
		switch {
		case overload.Unary != nil:
			fn = cel.Function(fd.Name,
				cel.Overload(
					overload.Operator,
					overload.Args,
					overload.ResultType,
					cel.UnaryBinding(overload.Unary),
				),
			)
		case overload.Binary != nil:
			fn = cel.Function(fd.Name,
				cel.Overload(
					overload.Operator,
					overload.Args,
					overload.ResultType,
					cel.BinaryBinding(overload.Binary),
				),
			)
		default:
			panic("invalid overload definition")
		}
		opts = append(opts, fn)
	}

	return opts
}

func GetAllFunctions() []*FunctionDefinition {
	return []*FunctionDefinition{
		ToUpperFunc(),
		PHPDeserializeStringArrayFunc(),
		PHPSerializeStringArrayFunc(),
		TitleCaseFunc(),
		ToLowerFunc(),
		SlugifyFunc(),
	}
}

func GetAllOptions() []cel.EnvOption {
	var opts []cel.EnvOption

	for _, fn := range GetAllFunctions() {
		opts = append(opts, fn.GetOptions()...)
	}

	return opts
}
