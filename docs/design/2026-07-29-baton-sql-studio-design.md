# baton-sql Studio — Design

**Date:** 2026-07-29
**Status:** Approved design (pre-implementation), revised after config survey
**Author:** Charles Kramer (with Claude)

## 1. Problem

Authoring a `baton-sql` connector config today is slow, error-prone, and largely
unguided. Compared to platforms like Lumos — where you "just plug in the queries as
you define them" — ConductorOne requires substantially more structure, and there is
no easy way to validate a config as you build it.

The difficulty breaks into four layers, which customers hit **roughly equally**:

1. **The model gap.** The customer thinks in "my users table, my roles table, who
   has what." C1 thinks in *resource types → entitlements → grants → principals*.
2. **The expression gap.** The author must hand-write CEL and structure YAML exactly
   right (`map.id`, `traits.user.emails[0]`, static vs. dynamic entitlements,
   `principal_type`, resource-scoped grant `vars`, etc.).
3. **The silent-failure gap.** A config can *parse and even run* yet produce wrong or
   empty data with **no error** — ~17 known traps (`mfa_enabled` is a no-op;
   `manager_id` dropped if `profile` is empty; pagination block with no
   `?<Offset>`/`?<Cursor>` token silently ignored; dynamic entitlements silently
   require `slug`). You don't know you failed until a sync looks wrong.
4. **The feedback-loop gap.** You only find out it's wrong *late*, after wiring up
   the connector and running a full sync.

**Constraint:** No AI/LLM in the loop, at least short-term. All intelligence must be
deterministic.

## 2. Audience & sequencing

The real audience is **always the customer** — the person who generates these
configs. A ConductorOne SE is only the *first test user*.

- **v0.0.1 (MVP):** Configure + Validate over the resource-type model (below).
- **Fast-follows (build next, in order):** dynamic-entitlement mode & `expandable`
  → service-account Discovery → Remediate → Manage. "Deferred" means *build next*,
  not *someday*.

## 3. What the config survey changed (evidence)

Before finalizing, we audited **21 real baton-sql configs** across the `baton-sql`
`examples/`, `SkunkWorks/baton-sql-demos`, and `enablement-tools/baton-sql-tools`
repos. The original design (four flat global slots — Users/Resources/Entitlements/
Grants — with column→field mapping expressed only as SQL aliases) proved far too
narrow. It cleanly covered only **~2 of 21** configs. The dominant real patterns break
a flat/alias-only model **structurally**:

- **Multiple resource types per connector** — 3 to 6 (redshift = 6). ~19 of 21 have
  more than one. *The single biggest gap.*
- **Grant queries parameterized by the current resource** — `vars: {sg: "resource.ID"}`
  → `WHERE col = ?<sg>`, in ~17 of 21. Grants are authored *per resource type*, not
  as one global query.
- **Dynamic entitlement queries** — 5 configs (jesta + the four Oracle/Postgres EBS
  configs) produce entitlements from SQL, not a static list.
- **Non-trivial CEL in `id`/`display_name`/`entitlement_id`** — ~18 of 21: ternaries
  (`STATUS == 0 ? 'enabled':'disabled'`), composite keys (`db + '.' + schema + '.' +
  table`), `slugify(...)`, `titleCase(...)`, type coercion, even
  `phpDeserializeStringArray(...)`. A plain `SELECT col AS id` alias handles almost
  none of these.
- **Richer field vocabulary** than a 7-field user set: `last_login`, `employee_ids`,
  `status_details`, `manager_email`, emails-as-array, plus an arbitrary `profile.*`
  bag (department, job_title, …); non-user traits (group/role/app; and rarely
  secret/agent/NHI); entitlement `purpose` and `grantable_to` (the latter appears
  almost everywhere).
- **`expandable`** (group/role membership fan-out) in 8 configs; write-side features
  (`account_provisioning` 8, `actions` 7, `credential_rotation` 3) in many.

**Conclusion:** "Lumos-simple" and "baton-sql-complete" are in genuine tension,
because C1 deliberately models more. A pure Lumos-clone front-end cannot be both easy
*and* cover real configs. So Studio keeps the *ergonomics* that survive complexity —
guided structure, live query-running, trap prevention, assisted mapping — but
restructures its model around the **resource type**, and treats CEL as a first-class,
*assisted* activity rather than a rare escape hatch.

