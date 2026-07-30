package studio

import (
	"strings"
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

// TestImport_MultiRowGrantMapImportsAllRows is the inverse of the old
// loud-error test: a grants query whose map: has more than one row now imports
// to one GrantMapping per row WITHOUT error. Reconstructing only row [0] would
// be silent data loss — GrantSpec.Mappings models every row, so redshift-style
// configs (one grants query fanning out over several principal_type/entitlement
// pairs) import fully.
func TestImport_MultiRowGrantMapImportsAllRows(t *testing.T) {
	yamlIn := []byte(`
app_name: Test App
resource_types:
    roles:
        name: Roles
        list:
            query: SELECT role_id FROM roles
            map:
                id: .role_id
                display_name: .role_id
        static_entitlements:
            - id: member
              display_name: Member
        grants:
            - query: SELECT user_id, identity_type FROM role_members WHERE role_id = ?<rid>
              vars:
                rid: resource.ID
              map:
                - skip_if: ".identity_type != 'user'"
                  principal_id: .user_id
                  principal_type: user
                  entitlement_id: member
                - skip_if: ".identity_type != 'group'"
                  principal_id: .user_id
                  principal_type: group
                  entitlement_id: member
`)
	spec, err := SpecFromYAML(yamlIn)
	if err != nil {
		t.Fatalf("expected a 2-row grant map to import without error, got: %v", err)
	}
	rt := spec.ResourceTypes[0]
	if len(rt.Grants) != 1 {
		t.Fatalf("expected 1 grants query, got %d", len(rt.Grants))
	}
	g := rt.Grants[0]
	if g.ResourceVar != "rid" {
		t.Fatalf("expected resource_var rid, got %q", g.ResourceVar)
	}
	if len(g.Mappings) != 2 {
		t.Fatalf("expected 2 grant mappings, got %d: %+v", len(g.Mappings), g.Mappings)
	}
	if g.Mappings[0].PrincipalType != "user" || g.Mappings[1].PrincipalType != "group" {
		t.Fatalf("mapping types/order wrong: %q, %q", g.Mappings[0].PrincipalType, g.Mappings[1].PrincipalType)
	}
	if g.Mappings[0].PrincipalID.Column != "user_id" {
		t.Fatalf("row 0 principal_id column = %q, want user_id", g.Mappings[0].PrincipalID.Column)
	}
	if g.Mappings[0].SkipIf == nil || g.Mappings[0].SkipIf.Transform == nil ||
		g.Mappings[0].SkipIf.Transform.RawCEL != ".identity_type != 'user'" {
		t.Fatalf("row 0 skip_if not reconstructed: %+v", g.Mappings[0].SkipIf)
	}
	if g.Mappings[1].Entitlement != "member" {
		t.Fatalf("row 1 entitlement = %q, want member", g.Mappings[1].Entitlement)
	}
}

// TestImport_MultiRowGrantRoundTrip is the core proof that redshift-style
// multi-row grant configs round-trip byte-identically: build a Spec whose one
// grants query fans out to 3 GrantMappings (distinct principal_types and
// skip_ifs), Generate -> SpecFromYAML -> Generate, and assert the two YAMLs are
// identical AND the imported spec still carries all 3 mappings.
func TestImport_MultiRowGrantRoundTrip(t *testing.T) {
	spec := &Spec{
		AppName: "Warehouse",
		ResourceTypes: []ResourceTypeSpec{
			{ID: "users", Name: "Users", Trait: "user",
				List:         ListSpec{Query: "SELECT id FROM u", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "roles", Name: "Roles", Trait: "role",
				List:         ListSpec{Query: "SELECT id FROM r", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "groups", Name: "Groups", Trait: "group",
				List:         ListSpec{Query: "SELECT id FROM g", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "tables", Name: "Tables", Trait: "role",
				List:         ListSpec{Query: "SELECT id FROM t", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "select", DisplayName: "Select", Purpose: "permission"}}},
				Grants: []GrantSpec{{
					Query:       "SELECT grantee, kind FROM grants WHERE table_id = ?<tid>",
					ResourceVar: "tid",
					Mappings: []GrantMapping{
						{PrincipalID: FieldMapping{Field: "principal_id", Column: "grantee"}, PrincipalType: "users", Entitlement: "select",
							SkipIf: &FieldMapping{Field: "skip_if", Transform: &Transform{Recipe: RecipeRaw, RawCEL: ".kind != 'user'"}}},
						{PrincipalID: FieldMapping{Field: "principal_id", Column: "grantee"}, PrincipalType: "roles", Entitlement: "select",
							SkipIf: &FieldMapping{Field: "skip_if", Transform: &Transform{Recipe: RecipeRaw, RawCEL: ".kind != 'role'"}}},
						{PrincipalID: FieldMapping{Field: "principal_id", Column: "grantee"}, PrincipalType: "groups", Entitlement: "select",
							SkipIf: &FieldMapping{Field: "skip_if", Transform: &Transform{Recipe: RecipeRaw, RawCEL: ".kind != 'group'"}}},
					},
				}},
			},
		},
	}

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
		t.Fatalf("multi-row grant round-trip mismatch:\n--- original ---\n%s\n--- reconstructed ---\n%s", yaml1, yaml2)
	}

	var tables *ResourceTypeSpec
	for i := range spec2.ResourceTypes {
		if spec2.ResourceTypes[i].ID == "tables" {
			tables = &spec2.ResourceTypes[i]
		}
	}
	if tables == nil {
		t.Fatal("imported spec missing the tables resource type")
	}
	if len(tables.Grants) != 1 {
		t.Fatalf("expected 1 grants query, got %d", len(tables.Grants))
	}
	if len(tables.Grants[0].Mappings) != 3 {
		t.Fatalf("expected 3 grant mappings after import, got %d", len(tables.Grants[0].Mappings))
	}
}

// TestImport_MultiRowEntitlementsMapErrorsLoudly is the same CRITICAL fix,
// for dynamic entitlements: EntitlementsSpec also models exactly one
// entitlement mapping per query.
func TestImport_MultiRowEntitlementsMapErrorsLoudly(t *testing.T) {
	yamlIn := []byte(`
app_name: Test App
resource_types:
    tables:
        name: Tables
        list:
            query: SELECT id FROM tables
            map:
                id: .id
                display_name: .id
        entitlements:
            query: SELECT priv_id, priv_name FROM privs
            map:
                - id: .priv_id
                  display_name: .priv_name
                  slug: .priv_id
                - id: .priv_id
                  display_name: .priv_name
                  slug: .priv_id
`)
	spec, err := SpecFromYAML(yamlIn)
	if err == nil {
		t.Fatalf("expected an error for a 2-row entitlements map, got a spec instead: %+v", spec)
	}
	if !strings.Contains(err.Error(), `"tables"`) {
		t.Fatalf("expected error to name the resource type %q, got: %v", "tables", err)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Fatalf("expected error to mention the row count (2), got: %v", err)
	}
}

// TestImport_DynamicEntitlementRoundTrip proves a query-mode (dynamic)
// entitlement - including an explicit slug and grantable_to - round-trips
// byte-identically, the same property TestImport_RoundTripStability proves
// for the golden fixture's static-entitlements/none-mode resource types
// (which never exercised the query-mode entitlements path).
func TestImport_DynamicEntitlementRoundTrip(t *testing.T) {
	spec := &Spec{
		AppName: "Test App",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "roles", Name: "Roles", Trait: "role",
			List: ListSpec{
				Query: "SELECT role_id, role_name FROM roles",
				Fields: []FieldMapping{
					{Field: "id", Column: "role_id"},
					{Field: "display_name", Column: "role_name"},
				},
			},
			Entitlements: EntitlementsSpec{
				Mode:  "query",
				Query: "SELECT perm_id, perm_name, perm_type FROM role_perms",
				Fields: []FieldMapping{
					{Field: "id", Column: "perm_id"},
					{Field: "display_name", Column: "perm_name"},
					{Field: "purpose", Column: "perm_type"},
					{Field: "slug", Column: "perm_id"},
				},
				GrantableTo: []string{"roles"},
			},
		}},
	}

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
		t.Fatalf("round-trip YAML mismatch:\n--- original ---\n%s\n--- reconstructed ---\n%s", yaml1, yaml2)
	}

	rt := spec2.ResourceTypes[0]
	if rt.Entitlements.Mode != "query" {
		t.Fatalf("expected mode %q, got %q", "query", rt.Entitlements.Mode)
	}
	slugField := findField(t, rt.Entitlements.Fields, "slug")
	if slugField.Column != "perm_id" {
		t.Fatalf("expected slug column %q, got %q", "perm_id", slugField.Column)
	}
	if len(rt.Entitlements.GrantableTo) != 1 || rt.Entitlements.GrantableTo[0] != "roles" {
		t.Fatalf("expected grantable_to [roles], got %+v", rt.Entitlements.GrantableTo)
	}
}

