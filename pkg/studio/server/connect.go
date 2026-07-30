package server

import (
	"encoding/json"
	"net/http"

	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/conductorone/baton-sql/pkg/studio"
)

// connectResponse is the body returned by POST /api/connect. A failed
// connection is a normal outcome for this endpoint (the user is exploring
// credentials), so it is always reported with HTTP 200 and OK=false rather
// than an HTTP error status.
type connectResponse struct {
	OK     bool   `json:"ok"`
	Engine string `json:"engine,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleConnect implements POST /api/connect: it decodes a studio.ConnectConfig,
// attempts to open a database connection (via s.connect, normally
// database.Connect), and on success replaces the server's session.
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, connectResponse{Error: "method not allowed"})
		return
	}

	var cfg studio.ConnectConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusOK, connectResponse{OK: false, Error: "invalid request body: " + err.Error()})
		return
	}

	opts := database.ConnectOptions{
		Scheme:   cfg.Scheme,
		Host:     cfg.Host,
		Port:     cfg.Port,
		Database: cfg.Database,
		User:     cfg.User,
		Password: cfg.Password,
		Params:   cfg.Params,
	}

	ctx := r.Context()
	db, engine, err := s.connect(ctx, opts)
	if err != nil {
		writeJSON(w, http.StatusOK, connectResponse{OK: false, Error: err.Error()})
		return
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		writeJSON(w, http.StatusOK, connectResponse{OK: false, Error: err.Error()})
		return
	}

	s.mu.Lock()
	prev := s.db
	s.db = db
	s.engine = engine
	s.connected = true
	s.mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}

	writeJSON(w, http.StatusOK, connectResponse{OK: true, Engine: engineName(engine)})
}
