package server

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/conductorone/baton-sql/pkg/studio"
)

// smallSpec returns a minimal valid Spec used across generate/validate tests.
func smallSpec() *studio.Spec {
	return &studio.Spec{
		AppName: "Finance DB",
		Connect: studio.ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance", User: "svc", Password: "pw"},
		ResourceTypes: []studio.ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: studio.ListSpec{
				Query: "SELECT id, email FROM employees",
				Fields: []studio.FieldMapping{
					{Field: "id", Column: "id"},
					{Field: "emails", Column: "email"},
				},
			},
			Entitlements: studio.EntitlementsSpec{Mode: "none"},
		}},
	}
}

func TestGenerate_OK(t *testing.T) {
	s := New()
	body, err := json.Marshal(smallSpec())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(string(body)))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var resp struct {
		YAML  string `json:"yaml"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.YAML, "resource_types:") {
		t.Fatalf("expected yaml to contain resource_types:, got %s", resp.YAML)
	}
}

func TestGenerate_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/generate", nil)
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGenerate_MalformedJSON(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(`{not json`))
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

func TestValidate_NoSession_OK(t *testing.T) {
	s := New()
	body, err := json.Marshal(smallSpec())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/validate", strings.NewReader(string(body)))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var report studio.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !report.OK {
		t.Fatalf("expected ok, got %+v", report)
	}
}

// TestValidate_WithSession_OK proves the active session (DB + engine) is
// threaded into studio.ValidateOptions rather than always validating
// offline.
func TestValidate_WithSession_OK(t *testing.T) {
	s := New()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.setSessionForTest(db, database.SQLite)

	body, err := json.Marshal(smallSpec())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/validate", strings.NewReader(string(body)))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var report studio.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !report.OK {
		t.Fatalf("expected ok, got %+v", report)
	}
}

func TestValidate_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/validate", nil)
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestValidate_MalformedJSON(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/validate", strings.NewReader(`{not json`))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var report studio.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if report.OK {
		t.Fatalf("expected not ok")
	}
}

func TestPreview_CompositeID(t *testing.T) {
	s := New()
	reqBody := map[string]any{
		"field": studio.FieldMapping{
			Field: "display_name",
			Transform: &studio.Transform{
				Recipe: studio.RecipeCompositeID,
				Args:   map[string]any{"columns": []any{"first_name", "last_name"}, "sep": " "},
			},
		},
		"row": map[string]any{"first_name": "Ada", "last_name": "Lovelace"},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(body)))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var resp struct {
		Value string `json:"value"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Value != "Ada Lovelace" {
		t.Fatalf("got %q want %q", resp.Value, "Ada Lovelace")
	}
}

func TestPreview_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/preview", nil)
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestPreview_MalformedJSON(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/preview", strings.NewReader(`{not json`))
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
