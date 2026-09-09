# Provisioning: `validation_queries` semantics

`validation_queries` run before the provisioning `queries` in a grant or revoke. What a
**no-rows** result means depends on one per-entitlement flag.

## Default: no rows fails the operation

By default a `validation_query` returning no rows **fails the operation**. It is an existence
precondition that aborts loudly. This is the behavior on every engine unless you opt in below.

## Opt-in: no rows means idempotent success

Some engines apply `GRANT`/`REVOKE` as DDL that does not report rows-affected (Db2, Oracle), so
the connector cannot tell from the statement whether it changed anything, and re-running an
already-applied statement raises an error (Oracle `ORA-01951` on a repeat revoke, for example).
To make grant and revoke idempotent on these engines, set `validation_queries_signal_idempotency:
true` on the grant or revoke:

```yaml
grant:
  validation_queries_signal_idempotency: true
  validation_queries:
    - SELECT 1 FROM ... # returns a row only while the grant is MISSING
  queries:
    - GRANT ...
revoke:
  validation_queries_signal_idempotency: true
  validation_queries:
    - SELECT 1 FROM ... # returns a row only while the grant is PRESENT
  queries:
    - REVOKE ...
```

With the flag on, a `validation_query` returning no rows is reported as an **idempotent success**
(`GrantAlreadyExists` on grant, `GrantAlreadyRevoked` on revoke): no rows means "the state is
already as desired, there is no work to do".

The flag only takes effect on the DDL engines that need it (Db2, Oracle). On every other engine a
no-rows result still fails loudly regardless of the flag, because those engines report
rows-affected and don't need the reinterpretation. Default off, so existing configs are unchanged.

## Writing the query: "is there work to do?", not "does this exist?"

When the flag is on, your `validation_queries` must answer **"is there work to do?"**, not
**"does this principal or role exist?"**.

**Do not use them as existence preconditions.** A no-rows result is swallowed as idempotent
success, so a missing, deleted, or mistyped principal or role is reported as "already done"
instead of erroring. For example, a query like `SELECT 1 FROM users WHERE name = ?<user_id>`
silently masks a bad `user_id`: it returns no rows, and the grant is reported as
`GrantAlreadyExists` even though nothing was granted.

Write the query so no-rows genuinely means idempotent. For a grant, check whether the target
membership is **missing** (no rows => already granted); for a revoke, check whether it is
**present** (no rows => already revoked).

This mirrors the warning on `EntitlementProvisioningQueries.ValidationQueries` in
`pkg/bsql/config.go`. See [oracle.md](oracle.md) and [db2.md](db2.md) for engine-specific notes.
