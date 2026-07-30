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
