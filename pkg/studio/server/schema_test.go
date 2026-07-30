package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/conductorone/baton-sql/pkg/database"
)

// newMockSession opens a sqlmock-backed *sql.DB using exact (whitespace-
// normalized) query matching — the default regexp matcher trips over the
// regexp-special characters ($1, ?, parens, backticks, ...) that appear in
// real catalog SQL — and wires it into s as the active session for engine.
func newMockSession(t *testing.T, s *Server, engine database.DbEngine) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s.setSessionForTest(db, engine)
	return mock
}

func doGet(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func doSchema(t *testing.T, s *Server, path string) schemaResponse {
	t.Helper()
	rec := doGet(t, s, path)
	if rec.Code != 200 {
		t.Fatalf("code %d, body %s", rec.Code, rec.Body.String())
	}
	var resp schemaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	return resp
}

func doSchemaTable(t *testing.T, s *Server, path string) schemaTableResponse {
	t.Helper()
	rec := doGet(t, s, path)
	if rec.Code != 200 {
		t.Fatalf("code %d, body %s", rec.Code, rec.Body.String())
	}
	var resp schemaTableResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	return resp
}

// --- /api/schema ---

func TestSchema_NotConnected(t *testing.T) {
	s := New()
	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "not connected" {
		t.Fatalf("expected 'not connected' error, got %+v", resp)
	}
}

func TestSchema_UnsupportedEngine(t *testing.T) {
	s := New()
	newMockSession(t, s, database.Oracle)

	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "schema discovery is not yet supported for oracle" {
		t.Fatalf("unexpected error: %+v", resp)
	}
}

func TestSchema_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/schema", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestSchema_Postgres_ListAll(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	mock.ExpectQuery(
		`SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('pg_catalog', 'information_schema') ORDER BY schema_name`,
	).WillReturnRows(sqlmock.NewRows([]string{"schema_name"}).
		AddRow("public"))

	mock.ExpectQuery(
		`SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name`,
	).WithArgs("public").WillReturnRows(sqlmock.NewRows([]string{"table_name", "table_type"}).
		AddRow("users", "BASE TABLE").
		AddRow("active_users", "VIEW"))

	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Engine != "postgres" {
		t.Fatalf("expected engine postgres, got %+v", resp)
	}
	if resp.Truncated {
		t.Fatalf("expected top-level truncated=false, got %+v", resp)
	}
	if len(resp.Schemas) != 1 || resp.Schemas[0].Name != "public" {
		t.Fatalf("unexpected schemas: %+v", resp.Schemas)
	}
	sch := resp.Schemas[0]
	if sch.Truncated {
		t.Fatalf("expected schema truncated=false, got %+v", sch)
	}
	if len(sch.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %+v", sch.Tables)
	}
	if sch.Tables[0].Name != "users" || sch.Tables[0].Type != "table" {
		t.Fatalf("unexpected first table: %+v", sch.Tables[0])
	}
	if sch.Tables[1].Name != "active_users" || sch.Tables[1].Type != "view" {
		t.Fatalf("unexpected second table: %+v", sch.Tables[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSchema_Postgres_SingleSchema(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	// With ?schema=, the schemata list query must NOT be issued.
	mock.ExpectQuery(
		`SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name`,
	).WithArgs("app").WillReturnRows(sqlmock.NewRows([]string{"table_name", "table_type"}).
		AddRow("widgets", "BASE TABLE"))

	resp := doSchema(t, s, "/api/schema?schema=app")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Schemas) != 1 || resp.Schemas[0].Name != "app" {
		t.Fatalf("unexpected schemas: %+v", resp.Schemas)
	}
	if len(resp.Schemas[0].Tables) != 1 || resp.Schemas[0].Tables[0].Name != "widgets" {
		t.Fatalf("unexpected tables: %+v", resp.Schemas[0].Tables)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSchema_Postgres_SchemataQueryError(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	mock.ExpectQuery(
		`SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('pg_catalog', 'information_schema') ORDER BY schema_name`,
	).WillReturnError(errPermissionDenied)

	resp := doSchema(t, s, "/api/schema")
	if resp.Error == "" {
		t.Fatalf("expected error, got %+v", resp)
	}
}

func TestSchema_Postgres_TablesTruncated(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	mock.ExpectQuery(
		`SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('pg_catalog', 'information_schema') ORDER BY schema_name`,
	).WillReturnRows(sqlmock.NewRows([]string{"schema_name"}).AddRow("public"))

	rows := sqlmock.NewRows([]string{"table_name", "table_type"})
	for i := 0; i < maxTablesPerSchema+1; i++ {
		rows.AddRow(nthName(i), "BASE TABLE")
	}
	mock.ExpectQuery(
		`SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name`,
	).WithArgs("public").WillReturnRows(rows)

	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Schemas) != 1 {
		t.Fatalf("unexpected schemas: %+v", resp.Schemas)
	}
	sch := resp.Schemas[0]
	if !sch.Truncated {
		t.Fatalf("expected schema-level truncated=true")
	}
	if len(sch.Tables) != maxTablesPerSchema {
		t.Fatalf("expected %d tables, got %d", maxTablesPerSchema, len(sch.Tables))
	}
}

func TestSchema_Postgres_SchemasTruncated(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	rows := sqlmock.NewRows([]string{"schema_name"})
	for i := 0; i < maxSchemas+1; i++ {
		rows.AddRow(nthName(i))
	}
	mock.ExpectQuery(
		`SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('pg_catalog', 'information_schema') ORDER BY schema_name`,
	).WillReturnRows(rows)

	for i := 0; i < maxSchemas; i++ {
		mock.ExpectQuery(
			`SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name`,
		).WithArgs(nthName(i)).WillReturnRows(sqlmock.NewRows([]string{"table_name", "table_type"}))
	}

	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !resp.Truncated {
		t.Fatalf("expected top-level truncated=true, got %+v", resp)
	}
	if len(resp.Schemas) != maxSchemas {
		t.Fatalf("expected %d schemas, got %d", maxSchemas, len(resp.Schemas))
	}
}

func TestSchema_MySQL_ListAll(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.MySQL)

	mock.ExpectQuery(
		`SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys') ORDER BY schema_name`,
	).WillReturnRows(sqlmock.NewRows([]string{"schema_name"}).AddRow("appdb"))

	mock.ExpectQuery(
		`SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name`,
	).WithArgs("appdb").WillReturnRows(sqlmock.NewRows([]string{"table_name", "table_type"}).
		AddRow("orders", "BASE TABLE"))

	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Engine != "mysql" {
		t.Fatalf("expected engine mysql, got %+v", resp)
	}
	if len(resp.Schemas) != 1 || len(resp.Schemas[0].Tables) != 1 {
		t.Fatalf("unexpected schemas: %+v", resp.Schemas)
	}
}

func TestSchema_MSSQL_ListAll(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.MSSQL)

	mock.ExpectQuery(
		`SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('sys', 'INFORMATION_SCHEMA', 'guest') AND schema_name NOT LIKE 'db\_%' ESCAPE '\' ORDER BY schema_name`,
	).WillReturnRows(sqlmock.NewRows([]string{"schema_name"}).AddRow("dbo"))

	mock.ExpectQuery(
		`SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = @p1 ORDER BY table_name`,
	).WithArgs("dbo").WillReturnRows(sqlmock.NewRows([]string{"table_name", "table_type"}).
		AddRow("customers", "BASE TABLE"))

	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Engine != "sqlserver" {
		t.Fatalf("expected engine sqlserver, got %+v", resp)
	}
	if len(resp.Schemas) != 1 || len(resp.Schemas[0].Tables) != 1 {
		t.Fatalf("unexpected schemas: %+v", resp.Schemas)
	}
}

func TestSchema_Vertica_ListAll(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.Vertica)

	mock.ExpectQuery(
		`SELECT schema_name FROM v_catalog.schemata WHERE HAS_SCHEMA_PRIVILEGE(schema_name, 'USAGE') ORDER BY schema_name`,
	).WillReturnRows(sqlmock.NewRows([]string{"schema_name"}).AddRow("public"))

	mock.ExpectQuery(
		`SELECT table_name, 'TABLE' FROM v_catalog.tables WHERE table_schema = ? UNION ALL SELECT table_name, 'VIEW' FROM v_catalog.views WHERE table_schema = ? ORDER BY table_name`,
	).WithArgs("public", "public").WillReturnRows(sqlmock.NewRows([]string{"table_name", "table_type"}).
		AddRow("events", "TABLE").
		AddRow("events_v", "VIEW"))

	resp := doSchema(t, s, "/api/schema")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Engine != "vertica" {
		t.Fatalf("expected engine vertica, got %+v", resp)
	}
	if len(resp.Schemas) != 1 || len(resp.Schemas[0].Tables) != 2 {
		t.Fatalf("unexpected schemas: %+v", resp.Schemas)
	}
}

