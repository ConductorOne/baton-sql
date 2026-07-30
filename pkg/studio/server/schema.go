package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/conductorone/baton-sql/pkg/database"
)

// schemaTimeout bounds how long a single schema-discovery request (which may
// issue several catalog queries) may run. It is intentionally shorter than
// queryTimeout (used by /api/run) since catalog queries against
// information_schema-style views are expected to be fast, metadata-only
// lookups rather than user data scans.
const schemaTimeout = 10 * time.Second

// maxSchemas and maxTablesPerSchema bound the amount of catalog metadata a
// single request returns: the Studio UI uses this to populate a picker, not
// to enumerate an entire warehouse.
const (
	maxSchemas         = 200
	maxTablesPerSchema = 500
)

// mysqlIdentRegex validates a bare SQL identifier before it is interpolated
// (backtick-quoted) into a probe query for MySQL's can_select check. MySQL
// has no privilege-introspection function equivalent to postgres's
// has_table_privilege, so the only way to test SELECT access is to attempt a
// zero-row SELECT against the actual table — and unlike schema/table name
// values used as bind parameters elsewhere in this file, an identifier in
// that position can never be a bind parameter. Restricting it to
// [A-Za-z0-9_] before quoting closes that gap.
var mysqlIdentRegex = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// tableInfo describes one table or view within a schema, without columns
// (see schemaTableResponse for a single table's columns).
type tableInfo struct {
	Name string `json:"name"`
	// Type is normalized to lowercase "table" or "view".
	Type string `json:"type"`
}

// schemaInfo describes one schema and the tables/views directly in it.
type schemaInfo struct {
	Name   string      `json:"name"`
	Tables []tableInfo `json:"tables"`
	// Truncated reports whether this schema's table list was cut off at
	// maxTablesPerSchema.
	Truncated bool `json:"truncated"`
}

// schemaResponse is the body returned by GET /api/schema.
type schemaResponse struct {
	Engine  string       `json:"engine,omitempty"`
	Schemas []schemaInfo `json:"schemas,omitempty"`
	// Truncated reports whether the schema list itself was cut off at
	// maxSchemas. It is distinct from each schemaInfo's own Truncated, which
	// tracks that schema's table list.
	Truncated bool   `json:"truncated,omitempty"`
	Error     string `json:"error,omitempty"`
}

// columnInfo describes one column of a table. IsPrimaryKey/References are a
// nice-to-have left unpopulated in this version: v1 focuses on getting
// name/type/nullable right across all four supported engines rather than
// keying/foreign-key introspection, which varies more per dialect.
type columnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// schemaTableResponse is the body returned by GET /api/schema/table.
// CanSelect is a *bool so "unknown" (the privilege check itself failed, or
// couldn't safely be attempted) is distinguishable from a confirmed
// true/false: it is omitted from the JSON response in that case.
type schemaTableResponse struct {
	Schema    string       `json:"schema,omitempty"`
	Table     string       `json:"table,omitempty"`
	Type      string       `json:"type,omitempty"`
	CanSelect *bool        `json:"can_select,omitempty"`
	Columns   []columnInfo `json:"columns,omitempty"`
	Error     string       `json:"error,omitempty"`
}

// engineSupportsSchemaDiscovery reports whether schema.go has catalog SQL for
// engine. Oracle, HDB, and Unknown fall through to a graceful per-request
// error rather than being wired up with guessed catalog SQL.
func engineSupportsSchemaDiscovery(engine database.DbEngine) bool {
	switch engine {
	case database.PostgreSQL, database.MySQL, database.MSSQL, database.Vertica:
		return true
	default:
		return false
	}
}

// unsupportedEngineError formats the graceful, non-crashing error returned
// for engines schema.go doesn't have catalog SQL for.
func unsupportedEngineError(engine database.DbEngine) error {
	return fmt.Errorf("schema discovery is not yet supported for %s", engineName(engine))
}

// normalizeTableType maps a dialect's raw table-type string (e.g. postgres's
// "BASE TABLE", mysql's "BASE TABLE"/"SYSTEM VIEW", mssql's "BASE TABLE") to
// the lowercase "table"/"view" this package's JSON responses always use.
func normalizeTableType(raw string) string {
	if strings.Contains(strings.ToLower(raw), "view") {
		return "view"
	}
	return "table"
}

// normalizeNullable maps a scanned is_nullable catalog value to a bool.
// postgres/mysql/mssql's information_schema.columns.is_nullable is the
// ANSI-standard "YES"/"NO" string. Vertica's v_catalog.columns.is_nullable is
// reported as a native boolean by some driver versions and as "t"/"f" text by
// others depending on how the driver decodes it — this is part of the
// vertica path called out in the package docs as unverified against a live
// instance, so both shapes are handled defensively here rather than assuming
// the ANSI string form.
func normalizeNullable(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "yes" || s == "true" || s == "t" || s == "1"
	case []byte:
		return normalizeNullable(string(t))
	default:
		return false
	}
}

