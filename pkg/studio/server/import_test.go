package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/conductorone/baton-sql/pkg/studio"
)

const importTestYAML = `
app_name: Finance DB
resource_types:
    users:
        name: Users
        list:
            query: SELECT emp_id, email FROM employees
            map:
                id: .emp_id
                display_name: .emp_id
                traits:
                    user:
                        emails:
                            - .email
        skip_entitlements_and_grants: true
`

func TestImport_OK(t *testing.T) {
	s := New()
	body, err := json.Marshal(importRequest{YAML: importTestYAML})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/import", strings.NewReader(string(body)))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}

	var resp struct {
		Spec  *studio.Spec `json:"spec"`
		Error string       `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Spec == nil {
		t.Fatal("expected a spec in the response")
	}
	if resp.Spec.AppName != "Finance DB" {
		t.Fatalf("expected app_name %q, got %q", "Finance DB", resp.Spec.AppName)
	}
	if len(resp.Spec.ResourceTypes) != 1 {
		t.Fatalf("expected 1 resource type, got %d", len(resp.Spec.ResourceTypes))
	}
	rt := resp.Spec.ResourceTypes[0]
	if rt.ID != "users" {
		t.Fatalf("expected resource type id %q, got %q", "users", rt.ID)
	}
	if rt.Trait != "user" {
		t.Fatalf("expected trait %q, got %q", "user", rt.Trait)
	}
}

func TestImport_BadYAML(t *testing.T) {
	s := New()
	body, err := json.Marshal(importRequest{YAML: "{not: [valid"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/import", strings.NewReader(string(body)))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 for a parse-error result, got %d", rec.Code)
	}

	var resp struct {
		Spec  *studio.Spec `json:"spec"`
		Error string       `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected an error for malformed YAML")
	}
	if resp.Spec != nil {
		t.Fatalf("expected no spec on error, got %+v", resp.Spec)
	}
}

// TestImport_MultiRowGrantMapImportsAllRows verifies the multi-row grant fix
// end-to-end over HTTP: a config whose grants query has more than one map row
// now imports to a full spec (one GrantMapping per row) with no error, instead
// of the old loud-error / truncation behavior.
func TestImport_MultiRowGrantMapImportsAllRows(t *testing.T) {
	const multiRowGrantYAML = `
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
`
	s := New()
	body, err := json.Marshal(importRequest{YAML: multiRowGrantYAML})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/import", strings.NewReader(string(body)))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Spec  *studio.Spec `json:"spec"`
		Error string       `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("expected no error importing a multi-row grant map, got: %s", resp.Error)
	}
	if resp.Spec == nil || len(resp.Spec.ResourceTypes) != 1 {
		t.Fatalf("expected a spec with 1 resource type, got %+v", resp.Spec)
	}
	grants := resp.Spec.ResourceTypes[0].Grants
	if len(grants) != 1 || len(grants[0].Mappings) != 2 {
		t.Fatalf("expected 1 grants query with 2 mappings, got %+v", grants)
	}
	if grants[0].Mappings[0].PrincipalType != "user" || grants[0].Mappings[1].PrincipalType != "group" {
		t.Fatalf("mapping types/order wrong: %+v", grants[0].Mappings)
	}
}

func TestImport_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/import", nil)
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestImport_MalformedJSON(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/import", strings.NewReader(`{not json`))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == "" {
		t.Fatalf("expected error in response")
	}
}
