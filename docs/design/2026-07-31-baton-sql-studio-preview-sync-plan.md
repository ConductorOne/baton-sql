# baton-sql Studio — Preview Sync / "Dry Run" (Pathway B)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Let the wizard actually RUN the connector's sync logic against the live session and show the resulting **resources, entitlements, and grants (who-has-what)** — closing the feedback loop from "my queries run + validate" to "here's exactly what the connector will produce."

**Architecture (Pathway B — reuse the syncers in-process):** Studio already builds `Config.GetSQLSyncers(ctx, dbs, dbEngine, celEnv)` for validation. A new engine **preview driver** drives each returned syncer's real `List` → per-resource `Entitlements` + `Grants` (the exact SDK `ResourceSyncer` methods the production connector uses), collecting a **bounded** slice of the synced graph. A `POST /api/preview-sync` endpoint runs it against the live session; a **"Dry run"** panel in Review renders it. No `.c1z`, no subprocess, no `connect:` block needed (uses the session you're connected to).

**Tech Stack:** Go 1.25.2; reuse `pkg/studio` (Generate/GetSQLSyncers path), `pkg/bsql`, `pkg/bcel`, `pkg/database`, baton-sdk types (`v2.Resource/Entitlement/Grant`, `pagination.Token`, `connectorbuilder.ResourceSyncer`). Tests: in-memory `modernc.org/sqlite` (seed a table, run a real Spec through the driver). UI: the existing self-contained `index.html`.

## Global Constraints
- Module `github.com/conductorone/baton-sql`; Go 1.25.2. Never modify `pkg/bsql`/`pkg/bcel`/`pkg/database`/`baton-sdk` — consume their exported API.
- **Read-only + bounded.** The driver calls only `List`/`Entitlements`/`Grants` (never provisioning). Cap: `maxResourcesPerType` (default 200), `maxPerResourceEntitlements`/`maxPerResourceGrants` (default 100 each), `maxPages` per pagination loop (default 20), and a `previewTimeout` (default 60s) on the whole run — with `truncated` flags surfaced. It must never stream an unbounded table into memory/the browser.
- Uses the LIVE session `*sql.DB` + `DbEngine` + `bcel.Env` (same handles validation uses). Offline (no session) → an error result, not a crash.
- Syncer method signatures (confirmed): `List(ctx, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error)`; `Entitlements(ctx, resource *v2.Resource, pToken) ([]*v2.Entitlement, ...)`; `Grants(ctx, resource *v2.Resource, pToken) ([]*v2.Grant, ...)`; `ResourceType(ctx) *v2.ResourceType`. Pagination: loop passing `&pagination.Token{Size: N}` / the returned next-token string until it's `""`, capped by `maxPages`.
- v1 scope: **flat resource types** (call `List` with `parentResourceID = nil`). Hierarchical/parent-child types and multi-DB iteration are out of scope for v1 — if a syncer needs a parent, collect its top level only and note the limitation. Document, don't silently mislead.

## File Structure
- `pkg/studio/previewsync.go` — the driver + result types.
- `pkg/studio/previewsync_test.go` — sqlite-backed test.
- `pkg/studio/server/previewsync.go` + `_test.go` — the `/api/preview-sync` handler.
- `cmd/baton-sql-studio/web/index.html` — the Dry-run panel (UI task).

---

## Task 1: Engine preview driver
**Files:** Create `pkg/studio/previewsync.go`, `pkg/studio/previewsync_test.go`.

**Produces:**
- Result types (JSON-tagged):
  ```go
  type PreviewSync struct {
      ResourceTypes []PreviewResourceType `json:"resource_types"`
      Error         string                `json:"error,omitempty"`
  }
  type PreviewResourceType struct {
      ID, Name           string             `json:"id"`   // Name has its own tag
      Resources          []PreviewResource  `json:"resources"`
      ResourcesTruncated bool               `json:"resources_truncated"`
      Grants             []PreviewGrant     `json:"grants"`
      GrantsTruncated    bool               `json:"grants_truncated"`
  }
  type PreviewResource struct {
      ID, DisplayName string `json:"id"` // DisplayName own tag
      Entitlements    []PreviewEntitlement `json:"entitlements,omitempty"`
  }
  type PreviewEntitlement struct{ ID, DisplayName, Slug string `json:"id"` } // own tags
  type PreviewGrant struct {
      PrincipalType, PrincipalID, PrincipalDisplay string `json:"principal_type"` // own tags
      EntitlementID, EntitlementDisplay            string `json:"entitlement_id"`  // own tags
  }
  ```
  (fix the struct tags — one tag per field; the shorthand above is illustrative.)
