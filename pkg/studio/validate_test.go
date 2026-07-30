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
