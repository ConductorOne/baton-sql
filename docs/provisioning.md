# Provisioning: `validation_queries` semantics

`validation_queries` run before the provisioning `queries` in a grant or revoke. What a
**no-rows** result means depends on the engine.

## Default: no rows fails the operation

On every engine except Db2, a `validation_query` returning no rows **fails the operation**.
It is an existence precondition that aborts loudly. This includes the DDL engines that don't
report rows-affected (Oracle): treating their no-rows as idempotency is a follow-up that needs
a per-config opt-in first, so today they still fail loudly like everyone else.

## Db2 (opt-in behind the `db2` build tag)

Db2 applies `GRANT`/`REVOKE` as DDL that does not report rows-affected, so the connector
cannot tell from the statement itself whether it changed anything, and an already-applied
statement raises an error. To make grant and revoke idempotent, a `validation_query`
returning no rows is reported as an **idempotent success** (`GrantAlreadyExists` on grant,
`GrantAlreadyRevoked` on revoke). No rows means "the state is already as desired, there is no
work to do". Db2 ships opt-in behind the `db2` build tag, so no default-build engine changes
behavior.

Because of this, on Db2 your `validation_queries` must answer **"is there work to do?"**, not
**"does this principal or role exist?"**.

**Do not use `validation_queries` as existence preconditions on Db2.** A no-rows result is
swallowed as idempotent success, so a missing, deleted, or mistyped principal or role is
reported as "already done" instead of erroring. For example, a validation query like
`SELECT 1 FROM users WHERE name = ?<user_id>` will silently mask a bad `user_id`: it returns
no rows, and the grant is reported as `GrantAlreadyExists` even though nothing was granted.

Write the query so no-rows genuinely means idempotent. For a grant, check whether the target
membership is **missing** (no rows => already granted); for a revoke, check whether it is
**present** (no rows => already revoked).

This mirrors the warning on `EntitlementProvisioningQueries.ValidationQueries` in
`pkg/bsql/config.go`.
