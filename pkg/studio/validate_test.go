package studio

import (
	"context"
	"testing"
)

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
