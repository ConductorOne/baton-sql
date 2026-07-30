// Package server exposes the baton-sql Studio engine (pkg/studio) over local
// HTTP for the Studio UI. It is a localhost-only development tool: a single
// mutex-guarded session (one DB connection at a time), no auth, no web
// framework — just stdlib net/http and encoding/json.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
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

	// static, when set via SetStatic, serves the Studio UI's static assets
	// at "/". Left nil, Handler falls back to a small coded page so "/"
	// never 404s even when no UI build is embedded.
	static fs.FS
}

// SetStatic sets the filesystem Handler serves static files from at "/".
// Typically this is the "web" subdirectory of an embed.FS built into the
// binary (see cmd/baton-sql-studio/serve.go).
func (s *Server) SetStatic(fsys fs.FS) {
	s.static = fsys
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

// Handler returns the http.Handler serving all Studio API routes, wrapped in
// localOnly so every route — API and static alike — is guarded against
// non-loopback Host/Origin requests. Later tasks register additional routes
// on the same mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/connect", s.handleConnect)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/generate", s.handleGenerate)
	mux.HandleFunc("/api/validate", s.handleValidate)
	mux.HandleFunc("/api/preview", s.handlePreview)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/disconnect", s.handleDisconnect)
	mux.Handle("/", s.staticHandler())
	return localOnly(mux)
}

// staticHandler serves the Studio UI's static assets from s.static, or a
// small coded fallback page when no static FS has been set (e.g. a build
// that doesn't embed the UI yet). Registered on "/", the ServeMux catch-all
// pattern, so it never shadows the more specific "/api/..." routes above.
func (s *Server) staticHandler() http.Handler {
	if s.static == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fallbackPageHTML))
		})
	}
	return http.FileServerFS(s.static)
}

// fallbackPageHTML is served at "/" when no static UI assets have been set
// via SetStatic, so the server is still reachable without a 404.
const fallbackPageHTML = `<!DOCTYPE html>
<html>
<head><title>baton-sql Studio</title></head>
<body>
<h1>baton-sql Studio</h1>
<p>API up. No static UI assets configured.</p>
</body>
</html>
`

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
