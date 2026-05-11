# VALIDITY Report — baton-sql

_Generated 2026-05-11 (post-fix)_
_Branch validated: `cxh-1396-multi-database-iteration` (base: `main`)_

## Summary

SAFE TO MERGE after one CONTRADICTED claim was repaired in the diff. No UNBACKED claims; no BLOCKING issues remain after the fix.

## Required changes (in order of importance)

1. **[FIXED in this branch]** `resolveProvisioningDB` provisioning-fallback routed to the current iteration handle (i.e. the lexicographically last database after sync) rather than the primary handle the README documents. Coupling provisioning to sync state would have caused per-table provisioning that omits the `database` var to land in the wrong database. Code now returns the primary handle when the var is unset or empty; the test was tightened to point `s.db` at a non-primary handle so the assertion is independent of sync state. (`pkg/bsql/query.go` `resolveProvisioningDB`, `pkg/bsql/multidb_test.go` `TestResolveProvisioningDB_RoutesByVars`.)
2. [NIT — defer] `ConnectMany` engine-mismatch path relies on the implicit contract that `Connect` returns `nil` on every error path. Currently safe; defensive guard would tighten the contract.
3. [NIT — defer] README does not call out that the `database` provisioning var name is reserved for routing, nor that the synthetic `.database` row column is also injected in single-DB configs (additive, harmless, but worth a sentence).
4. [NIT — defer] `resourceToCELMap` does not nil-guard `resource.Id` before dereferencing `.Resource` / `.ResourceType`. Callers gate on `resource != nil` but not `resource.Id`. Unlikely to fire in practice (SDK populates Id on every reconstructed resource) but a one-line guard would close the panic surface.
5. [NIT — defer] No direct unit test for `resourceToCELMap`'s empty-profile default (the `has(resource.profile.X) == false` no-trait case). Exercised indirectly only.

## Documentation backing

| Change | Verdict | Source |
|---|---|---|
| `?<var\|identifier>` engine-aware quoting (MySQL backticks; ANSI `"…"` elsewhere; embedded quotes doubled) | BACKED | PostgreSQL §4.1.1, MySQL identifiers, Oracle 19c SQL Reference, Redshift `r_names.html`; `pkg/bsql/query.go` `quoteIdentifier`; tests in `query_test.go`. |
| `?<var\|unquoted>` strips non-alphanumeric; drops not escapes | BACKED | `pkg/bsql/query.go` `SanitizeIdentifier`; injection-attempt test in `query_test.go`. |
| `identifier` and `unquoted` mutually exclusive | BACKED | `pkg/bsql/query.go` `parseToken`; test in `query_test.go`. |
| `connect.databases` `static` XOR `discovery_query` | BACKED | `pkg/bsql/config.go` `DatabasesConfig.Validate`; `TestDatabasesConfig_Validate`. |
| `scope: cluster` runs once against lexicographically first DB | BACKED | `pkg/bsql/multidb.go`; `pkg/connector/connector.go` `openDatabases` (sort.Strings); `TestIterateDBs_ClusterScopeRunsOnceAgainstPrimary`. |
| Synthetic `.database` row column; real column wins | BACKED | `pkg/bsql/query.go` row-map injection. |
| Provisioning routes via `vars.database`, falls back to primary when unset | BACKED **after fix** (was CONTRADICTED) | `pkg/bsql/query.go` `resolveProvisioningDB`; `TestResolveProvisioningDB_RoutesByVars`. |
| `resource.profile.<key>` readable from CEL | BACKED | cel-go `MapType(StringType, AnyType)` allows nested selection (`checker/checker.go` `MapKind`); `pkg/bcel/bcel.go` `resourceToCELMap`. |
| Single-database configs unchanged (wire-compatible page tokens) | BACKED | `pkg/bsql/multidb.go` short-circuit returns inner token byte-for-byte; `TestIterateDBs_SingleDatabasePathIsTransparent`. |
| `pagination.Bag` is LIFO; reverse-push yields sorted iteration | BACKED | `vendor/baton-sdk/pkg/pagination/pagination.go` (`Current()` returns most-recently-pushed); `TestIterateDBs_MultiDatabaseVisitsEveryDBInOrder`. |
| `ConnectMany` closes all prior handles on per-database failure | BACKED | `pkg/database/database.go` `closeAll`; `Connect`'s nil-on-error contract. |

## Code review findings

`go build ./...` — exit 0.  
`go vet ./...` — exit 0.  
`go test ./...` — all packages pass.  
`golangci-lint run ./pkg/bsql/... ./pkg/database/... ./pkg/connector/... ./pkg/bcel/...` — 20 findings, all pre-existing `goconst` warnings on lines untouched by this PR.

No swallowed errors, no hardcoded secrets, every signature change propagated. Multi-DB state mutation (`s.db` / `s.currentDBName`) safe given the SDK serializes List/Entitlements/Grants per syncer; no goroutines spawned inside iteration.

## Pagination verification

Skipped — diff does not touch HTTP pagination (the `pagination.Bag` use here drives multi-DB fan-out, not page-cursor following).
