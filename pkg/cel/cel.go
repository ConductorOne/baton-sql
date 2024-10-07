package cel

import (
	"context"

	"github.com/google/cel-go/cel"

	"github.com/conductorone/baton-sql/pkg/cel/functions"
)

type TemplateEnv struct {
	env *cel.Env
}

func NewTemplateEnv(ctx context.Context) (*TemplateEnv, error) {
	var celOpts []cel.EnvOption

	// CEL variables
	celOpts = append(celOpts,
		cel.Variable("input", cel.StringType),
	)

	// CEL functions
	celOpts = append(celOpts, functions.GetAllOptions()...)

	celEnv, err := cel.NewEnv(celOpts...)
	if err != nil {
		return nil, err
	}
	return &TemplateEnv{
		env: celEnv,
	}, nil
}

func (t *TemplateEnv) Evaluate(ctx context.Context, expr string, input map[string]any) (any, error) {
	ast, issues := t.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return "", issues.Err()
	}

	prg, err := t.env.Program(ast)
	if err != nil {
		return "", err
	}

	out, _, err := prg.ContextEval(ctx, input)
	if err != nil {
		return "", err
	}
	return out.Value(), nil
}
