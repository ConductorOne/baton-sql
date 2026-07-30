# baton-sql Studio — Web Wizard UI (Plan 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax. This is a FRONTEND plan — verification is reviewer code-inspection + a Go marker test + (where possible) a browser smoke-load, not Go unit TDD. Do not fake tests; where a step's acceptance is visual/behavioral, state exactly what to inspect.

**Goal:** Replace the placeholder `cmd/baton-sql-studio/web/index.html` with the real guided wizard — a self-contained single-page app that talks to the Plan 2 server endpoints and takes a user from "connect to a DB" to "download a validated baton-sql config." This is the loadable HTML.

**Architecture:** ONE self-contained `index.html` (inline CSS + vanilla JS, NO build step, NO npm, NO external CDNs — it's served locally and must work offline). A single client-side `state` object mirrors the `studio.Spec` being authored; the UI reads/writes it and POSTs to the server. Design system is ported from the approved mockup (slate neutrals + a single teal accent; system-ui for UI, monospace for SQL/columns/YAML). The flow mirrors the validated mockup: **Connect → Declare resource types → per-resource-type (List / Entitlements / Grants) → Review & export.**

**Tech Stack:** HTML5 + CSS + vanilla ES2020 (`fetch`, no framework). Served by `cmd/baton-sql-studio serve` (Plan 2) from the embedded `web/` dir. A small Go test asserts the served page's structure.

## Global Constraints

- ONE file: `cmd/baton-sql-studio/web/index.html`. No JS/CSS build tooling, no package.json, no external network requests (no CDN fonts/scripts — inline everything; system font stacks only). Must run from `file://`-style embedding with no console errors on load.
- Talk ONLY to the Plan 2 endpoints, with these exact contracts (verify against `pkg/studio/server/*.go` before coding each task):
  - `POST /api/connect` ← `studio.ConnectConfig{scheme,host,port,database,user,password,params}` → `{ok, engine, error}`
  - `POST /api/run` ← `{query, vars?:{name:value}}` → `{columns:[], rows:[[...]], row_count, truncated, error}`
  - `POST /api/generate` ← `studio.Spec` → `{yaml, error}`
  - `POST /api/validate` ← `studio.Spec` → `studio.Report{ok, errors:[{scope,field,message}]}`
  - `POST /api/preview` ← `{field: FieldMapping, row: {col:val}}` → `{value, error}`
  - `GET /api/status` → `{connected, engine}` (call on page load to restore connection state after a refresh)
  - `POST /api/disconnect` → `{ok}` (offer a "Disconnect" affordance on the connected pill)
- The page is served same-origin by the binary, so `fetch` calls carry a loopback `Host`/`Origin` and pass the server's localhost guard automatically — do NOT hardcode an absolute `http://127.0.0.1` base; use relative paths (`fetch('/api/...')`).
- The client `state` must serialize to EXACTLY the `studio.Spec` JSON shape (verify field names against `pkg/studio/spec.go`): `{app_name, connect, resource_types:[{id,name,trait,list:{query,fields:[{field,column,transform?}]}, entitlements:{mode,static?,query?,fields?,grantable_to?}, grants:[{query,resource_var,principal_type,entitlement,fields}]}]}`. Transforms: `{recipe, args?, raw_cel?}` with recipes `slugify|title_case|coerce_string|null_default|composite_id|status_ternary|account_type_ternary|raw`.
- Accessibility/quality: theme-aware (light/dark), keyboard-focusable controls, no horizontal body scroll (wide tables/YAML scroll in their own container), errors shown inline with actionable text.
- Design tokens (port from the approved mockup): light — bg `#eef1f5`, surface `#fff`, border `#dde3ec`, ink `#131a22`, accent `#0d7d8a`; dark — bg `#0d1219`, surface `#151d27`, border `#26313f`, ink `#e7edf4`, accent `#22b3c4`; semantic good `#1f9d5b` / warn `#b9791a` / crit `#d24b4b`. Mono stack for SQL/columns/YAML.

---

## File Structure
- `cmd/baton-sql-studio/web/index.html` — the entire app (replace the placeholder). Sections: `<style>` (tokens + components), markup skeleton, `<script>` (state, api client, render fns, event wiring).
- `cmd/baton-sql-studio/web_test.go` — a Go test that reads the embedded/served page and asserts structural markers per task.

Because it's one file, tasks are ADDITIVE slices of the same file. Each task appends/extends and must leave the page loading cleanly. Keep functions small and grouped by concern (api client, state, each screen's render).

---

## Task 1: App shell, design system, API client, Connect screen

