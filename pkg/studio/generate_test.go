package studio

import (
	"reflect"
	"strings"
	"testing"

	"github.com/conductorone/baton-sql/pkg/bsql"
)

func TestGenerate_UsersListParsesAndMapsTraits(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance", User: "svc", Password: "pw"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{
				Query: "SELECT id, email, first_name, last_name, manager_id FROM employees",
				Fields: []FieldMapping{
					{Field: "id", Column: "id"},
					{Field: "display_name", Transform: &Transform{Recipe: RecipeCompositeID, Args: map[string]any{"columns": []any{"first_name", "last_name"}, "sep": " "}}},
					{Field: "emails", Column: "email"},
					{Field: "manager_id", Column: "manager_id"},
				},
			},
			Entitlements: EntitlementsSpec{Mode: "none"},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 1. Must parse via baton-sql's own parser.
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("bsql.Parse rejected generated yaml: %v\n---\n%s", err, out)
	}
	s := string(out)
	// 2. Trap: no no-op keys.
	if strings.Contains(s, "mfa_enabled") || strings.Contains(s, "sso_enabled") {
		t.Errorf("generated no-op key; yaml:\n%s", s)
	}
	// 3. Trap: manager present => profile non-empty.
	if !strings.Contains(s, "profile:") {
		t.Errorf("manager mapped but no profile block emitted; yaml:\n%s", s)
	}
	// 4. Trap: no E&G => skip flag.
	if !strings.Contains(s, "skip_entitlements_and_grants: true") {
		t.Errorf("expected skip_entitlements_and_grants for E&G-less type; yaml:\n%s", s)
	}
}

// TestGenerate_ManagerIDFromDifferentColumn_ProfileUsesRealCEL guards against a
// regression where the manager->profile fallback hardcoded the literal CEL
// ".manager_id" instead of the actual computed expression for whatever column
// manager_id was mapped from. Here manager_id comes from column "mgr_id", so the
// fallback profile entry must reference ".mgr_id", never ".manager_id".
func TestGenerate_ManagerIDFromDifferentColumn_ProfileUsesRealCEL(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{
				Query: "SELECT id, email, mgr_id FROM employees",
				Fields: []FieldMapping{
					{Field: "id", Column: "id"},
					{Field: "emails", Column: "email"},
					{Field: "manager_id", Column: "mgr_id"},
				},
			},
			Entitlements: EntitlementsSpec{Mode: "none"},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("bsql.Parse rejected generated yaml: %v\n---\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, ".mgr_id") {
		t.Errorf("expected profile fallback to reference the real column .mgr_id; yaml:\n%s", s)
	}
	if strings.Contains(s, ".manager_id") {
		t.Errorf("bogus hardcoded .manager_id leaked into output when column was mgr_id; yaml:\n%s", s)
	}
}

// TestGenerate_EmployeeIDsEmitted guards against employee_ids (a canonical
// FieldMapping field the Studio web UI's List-tab mapping widget offers for
// the user trait, mirroring bsql.UserTraitMapping.EmployeeIDs) being silently
// dropped by the generator's field switch, the way an unrecognized field
// falls through to profileKey() and is discarded when it doesn't match
// "profile.*".
func TestGenerate_EmployeeIDsEmitted(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{
				Query: "SELECT id, email, emp_id FROM employees",
				Fields: []FieldMapping{
					{Field: "id", Column: "id"},
					{Field: "emails", Column: "email"},
					{Field: "employee_ids", Column: "emp_id"},
				},
			},
			Entitlements: EntitlementsSpec{Mode: "none"},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("bsql.Parse rejected generated yaml: %v\n---\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "employee_ids:") {
		t.Errorf("expected employee_ids to be emitted as a trait field; yaml:\n%s", s)
	}
	if !strings.Contains(s, ".emp_id") {
		t.Errorf("expected employee_ids CEL to reference the mapped column .emp_id; yaml:\n%s", s)
	}
}

