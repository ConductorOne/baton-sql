package studio

import (
	"testing"
)

// TestImport_RoundTripStability is the key proof that import faithfully
// reconstructs a Spec that regenerates the same config: Generate the golden
// finance fixture to YAML, reverse that YAML back into a Spec via
// SpecFromYAML, Generate the reconstructed Spec again, and assert the two
// YAML outputs are byte-identical. This exercises a composite_id transform, a
// status_ternary transform, a profile field, a manager_id field, static
// entitlements with grantable_to, and a resource-scoped grant - all of it
// through the "none" mode, "static" mode, plain column picks, and raw-CEL
// round-tripping at once.
func TestImport_RoundTripStability(t *testing.T) {
	spec := goldenFinanceSpec(t)

	yaml1, err := Generate(spec)
	if err != nil {
		t.Fatalf("Generate(original): %v", err)
	}

	spec2, err := SpecFromYAML(yaml1)
	if err != nil {
		t.Fatalf("SpecFromYAML: %v", err)
	}

	yaml2, err := Generate(spec2)
	if err != nil {
		t.Fatalf("Generate(reconstructed): %v", err)
	}

	if string(yaml1) != string(yaml2) {
		t.Fatalf("round-trip YAML mismatch:\n--- generated from original spec ---\n%s\n--- generated from imported spec ---\n%s", yaml1, yaml2)
	}
}

func findField(t *testing.T, fields []FieldMapping, field string) FieldMapping {
	t.Helper()
	for _, fm := range fields {
		if fm.Field == field {
			return fm
		}
	}
	t.Fatalf("field %q not found in %+v", field, fields)
	return FieldMapping{}
}

// TestImport_PlainColumnBecomesColumnPick proves a simple ".<column>" CEL
// mapping reverses into a clean column pick (no transform), not a raw-CEL
// escape hatch.
func TestImport_PlainColumnBecomesColumnPick(t *testing.T) {
	yamlIn := []byte(`
app_name: Test App
resource_types:
    users:
        name: Users
        list:
            query: SELECT id, name FROM users
            map:
                id: .id
                display_name: .name
        skip_entitlements_and_grants: true
`)
	spec, err := SpecFromYAML(yamlIn)
	if err != nil {
		t.Fatalf("SpecFromYAML: %v", err)
	}
	if len(spec.ResourceTypes) != 1 {
		t.Fatalf("expected 1 resource type, got %d", len(spec.ResourceTypes))
	}
	rt := spec.ResourceTypes[0]
	if rt.ID != "users" {
		t.Fatalf("expected id %q, got %q", "users", rt.ID)
	}

	idField := findField(t, rt.List.Fields, "id")
	if idField.Column != "id" {
		t.Fatalf("expected column %q, got %q", "id", idField.Column)
	}
	if idField.Transform != nil {
		t.Fatalf("expected no transform for a plain column pick, got %+v", idField.Transform)
	}

	nameField := findField(t, rt.List.Fields, "display_name")
	if nameField.Column != "name" {
		t.Fatalf("expected column %q, got %q", "name", nameField.Column)
	}
	if nameField.Transform != nil {
		t.Fatalf("expected no transform for a plain column pick, got %+v", nameField.Transform)
	}
}

// TestImport_CompositeCELBecomesRawTransform proves that richer CEL (here, a
// composite_id-shaped concatenation) reverses into an editable raw-CEL
// transform rather than an attempt to detect the recipe that produced it.
func TestImport_CompositeCELBecomesRawTransform(t *testing.T) {
	yamlIn := []byte(`
app_name: Test App
resource_types:
    users:
        name: Users
        list:
            query: SELECT id, first_name, last_name FROM users
            map:
                id: .id
                display_name: string(.first_name) + ' ' + string(.last_name)
        skip_entitlements_and_grants: true
`)
	spec, err := SpecFromYAML(yamlIn)
	if err != nil {
		t.Fatalf("SpecFromYAML: %v", err)
	}
	rt := spec.ResourceTypes[0]

	nameField := findField(t, rt.List.Fields, "display_name")
	if nameField.Column != "" {
		t.Fatalf("expected empty column for a raw-CEL mapping, got %q", nameField.Column)
	}
	if nameField.Transform == nil {
		t.Fatalf("expected a transform for composite CEL, got none")
	}
	if nameField.Transform.Recipe != RecipeRaw {
		t.Fatalf("expected recipe %q, got %q", RecipeRaw, nameField.Transform.Recipe)
	}
	want := "string(.first_name) + ' ' + string(.last_name)"
	if nameField.Transform.RawCEL != want {
		t.Fatalf("expected raw_cel %q, got %q", want, nameField.Transform.RawCEL)
	}
}

func TestImport_InvalidYAMLReturnsError(t *testing.T) {
	if _, err := SpecFromYAML([]byte("{not: [valid")); err == nil {
		t.Fatal("expected an error for malformed YAML, got nil")
	}
}
