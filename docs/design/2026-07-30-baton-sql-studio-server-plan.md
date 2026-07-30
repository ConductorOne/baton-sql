# baton-sql Studio — Local Server + Live Query Runner (Plan 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A `baton-sql-studio serve` binary that exposes the engine (Plan 1) over local HTTP, connects to a real database, runs queries live, and serves the static UI — the backend the Plan 3 wizard talks to.

**Architecture:** New package `pkg/studio/server` holds an HTTP `Server` with a single mutex-guarded session (`*sql.DB` + `database.DbEngine` + `*bcel.Env`). JSON endpoints wrap the engine's pure functions (`studio.Generate`/`Validate`/`PreviewField`) and add two stateful capabilities: connect-to-DB and run-a-query. The `serve` subcommand in `cmd/baton-sql-studio` wires routes + serves an embedded static dir. DB access is injectable (`connectFn`) so handlers are testable without a live database.

**Tech Stack:** Go 1.25.2 stdlib `net/http` + `encoding/json` (no web framework); `net/http/httptest` for tests; in-memory `modernc.org/sqlite` for the query-runner tests; reuse `pkg/studio`, `pkg/database`, `pkg/bcel`.

## Global Constraints

- Module path `github.com/conductorone/baton-sql`; Go 1.25.2.
- **No web framework, no new third-party deps** — stdlib `net/http` only.
- Never modify `pkg/bsql`/`pkg/bcel`/`pkg/database`; consume the engine (`pkg/studio`) and these packages via their exported API only.
- **Single local session** (this is a localhost dev tool): one connection at a time, guarded by a `sync.Mutex`. No auth, bind to `127.0.0.1` only.
- The query runner is **read-only and bounded**: cap returned rows at `maxRows = 100`, apply a context timeout (`queryTimeout = 30s`), and never execute anything but the caller's single statement. Report truncation explicitly.
- All row values must be **JSON-safe**: `[]byte`→string, `nil`→null, driver numerics/time preserved; never return a raw `driver.Value` that fails `json.Marshal`.
- Engine API consumed (Plan 1, stable): `studio.Generate(*Spec) ([]byte,error)`; `studio.Validate(ctx,*Spec,studio.ValidateOptions{DB,DBEngine}) (*studio.Report,error)`; `studio.PreviewField(ctx,*bcel.Env,studio.FieldMapping,map[string]any) (string,error)`; types `studio.Spec`, `studio.FieldMapping`, `studio.ConnectConfig`, `studio.Report`.
- DB connect API: `database.Connect(ctx, database.ConnectOptions{DSN,Scheme,Host,Port,Database,User,Password,Params}) (*sql.DB, database.DbEngine, error)`.

---

## File Structure

- `pkg/studio/server/server.go` — `Server` struct, session state, `connectFn`, constructor, `Handler()` (returns `http.Handler` with routes).
- `pkg/studio/server/connect.go` — `POST /api/connect` handler.
- `pkg/studio/server/run.go` — `POST /api/run` live query runner (+ value encoding, token substitution).
- `pkg/studio/server/engine.go` — `POST /api/generate`, `/api/validate`, `/api/preview` handlers.
- `pkg/studio/server/*_test.go` — one per file, httptest-based.
- `cmd/baton-sql-studio/web/index.html` — placeholder static page (Plan 3 replaces).
- `cmd/baton-sql-studio/serve.go` — `serve` subcommand + `go:embed web`.
- `cmd/baton-sql-studio/main.go` — MODIFY: route `serve` subcommand alongside existing `compile`.

---

## Task 1: Server struct, session, and /api/connect

**Files:** Create `pkg/studio/server/server.go`, `pkg/studio/server/connect.go`, `pkg/studio/server/connect_test.go`.

**Interfaces produced:**
- `type Server struct { ... }` with unexported mutex-guarded session fields (`db *sql.DB`, `engine database.DbEngine`, `celEnv *bcel.Env`, `connected bool`) and a `connect func(context.Context, database.ConnectOptions) (*sql.DB, database.DbEngine, error)` field (defaults to `database.Connect`).
- `func New() *Server` — sets `connect = database.Connect` and builds `celEnv` via `bcel.NewEnv`.
- `func (s *Server) Handler() http.Handler` — a `*http.ServeMux` with routes registered (this task registers `/api/connect`).
- `type connectResponse struct { OK bool json:"ok"; Engine string json:"engine,omitempty"; Error string json:"error,omitempty" }`.
- Helper `writeJSON(w http.ResponseWriter, status int, v any)`.

