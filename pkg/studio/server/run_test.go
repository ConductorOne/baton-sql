package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/conductorone/baton-sql/pkg/database"
)

// seedDB opens an in-memory sqlite database and creates table t(id, name)
// with n rows: (1,"name-1"), (2,"name-2"), ...
func seedDB(t *testing.T, n int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE t (id INTEGER, name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 1; i <= n; i++ {
		if _, err := db.Exec("INSERT INTO t (id, name) VALUES (?, ?)", i, fmt.Sprintf("name-%d", i)); err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}
	return db
}

func doRun(t *testing.T, s *Server, body string) runResponse {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/run", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d, body %s", rec.Code, rec.Body.String())
	}
	var resp runResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	return resp
}

func TestRun_OK(t *testing.T) {
	s := New()
	db := seedDB(t, 3)
	s.setSessionForTest(db, database.SQLite)

	resp := doRun(t, s, `{"query":"SELECT id, name FROM t ORDER BY id"}`)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Columns) != 2 || resp.Columns[0] != "id" || resp.Columns[1] != "name" {
		t.Fatalf("unexpected columns: %+v", resp.Columns)
	}
	if resp.RowCount != 3 {
		t.Fatalf("expected row_count 3, got %d", resp.RowCount)
	}
	if resp.Truncated {
		t.Fatalf("expected not truncated")
	}
	if len(resp.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(resp.Rows))
	}
	first := resp.Rows[0]
	if len(first) != 2 {
		t.Fatalf("expected 2 cells in first row, got %+v", first)
	}
	// JSON round-trip normalizes numerics to float64.
	if id, ok := first[0].(float64); !ok || id != 1 {
		t.Fatalf("expected first row id==1, got %+v (%T)", first[0], first[0])
	}
	if name, ok := first[1].(string); !ok || name != "name-1" {
		t.Fatalf("expected first row name==name-1, got %+v (%T)", first[1], first[1])
	}
}

func TestRun_Truncated(t *testing.T) {
	s := New()
	db := seedDB(t, 150)
	s.setSessionForTest(db, database.SQLite)

	resp := doRun(t, s, `{"query":"SELECT id, name FROM t ORDER BY id"}`)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.RowCount != 100 {
		t.Fatalf("expected row_count 100, got %d", resp.RowCount)
	}
	if !resp.Truncated {
		t.Fatalf("expected truncated==true")
	}
	if len(resp.Rows) != 100 {
		t.Fatalf("expected 100 rows returned, got %d", len(resp.Rows))
	}
}

func TestRun_TokenPresent(t *testing.T) {
	s := New()
	db := seedDB(t, 3)
	s.setSessionForTest(db, database.SQLite)

	resp := doRun(t, s, `{"query":"SELECT id FROM t WHERE id = ?<rid>"}`)
	if resp.Error == "" {
		t.Fatalf("expected error for query with ?<var> token, got %+v", resp)
	}
	if !strings.Contains(resp.Error, "?<var>") {
		t.Fatalf("expected error to mention ?<var> tokens, got %q", resp.Error)
	}
}

func TestRun_NotConnected(t *testing.T) {
	s := New()
	resp := doRun(t, s, `{"query":"SELECT 1"}`)
	if resp.Error != "not connected" {
		t.Fatalf("expected 'not connected' error, got %+v", resp)
	}
}

func TestRun_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/run", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestRun_InvalidQuery(t *testing.T) {
	s := New()
	db := seedDB(t, 3)
	s.setSessionForTest(db, database.SQLite)

	resp := doRun(t, s, `{"query":"SELECT * FROM nonexistent_table"}`)
	if resp.Error == "" {
		t.Fatalf("expected error for invalid query, got %+v", resp)
	}
}

func TestRun_MalformedJSON(t *testing.T) {
	s := New()
	db := seedDB(t, 3)
	s.setSessionForTest(db, database.SQLite)

	resp := doRun(t, s, `{not json`)
	if resp.Error == "" {
		t.Fatalf("expected error for malformed JSON body, got %+v", resp)
	}
}