// schemataQuery returns the catalog SQL to list a database's user (non-system)
// schema names for engine. schema/table *values* are always passed as bind
// parameters elsewhere in this file; the exclusion lists here are fixed,
// hardcoded literals baked into the query text, never derived from request
// input, so they carry no injection risk.
func schemataQuery(engine database.DbEngine) (string, error) {
	switch engine {
	case database.PostgreSQL:
		return `SELECT schema_name FROM information_schema.schemata ` +
			`WHERE schema_name NOT IN ('pg_catalog', 'information_schema') ` +
			`ORDER BY schema_name`, nil
	case database.MySQL:
		return `SELECT schema_name FROM information_schema.schemata ` +
			`WHERE schema_name NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys') ` +
			`ORDER BY schema_name`, nil
	case database.MSSQL:
		return `SELECT schema_name FROM information_schema.schemata ` +
			`WHERE schema_name NOT IN ('sys', 'INFORMATION_SCHEMA', 'guest') ` +
			`AND schema_name NOT LIKE 'db\_%' ESCAPE '\' ` +
			`ORDER BY schema_name`, nil
	case database.Vertica:
		// Unverified against a live Vertica instance: HAS_SCHEMA_PRIVILEGE is
		// documented Vertica syntax, but the exact catalog/privilege behavior
		// here has not been exercised against a real cluster.
		return `SELECT schema_name FROM v_catalog.schemata ` +
			`WHERE HAS_SCHEMA_PRIVILEGE(schema_name, 'USAGE') ` +
			`ORDER BY schema_name`, nil
	default:
		return "", unsupportedEngineError(engine)
	}
}

