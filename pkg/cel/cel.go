package cel

import (
	"context"

	"github.com/google/cel-go/cel"

	"github.com/conductorone/baton-sql/pkg/cel/functions"
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
	ast, issues := t.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return "", issues.Err()
	}

	prg, err := t.celEnv.Program(ast)
	if err != nil {
		return "", err
	}

	out, _, err := prg.ContextEval(ctx, inputs)
	if err != nil {
		return "", err
	}
	return out.Value(), nil
}