// TestGenerate_LoginAliasesEmitted guards against login_aliases (a canonical
// FieldMapping field the Studio web UI's List-tab mapping widget offers for
// the user trait, mirroring bsql.UserTraitMapping.LoginAliases) being
// silently dropped by the generator's field switch, the way an unrecognized
// field falls through to profileKey() and is discarded when it doesn't match
// "profile.*".
func TestGenerate_LoginAliasesEmitted(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{
				Query: "SELECT id, email, alias FROM employees",
				Fields: []FieldMapping{
					{Field: "id", Column: "id"},
					{Field: "emails", Column: "email"},
					{Field: "login_aliases", Column: "alias"},
				},
			},
			Entitlements: EntitlementsSpec{Mode: "none"},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("bsql.Parse rejected generated yaml: %v\n---\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "login_aliases:") {
		t.Errorf("expected login_aliases to be emitted as a trait field; yaml:\n%s", s)
	}
	if !strings.Contains(s, ".alias") {
		t.Errorf("expected login_aliases CEL to reference the mapped column .alias; yaml:\n%s", s)
	}
}

// TestGenerate_ManagerEmailOnly_NoHardcodedManagerID guards against the same
// regression for the manager_email-only case: no manager_id field is mapped at
// all, so the fallback must key off manager_email (and never emit a bogus
// profile.manager_id referencing a column that was never selected).
func TestGenerate_ManagerEmailOnly_NoHardcodedManagerID(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{
				Query: "SELECT id, email, mgr_email FROM employees",
				Fields: []FieldMapping{
					{Field: "id", Column: "id"},
					{Field: "emails", Column: "email"},
					{Field: "manager_email", Column: "mgr_email"},
				},
			},
			Entitlements: EntitlementsSpec{Mode: "none"},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("bsql.Parse rejected generated yaml: %v\n---\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "profile:") {
		t.Errorf("manager_email mapped but no profile block emitted; yaml:\n%s", s)
	}
	if !strings.Contains(s, "manager_email: .mgr_email") {
		t.Errorf("expected profile.manager_email to reference the real column .mgr_email; yaml:\n%s", s)
	}
	if strings.Contains(s, "manager_id") || strings.Contains(s, ".manager_id") {
		t.Errorf("bogus manager_id leaked into output when only manager_email was mapped; yaml:\n%s", s)
	}
}

func TestGenerate_DynamicEntitlementsAlwaysSlug(t *testing.T) {
	spec := &Spec{
		AppName: "EBS",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "menu", Name: "Menu", Trait: "role",
			List: ListSpec{Query: "SELECT menu_id, menu_name FROM menus", Fields: []FieldMapping{
				{Field: "id", Column: "menu_id"}, {Field: "display_name", Column: "menu_name"},
			}},
			Entitlements: EntitlementsSpec{
				Mode:  "query",
				Query: "SELECT function_id, function_name FROM functions WHERE menu_id = ?<menu_id>",
				Fields: []FieldMapping{
					{Field: "id", Column: "function_id"},
					{Field: "display_name", Column: "function_name"},
				},
			},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "slug:") {
		t.Errorf("dynamic entitlements must always emit slug; yaml:\n%s", out)
	}
}

// TestGenerate_DynamicEntitlementsSlugFallsBackToID guards against a
// regression (fix round 1) where the default-slug computation only looked at
// a mapped display_name field. When display_name is omitted but id is
// mapped, the slug default must fall back to slugify(<id CEL>) instead of
// emitting the malformed slugify() with an empty argument.
func TestGenerate_DynamicEntitlementsSlugFallsBackToID(t *testing.T) {
	spec := &Spec{
		AppName: "EBS",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "menu", Name: "Menu", Trait: "role",
			List: ListSpec{Query: "SELECT menu_id, menu_name FROM menus", Fields: []FieldMapping{
				{Field: "id", Column: "menu_id"}, {Field: "display_name", Column: "menu_name"},
			}},
			Entitlements: EntitlementsSpec{
				Mode:  "query",
				Query: "SELECT function_id FROM functions WHERE menu_id = ?<menu_id>",
				Fields: []FieldMapping{
					{Field: "id", Column: "function_id"},
				},
			},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "slug: slugify(.function_id)") {
		t.Errorf("expected slug to fall back to slugify(.function_id); yaml:\n%s", s)
	}
	if strings.Contains(s, "slugify()") {
		t.Errorf("must never emit slugify() with an empty argument; yaml:\n%s", s)
	}
}