## 4. Core model — the resource type is the unit

A connector in Studio is **one or more resource types**. "Users" is simply the first
resource type (the one carrying user traits and usually the grant principal). Each
resource type is authored through the same repeatable sub-flow:

- **List query** → column mapping to `id`, `display_name`, and (per trait kind) the
  trait fields.
- **Entitlements** → a **static list** *or* a **dynamic query** (mode toggle).
- **Grants** → a **resource-scoped** query (parameterized by the current resource via
  `vars: {x: "resource.ID"}`), mapping the principal and the entitlement, with
  `principal_type` chosen from the resource types already defined.

This directly answers the model gap for real connectors: the customer declares "what
kinds of things do people get access to?" (roles, groups, databases, schemas…) and
Studio generates one `resource_types.<id>` block per answer, wiring the list/
entitlements/grants underneath. Studio always generates the CEL/YAML itself, so the
17 traps remain **structurally impossible** (no no-op keys; auto-`profile` when a
manager field is mapped; `slug` always emitted on dynamic entitlements; pagination
tokens injected into the query; `principal_type` constrained to defined types).

### Mapping mechanism — alias fast-path + assisted CEL

For every mapped field, in escalating order of power:

1. **Pick a column** (the Lumos-style fast path — trivial rename, ~most fields).
   Internally this is a plain column reference; optionally surfaced as a
   `SELECT col AS canonical` alias.
2. **Apply a transform recipe** — a curated, deterministic CEL library offered as
   one-click options on a mapped column. **The recipe set is seeded directly from the
   CEL patterns observed in the 21 surveyed configs** (not an invented canonical set);
   those examples are the guide for which recipes ship and how they behave. The
   observed patterns are: `slugify`, **composite id** (concatenate N columns with a
   separator — e.g. `db + '.' + schema + '.' + table`), **status ternary** (map raw
   values → enabled/disabled/deleted — e.g. `STATUS == 0 ? 'enabled':'disabled'`),
   `titleCase`, **type coerce** (`string(...)`, int), **null-default**
   (`x != null ? string(x) : ''`), and an **account-type ternary** (e.g.
   `name.startsWith('_SYS') ? 'system' : 'human'`). Studio generates the CEL.
3. **Raw CEL with live preview** — a field-level CEL editor that runs against the
   query's sample rows and shows the computed value in real time. The escape hatch
   for the long tail (e.g. `phpDeserializeStringArray`), used *inside* the wizard
   without abandoning it.

The recipe library (2) is the key move: it keeps the ~85% of configs that need *some*
CEL out of hand-writing it, while (3) honestly admits CEL is sometimes required.

### Field vocabulary (v0.0.1)

- **User trait:** `id`, `display_name`, `description`, `emails[]`, `login`,
  `login_aliases[]`, `status`, `status_details`, `account_type`, `last_login`,
  `employee_ids[]`, `manager_id`, `manager_email`, and arbitrary `profile.<key>`.
- **Group / role / app trait:** `profile.<key>` bag (+ `help_url` for app).
- **Entitlement:** `id`, `display_name`, `description`, `slug`, `purpose`
  (assignment/permission), `grantable_to`, `immutable`.
- **Grant:** `principal_id`, `principal_type`, `entitlement`, `resource_id`,
  `skip_if`.

## 5. The wizard flow (v0.0.1)

1. **Connect.** DB scheme/host/port/database/credentials (postgres, mysql, sqlserver,
   oracle, hdb, vertica). "Test connection" must go green before proceeding.
2. **Declare resource types.** "What do people get access to?" → produces the Users
   type plus one block per resource type (role, group, database, …).
3. **Per resource type (repeatable sub-flow):**
   a. **List** — author/paste the list query, run live, map `id`/`display_name`
      (+ trait fields for Users), using column-pick / recipe / raw-CEL as needed.
   b. **Entitlements** — choose static list or dynamic query; map
      `id`/`display_name`/`purpose`/`grantable_to` (+ `slug`); run dynamic queries
      live.
   c. **Grants** — author the resource-scoped grant query (Studio offers the
      `resource.ID` binding), map principal + entitlement, pick `principal_type` from
      defined types, optionally add `skip_if`; run live.
