# baton-sql Studio — Design

**Date:** 2026-07-29
**Status:** Approved design (pre-implementation)
**Author:** Charles Kramer (with Claude)

## 1. Problem

Authoring a `baton-sql` connector config today is slow, error-prone, and largely
unguided. Compared to platforms like Lumos — where you "just plug in the queries as
you define them" — ConductorOne requires substantially more structure, and there is
no easy way to validate a config as you build it.

The difficulty breaks into four distinct layers, which customers hit **roughly
equally** (there is no single dominant blocker):

1. **The model gap.** The customer thinks in "my users table, my roles table, who
   has what." C1 thinks in *resource types → entitlements → grants → principals*.
   Translating one into the other is a conceptual leap, not a syntax problem.
2. **The expression gap.** Even knowing what they want, the author must hand-write
   CEL and structure YAML exactly right (`map.id`, `traits.user.emails[0]`, static
   vs. dynamic entitlements, `principal_type`, etc.).
3. **The silent-failure gap.** A config can *parse and even run* yet produce wrong
   or empty data with **no error**. There are ~17 known traps (e.g. `mfa_enabled`
   is a no-op; `manager_id` is dropped if `profile` is otherwise empty; a
   `pagination` block with no `?<Offset>`/`?<Cursor>` token is silently ignored;
   dynamic entitlements silently require `slug`). This is the worst layer — you
   don't know you failed until a sync looks wrong.
4. **The feedback-loop gap.** You only find out it's wrong *late*, after wiring the
   connector up and running a full sync. There is no "does this query work, and do
   its columns cover what C1 needs" moment during authoring.

**Constraint:** No AI/LLM in the loop, at least short-term. All intelligence must be
deterministic.

## 2. Audience & sequencing

The real audience is **always the customer** — the person who generates these
configs. A ConductorOne SE is only the *first test user*; the tool must ultimately
be safe and usable by a customer who does not know C1's model.

- **v0.0.1 (MVP):** Configure + Validate.
- **Fast-follows (built next, in order):** service-account Discovery → Remediate →
  Manage. "Deferred" here means *build next*, not *someday*.

## 3. Core idea — slots carry the model, aliases carry the mapping

Two deterministic mechanisms erase the four gaps without AI:

**(a) Slots carry the model.** The customer never learns C1's vocabulary. Studio
presents C1's required **query slots** (Users, Resources, Entitlements, Grants), and
for each the customer supplies one simple SQL query. The *slots themselves* encode
C1's model — the author only ever answers "here's my users query," "here's my
who-has-what query." This closes the **model gap** structurally.

**(b) Aliases carry the mapping (Lumos-parity).** Column→field mapping is expressed
as **SQL column aliases inside the SELECT**, exactly as Lumos's JDBC connector does
(`SELECT user_id AS id, email AS email, status AS status …`). Studio publishes a small,
documented set of **canonical field names** per slot; the customer aliases their
columns to those names. Studio then **deterministically compiles alias → baton CEL/
YAML**. Because Studio generates the CEL (the customer never writes it), the
silent-failure traps become **structurally impossible**: Studio will not emit a
no-op key, auto-adds a `profile` entry when a manager alias is present, requires
`slug` on dynamic entitlements, ensures pagination tokens appear in the query, etc.
This closes the **expression** and **silent-failure** gaps.

Running each query live against the real database — showing returned columns and
rows — closes the **feedback-loop** gap.

### Why aliases over a bespoke dropdown-mapping layer

baton-sql maps columns via separate CEL in YAML, which is exactly where authors ship
broken configs. Lumos maps via aliases in the query — SQL the author already knows,
with no separate expression layer. Studio adopts the alias mechanism as the source
of truth. **Dropdowns are optional sugar:** after a query runs live, any unaliased
column can be assigned a canonical field via a dropdown, which simply rewrites the
alias into the SELECT. Alias-in-query and dropdown are two views of the same truth;
an existing alias pre-fills the dropdown, and a dropdown choice writes the alias.

### Relationship to Lumos (reference)

Lumos's JDBC connector uses **three** read queries, each pasted into their UI, mapped
via required output-column aliases:

- **Read Accounts:** `id AS integration_specific_id, email, first_name AS given_name,
  last_name AS family_name, uname AS username, status AS user_status`
- **Read Entitlements:** `type AS entitlement_type, id AS integration_specific_id,
  resource_id AS integration_specific_resource_id, assignable AS is_assignable,
  name AS label, description`
- **Read Associations (grants):** `user_id AS account_id, entitlement_id AS
  integration_specific_entitlement_id, resource_id AS integration_specific_resource_id`

Key lessons we adopt:

- **Entitlement "type" (role/group/permission) is just a column value**, not a
  structural choice. This defuses baton-sql's static-list-vs-dynamic-query fork:
  Studio's default is "entitlements come from a query" with a `type` column. A static
  list remains an escape hatch, not the front door.
