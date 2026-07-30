package studio

import "testing"

func TestCompileField_SimpleRecipes(t *testing.T) {
	cases := []struct {
		name string
		fm   FieldMapping
		want string
	}{
		{"plain column", FieldMapping{Field: "id", Column: "id"}, ".id"},
		{"coerce string", FieldMapping{Field: "id", Column: "user_id", Transform: &Transform{Recipe: RecipeCoerceString}}, "string(.user_id)"},
		{"null default", FieldMapping{Field: "last_login", Column: "last_login", Transform: &Transform{Recipe: RecipeNullDefault}}, ".last_login != null ? string(.last_login) : ''"},
		{"slugify", FieldMapping{Field: "slug", Column: "role_name", Transform: &Transform{Recipe: RecipeSlugify}}, "slugify(.role_name)"},
		{"title case", FieldMapping{Field: "display_name", Column: "role_name", Transform: &Transform{Recipe: RecipeTitleCase}}, "titleCase(.role_name)"},
		{"raw cel", FieldMapping{Field: "id", Transform: &Transform{Recipe: RecipeRaw, RawCEL: ".a + .b"}}, ".a + .b"},
	}
	for _, c := range cases {
		got, err := CompileField(c.fm)
		if err != nil {
			t.Fatalf("%s: err %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestCompileTransform_CompositeAndTernary(t *testing.T) {
	comp := &Transform{Recipe: RecipeCompositeID, Args: map[string]any{
		"columns": []any{"database_name", "schema_name", "table_name"}, "sep": ".",
	}}
	got, err := CompileTransform(comp, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "string(.database_name) + '.' + string(.schema_name) + '.' + string(.table_name)"
	if got != want {
		t.Errorf("composite: got %q want %q", got, want)
	}

	st := &Transform{Recipe: RecipeStatusTernary, Args: map[string]any{
		"column": "status", "enabled": []any{"1"},
	}}
	got, err = CompileTransform(st, "status")
	if err != nil {
		t.Fatal(err)
	}
	if got != "string(.status) == '1' ? 'enabled' : 'disabled'" {
		t.Errorf("status ternary: got %q", got)
	}

	at := &Transform{Recipe: RecipeAccountTypeTernary, Args: map[string]any{"system_prefix": "_SYS"}}
	got, err = CompileTransform(at, "user_name")
	if err != nil {
		t.Fatal(err)
	}
	if got != "string(.user_name).startsWith('_SYS') ? 'system' : 'human'" {
		t.Errorf("account_type ternary: got %q", got)
	}
}
