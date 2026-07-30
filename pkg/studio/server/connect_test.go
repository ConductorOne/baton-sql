package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/conductorone/baton-sql/pkg/database"
)

func TestConnect_OK(t *testing.T) {
	s := New()
	s.connect = func(ctx context.Context, o database.ConnectOptions) (*sql.DB, database.DbEngine, error) {
		db, _ := sql.Open("sqlite", ":memory:")
		return db, database.MySQL, nil
	}
	body := `{"scheme":"mysql","host":"h","port":"3306","database":"d","user":"u","password":"p"}`
	req := httptest.NewRequest("POST", "/api/connect", strings.NewReader(body))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var resp connectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok, got %+v", resp)
	}
	if resp.Engine != "mysql" {
		t.Fatalf("expected engine mysql, got %+v", resp)
	}
}

func TestConnect_Failure_Is200NotOK(t *testing.T) {
	s := New()
	s.connect = func(ctx context.Context, o database.ConnectOptions) (*sql.DB, database.DbEngine, error) {
		return nil, database.Unknown, fmt.Errorf("dial tcp: refused")
	}
	req := httptest.NewRequest("POST", "/api/connect", strings.NewReader(`{"scheme":"mysql"}`))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var resp connectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.OK || resp.Error == "" {
		t.Fatalf("expected not-ok with error, got %+v", resp)
	}
}

func TestConnect_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/connect", nil)
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestConnect_MalformedJSON(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/connect", strings.NewReader(`{not json`))
	req.Host = "127.0.0.1" // localOnly guard requires a loopback Host.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	var resp connectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.OK || resp.Error == "" {
		t.Fatalf("expected not-ok with error, got %+v", resp)
	}
}
