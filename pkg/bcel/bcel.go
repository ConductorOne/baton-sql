package bcel

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"

	"github.com/conductorone/baton-sql/pkg/bcel/functions"
)

type Env struct {
	celEnv *cel.Env
}

func NewEnv(ctx context.Context) (*Env, error) {
	var celOpts []cel.EnvOption

	// CEL variables
	celOpts = append(celOpts,
		cel.Variable("cols", cel.MapType(cel.StringType, cel.AnyType)),
	)

	// CEL functions
	celOpts = append(celOpts, functions.GetAllOptions()...)

	celEnv, err := cel.NewEnv(celOpts...)
	if err != nil {
		return nil, err
	}
	return &Env{
		celEnv: celEnv,
	}, nil
}

func (t *Env) Evaluate(ctx context.Context, expr string, inputs map[string]any) (any, error) {
	expr = preprocessExpressions(expr)

	ast, issues := t.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return "", issues.Err()
	}

	prg, err := t.celEnv.Program(ast)
	if err != nil {
		return "", err
	}

	// Make sure that our input always has the 'cols' member
	if _, ok := inputs["cols"]; !ok {
		inputs["cols"] = make(map[string]any)
	}

	out, _, err := prg.ContextEval(ctx, inputs)
	if err != nil {
		return "", err
	}

	return out.Value(), nil
}

func (t *Env) EvaluateString(ctx context.Context, expr string, inputs map[string]any) (string, error) {
	out, err := t.Evaluate(ctx, expr, inputs)
	if err != nil {
		return "", err
	}

	switch ret := out.(type) {
	case string:
		return ret, nil
	default:
		return fmt.Sprintf("%s", out), nil
	}
}

func (t *Env) BaseInputs(rowMap map[string]any) (map[string]any, error) {
	ret := make(map[string]any)
	ret["cols"] = rowMap

	return ret, nil
}