// --- /api/schema/table ---

func TestSchemaTable_NotConnected(t *testing.T) {
	s := New()
	resp := doSchemaTable(t, s, "/api/schema/table?schema=public&table=users")
	if resp.Error != "not connected" {
		t.Fatalf("expected 'not connected' error, got %+v", resp)
	}
}

func TestSchemaTable_UnsupportedEngine(t *testing.T) {
	s := New()
	newMockSession(t, s, database.HDB)

	resp := doSchemaTable(t, s, "/api/schema/table?schema=public&table=users")
	if resp.Error != "schema discovery is not yet supported for hdb" {
		t.Fatalf("unexpected error: %+v", resp)
	}
}

func TestSchemaTable_MissingParams(t *testing.T) {
	s := New()
	newMockSession(t, s, database.PostgreSQL)

	resp := doSchemaTable(t, s, "/api/schema/table?schema=public")
	if resp.Error == "" {
		t.Fatalf("expected error for missing table param, got %+v", resp)
	}
}

func TestSchemaTable_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/schema/table", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestSchemaTable_Postgres_OK(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2`,
	).WithArgs("public", "users").WillReturnRows(sqlmock.NewRows([]string{"table_type"}).AddRow("BASE TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`,
	).WithArgs("public", "users").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}).
		AddRow("id", "integer", "NO").
		AddRow("email", "text", "YES"))

	mock.ExpectQuery(
		`SELECT has_table_privilege($1, 'SELECT')`,
	).WithArgs("public.users").WillReturnRows(sqlmock.NewRows([]string{"has_table_privilege"}).AddRow(true))

	resp := doSchemaTable(t, s, "/api/schema/table?schema=public&table=users")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Schema != "public" || resp.Table != "users" || resp.Type != "table" {
		t.Fatalf("unexpected header fields: %+v", resp)
	}
	if resp.CanSelect == nil || !*resp.CanSelect {
		t.Fatalf("expected can_select=true, got %+v", resp.CanSelect)
	}
	if len(resp.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %+v", resp.Columns)
	}
	if resp.Columns[0].Name != "id" || resp.Columns[0].Type != "integer" || resp.Columns[0].Nullable {
		t.Fatalf("unexpected first column: %+v", resp.Columns[0])
	}
	if resp.Columns[1].Name != "email" || !resp.Columns[1].Nullable {
		t.Fatalf("unexpected second column: %+v", resp.Columns[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSchemaTable_Postgres_CanSelectFalse(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2`,
	).WithArgs("public", "secret").WillReturnRows(sqlmock.NewRows([]string{"table_type"}).AddRow("BASE TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`,
	).WithArgs("public", "secret").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}).
		AddRow("id", "integer", "NO"))

	mock.ExpectQuery(
		`SELECT has_table_privilege($1, 'SELECT')`,
	).WithArgs("public.secret").WillReturnRows(sqlmock.NewRows([]string{"has_table_privilege"}).AddRow(false))

	resp := doSchemaTable(t, s, "/api/schema/table?schema=public&table=secret")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.CanSelect == nil || *resp.CanSelect {
		t.Fatalf("expected can_select=false, got %+v", resp.CanSelect)
	}
}