- `func PreviewSyncFromConfig(ctx, cfg *bsql.Config, opts PreviewOptions) (*PreviewSync, error)` where `PreviewOptions{DB *sql.DB, DBEngine database.DbEngine}` (nil DB → in-memory sqlite fallback for offline, same as Validate). Internally: `celEnv := bcel.NewEnv(ctx)`; `syncers := cfg.GetSQLSyncers(...)`; for each syncer: `rt := syncer.(interface{ ResourceType(context.Context) *v2.ResourceType }).ResourceType(ctx)`; page `List(ctx, nil, tok)` collecting up to `maxResourcesPerType`; for each collected resource, page `Entitlements` and `Grants` (bounded), mapping v2 objects → the Preview* structs. Resolve a grant's principal to `PrincipalType/PrincipalID` from `grant.Principal.Id`; `PrincipalDisplay` best-effort (leave "" in v1, or fill if the principal is among already-collected resources — nice-to-have).
- A convenience `func PreviewSyncFromSpec(ctx, spec *Spec, opts) (*PreviewSync, error)` = Generate → bsql.Parse → PreviewSyncFromConfig (so the server can pass a Spec).

- [ ] Step 1: **Failing test** — seed an in-memory sqlite DB with `employees(id,name)` + `emp_roles(user_id,role)`; build a small `Spec` (a users type + a roles type with a static entitlement + a resource-scoped grant) OR reuse the finance fixture shape; call `PreviewSyncFromSpec`; assert: the users resource type lists N resources with display names; the roles type's grant list contains principal→entitlement pairs matching the seed; and a truncation case (seed > cap rows → `ResourcesTruncated`). Assert bounded (never more than the caps).
- [ ] Step 2: Run — FAIL (undefined).
- [ ] Step 3: Implement `previewsync.go` per above. Pagination loop capped by `maxPages`; per-call errors → set `Error` on the result (don't panic); overall `previewTimeout` via `context.WithTimeout`.
- [ ] Step 4: Run — PASS; `go build ./...`, `go vet`, `gofmt -l`.
- [ ] Step 5: Commit `feat(studio): preview-sync driver (run the connector's syncers, bounded)`.

## Task 2: `POST /api/preview-sync`
**Files:** Create `pkg/studio/server/previewsync.go`, `_test.go`; register route in `Handler()`.
- POST only (405 else). Body = `studio.Spec`. Require active session (`{error:"not connected"}` else). Call `studio.PreviewSyncFromSpec(ctx, &spec, PreviewOptions{DB: s.db, DBEngine: s.engine})` (session read under the mutex, like `handleValidate`). Return the `PreviewSync` JSON; a driver/parse error → HTTP 200 `{... , error}` (convention). Guarded by `localOnly` automatically.
- [ ] Step 1: Failing test — inject a seeded in-memory sqlite session (via `setSessionForTest`), POST a small Spec, assert the response has the expected resource types/resources/grants; not-connected → error; 405 on GET.
- [ ] Step 2–4: RED → implement → GREEN (`-race`), build/vet/gofmt.
- [ ] Step 5: Commit `feat(studio/server): /api/preview-sync endpoint`.

## Task 3: "Dry run" panel in Review (UI)
**Files:** `cmd/baton-sql-studio/web/index.html` (+ `web_test.go` marker).
- In the Review step (near Validate/View YAML), add a **"Dry run"** (or "Preview sync") button → `POST /api/preview-sync` with the current `state` → render the result:
  - Per resource type: a collapsible section showing the synced **resources** (id + display name), its **entitlements**, and the **grants** as `principal → entitlement` rows (e.g. `users:1 → role:fin_admin:assigned`), with a searchable filter (reuse the results-search pattern) and truncation notes.
  - Loading state + inline error (e.g. "not connected", or the driver error) via textContent. XSS-safe (all synced values via textContent/DOM — this renders UNTRUSTED DB-derived data, so NO innerHTML interpolation).
- Verify live against `studio_demo` (Playwright, port ≠ 8787): build users+roles config → Dry run → see the 4 users as resources and the role grants (Ada/Katherine → fin_admin) rendered. If flaky, reason through + say so; don't fake.
- [ ] Steps: marker test + build/vet/gofmt/node-check; commit `feat(studio/web): Dry run — preview the actual sync (resources/entitlements/grants)`.

## Self-Review / Risks
- **Bounded everywhere** (caps + truncated flags + timeout) — the one hard requirement; a preview must never OOM or hang on a big table.
- **Read-only** — only List/Entitlements/Grants; provisioning is never invoked.
- **Flat-types v1** — parent/child hierarchies + multi-DB are out of scope; note in the UI if a type looked hierarchical.
- **Principal display** — v1 shows `type:id`; resolving to display names (cross-ref collected resources) is a fast follow.
- Reuses the exact connector sync code (authoritative), consistent with the whole tool's design.

## Handoff
Execute via subagent-driven-development, one code-writer at a time (shared working tree). Task 1 (engine) is standard/higher-tier; Task 2 (endpoint) standard; Task 3 (UI) standard with a live Playwright check.