// listSchemas runs schemataQuery for engine and returns up to maxSchemas
// schema names plus whether the result was truncated.
func listSchemas(ctx context.Context, db *sql.DB, engine database.DbEngine) ([]string, bool, error) {
	query, err := schemataQuery(engine)
	if err != nil {
		return nil, false, err
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	names := []string{}
	truncated := false
	for rows.Next() {
		if len(names) >= maxSchemas {
			truncated = true
			break
		}
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, false, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return names, truncated, nil
}

// tablesQuery returns the catalog SQL to list the tables and views directly
// in one schema for engine, plus the (already-bound) args to pass alongside
// it — schemaName is always supplied as a bind parameter via placeholderFor,
// never string-concatenated into the query text.
func tablesQuery(engine database.DbEngine, schemaName string) (string, []any, error) {
	switch engine {
	case database.PostgreSQL, database.MySQL, database.MSSQL:
		q := fmt.Sprintf(
			"SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = %s ORDER BY table_name",
			placeholderFor(engine, 1),
		)
		return q, []any{schemaName}, nil
	case database.Vertica:
		// Unverified against a live Vertica instance.
		q := fmt.Sprintf(
			"SELECT table_name, 'TABLE' FROM v_catalog.tables WHERE table_schema = %s "+
				"UNION ALL SELECT table_name, 'VIEW' FROM v_catalog.views WHERE table_schema = %s "+
				"ORDER BY table_name",
			placeholderFor(engine, 1), placeholderFor(engine, 2),
		)
		return q, []any{schemaName, schemaName}, nil
	default:
		return "", nil, unsupportedEngineError(engine)
	}
}

// listTables runs tablesQuery for (engine, schemaName) and returns up to
// maxTablesPerSchema tables/views plus whether the result was truncated.
func listTables(ctx context.Context, db *sql.DB, engine database.DbEngine, schemaName string) ([]tableInfo, bool, error) {
	query, args, err := tablesQuery(engine, schemaName)
	if err != nil {
		return nil, false, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	tables := []tableInfo{}
	truncated := false
	for rows.Next() {
		if len(tables) >= maxTablesPerSchema {
			truncated = true
			break
		}
		var name, rawType string
		if err := rows.Scan(&name, &rawType); err != nil {
			return nil, false, err
		}
		tables = append(tables, tableInfo{Name: name, Type: normalizeTableType(rawType)})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return tables, truncated, nil
}

// handleSchema implements GET /api/schema: with no query string it lists
// every user schema in the connected database along with the tables/views
// directly in each; with ?schema=<name> it returns just that one schema (for
// the UI's lazy-expand case). Catalog-query failures are reported as HTTP 200
// {error:...} rather than an HTTP error status or a panic, matching the
// degrade-gracefully convention used by handleRun and handleConnect.
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, schemaResponse{Error: "method not allowed"})
		return
	}

	s.mu.Lock()
	db := s.db
	engine := s.engine
	connected := s.connected
	s.mu.Unlock()

	if !connected || db == nil {
		writeJSON(w, http.StatusOK, schemaResponse{Error: "not connected"})
		return
	}

	if !engineSupportsSchemaDiscovery(engine) {
		writeJSON(w, http.StatusOK, schemaResponse{Error: unsupportedEngineError(engine).Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), schemaTimeout)
	defer cancel()

	requested := strings.TrimSpace(r.URL.Query().Get("schema"))

	var (
		names            []string
		schemasTruncated bool
	)
	if requested != "" {
		names = []string{requested}
	} else {
		var err error
		names, schemasTruncated, err = listSchemas(ctx, db, engine)
		if err != nil {
			writeJSON(w, http.StatusOK, schemaResponse{Engine: engineName(engine), Error: err.Error()})
			return
		}
	}

	schemas := make([]schemaInfo, 0, len(names))
	for _, name := range names {
		tables, truncated, err := listTables(ctx, db, engine, name)
		if err != nil {
			writeJSON(w, http.StatusOK, schemaResponse{Engine: engineName(engine), Error: err.Error()})
			return
		}
		schemas = append(schemas, schemaInfo{Name: name, Tables: tables, Truncated: truncated})
	}

	writeJSON(w, http.StatusOK, schemaResponse{
		Engine:    engineName(engine),
		Schemas:   schemas,
		Truncated: schemasTruncated,
	})
}

// columnsQuery returns the catalog SQL to list one table's columns for
// engine, in ordinal order.
func columnsQuery(engine database.DbEngine) (string, error) {
	switch engine {
	case database.PostgreSQL, database.MySQL, database.MSSQL:
		return fmt.Sprintf(
			"SELECT column_name, data_type, is_nullable FROM information_schema.columns "+
				"WHERE table_schema = %s AND table_name = %s ORDER BY ordinal_position",
			placeholderFor(engine, 1), placeholderFor(engine, 2),
		), nil
	case database.Vertica:
		// Unverified against a live Vertica instance.
		return fmt.Sprintf(
			"SELECT column_name, data_type, is_nullable FROM v_catalog.columns "+
				"WHERE table_schema = %s AND table_name = %s ORDER BY ordinal_position",
			placeholderFor(engine, 1), placeholderFor(engine, 2),
		), nil
	default:
		return "", unsupportedEngineError(engine)
	}
}

// listColumns runs columnsQuery for (engine, schemaName, tableName).
func listColumns(ctx context.Context, db *sql.DB, engine database.DbEngine, schemaName, tableName string) ([]columnInfo, error) {
	query, err := columnsQuery(engine)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := []columnInfo{}
	for rows.Next() {
		var (
			name, dataType string
			nullableRaw    any
		)
		if err := rows.Scan(&name, &dataType, &nullableRaw); err != nil {
			return nil, err
		}
		columns = append(columns, columnInfo{
			Name:     name,
			Type:     dataType,
			Nullable: normalizeNullable(nullableRaw),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// tableType looks up whether (schemaName, tableName) is a table or a view.
// A no-such-table result (sql.ErrNoRows) is not treated as an error — it
// yields an empty type string, since the caller may still want partial
// information (e.g. a since-dropped table) rather than a hard failure.
func tableType(ctx context.Context, db *sql.DB, engine database.DbEngine, schemaName, tableName string) (string, error) {
	var query string
	var args []any
	switch engine {
	case database.PostgreSQL, database.MySQL, database.MSSQL:
		query = fmt.Sprintf(
			"SELECT table_type FROM information_schema.tables WHERE table_schema = %s AND table_name = %s",
			placeholderFor(engine, 1), placeholderFor(engine, 2),
		)
		args = []any{schemaName, tableName}
	case database.Vertica:
		// Unverified against a live Vertica instance.
		query = fmt.Sprintf(
			"SELECT 'TABLE' FROM v_catalog.tables WHERE table_schema = %s AND table_name = %s "+
				"UNION ALL SELECT 'VIEW' FROM v_catalog.views WHERE table_schema = %s AND table_name = %s",
			placeholderFor(engine, 1), placeholderFor(engine, 2), placeholderFor(engine, 3), placeholderFor(engine, 4),
		)
		args = []any{schemaName, tableName, schemaName, tableName}
	default:
		return "", unsupportedEngineError(engine)
	}

	var raw string
	err := db.QueryRowContext(ctx, query, args...).Scan(&raw)
	switch {
	case err == nil:
		return normalizeTableType(raw), nil
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	default:
		return "", err
	}
}

// canSelect probes whether the current DB session has SELECT access to
// (schemaName, tableName), engine-appropriately. It returns nil ("unknown")
// rather than an error whenever the check itself can't be answered
// confidently, per the package's degrade-gracefully convention: an
// unanswerable can_select must never fail the whole /api/schema/table
// request.
func canSelect(ctx context.Context, db *sql.DB, engine database.DbEngine, schemaName, tableName string) *bool {
	qualified := schemaName + "." + tableName

	switch engine {
	case database.PostgreSQL, database.Vertica:
		// Unverified against a live Vertica instance (postgres's
		// has_table_privilege is well-established; whether Vertica's
		// dialect-compatible function behaves identically for a
		// "schema.table" argument has not been confirmed).
		var ok bool
		query := fmt.Sprintf("SELECT has_table_privilege(%s, 'SELECT')", placeholderFor(engine, 1))
		if err := db.QueryRowContext(ctx, query, qualified).Scan(&ok); err != nil {
			return nil
		}
		return &ok

	case database.MSSQL:
		var raw any
		query := fmt.Sprintf("SELECT HAS_PERMS_BY_NAME(%s, 'OBJECT', 'SELECT')", placeholderFor(engine, 1))
		if err := db.QueryRowContext(ctx, query, qualified).Scan(&raw); err != nil {
			return nil
		}
		ok, valid := asBool(raw)
		if !valid {
			return nil
		}
		return &ok

	case database.MySQL:
		// HAS_PERMS_BY_NAME/has_table_privilege has no MySQL equivalent, so
		// the only way to test SELECT access is a real, zero-row probe
		// query. The identifiers can't be bind parameters in this position,
		// so they're validated against a strict allow-list before being
		// interpolated (backtick-quoted) into the query text.
		if !mysqlIdentRegex.MatchString(schemaName) || !mysqlIdentRegex.MatchString(tableName) {
			return nil
		}
		probe := fmt.Sprintf("SELECT 1 FROM `%s`.`%s` LIMIT 0", schemaName, tableName)
		rows, err := db.QueryContext(ctx, probe)
		if err != nil {
			f := false
			return &f
		}
		_ = rows.Close()
		t := true
		return &t

	default:
		return nil
	}
}

// asBool interprets a scanned driver value as a bool, for engines (like
// mssql's HAS_PERMS_BY_NAME, which returns a bit/int) that don't hand back a
// native Go bool. The second return is false when v is nil or of a type this
// function doesn't recognize, so the caller can treat the check as
// unanswered rather than guessing.
func asBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case int64:
		return t != 0, true
	case float64:
		return t != 0, true
	case []byte:
		return asBool(string(t))
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "1", "true", "t", "yes":
			return true, true
		case "0", "false", "f", "no":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

// handleSchemaTable implements GET /api/schema/table?schema=<s>&table=<t>: it
// returns one table's type, columns, and best-effort SELECT-access check.
// Like handleSchema, catalog-query failures degrade to HTTP 200 {error:...}
// rather than an HTTP error status.
func (s *Server) handleSchemaTable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, schemaTableResponse{Error: "method not allowed"})
		return
	}

	s.mu.Lock()
	db := s.db
	engine := s.engine
	connected := s.connected
	s.mu.Unlock()

	if !connected || db == nil {
		writeJSON(w, http.StatusOK, schemaTableResponse{Error: "not connected"})
		return
	}

	if !engineSupportsSchemaDiscovery(engine) {
		writeJSON(w, http.StatusOK, schemaTableResponse{Error: unsupportedEngineError(engine).Error()})
		return
	}

	schemaName := strings.TrimSpace(r.URL.Query().Get("schema"))
	tableName := strings.TrimSpace(r.URL.Query().Get("table"))
	if schemaName == "" || tableName == "" {
		writeJSON(w, http.StatusOK, schemaTableResponse{Error: "schema and table query parameters are required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), schemaTimeout)
	defer cancel()

	typ, err := tableType(ctx, db, engine, schemaName, tableName)
	if err != nil {
		writeJSON(w, http.StatusOK, schemaTableResponse{Schema: schemaName, Table: tableName, Error: err.Error()})
		return
	}

	columns, err := listColumns(ctx, db, engine, schemaName, tableName)
	if err != nil {
		writeJSON(w, http.StatusOK, schemaTableResponse{Schema: schemaName, Table: tableName, Type: typ, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, schemaTableResponse{
		Schema:    schemaName,
		Table:     tableName,
		Type:      typ,
		CanSelect: canSelect(ctx, db, engine, schemaName, tableName),
		Columns:   columns,
	})
}