Handler behavior: decode `studio.ConnectConfig` from body → build `database.ConnectOptions` → call `s.connect(ctx, opts)` → on success store session under lock, `Ping` the DB, respond `{ok:true, engine:"<engine>"}`; on error respond `200` with `{ok:false, error:"..."}` (a failed connection is a normal result, not an HTTP 500). Map `database.DbEngine` to a string via a small `engineName(DbEngine) string`.

- [ ] **Step 1: Write the failing test** — using an injected `connect` stub returning an in-memory sqlite `*sql.DB` (`sql.Open("sqlite", ":memory:")`, blank import `_ "modernc.org/sqlite"`) and `database.MySQL`:

```go
func TestConnect_OK(t *testing.T) {
	s := New()
	s.connect = func(ctx context.Context, o database.ConnectOptions) (*sql.DB, database.DbEngine, error) {
		db, _ := sql.Open("sqlite", ":memory:")
		return db, database.MySQL, nil
	}
	body := `{"scheme":"mysql","host":"h","port":"3306","database":"d","user":"u","password":"p"}`
	req := httptest.NewRequest("POST", "/api/connect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 { t.Fatalf("code %d", rec.Code) }
	var resp connectResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.OK { t.Fatalf("expected ok, got %+v", resp) }
}

func TestConnect_Failure_Is200NotOK(t *testing.T) {
	s := New()
	s.connect = func(ctx context.Context, o database.ConnectOptions) (*sql.DB, database.DbEngine, error) {
		return nil, database.Unknown, fmt.Errorf("dial tcp: refused")
	}
	req := httptest.NewRequest("POST", "/api/connect", strings.NewReader(`{"scheme":"mysql"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 { t.Fatalf("code %d", rec.Code) }
	var resp connectResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.OK || resp.Error == "" { t.Fatalf("expected not-ok with error, got %+v", resp) }
}
```

- [ ] **Step 2: Run — expect FAIL** (`New`/`Handler` undefined). `go test ./pkg/studio/server/ -run TestConnect -v`
- [ ] **Step 3: Implement** `server.go` (struct, `New`, `Handler`, `writeJSON`, `engineName`) and `connect.go` (the handler mapping `ConnectConfig`→`ConnectOptions`, calling `s.connect`, `db.PingContext`, storing session under mutex). Guard: reject non-POST with 405; malformed JSON → `{ok:false,error:...}`.
- [ ] **Step 4: Run — expect PASS.** Then `go test ./pkg/studio/server/`, `go build ./...`, `go vet ./pkg/studio/server/`.
- [ ] **Step 5: Commit** `feat(studio/server): session + /api/connect`.

---

## Task 2: /api/run — live query runner with bounded, JSON-safe results

**Files:** Create `pkg/studio/server/run.go`, `pkg/studio/server/run_test.go`.

**Interfaces produced:**
- `type runRequest struct { Query string json:"query"; Vars map[string]string json:"vars,omitempty" }`.
- `type runResponse struct { Columns []string json:"columns,omitempty"; Rows [][]any json:"rows,omitempty"; RowCount int json:"row_count"; Truncated bool json:"truncated"; Error string json:"error,omitempty" }`.
- `func (s *Server) handleRun(w http.ResponseWriter, r *http.Request)` registered at `/api/run`.
- `func encodeValue(v any) any` — `[]byte`→`string`, else pass through (JSON-safe).
- Constants `maxRows = 100`, `queryTimeout = 30 * time.Second`.

Behavior: require an active session (else `{error:"not connected"}`); this task handles queries **without** `?<...>` tokens (token substitution is Task 3 — if a token is present here, return `{error:"query has ?<var> tokens; provide sample vars"}` so Task 3 has a clear seam). Run `db.QueryContext(ctx, query)` under a `queryTimeout`; read `rows.Columns()`; scan into `[]any` per row via `*[]any`+`sql.RawBytes`-safe scanning (use `[]any` of `*any`), apply `encodeValue`, stop at `maxRows` and set `Truncated=true` if more remain. Always `rows.Close()`.

- [ ] **Step 1: Failing test** — seed an in-memory sqlite session directly (set `s.db`/`s.connected` via a small test helper `s.setSessionForTest(db, engine)`), create a table with 3 rows, POST `{"query":"SELECT id, name FROM t ORDER BY id"}`, assert `columns==["id","name"]`, `row_count==3`, first row values. Add a second test: 150 rows → `row_count==100`, `Truncated==true`.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** `run.go`. Scanning pattern (portable):

```go
cols, _ := rows.Columns()
out := [][]any{}
truncated := false
for rows.Next() {
	if len(out) >= maxRows { truncated = true; break }
	cells := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range cells { ptrs[i] = &cells[i] }
	if err := rows.Scan(ptrs...); err != nil { /* return error resp */ }
	for i := range cells { cells[i] = encodeValue(cells[i]) }
	out = append(out, cells)
}
```

- [ ] **Step 4: Run — expect PASS**; full package + build + vet.
- [ ] **Step 5: Commit** `feat(studio/server): bounded live query runner /api/run`.

---

## Task 3: /api/run — `?<var>` token substitution with sample values

**Files:** MODIFY `pkg/studio/server/run.go`; add tests to `run_test.go`.

**Interfaces produced:** `func substituteTokens(query string, vars map[string]string, engine database.DbEngine) (rewritten string, args []any, missing []string)` — replaces each `?<name>` token with the engine's positional placeholder (`?` for mysql/sqlite/vertica, `$N` postgres, `@pN` sqlserver, `:N` oracle) and collects the matching `vars[name]` value into `args`; returns any token names absent from `vars` as `missing`. Token regex: `` `\?\<([a-zA-Z0-9_]+)(?:\|[a-zA-Z0-9_]+)?\>` `` (mirrors bsql's `queryOptRegex`; the optional `|ident` group is ignored for preview).

`handleRun` now: if tokens present and any `missing`, return `{error:"missing sample values for: a, b"}`; else run the rewritten query with `args` as bound params.

- [ ] **Step 1: Failing test** — sqlite session; `{"query":"SELECT id FROM t WHERE id = ?<rid>","vars":{"rid":"2"}}` → returns the matching row; and a missing-var case → error names the missing var.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** `substituteTokens` (placeholder style switch on `database.DbEngine`) and wire it into `handleRun` before `QueryContext` (pass `args...`).
- [ ] **Step 4: Run — expect PASS**; full package + build + vet.
- [ ] **Step 5: Commit** `feat(studio/server): resource-var sample substitution in /api/run`.

---

## Task 4: /api/generate, /api/validate, /api/preview

**Files:** Create `pkg/studio/server/engine.go`, `pkg/studio/server/engine_test.go`.

**Interfaces produced:** handlers `handleGenerate`, `handleValidate`, `handlePreview` registered at the three routes.
- `/api/generate`: body `studio.Spec` → `{yaml: string, error?}` via `studio.Generate`.
- `/api/validate`: body `studio.Spec` → the `studio.Report` (OK + Errors) via `studio.Validate`, passing `studio.ValidateOptions{DB: s.db, DBEngine: s.engine}` when a session is active, else zero options (offline validation).
- `/api/preview`: body `{field: studio.FieldMapping, row: map[string]any}` → `{value: string, error?}` via `studio.PreviewField(ctx, s.celEnv, field, row)`.

- [ ] **Step 1: Failing tests** — generate a small Spec → assert `yaml` contains `resource_types:`; validate the same Spec (no session) → assert `ok:true`; preview a composite_id field over `{"first_name":"Ada","last_name":"Lovelace"}` → assert `value=="Ada Lovelace"`.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** the three thin handlers (decode → call engine → `writeJSON`). Validate/preview use `context` from the request.
- [ ] **Step 4: Run — expect PASS**; full package + build + vet.
- [ ] **Step 5: Commit** `feat(studio/server): generate/validate/preview endpoints`.

---

## Task 5: Static serving + `serve` subcommand

**Files:** Create `cmd/baton-sql-studio/web/index.html` (placeholder), `cmd/baton-sql-studio/serve.go`; MODIFY `cmd/baton-sql-studio/main.go`; test `cmd/baton-sql-studio/serve_test.go`.

**Interfaces produced:** `func (s *Server) Handler()` also serves static files at `/` from an `fs.FS` set via `func (s *Server) SetStatic(fsys fs.FS)` (default: an empty/coded fallback page if unset). `serve.go`: `//go:embed web` `var webFS embed.FS`; `runServe(args []string) error` parses a `-addr` flag (default `127.0.0.1:8787`), builds `Server`, `s.SetStatic(sub of webFS)`, prints `Studio running at http://<addr>`, `http.ListenAndServe`. `main.go` dispatches `os.Args[1]`: `compile` (existing) or `serve`.