func TestSchemaTable_Postgres_CanSelectCheckErrors_IsUnknown(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2`,
	).WithArgs("public", "users").WillReturnRows(sqlmock.NewRows([]string{"table_type"}).AddRow("BASE TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`,
	).WithArgs("public", "users").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}).
		AddRow("id", "integer", "NO"))

	mock.ExpectQuery(
		`SELECT has_table_privilege($1, 'SELECT')`,
	).WithArgs("public.users").WillReturnError(errPermissionDenied)

	resp := doSchemaTable(t, s, "/api/schema/table?schema=public&table=users")
	if resp.Error != "" {
		t.Fatalf("expected the overall request to still succeed, got error: %s", resp.Error)
	}
	if resp.CanSelect != nil {
		t.Fatalf("expected can_select to be omitted/unknown, got %+v", *resp.CanSelect)
	}
}

func TestSchemaTable_Postgres_TableNotFound(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.PostgreSQL)

	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2`,
	).WithArgs("public", "ghost").WillReturnRows(sqlmock.NewRows([]string{"table_type"}))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`,
	).WithArgs("public", "ghost").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}))

	mock.ExpectQuery(
		`SELECT has_table_privilege($1, 'SELECT')`,
	).WithArgs("public.ghost").WillReturnError(errPermissionDenied)

	resp := doSchemaTable(t, s, "/api/schema/table?schema=public&table=ghost")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Type != "" {
		t.Fatalf("expected empty type for a not-found table, got %q", resp.Type)
	}
	if len(resp.Columns) != 0 {
		t.Fatalf("expected no columns, got %+v", resp.Columns)
	}
}

