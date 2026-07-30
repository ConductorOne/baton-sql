package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/conductorone/baton-sql/pkg/studio"
)

// grantQuery is the single ?<role_id>-tokenized grant query exercised both as
// an ad hoc /api/run call (step 3) and as the roles resource type's
// GrantSpec.Query fed to /api/generate + /api/validate (steps 4-5), so the
// two are provably the same query rather than two queries that merely look
// similar.
const grantQuery = "SELECT user_id FROM user_roles WHERE role_id = ?<role_id>"

// seedIntegrationDB opens an in-memory sqlite database seeded with two
// tables: employees(id, first_name, last_name) with two rows, and
// user_roles(user_id, role_id) with three rows (two "admin", one "user") so
// a query filtered to role_id='admin' has more than one matching row to
// prove real filtering, not a coincidental single-row match. role_id is
// TEXT: sqlite is dynamically typed, so a string sample value ("admin")
// compares correctly against it.
func seedIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// A bare ":memory:" DSN gives each new pooled connection its own private
	// database; pinning the pool to a single connection guarantees every
	// query in this test (and every query the real HTTP handlers issue on
	// s.db) lands on the one connection that was actually seeded below.
	db.SetMaxOpenConns(1)

	ddl := []string{
		`CREATE TABLE employees (id INTEGER, first_name TEXT, last_name TEXT)`,
		`CREATE TABLE user_roles (user_id INTEGER, role_id TEXT)`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("ddl %q: %v", stmt, err)
		}
	}

	employees := []struct {
		id                  int
		firstName, lastName string
	}{
		{1, "Ada", "Lovelace"},
		{2, "Alan", "Turing"},
	}
	for _, e := range employees {
		if _, err := db.Exec(`INSERT INTO employees (id, first_name, last_name) VALUES (?, ?, ?)`, e.id, e.firstName, e.lastName); err != nil {
			t.Fatalf("insert employee %d: %v", e.id, err)
		}
	}

	userRoles := []struct {
		userID int
		roleID string
	}{
		{1, "admin"},
		{2, "user"},
		{3, "admin"},
	}
	for _, ur := range userRoles {
		if _, err := db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, ur.userID, ur.roleID); err != nil {
			t.Fatalf("insert user_role (%d,%s): %v", ur.userID, ur.roleID, err)
		}
	}

	return db
}

// integrationSpec returns a two-resource-type Spec (users + roles) matching
// seedIntegrationDB's schema: roles carries a static "assigned" entitlement
// and a resource-scoped grant using the exact grantQuery above, following the
// same shape as the proven-good testdata/finance.spec.json fixture.
func integrationSpec() *studio.Spec {
	return &studio.Spec{
		AppName: "Employee DB",
		Connect: studio.ConnectConfig{
			Scheme: "mysql", Host: "db.internal", Port: "3306",
			Database: "employees_db", User: "svc", Password: "pw",
		},
		ResourceTypes: []studio.ResourceTypeSpec{
			{
				ID: "users", Name: "Users", Trait: "user",
				List: studio.ListSpec{
					Query: "SELECT id, first_name, last_name FROM employees",
					Fields: []studio.FieldMapping{
						{Field: "id", Column: "id"},
						{
							Field: "display_name",
							Transform: &studio.Transform{
								Recipe: studio.RecipeCompositeID,
								Args:   map[string]any{"columns": []any{"first_name", "last_name"}, "sep": " "},
							},
						},
					},
				},
				Entitlements: studio.EntitlementsSpec{Mode: "none"},
			},
			{
				ID: "roles", Name: "Roles", Trait: "role",
				List: studio.ListSpec{
					Query: "SELECT DISTINCT role_id FROM user_roles",
					Fields: []studio.FieldMapping{
						{Field: "id", Column: "role_id"},
						{Field: "display_name", Column: "role_id"},
					},
				},
				Entitlements: studio.EntitlementsSpec{
					Mode: "static",
					Static: []studio.StaticEntitlement{
						{ID: "assigned", DisplayName: "Assigned", Purpose: "assignment", GrantableTo: []string{"users"}},
					},
				},
				Grants: []studio.GrantSpec{
					{
						Query:       grantQuery,
						ResourceVar: "role_id",
						Mappings: []studio.GrantMapping{
							{
								PrincipalID:   studio.FieldMapping{Field: "principal_id", Column: "user_id"},
								PrincipalType: "users",
								Entitlement:   "assigned",
							},
						},
					},
				},
			},
		},
	}
}

// postJSON marshals body, POSTs it to url on client, requires an HTTP 200
// response (failing with the raw body otherwise), and decodes the response
// JSON into out.
func postJSON(t *testing.T, client *http.Client, url string, body, out any) {
	t.Helper()
	reqBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body for %s: %v", url, err)
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("POST %s: read response body: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: expected HTTP 200, got %d, body: %s", url, resp.StatusCode, respBytes)
	}
	if err := json.Unmarshal(respBytes, out); err != nil {
		t.Fatalf("POST %s: unmarshal response: %v (body=%s)", url, err, respBytes)
	}
}

