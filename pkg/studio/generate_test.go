package studio

import (
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