- **Grants are always a dedicated join query** returning the principal id + the
  entitlement id (+ optional resource id); identifier columns link the queries.
- **Resources ride along** as a `resource_id` column rather than always being a
  hand-authored slot.

Key difference we keep: we do **not** flatten to Lumos's three-query world. Studio
retains baton-sql's model depth (multiple resource types, user traits) via the slots;
Lumos only informs the *mapping ergonomics*.

Sources: developers.lumos.com — Database (JDBC) Connector, Modeling Permissions,
Object Model, Core Concepts.

## 4. The wizard flow (v0.0.1)

1. **Connect.** DB scheme/host/port/database/credentials. "Test connection" must go
   green before proceeding. (baton-sql schemes: postgres, mysql, sqlserver, oracle,
   hdb, vertica.)
2. **Users.** Author/paste a query and alias columns to the Users canonical vocab.
   Run live → see real returned columns and sample rows. Studio compiles the aliased
   query into `resource_types.<user>.list` with the correct `map` + `traits.user`
   CEL. Optional dropdowns assign fields to any unaliased columns.
3. **Resources.** The things access is *to*. Author/alias a query → map `id` /
   `display_name`. (May also be satisfied by a ride-along `resource_id` on the
   entitlements/grants queries.)
4. **Entitlements.** Default: entitlements come from a query returning `id`,
   `display_name`, and a `type` column. Studio compiles to a dynamic entitlements
   block, **always emitting `slug`**. Static list available as an escape hatch.
5. **Grants.** The "who-has-what" join query. Alias the principal id column and the
   entitlement id column; `principal_type` is chosen from a dropdown of the resource
   types already defined (so it cannot mismatch). Studio compiles to `grants[]`.
6. **Review & export.** Read-only YAML preview + a green static-validation summary +
   download of the `.yaml`. (No full split editor in v1; the preview is read-only.)

## 5. Validation (layered)

- **Static, instant, authoritative.** Compile the aliased queries to YAML, then parse
  and validate that YAML through baton-sql's **own** `pkg/bsql` (`config.go` +
  `validate.go`) — never a reimplementation, so the validator cannot drift from what
  the connector actually accepts. Additional checks: every canonical field's aliased
  column exists in the query's SELECT list; the gotcha/trap ruleset.
- **Live, on-demand.** Run a query against the connected DB; show returned columns
  and sample rows; confirm mapping targets exist. Any query not yet run live is
  flagged **"unproven."**

## 6. Architecture (kept light)

A single local Go binary serves a browser UI. The Go backend imports `pkg/bsql`
directly for authoritative validation and opens the customer DB connection to run/
preview queries. The **engine** — alias→YAML compiler, validation, and mapping logic
— is a reusable Go core; the wizard is its first consumer. The same binary ships to
customers later, so DB access stays local / behind the customer's firewall (a
cloud-hosted backend could not reach a customer DB, which is why "hosted in the C1
platform" is not the v1 form factor).

## 7. Scope

**In (v0.0.1):** Configure + Validate over the aliased-query flow for the sync/read
path (`list` / `entitlements` / `grants`); export valid YAML; read-only YAML preview.

**Out of v1 / fast-follows, in order:**
1. **Discovery** — read what the service account can access; show tables/columns for
   point-and-click; assemble starter queries; FK/join heuristics to suggest grant
   sources (deterministic, no AI).
2. **Remediate** — import an existing YAML, validate it, offer actionable (often
   one-click) fixes for the trap catalog.
3. **Manage** — save/load configs; later push-to-C1 / full-sync test.

**Explicitly out (no plans in this cycle):** any AI/LLM; provisioning, credential
rotation, and actions (write path); multi-DB iteration wizard support (raw-YAML only);
a full split code editor.

## 8. Success criterion

Someone who knows only their own SQL — not C1's model — aliases a few columns across
four queries and produces a valid, **live-verified** baton-sql config for a typical
users+roles+grants database in one sitting: zero silent-failure traps, zero
hand-written CEL.

## 9. Risks & open questions

- **Canonical vocab completeness.** The alias vocabulary must cover the common cases
  cleanly and offer a graceful escape for the uncommon ones (e.g. a field needing a
  real CEL transform, not a straight column rename). Escape hatch: allow a raw CEL
  override on an individual field without abandoning the wizard.
- **Entitlements shape variance.** Even with the "type-as-column" default, customers
  model roles/permissions inconsistently (dedicated table vs. values embedded in the
  grants query). The Entitlements slot needs strong defaults and should support
  "my entitlement values come from a column in the grants query" as a first-class
  path.
- **Alias collisions / SQL dialect quirks.** Aliasing to reserved-ish names across
  postgres/mysql/sqlserver/oracle/vertica may need quoting; the compiler must handle
  per-dialect identifier rules (baton-sql already maps schemes → placeholder styles).
- **Multiple resource types & traits** are baton-sql capabilities Lumos lacks;
  exposing them without reintroducing model-gap complexity is a UX design task for a
  later iteration.