**Deliverable:** The page loads with the ported design system, a top bar, a left "flow" rail (Connect → Resource types → Review, Connect active), and a **Connect** form (scheme dropdown [postgres/mysql/sqlserver/oracle/vertica/hdb], host, port, database, user, password). On load, call `GET /api/status` and, if already connected, skip straight to the connected state. A "Test connection" button POSTs `/api/connect`; on `ok:true` it shows a green "Connected · <engine>" pill (with a "Disconnect" action → `POST /api/disconnect`) and advances the flow to "Declare resource types"; on `ok:false` it shows the error inline. Use relative `fetch('/api/...')` paths. Include a tiny `api(path, body)` fetch helper and the global `state` object skeleton (`{app_name, connect, resource_types:[]}`).

- [ ] **Step 1:** Write `index.html`: `<style>` with the tokens (both themes via `prefers-color-scheme` + `:root[data-theme]`), top bar, left rail component, and the connect form. Add `<script>` with `state`, `api()`, and connect handler. No external requests.
- [ ] **Step 2:** Write `web_test.go`: assert the served `/` page (via `server.New()` + `SetStatic(fs.Sub(embedded,"web"))` OR read the file directly) contains markers: `baton-sql Studio`, `id="connect"`, `Test connection`, and does NOT reference `http://` external hosts or `cdn`/`googleapis` (offline check).
- [ ] **Step 3:** Run `go test ./cmd/baton-sql-studio/... -run TestWeb`; `go build ./...`; `gofmt -l`. Manually (note in report): `go run ./cmd/baton-sql-studio serve` then load `http://127.0.0.1:8787` and confirm the shell + connect form render with no console errors (the reviewer/controller will also load it).
- [ ] **Step 4:** Commit `feat(studio/web): app shell + connect screen`.

## Task 2: Resource-type declaration + card rail + state model

**Deliverable:** After connect, the "Declare resource types" step shows the plain-language prompt ("What do people get access to?") with a Users type pre-seeded and chips to add Roles/Groups/Databases/etc (each adds a `resource_types[]` entry `{id,name,trait,list:{query,fields:[]},entitlements:{mode:'none'},grants:[]}`). The left rail renders one **card** per resource type (name + trait badge + per-tab state dots), clicking a card selects it and opens its workspace (tabs List/Entitlements/Grants — bodies filled by later tasks; for now a placeholder per tab). A `renderRail()` and `selectRT(id)` drive it. `state.resource_types` is the single source of truth.

- [ ] **Step 1:** Extend the script: RT declaration UI, `addResourceType(name,trait)`, `renderRail()`, `selectRT`, tab shell. Cards not a tree (per the validated mockup).
- [ ] **Step 2:** Extend `web_test.go` markers: `Declare resource types` / `data-rt-card` / tab labels present.
- [ ] **Step 3:** `go test`/`build`/`gofmt`; manual load note (add a role, see a card appear, click it, see tabs).
- [ ] **Step 4:** Commit `feat(studio/web): resource-type declaration + card rail`.

## Task 3: List tab — query editor, live run, column→field mapping + recipes + preview

**Deliverable (the core screen):** For the selected resource type's **List** tab: a mono SQL editor + "Run" button → POST `/api/run` → render the returned columns + sample rows in a results panel (scrolls in its own container; shows "unproven"/"proven N rows"/"stale"). A mapping table: one row per canonical field for the type's trait (Users: id\*, display_name\*, email, status, account_type, login, last_login, manager_id, manager_email, employee_ids, profile.\*; others: id\*, display_name\*, description, profile.\*). Each field: a **column dropdown** (populated from the last run's columns), an optional **transform** chooser (recipe chips: slugify/composite id/status ternary/title case/coerce/null-default/account-type ternary, or "raw CEL…"), and a **live preview** value from POST `/api/preview` (field + the first returned row). Writing the mapping updates `state.resource_types[i].list` (query + fields[]). Raw-CEL opens a small editor with the live preview updating on input.

- [ ] **Step 1:** Implement the List tab: `runQuery(rtId,'list')`, results render, `renderMapping(rtId,'list')`, column dropdowns, recipe chooser → builds `{field,column,transform}`, `previewField()` calling `/api/preview`. Keep functions reusable (Entitlements/Grants reuse the mapping widget).
- [ ] **Step 2:** `web_test.go` markers for the mapping widget (`data-map-field`, `Run`, `Map columns`).
- [ ] **Step 3:** test/build/gofmt; manual: run a query against a real DB (note: needs a live DB — reviewer/controller verifies the render + that a mapping updates state + a preview call fires; full DB round-trip is a user smoke-test).
- [ ] **Step 4:** Commit `feat(studio/web): List tab — run, column mapping, recipes, live preview`.

## Task 4: Entitlements tab

**Deliverable:** For the selected type's **Entitlements** tab: a mode toggle **Static list** vs **From a query**. Static: add rows `{id, display_name, purpose?}` (purpose dropdown: assignment/permission), plus a `grantable_to` multiselect of the currently-defined resource-type IDs, plus optional description/immutable. Query mode: a SQL editor + Run + the mapping widget (Task 3) for id/display_name/purpose/slug/description, plus the same `grantable_to` multiselect (applies to all rows) — `slug` auto-defaults if unmapped (mirror engine behavior; show a hint). Updates `state.resource_types[i].entitlements` (mode/static/query/fields/grantable_to). Enforce purpose ∈ {assignment,permission} in the UI.

- [ ] **Step 1:** Implement the Entitlements tab (both modes + grantable_to multiselect from defined RT ids).
- [ ] **Step 2:** markers (`Entitlements`, mode toggle, `grantable_to`).
- [ ] **Step 3:** test/build/gofmt; manual render note.
- [ ] **Step 4:** Commit `feat(studio/web): Entitlements tab (static + query, grantable_to)`.

## Task 5: Grants tab (resource-scoped)

**Deliverable:** For the selected type's **Grants** tab: a SQL editor for the who-has-what query; a **resource binding** control — pick the `?<var>` name that binds to the current resource (`resource_var`), with an inline hint "bound to resource.ID"; a "Run" that requires a sample value for the var (prompt for it, pass as `vars`); the mapping widget for `principal_id`\* (+ optional skip_if); a `principal_type` dropdown populated from the defined resource-type IDs; and an `entitlement` picker (the type's defined entitlement id/slug). Updates `state.resource_types[i].grants[0]` (query, resource_var, principal_type, entitlement, fields). (Do NOT offer `resource_id` — the engine rejects it.)

