// Package server exposes the baton-sql Studio engine (pkg/studio) over local
// HTTP for the Studio UI. It is a localhost-only development tool: a single
// mutex-guarded session (one DB connection at a time), no auth, no web
// framework — just stdlib net/http and encoding/json.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
)

// Server holds the single local Studio session: an optional live DB
// connection plus the CEL environment used for preview/transform evaluation.
// All session fields are guarded by mu since handlers may run concurrently.
type Server struct {
	mu        sync.Mutex
	db        *sql.DB
	engine    database.DbEngine
	celEnv    *bcel.Env
	connected bool

	// connect is injectable so handlers can be tested without a live
	// database. Defaults to database.Connect.
	connect func(context.Context, database.ConnectOptions) (*sql.DB, database.DbEngine, error)
}

// New builds a Server ready to serve. The CEL environment is built once at
// startup and reused for the lifetime of the process.
func New() *Server {
	s := &Server{
		connect: database.Connect,
	}
	celEnv, err := bcel.NewEnv(context.Background())
	if err != nil {
		// bcel.NewEnv only fails if the CEL environment options themselves are
		// invalid (a programming error in this package or bcel), never from
		// runtime/user input, so a panic here surfaces the bug immediately
		// rather than leaving the Server in a half-built, unusable state.
		panic("studio/server: failed to build CEL environment: " + err.Error())
	}
	s.celEnv = celEnv
	return s
}

// Handler returns the http.Handler serving all Studio API routes. Later
// tasks register additional routes on the same mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/connect", s.handleConnect)
	mux.HandleFunc("/api/run", s.handleRun)
	return mux
}

// engineName maps a database.DbEngine to the lowercase scheme string used in
// ConnectConfig/ConnectOptions and in JSON responses.
func engineName(e database.DbEngine) string {
	switch e {
	case database.MySQL:
		return "mysql"
	case database.PostgreSQL:
		return "postgres"
	case database.SQLite:
		return "sqlite"
	case database.MSSQL:
		return "sqlserver"
	case database.Oracle:
		return "oracle"
	case database.HDB:
		return "hdb"
	case database.Vertica:
		return "vertica"
	default:
		return "unknown"
	}
}

// writeJSON encodes v as JSON to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