// TestGenerate_ResourceScopedGrant verifies that a GrantSpec on a resource
// type emits a grants: sequence with a resource.ID var binding (keyed by
// ResourceVar) and a one-element map: sequence carrying principal_id,
// principal_type, and entitlement_id.
func TestGenerate_ResourceScopedGrant(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		ResourceTypes: []ResourceTypeSpec{
			{ID: "users", Name: "Users", Trait: "user",
				List:         ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "roles", Name: "Roles", Trait: "role",
				List:         ListSpec{Query: "SELECT role_id, role_name FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_name"}}},
				Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned", Purpose: "assignment"}}},
				Grants: []GrantSpec{{
					Query:       "SELECT user_id FROM user_roles WHERE role_id = ?<role_id>",
					ResourceVar: "role_id", PrincipalType: "users", Entitlement: "assigned",
					Fields: []FieldMapping{{Field: "principal_id", Column: "user_id"}},
				}},
			},
		},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, `role_id: resource.ID`) && !strings.Contains(s, `role_id: "resource.ID"`) {
		t.Errorf("expected resource-scoped var binding; yaml:\n%s", s)
	}
	if !strings.Contains(s, "principal_type: users") {
		t.Errorf("expected principal_type users; yaml:\n%s", s)
	}
}

// TestGenerate_DynamicEntitlementsNoIDOrDisplayName_Errors guards against
// silently emitting an invalid slugify() when a query-mode entitlements spec
// maps neither display_name nor id: there is nothing to derive a default
// slug from, so Generate must return an error rather than emit malformed
// CEL.
func TestGenerate_DynamicEntitlementsNoIDOrDisplayName_Errors(t *testing.T) {
	spec := &Spec{
		AppName: "EBS",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "menu", Name: "Menu", Trait: "role",
			List: ListSpec{Query: "SELECT menu_id, menu_name FROM menus", Fields: []FieldMapping{
				{Field: "id", Column: "menu_id"}, {Field: "display_name", Column: "menu_name"},
			}},
			Entitlements: EntitlementsSpec{
				Mode:  "query",
				Query: "SELECT function_id, description FROM functions WHERE menu_id = ?<menu_id>",
				Fields: []FieldMapping{
					{Field: "description", Column: "description"},
				},
			},
		}},
	}
	if _, err := Generate(spec); err == nil {
		t.Fatal("expected Generate to error when dynamic entitlements map neither id nor display_name")
	}
}

// TestGenerate_StaticGrantableToLiteralList verifies FIX-1/M-4: a static
// entitlement emits grantable_to as a literal !!seq of resource-type IDs (not a
// per-row CEL ref that would silently match nothing), plus description and
// immutable, and that this round-trips through bsql.Parse with GrantableTo
// populated as the exact []string.
func TestGenerate_StaticGrantableToLiteralList(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		ResourceTypes: []ResourceTypeSpec{
			{ID: "users", Name: "Users", Trait: "user",
				List:         ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "roles", Name: "Roles", Trait: "role",
				List: ListSpec{Query: "SELECT role_id, role_name FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_name"}}},
				Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{
					ID: "assigned", DisplayName: "Assigned", Purpose: "assignment",
					Description: "Membership", Immutable: true, GrantableTo: []string{"users"},
				}}},
			},
		},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := bsql.Parse(out)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	se := cfg.ResourceTypes["roles"].StaticEntitlements
	if len(se) != 1 {
		t.Fatalf("expected 1 static entitlement, got %d", len(se))
	}
	if !reflect.DeepEqual(se[0].GrantableTo, []string{"users"}) {
		t.Errorf("grantable_to = %#v, want [users]", se[0].GrantableTo)
	}
	if se[0].Description != "Membership" {
		t.Errorf("description = %q, want Membership", se[0].Description)
	}
	if !se[0].Immutable {
		t.Error("immutable not set on parsed static entitlement")
	}
	s := string(out)
	// literal id, not a CEL ref like ".users" or ".column"
	if !strings.Contains(s, "grantable_to:") || !strings.Contains(s, "- users") {
		t.Errorf("expected literal grantable_to list with id 'users'; yaml:\n%s", s)
	}
	if strings.Contains(s, ".users") {
		t.Errorf("grantable_to must be a literal id, not a CEL ref; yaml:\n%s", s)
	}
}