- [ ] **Step 1: Failing test** — `serve_test.go`: build a `Server` with `SetStatic(os.DirFS/embedded)`, GET `/` via httptest → 200 and body contains a known marker string from the placeholder page (e.g. `baton-sql Studio`). Also assert an unknown `/api/nope` → 404.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** placeholder `index.html` (minimal: `<h1>baton-sql Studio</h1><p>API up.</p>` — Plan 3 replaces it), `SetStatic` + static file route (`http.FileServerFS`), `serve.go`, and `main.go` subcommand routing. Keep `compile` behavior identical.
- [ ] **Step 4: Run — expect PASS**; `go build ./...`, `go vet ./cmd/baton-sql-studio/...`, and confirm `go run ./cmd/baton-sql-studio serve -addr 127.0.0.1:0` starts and is killable (manual note in report; don't block a test on a live listen).
- [ ] **Step 5: Commit** `feat(studio): serve subcommand + static UI hosting`.

---

## Task 6: End-to-end integration test

**Files:** Create `pkg/studio/server/integration_test.go`.

**Interfaces:** consumes all handlers. No new production code (if a genuine gap forces one, report it).

Flow: build `Server` with an injected `connect` stub returning an in-memory sqlite DB pre-seeded (users + user_roles tables); drive the real `Handler()` via `httptest.Server`: (1) POST `/api/connect` → ok; (2) POST `/api/run` a users SELECT → columns/rows; (3) POST `/api/run` a grant query with `?<role_id>` + sample var → rows; (4) POST `/api/generate` a two-resource-type Spec → yaml; (5) POST `/api/validate` same Spec **with the live session** → `ok:true`. Assert each step.

- [ ] **Step 1: Write the integration test** (all five steps above).
- [ ] **Step 2: Run — expect FAIL** if any wiring gap; otherwise PASS.
- [ ] **Step 3: Fix any wiring gaps surfaced** (only within `pkg/studio/server`).
- [ ] **Step 4: Run — expect PASS**; full `go test ./pkg/studio/... ./cmd/baton-sql-studio/...`, `go build ./...`, `go vet`, `gofmt -l`.
- [ ] **Step 5: Commit** `test(studio/server): end-to-end connect→run→generate→validate`.

---

## Self-Review

- **Coverage vs design §6/§7 (validation layered; local Go binary reusing pkg/bsql + opening DB):** connect (T1), live run incl. resource-var binding (T2–T3), generate/validate/preview reusing the engine (T4), serve+static (T5), e2e (T6). ✓
- **Placeholder scan:** every code step has real code or a precise interface; the placeholder `index.html` is intentional and named as such (Plan 3 replaces it).
- **Type consistency:** `Server`, `connectResponse`, `runRequest/runResponse`, `substituteTokens`, `encodeValue`, `SetStatic`, `Handler` used identically across tasks; engine types referenced by their Plan-1 exported names.
- **Deferred/known:** read-only enforcement is bound-by-cap-and-timeout, not SQL parsing (documented constraint); multi-session/auth intentionally omitted (localhost tool); the `connect:`-block export gap from Plan 1 is not needed here (validation uses the live handle) but IS needed for a downloadable config — carry to Plan 3's export or a follow-up.

## Handoff
Execute via subagent-driven-development. Most implementer tasks are standard-tier (http + integration). T6 is integration-judgment.
