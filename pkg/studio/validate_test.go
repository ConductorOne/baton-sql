package studio

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// goldenFinanceSpec reads the multi-resource-type Finance DB fixture used to
// prove the whole engine end-to-end (Task 9): a users list with a composite
// display_name, a status ternary, profile.department, and manager_id, plus a
// roles list with a static entitlement and a resource-scoped grant.
func goldenFinanceSpec(t *testing.T) *Spec {
	t.Helper()
	data, err := os.ReadFile("testdata/finance.spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

// TestValidate_GoodSpecOK proves the fixture used across the whole Studio
// engine (Generate, CLI, and this validation layer) is genuinely clean: no
// principal_type issues, and it round-trips through bsql.Parse and the
// authoritative syncer.Validate(ctx) check with zero errors.
func TestValidate_GoodSpecOK(t *testing.T) {
	spec := goldenFinanceSpec(t)
	rep, err := Validate(context.Background(), spec, ValidateOptions{})
	if err != nil {
		t.Fatalf("validate returned error: %v", err)
	}
	if !rep.OK {
		t.Fatalf("expected report OK for golden finance spec, got errors: %+v", rep.Errors)
	}
	if len(rep.Errors) != 0 {
		t.Fatalf("expected no errors, got %+v", rep.Errors)
	}
}

func TestValidate_BadPrincipalTypeReported(t *testing.T) {
	spec := &Spec{
		AppName: "x",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "roles", Name: "Roles", Trait: "role",
			List:         ListSpec{Query: "SELECT role_id FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_id"}}},
			Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned"}}},
			Grants:       []GrantSpec{{Query: "SELECT u FROM t", PrincipalType: "does_not_exist", Entitlement: "assigned", Fields: []FieldMapping{{Field: "principal_id", Column: "u"}}}},
		}},
	}
	rep, err := Validate(context.Background(), spec, ValidateOptions{})
	if err != nil {
		t.Fatalf("validate returned error: %v", err)
	}
	if rep.OK {
		t.Fatal("expected report NOT ok for undefined principal_type")
	}
	found := false
	for _, is := range rep.Errors {
		if is.Field == "principal_type" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a principal_type issue, got %+v", rep.Errors)
	}
}

// TestValidate_BsqlValidationErrorReported proves that Validate's delegation to
// pkg/bsql (Generate -> bsql.Parse -> GetSQLSyncers -> syncer.Validate) actually
// catches a real bsql-layer error, not just the spec-level principal_type check.
// The spec here uses a VALID principal_type (so the spec-level check produces no
// issue) but a grant query referencing an undeclared query var (?<rid>) with no
// corresponding ResourceVar/vars entry - a case Generate + bsql.Parse both accept
// structurally, but which pkg/bsql's own static validation
// (validateVarsInQuery, pkg/bsql/validate.go:9) rejects at Validate(ctx) time.
func TestValidate_BsqlValidationErrorReported(t *testing.T) {
	spec := &Spec{
		AppName: "x",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "roles", Name: "Roles", Trait: "role",
			List:         ListSpec{Query: "SELECT role_id FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_id"}}},
			Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned"}}},
			Grants: []GrantSpec{{
				Query:         "SELECT u FROM t WHERE r = ?<rid>", // "rid" is never declared in vars
				PrincipalType: "roles",                            // valid: matches the resource type's own ID
				Entitlement:   "assigned",
				Fields:        []FieldMapping{{Field: "principal_id", Column: "u"}},
			}},
		}},
	}
	rep, err := Validate(context.Background(), spec, ValidateOptions{})
	if err != nil {
		t.Fatalf("validate returned error: %v", err)
	}
	if rep.OK {
		t.Fatal("expected report NOT ok for bsql-layer validation failure (undeclared query var)")
	}
	nonPrincipalFound := false
	for _, is := range rep.Errors {
		if is.Field == "principal_type" {
			t.Fatalf("did not expect a principal_type issue for a valid principal_type, got %+v", rep.Errors)
		}
		nonPrincipalFound = true
	}
	if !nonPrincipalFound {
		t.Fatalf("expected a bsql-layer validation issue, got %+v", rep.Errors)
	}
}

