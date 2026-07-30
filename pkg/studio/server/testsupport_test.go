package server

import (
	"database/sql"

	"github.com/conductorone/baton-sql/pkg/database"
)

// setSessionForTest injects a session directly, bypassing /api/connect, so
// handlers can be tested against an in-memory database. Test-only.
func (s *Server) setSessionForTest(db *sql.DB, engine database.DbEngine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
	s.engine = engine
	s.connected = true
}
