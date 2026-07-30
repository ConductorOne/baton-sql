package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// TestLocalOnly_RejectsNonLoopbackHost proves the DNS-rebinding defense: a
// request whose Host header names an attacker-controlled hostname is
// rejected before it reaches any handler, regardless of which route it
// targets.
func TestLocalOnly_RejectsNonLoopbackHost(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "evil.com"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for non-loopback Host, got %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestLocalOnly_AllowsLoopbackHost proves a loopback Host (what a real local
// curl/browser request carries) passes the guard and reaches the handler.
func TestLocalOnly_AllowsLoopbackHost(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "127.0.0.1:8787"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 for loopback Host, got %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestLocalOnly_AllowsLocalhostHost proves the "localhost" hostname (as
// opposed to the 127.0.0.1 literal) is also accepted.
func TestLocalOnly_AllowsLocalhostHost(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "localhost:8787"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 for localhost Host, got %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestLocalOnly_RejectsNonLoopbackOrigin proves a cross-site browser request
// (loopback Host, but an Origin header naming another site) is rejected,
// even though the Host check alone would have passed it.
func TestLocalOnly_RejectsNonLoopbackOrigin(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for non-loopback Origin, got %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestLocalOnly_AllowsLoopbackOrigin proves a same-machine Origin (e.g. a
// Studio UI served from and calling back to 127.0.0.1/localhost) passes.
func TestLocalOnly_AllowsLoopbackOrigin(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "http://localhost:8787")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 for loopback Origin, got %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestLocalOnly_NoOriginHeaderPasses proves requests with no Origin header at
// all (curl, same-origin navigations) are not penalized by the Origin check.
func TestLocalOnly_NoOriginHeaderPasses(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "127.0.0.1:8787"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 with no Origin header, got %d, body %s", rec.Code, rec.Body.String())
	}
	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// TestLocalOnly_RejectsNonLoopbackHost_StaticRoute proves the guard applies
// to the static "/" catch-all route too, not just "/api/...".
func TestLocalOnly_RejectsNonLoopbackHost_StaticRoute(t *testing.T) {
	s := New()
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "evil.com"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for non-loopback Host on static route, got %d, body %s", rec.Code, rec.Body.String())
	}
}