// TestValidate_GrantableToUndefinedResourceTypeReported verifies FIX-1's
// validation: a grantable_to entry (static or dynamic) that does not reference
// a defined resource type is flagged with Field "grantable_to".
func TestValidate_GrantableToUndefinedResourceTypeReported(t *testing.T) {
	// static case
	specStatic := &Spec{
		AppName: "x",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "roles", Name: "Roles", Trait: "role",
			List:         ListSpec{Query: "SELECT role_id FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_id"}}},
			Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned", GrantableTo: []string{"ghosts"}}}},
		}},
	}
	rep, err := Validate(context.Background(), specStatic, ValidateOptions{})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.OK || !hasField(rep.Errors, "grantable_to") {
		t.Fatalf("expected a grantable_to issue for static entitlement, got %+v (ok=%v)", rep.Errors, rep.OK)
	}

	// dynamic case
	specDyn := &Spec{
		AppName: "x",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "menu", Name: "Menu", Trait: "role",
			List: ListSpec{Query: "SELECT menu_id, menu_name FROM menus", Fields: []FieldMapping{{Field: "id", Column: "menu_id"}, {Field: "display_name", Column: "menu_name"}}},
			Entitlements: EntitlementsSpec{
				Mode: "query", Query: "SELECT fid, fname FROM functions WHERE menu_id = ?<menu_id>",
				GrantableTo: []string{"ghosts"},
				Fields:      []FieldMapping{{Field: "id", Column: "fid"}, {Field: "display_name", Column: "fname"}},
			},
		}},
	}
	rep, err = Validate(context.Background(), specDyn, ValidateOptions{})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.OK || !hasField(rep.Errors, "grantable_to") {
		t.Fatalf("expected a grantable_to issue for dynamic entitlement, got %+v (ok=%v)", rep.Errors, rep.OK)
	}
}

// TestValidate_GrantResourceIDReported verifies FIX-2's validation: a grant
// field mapping named resource_id is flagged (Field "resource_id") rather than
// silently dropped.
func TestValidate_GrantResourceIDReported(t *testing.T) {
	spec := &Spec{
		AppName: "x",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "roles", Name: "Roles", Trait: "role",
			List:         ListSpec{Query: "SELECT role_id FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_id"}}},
			Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned"}}},
			Grants: []GrantSpec{{
				Query: "SELECT u, r FROM t", PrincipalType: "roles", Entitlement: "assigned",
				Fields: []FieldMapping{{Field: "principal_id", Column: "u"}, {Field: "resource_id", Column: "r"}},
			}},
		}},
	}
	rep, err := Validate(context.Background(), spec, ValidateOptions{})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.OK || !hasField(rep.Errors, "resource_id") {
		t.Fatalf("expected a resource_id issue, got %+v (ok=%v)", rep.Errors, rep.OK)
	}
}

// TestValidate_InvalidPurposeReported verifies FIX-3: an invalid purpose is
// flagged (Field "purpose"), while "assignment", "permission", and "" are
// accepted.
func TestValidate_InvalidPurposeReported(t *testing.T) {
	mk := func(purpose string) *Spec {
		return &Spec{
			AppName: "x",
			ResourceTypes: []ResourceTypeSpec{{
				ID: "roles", Name: "Roles", Trait: "role",
				List:         ListSpec{Query: "SELECT role_id FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_id"}}},
				Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned", Purpose: purpose}}},
			}},
		}
	}
	// invalid
	rep, err := Validate(context.Background(), mk("Grants membership"), ValidateOptions{})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.OK || !hasField(rep.Errors, "purpose") {
		t.Fatalf("expected a purpose issue, got %+v (ok=%v)", rep.Errors, rep.OK)
	}
	// valid values accepted
	for _, p := range []string{"assignment", "permission", ""} {
		rep, err := Validate(context.Background(), mk(p), ValidateOptions{})
		if err != nil {
			t.Fatalf("validate(%q): %v", p, err)
		}
		if hasField(rep.Errors, "purpose") {
			t.Errorf("purpose %q should be accepted, got %+v", p, rep.Errors)
		}
	}
}

func hasField(errs []Issue, field string) bool {
	for _, is := range errs {
		if is.Field == field {
			return true
		}
	}
	return false
}