func TestSchemaTable_MySQL_CanSelectProbe(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.MySQL)

	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = ? AND table_name = ?`,
	).WithArgs("appdb", "orders").WillReturnRows(sqlmock.NewRows([]string{"table_type"}).AddRow("BASE TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`,
	).WithArgs("appdb", "orders").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}).
		AddRow("id", "int", "NO"))

	mock.ExpectQuery(
		"SELECT 1 FROM `appdb`.`orders` LIMIT 0",
	).WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	resp := doSchemaTable(t, s, "/api/schema/table?schema=appdb&table=orders")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.CanSelect == nil || !*resp.CanSelect {
		t.Fatalf("expected can_select=true, got %+v", resp.CanSelect)
	}
}

func TestSchemaTable_MySQL_CanSelectProbeFails_IsFalse(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.MySQL)

	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = ? AND table_name = ?`,
	).WithArgs("appdb", "secrets").WillReturnRows(sqlmock.NewRows([]string{"table_type"}).AddRow("BASE TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`,
	).WithArgs("appdb", "secrets").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}).
		AddRow("id", "int", "NO"))

	mock.ExpectQuery(
		"SELECT 1 FROM `appdb`.`secrets` LIMIT 0",
	).WillReturnError(errPermissionDenied)

	resp := doSchemaTable(t, s, "/api/schema/table?schema=appdb&table=secrets")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.CanSelect == nil || *resp.CanSelect {
		t.Fatalf("expected can_select=false after a failed probe, got %+v", resp.CanSelect)
	}
}