- [ ] **Step 1:** Implement the Grants tab incl. the resource-var binding UI + sample-value prompt for Run.
- [ ] **Step 2:** markers (`Grants`, `principal_type`, `resource.ID`).
- [ ] **Step 3:** test/build/gofmt; manual render note.
- [ ] **Step 4:** Commit `feat(studio/web): Grants tab with resource-scoped binding`.

## Task 6: Review & export — live validation + generated YAML + download

**Deliverable:** A **Review** screen (and a persistent "View YAML" drawer): POST `/api/validate` the assembled `state` → render the `Report` — a green "Valid" summary or a list of issues `{scope,field,message}`, each linking back to the resource type/tab it belongs to (best-effort: show scope+field+message and select the RT on click). POST `/api/generate` → show the YAML (mono, read-only, scrolls) and a **Download .yaml** button (client-side blob download; filename from app_name). A top-bar "Validate" affordance re-runs validation on demand and reflects per-tab dot state (proven/unproven/error). Also surface the known residual as an info hint where relevant: a column-sourced dynamic `purpose` can't be statically checked.

- [ ] **Step 1:** Implement Review screen + YAML drawer + validate/generate wiring + download. Map `Report.errors` to the rail/tabs (at least list them with scope/field and a click-to-select-RT).
- [ ] **Step 2:** markers (`Review`, `Download`, `data-yaml`); a Go test that (if feasible) POSTs a known-good Spec to a `server.New()` httptest handler `/api/validate` and asserts `ok:true` — reusing the engine fixture shape — to prove the UI's Spec shape matches the server (this closes the "does the JS state serialize to a valid Spec" risk without a browser).
- [ ] **Step 3:** test/build/gofmt; manual full-flow note.
- [ ] **Step 4:** Commit `feat(studio/web): review, live validation, YAML export`.

---

## Self-Review
- Coverage vs design §4/§5 wizard flow: connect (T1) → declare RTs (T2) → List/map/recipes/preview (T3) → Entitlements (T4) → Grants resource-scoped (T5) → review/validate/export (T6). ✓
- Every endpoint from Plan 2 is consumed; the client `state`→`Spec` shape is pinned to `pkg/studio/spec.go` and (T6) proven against `/api/validate`.
- Known residuals surfaced (dynamic purpose hint); no external network deps; theme-aware; wide content scrolls in-container.
- Frontend verification is inspection + marker tests + a Spec-shape validate test + manual browser smoke — explicitly NOT full browser automation in the loop (a real DB connect is a user smoke-test). This is called out, not hidden.

## Handoff
Execute via subagent-driven-development. Load the `frontend-design` skill's guidance for visual quality on Tasks 1–6. Reviewer should code-inspect against the endpoint/Spec contracts AND, where the environment allows, load the served page to confirm it renders without console errors. Final step (after Task 6): controller starts `serve` and loads the page in a browser to smoke-test render + a validate round-trip with the built-in fixture, then reports the loadable URL/path to the user.
