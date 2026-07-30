package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
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
// It mirrors bsql's queryOptRegex (pkg/bsql/query.go); the optional |ident
// group is captured but ignored here (see substituteTokens).
var tokenRegex = regexp.MustCompile(`\?\<([a-zA-Z0-9_]+)(?:\|[a-zA-Z0-9_]+)?\>`)

// runRequest is the body of POST /api/run.
type runRequest struct {
	Query string `json:"query"`
	// Vars supplies sample values for ?<var> tokens in Query, keyed by
	// token name (e.g. "rid" for "?<rid>").
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

// substituteTokens replaces each bsql-style ?<name> (or ?<name|ident>) token
// in query with the target engine's positional placeholder, in the order the
// tokens appear. For each token whose name is present in vars, the sample
// value is appended to args and the token is replaced with the placeholder;
// for each token whose name is absent from vars, the name is collected into
// missing (and the token is left unrewritten) so the caller can reject the
// query before execution rather than run it with a dangling token.
func substituteTokens(query string, vars map[string]string, engine database.DbEngine) (rewritten string, args []any, missing []string) {
	seenMissing := make(map[string]bool)
	rewritten = tokenRegex.ReplaceAllStringFunc(query, func(tok string) string {
		m := tokenRegex.FindStringSubmatch(tok)
		name := m[1]
		val, ok := vars[name]
		if !ok {
			if !seenMissing[name] {
				missing = append(missing, name)
				seenMissing[name] = true
			}
			return tok
		}
		args = append(args, val)
		return placeholderFor(engine, len(args))
	})
	return rewritten, args, missing
}

// placeholderFor returns the nth (1-based) positional placeholder for engine,
// mirroring bsql's getNextPlaceholder (pkg/bsql/query.go). Engines without an
// explicit case (e.g. HDB, Unknown) fall back to "?".
func placeholderFor(engine database.DbEngine, n int) string {
	switch engine {
	case database.PostgreSQL:
		return fmt.Sprintf("$%d", n)
	case database.MSSQL:
		return fmt.Sprintf("@p%d", n)
	case database.Oracle:
		return fmt.Sprintf(":%d", n)
	case database.MySQL, database.SQLite, database.Vertica:
		return "?"
	default:
		return "?"
	}
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
	engine := s.engine
	connected := s.connected
	s.mu.Unlock()

	if !connected || db == nil {
		writeJSON(w, http.StatusOK, runResponse{Error: "not connected"})
		return
	}

	query := req.Query
	var args []any
	if tokenRegex.MatchString(query) {
		rewritten, substArgs, missing := substituteTokens(query, req.Vars, engine)
		if len(missing) > 0 {
			writeJSON(w, http.StatusOK, runResponse{Error: "missing sample values for: " + strings.Join(missing, ", ")})
			return
		}
		query = rewritten
		args = substArgs
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, query, args...)
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