// TestImport_GroupTraitProfileRoundTrip proves a group-trait profile field
// reverses correctly and round-trips byte-identically - the golden fixture
// only exercises a user-trait profile field, and a role trait with no
// profile at all, so this covers the Group branch of traitFieldMappings that
// was otherwise untested.
func TestImport_GroupTraitProfileRoundTrip(t *testing.T) {
	spec := &Spec{
		AppName: "Test App",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "groups", Name: "Groups", Trait: "group",
			List: ListSpec{
				Query: "SELECT group_id, group_name, group_type FROM groups",
				Fields: []FieldMapping{
					{Field: "id", Column: "group_id"},
					{Field: "display_name", Column: "group_name"},
					{Field: "profile.type", Column: "group_type"},
				},
			},
			Entitlements: EntitlementsSpec{Mode: "none"},
		}},
	}

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
		t.Fatalf("round-trip YAML mismatch:\n--- original ---\n%s\n--- reconstructed ---\n%s", yaml1, yaml2)
	}

	rt := spec2.ResourceTypes[0]
	if rt.Trait != "group" {
		t.Fatalf("expected trait %q, got %q", "group", rt.Trait)
	}
	typeField := findField(t, rt.List.Fields, "profile.type")
	if typeField.Column != "group_type" {
		t.Fatalf("expected column %q, got %q", "group_type", typeField.Column)
	}
}