// TestIntegration_ConnectRunGenerateValidate drives the real Handler over a
// genuine HTTP round-trip (httptest.Server, not httptest.NewRequest/Recorder)
// through the full Studio flow a user would exercise interactively:
//
//  1. connect to a (stubbed) database,
//  2. run a plain list query against a live table,
//  3. run a token-substituted grant query with a sample var against another
//     live table,
//  4. generate connector YAML for a two-resource-type Spec, and
//  5. validate that same Spec, this time with the live session from step 1
//     threaded into Validate rather than the offline sqlite fallback.
//
// Every assertion is against real values produced by the real handlers; none
// of the fields consumed here are stubbed out or short-circuited by the test
// itself.
func TestIntegration_ConnectRunGenerateValidate(t *testing.T) {
	db := seedIntegrationDB(t)

	s := New()
	s.connect = func(ctx context.Context, o database.ConnectOptions) (*sql.DB, database.DbEngine, error) {
		return db, database.MySQL, nil
	}

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	client := srv.Client()

	// --- Step 1: POST /api/connect ---
	connectCfg := studio.ConnectConfig{
		Scheme: "mysql", Host: "db.internal", Port: "3306",
		Database: "employees_db", User: "svc", Password: "pw",
	}
	var connResp connectResponse
	postJSON(t, client, srv.URL+"/api/connect", connectCfg, &connResp)
	if !connResp.OK {
		t.Fatalf("connect: expected ok:true, got %+v", connResp)
	}
	if connResp.Engine != "mysql" {
		t.Fatalf("connect: expected engine %q, got %+v", "mysql", connResp)
	}

	// --- Step 2: POST /api/run - plain list query against employees ---
	var usersResp runResponse
	postJSON(t, client, srv.URL+"/api/run",
		runRequest{Query: "SELECT id, first_name, last_name FROM employees ORDER BY id"},
		&usersResp)
	if usersResp.Error != "" {
		t.Fatalf("run(users): unexpected error: %s", usersResp.Error)
	}
	wantCols := []string{"id", "first_name", "last_name"}
	if !reflect.DeepEqual(usersResp.Columns, wantCols) {
		t.Fatalf("run(users): expected columns %v, got %v", wantCols, usersResp.Columns)
	}
	if usersResp.RowCount != 2 {
		t.Fatalf("run(users): expected row_count 2 (seed size), got %d (rows=%+v)", usersResp.RowCount, usersResp.Rows)
	}
	if usersResp.Truncated {
		t.Fatalf("run(users): expected not truncated")
	}
	if len(usersResp.Rows) != 2 {
		t.Fatalf("run(users): expected 2 rows, got %+v", usersResp.Rows)
	}
	// JSON round-trip normalizes integer columns to float64.
	wantRow0 := []any{float64(1), "Ada", "Lovelace"}
	if !reflect.DeepEqual(usersResp.Rows[0], wantRow0) {
		t.Fatalf("run(users): expected first row %+v, got %+v", wantRow0, usersResp.Rows[0])
	}
	wantRow1 := []any{float64(2), "Alan", "Turing"}
	if !reflect.DeepEqual(usersResp.Rows[1], wantRow1) {
		t.Fatalf("run(users): expected second row %+v, got %+v", wantRow1, usersResp.Rows[1])
	}

	// --- Step 3: POST /api/run - token-substituted grant query ---
	var grantResp runResponse
	postJSON(t, client, srv.URL+"/api/run",
		runRequest{Query: grantQuery, Vars: map[string]string{"role_id": "admin"}},
		&grantResp)
	if grantResp.Error != "" {
		t.Fatalf("run(grant): unexpected error: %s", grantResp.Error)
	}
	if len(grantResp.Columns) != 1 || grantResp.Columns[0] != "user_id" {
		t.Fatalf("run(grant): expected columns [user_id], got %v", grantResp.Columns)
	}
	// Seed has two rows with role_id='admin' (user_id 1 and 3); assert the
	// set of matched user_ids without depending on row order.
	if grantResp.RowCount != 2 {
		t.Fatalf("run(grant): expected row_count 2 matching role_id=admin, got %d (rows=%+v)", grantResp.RowCount, grantResp.Rows)
	}
	var gotUserIDs []float64
	for _, row := range grantResp.Rows {
		if len(row) != 1 {
			t.Fatalf("run(grant): expected 1 cell per row, got %+v", row)
		}
		id, ok := row[0].(float64)
		if !ok {
			t.Fatalf("run(grant): expected numeric user_id, got %+v (%T)", row[0], row[0])
		}
		gotUserIDs = append(gotUserIDs, id)
	}
	sort.Float64s(gotUserIDs)
	wantUserIDs := []float64{1, 3}
	if !reflect.DeepEqual(gotUserIDs, wantUserIDs) {
		t.Fatalf("run(grant): expected matched user_ids %v, got %v", wantUserIDs, gotUserIDs)
	}

	// --- Step 4: POST /api/generate - two-resource-type Spec ---
	spec := integrationSpec()
	var genResp generateResponse
	postJSON(t, client, srv.URL+"/api/generate", spec, &genResp)
	if genResp.Error != "" {
		t.Fatalf("generate: unexpected error: %s", genResp.Error)
	}
	if !strings.Contains(genResp.YAML, "resource_types:") {
		t.Fatalf("generate: expected yaml to contain \"resource_types:\", got:\n%s", genResp.YAML)
	}
	if !strings.Contains(genResp.YAML, "users:") || !strings.Contains(genResp.YAML, "roles:") {
		t.Fatalf("generate: expected yaml to contain both resource types \"users:\" and \"roles:\", got:\n%s", genResp.YAML)
	}

	// --- Step 5: POST /api/validate - same Spec, WITH the live session ---
	var report studio.Report
	postJSON(t, client, srv.URL+"/api/validate", spec, &report)
	if !report.OK {
		t.Fatalf("validate: expected ok:true, got errors: %+v", report.Errors)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("validate: expected zero errors, got %+v", report.Errors)
	}
}