func TestSchemaTable_MySQL_CanSelectSkippedForBadIdentifier(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.MySQL)

	// schema name has a character outside [A-Za-z0-9_]; the probe must never
	// be attempted (it can't be parameterized), so can_select stays unknown.
	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = ? AND table_name = ?`,
	).WithArgs("app-db", "orders").WillReturnRows(sqlmock.NewRows([]string{"table_type"}).AddRow("BASE TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`,
	).WithArgs("app-db", "orders").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}))

	resp := doSchemaTable(t, s, "/api/schema/table?schema=app-db&table=orders")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.CanSelect != nil {
		t.Fatalf("expected can_select to be skipped/unknown for a non-identifier schema name, got %+v", *resp.CanSelect)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSchemaTable_MSSQL_CanSelect(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.MSSQL)

	mock.ExpectQuery(
		`SELECT table_type FROM information_schema.tables WHERE table_schema = @p1 AND table_name = @p2`,
	).WithArgs("dbo", "customers").WillReturnRows(sqlmock.NewRows([]string{"table_type"}).AddRow("BASE TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = @p1 AND table_name = @p2 ORDER BY ordinal_position`,
	).WithArgs("dbo", "customers").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}).
		AddRow("id", "int", "NO"))

	mock.ExpectQuery(
		`SELECT HAS_PERMS_BY_NAME(@p1, 'OBJECT', 'SELECT')`,
	).WithArgs("dbo.customers").WillReturnRows(sqlmock.NewRows([]string{"has_perms_by_name"}).AddRow(int64(1)))

	resp := doSchemaTable(t, s, "/api/schema/table?schema=dbo&table=customers")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.CanSelect == nil || !*resp.CanSelect {
		t.Fatalf("expected can_select=true, got %+v", resp.CanSelect)
	}
}

func TestSchemaTable_Vertica_OK(t *testing.T) {
	s := New()
	mock := newMockSession(t, s, database.Vertica)

	mock.ExpectQuery(
		`SELECT 'TABLE' FROM v_catalog.tables WHERE table_schema = ? AND table_name = ? UNION ALL SELECT 'VIEW' FROM v_catalog.views WHERE table_schema = ? AND table_name = ?`,
	).WithArgs("public", "events", "public", "events").WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow("TABLE"))

	mock.ExpectQuery(
		`SELECT column_name, data_type, is_nullable FROM v_catalog.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`,
	).WithArgs("public", "events").WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"}).
		AddRow("id", "int", false))

	mock.ExpectQuery(
		`SELECT has_table_privilege($1, 'SELECT')`,
	).WithArgs("public.events").WillReturnRows(sqlmock.NewRows([]string{"has_table_privilege"}).AddRow(true))

	resp := doSchemaTable(t, s, "/api/schema/table?schema=public&table=events")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Type != "table" {
		t.Fatalf("expected type=table, got %q", resp.Type)
	}
	if len(resp.Columns) != 1 || resp.Columns[0].Nullable {
		t.Fatalf("unexpected columns: %+v", resp.Columns)
	}
}

// nthName generates deterministic, distinct identifiers for truncation
// tests: n0, n1, n2, ...
func nthName(n int) string {
	return "n" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// errPermissionDenied is a stand-in database error used across tests to
// exercise the "catalog query fails" and "can_select check fails" paths.
var errPermissionDenied = &mockDBError{msg: "permission denied"}

type mockDBError struct{ msg string }

func (e *mockDBError) Error() string { return e.msg }

// TestSchemaTable_ResponsesAreJSONSafe is a light sanity check that the
// handlers never crash on an odd combination of query params (e.g. only
// whitespace), returning a graceful error instead.
func TestSchemaTable_WhitespaceOnlyParams(t *testing.T) {
	s := New()
	newMockSession(t, s, database.PostgreSQL)

	resp := doSchemaTable(t, s, "/api/schema/table?schema=+&table=+")
	if resp.Error == "" {
		t.Fatalf("expected error for whitespace-only schema/table, got %+v", resp)
	}
	if !strings.Contains(resp.Error, "required") {
		t.Fatalf("expected 'required' in error, got %q", resp.Error)
	}
}
