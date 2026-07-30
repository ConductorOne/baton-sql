package server

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/conductorone/baton-sql/pkg/database"
)

// doStatus issues GET /api/status against s and decodes the response.
func doStatus(t *testing.T, s *Server) statusResponse {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d, body %s", rec.Code, rec.Body.String())
	}
	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	return resp
}

// doDisconnect issues POST /api/disconnect against s and decodes the
// response.
func doDisconnect(t *testing.T, s *Server) disconnectResponse {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/disconnect", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d, body %s", rec.Code, rec.Body.String())
	}
	var resp disconnectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	return resp
}

func TestStatus_NotConnected(t *testing.T) {
	s := New()
	resp := doStatus(t, s)
	if resp.Connected {
		t.Fatalf("expected connected=false, got %+v", resp)
	}
	if resp.Engine != "" {
		t.Fatalf("expected empty engine, got %+v", resp)
	}
}

func TestStatus_Connected(t *testing.T) {
	s := New()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.setSessionForTest(db, database.MySQL)

	resp := doStatus(t, s)
	if !resp.Connected {
		t.Fatalf("expected connected=true, got %+v", resp)
	}
	if resp.Engine != "mysql" {
		t.Fatalf("expected engine mysql, got %+v", resp)
	}
}

func TestStatus_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("POST", "/api/status", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestDisconnect_WhenConnected(t *testing.T) {
	s := New()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s.setSessionForTest(db, database.MySQL)

	resp := doDisconnect(t, s)
	if !resp.OK {
		t.Fatalf("expected ok=true, got %+v", resp)
	}

	// The session's DB handle should have been closed, not just detached.
	if err := db.Ping(); err == nil {
		t.Fatalf("expected db to be closed after disconnect, but Ping succeeded")
	}

	after := doStatus(t, s)
	if after.Connected {
		t.Fatalf("expected connected=false after disconnect, got %+v", after)
	}
	if after.Engine != "" {
		t.Fatalf("expected empty engine after disconnect, got %+v", after)
	}
}

// TestDisconnect_WhenNotConnected proves disconnect is idempotent: calling it
// with no active session still returns ok=true rather than erroring.
func TestDisconnect_WhenNotConnected(t *testing.T) {
	s := New()
	resp := doDisconnect(t, s)
	if !resp.OK {
		t.Fatalf("expected ok=true when already disconnected, got %+v", resp)
	}
}

func TestDisconnect_MethodNotAllowed(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/disconnect", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
