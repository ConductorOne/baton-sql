package studio

import (
	"context"
	"testing"

	"github.com/conductorone/baton-sql/pkg/bcel"
)

func TestPreviewField_Composite(t *testing.T) {
	ctx := context.Background()
	env, err := bcel.NewEnv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fm := FieldMapping{Field: "display_name", Transform: &Transform{Recipe: RecipeCompositeID,
		Args: map[string]any{"columns": []any{"first_name", "last_name"}, "sep": " "}}}
	got, err := PreviewField(ctx, env, fm, map[string]any{"first_name": "Ada", "last_name": "Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Ada Lovelace" {
		t.Fatalf("preview: got %q want %q", got, "Ada Lovelace")
	}
}
