package studio

import (
	"context"

	"github.com/conductorone/baton-sql/pkg/bcel"
)

// PreviewField compiles fm to CEL and evaluates it against a single sample
// DB row, returning the string result the UI shows as the "→ value" preview.
func PreviewField(ctx context.Context, env *bcel.Env, fm FieldMapping, row map[string]any) (string, error) {
	expr, err := CompileField(fm)
	if err != nil {
		return "", err
	}
	inputs := env.SyncInputs(row)
	return env.EvaluateString(ctx, expr, inputs)
}