4. **Review & export.** Read-only YAML preview + green static-validation summary +
   `.yaml` download. (No full split editor in v1; preview is read-only.)

## 6. Validation (layered)

- **Static, instant, authoritative.** Compile the mapped queries to YAML, then parse
  and validate through baton-sql's **own** `pkg/bsql` (`config.go` + `validate.go`) —
  never a reimplementation, so the validator cannot drift from what the connector
  accepts. Plus: every mapped field's source column exists in the SELECT; the gotcha/
  trap ruleset; `principal_type` references a defined resource type.
- **Live, on-demand.** Run any query against the connected DB; show returned columns
  and sample rows; evaluate recipe/raw CEL against those rows for preview. Any query
  not yet run live is flagged **"unproven."**

## 7. Architecture (kept light)

A single local Go binary serves a browser UI. The backend imports `pkg/bsql` for
authoritative validation and opens the customer DB to run/preview queries. The
**engine** — the mapping/recipe→CEL compiler, YAML generation, and validation — is a
reusable Go core; the wizard is its first consumer. The same binary ships to customers
later so DB access stays local / behind their firewall (a cloud-hosted backend could
not reach a customer DB, which is why "hosted in the C1 platform" is not the v1 form).

## 8. Scope

**In (v0.0.1):** Configure + Validate over the resource-type model for the sync/read
path (`list` / `entitlements` / `grants`): multiple resource types; full user-trait
vocab + group/role/app profile traits; static **and** dynamic entitlements;
resource-scoped grants with `principal_type`/`skip_if`; alias-pick + transform-recipe
library + raw-CEL-with-live-preview; export valid YAML; read-only YAML preview.

**Out of v1 / fast-follows, in order:**
1. **`expandable`** (group/role membership fan-out) — an add-on on grants (8/21).
2. **Discovery** — read what the service account can access; show tables/columns for
   point-and-click; assemble starter queries; FK/join heuristics (deterministic).
3. **Remediate** — import an existing YAML, validate live, one-click trap fixes.
4. **Manage** — save/load configs; later push-to-C1 / full-sync test.

**Explicitly out (no plans this cycle):** any AI/LLM; write-side features
(`account_provisioning`, `credential_rotation`, `actions`); NHI/secret/agent traits
(rare — nhi-example only); multi-DB iteration (`connect.databases`; redshift only);
exclusions/denies (`exclusion_group`/`grant_replace`; 1 config — raw-CEL/hand-edit).

## 9. Success criterion

Someone who knows their own SQL — not C1's model — declares their resource types,
authors a query per (list/entitlements/grants) with column-pick + a recipe or two,
and produces a valid, **live-verified** baton-sql config for a *realistic*
multi-resource-type app (users + roles + groups, resource-scoped grants) in one
sitting: zero silent-failure traps, and CEL only where genuinely needed — assisted,
never hand-derived from scratch.

## 10. Risks & open questions

- **Recipe library coverage.** The transform recipes must cover the common CEL
  patterns (slugify, composite id, status ternary, coerce, null-default) well enough
  that raw-CEL is the exception. If it's not, users fall back to (3) too often and the
  "easy" promise erodes. Measure against the survey's CEL examples during build.
- **Dynamic-entitlement + resource-scoped-grant UX.** These are the structurally
  hardest steps (the EBS-family pattern) and where the model gap bites hardest;
  the guided sub-flow must make "bind this grant query to the current resource" and
  "these entitlements come from a query" feel obvious, not like YAML.
- **Multi-resource-type without re-creating YAML complexity.** Repeatable
  resource-type blocks risk feeling like a YAML tree in a UI. The declare-then-fill
  flow (step 2 → 3) must keep each block a small, self-contained task.
- **SQL dialect quirks.** Alias/identifier quoting differs across
  postgres/mysql/sqlserver/oracle/vertica; the compiler must respect per-dialect rules
  (baton-sql already maps schemes → placeholder styles).
- **`grantable_to`** is near-universal and required for provisioning-capable
  entitlements; ensure the entitlements step captures it as a first-class field, not
  an afterthought.
