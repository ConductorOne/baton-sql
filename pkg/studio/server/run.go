package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"github.com/conductorone/baton-sql/pkg/database"
)

// maxRows bounds the number of rows returned by /api/run: the Studio UI is a
// preview tool, not a data export path.
const maxRows = 100

// queryTimeout bounds how long a single live query may run before its
// context is cancelled.
const queryTimeout = 30 * time.Second

// tokenRegex matches bsql resource-var tokens (?<name> or ?<name|ident>).
// This task only needs to detect their presence; Task 3 adds substitution
// and mirrors this same pattern (bsql's queryOptRegex).
var tokenRegex = regexp.MustCompile(`\?\<([a-zA-Z0-9_]+)(?:\|[a-zA-Z0-9_]+)?\>`)

// runRequest is the body of POST /api/run.
type runRequest struct {
	Query string `json:"query"`
	// Vars supplies sample values for ?<var> tokens in Query. Unused by this
	// task: a query containing a token is rejected with a clear error so
	// Task 3 has a seam to implement substitution.
	Vars map[string]string `json:"vars,omitempty"`
}

// runResponse is the body returned by POST /api/run. RowCount and Truncated
// are always present (no omitempty) so the caller can distinguish "zero
// rows" from "field absent".
type runResponse struct {
	Columns   []string `json:"columns,omitempty"`
	Rows      [][]any  `json:"rows,omitempty"`
	RowCount  int      `json:"row_count"`
	Truncated bool     `json:"truncated"`
	Error     string   `json:"error,omitempty"`
}

// encodeValue makes a scanned driver value JSON-safe: []byte (the generic
// representation many drivers use for text/blob columns) becomes a string;
// everything else (nil, numerics, bool, time.Time, ...) passes through
// unchanged since encoding/json already handles those types.
func encodeValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// setSessionForTest injects a session directly, bypassing /api/connect, so
// handlers can be tested against an in-memory database. Test-only.
func (s *Server) setSessionForTest(db *sql.DB, engine database.DbEngine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
	s.engine = engine
	s.connected = true
}

// handleRun implements POST /api/run: it executes the caller's single query
// against the active session's database, read-only and bounded (maxRows,
// queryTimeout), and returns JSON-safe rows.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, runResponse{Error: "method not allowed"})
		return
	}

	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusOK, runResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	s.mu.Lock()
	db := s.db
	connected := s.connected
	s.mu.Unlock()

	if !connected || db == nil {
		writeJSON(w, http.StatusOK, runResponse{Error: "not connected"})
		return
	}

	if tokenRegex.MatchString(req.Query) {
		writeJSON(w, http.StatusOK, runResponse{Error: "query has ?<var> tokens; provide sample vars"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, req.Query)
	if err != nil {
		writeJSON(w, http.StatusOK, runResponse{Error: err.Error()})
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		writeJSON(w, http.StatusOK, runResponse{Error: err.Error()})
		return
	}

	out := [][]any{}
	truncated := false
	for rows.Next() {
		if len(out) >= maxRows {
			truncated = true
			break
		}
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			writeJSON(w, http.StatusOK, runResponse{Error: err.Error()})
			return
		}
		for i := range cells {
			cells[i] = encodeValue(cells[i])
		}
		out = append(out, cells)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusOK, runResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, runResponse{
		Columns:   cols,
		Rows:      out,
		RowCount:  len(out),
		Truncated: truncated,
	})
}