// TestGenerate_DynamicGrantableToLiteralList verifies FIX-1 for dynamic
// (query) entitlements: grantable_to comes from EntitlementsSpec.GrantableTo as
// a literal list, NOT from a query column compiled to a CEL ref.
func TestGenerate_DynamicGrantableToLiteralList(t *testing.T) {
	spec := &Spec{
		AppName: "EBS",
		ResourceTypes: []ResourceTypeSpec{
			{ID: "users", Name: "Users", Trait: "user",
				List:         ListSpec{Query: "SELECT id FROM u", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "menu", Name: "Menu", Trait: "role",
				List: ListSpec{Query: "SELECT menu_id, menu_name FROM menus", Fields: []FieldMapping{
					{Field: "id", Column: "menu_id"}, {Field: "display_name", Column: "menu_name"},
				}},
				Entitlements: EntitlementsSpec{
					Mode:        "query",
					Query:       "SELECT function_id, function_name FROM functions WHERE menu_id = ?<menu_id>",
					GrantableTo: []string{"users"},
					Fields: []FieldMapping{
						{Field: "id", Column: "function_id"},
						{Field: "display_name", Column: "function_name"},
					},
				},
			},
		},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := bsql.Parse(out)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	m := cfg.ResourceTypes["menu"].Entitlements.Map
	if len(m) != 1 {
		t.Fatalf("expected 1 entitlement mapping, got %d", len(m))
	}
	if !reflect.DeepEqual(m[0].GrantableTo, []string{"users"}) {
		t.Errorf("dynamic grantable_to = %#v, want [users]", m[0].GrantableTo)
	}
	s := string(out)
	if strings.Contains(s, ".users") {
		t.Errorf("dynamic grantable_to must be literal id, not CEL ref; yaml:\n%s", s)
	}
}

// TestGenerate_GrantResourceIDNotEmitted verifies FIX-2: even when a spec maps
// a resource_id grant field, Generate must NOT emit it (bsql's GrantMapping has
// no resource_id key, so emitting it is a silent no-op).
func TestGenerate_GrantResourceIDNotEmitted(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		ResourceTypes: []ResourceTypeSpec{
			{ID: "users", Name: "Users", Trait: "user",
				List:         ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "roles", Name: "Roles", Trait: "role",
				List:         ListSpec{Query: "SELECT role_id, role_name FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_name"}}},
				Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned", Purpose: "assignment"}}},
				Grants: []GrantSpec{{
					Query:       "SELECT user_id, r FROM user_roles WHERE role_id = ?<role_id>",
					ResourceVar: "role_id", PrincipalType: "users", Entitlement: "assigned",
					Fields: []FieldMapping{{Field: "principal_id", Column: "user_id"}, {Field: "resource_id", Column: "r"}},
				}},
			},
		},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "resource_id") {
		t.Errorf("resource_id must never be emitted on a grant; yaml:\n%s", out)
	}
}

// TestGenerate_SkipFlagOnEmptyMode verifies FIX-4: a resource type with an
// empty entitlements Mode ("") and no grants gets skip_entitlements_and_grants:
// true, keyed on real emptiness rather than the Mode=="none" sentinel.
func TestGenerate_SkipFlagOnEmptyMode(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List:         ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
			Entitlements: EntitlementsSpec{Mode: ""},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := bsql.Parse(out)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if !cfg.ResourceTypes["users"].SkipEntitlementsAndGrants {
		t.Errorf("expected skip_entitlements_and_grants for empty-mode type; yaml:\n%s", out)
	}
}

// TestGenerate_UnknownModeErrors verifies FIX-4: an unrecognized entitlements
// mode must be an error, not silently treated as "none".
func TestGenerate_UnknownModeErrors(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List:         ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
			Entitlements: EntitlementsSpec{Mode: "bogus"},
		}},
	}
	if _, err := Generate(spec); err == nil {
		t.Fatal("expected Generate to error on unknown entitlements mode")
	}
}
