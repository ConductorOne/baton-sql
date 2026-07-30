package server

import (
	"net/http"

	"github.com/conductorone/baton-sql/pkg/database"
)

// statusResponse is the body returned by GET /api/status.
type statusResponse struct {
	Connected bool   `json:"connected"`
	Engine    string `json:"engine,omitempty"`
}

// handleStatus implements GET /api/status: it reports whether the Studio
// session currently holds a live database connection and, if so, which
// engine it's connected to.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	s.mu.Lock()
	connected := s.connected
	engine := s.engine
	s.mu.Unlock()

	resp := statusResponse{Connected: connected}
	if connected {
		resp.Engine = engineName(engine)
	}
	writeJSON(w, http.StatusOK, resp)
}

// disconnectResponse is the body returned by POST /api/disconnect.
type disconnectResponse struct {
	OK bool `json:"ok"`
}

// handleDisconnect implements POST /api/disconnect: it closes the session's
// live DB connection, if any, and clears the session. It is idempotent —
// calling it while already disconnected is a no-op success — so the UI can
// call it unconditionally without first checking status.
func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	s.mu.Lock()
	db := s.db
	s.db = nil
	s.connected = false
	s.engine = database.Unknown
	s.mu.Unlock()

	if db != nil {
		_ = db.Close()
	}

	writeJSON(w, http.StatusOK, disconnectResponse{OK: true})
}
