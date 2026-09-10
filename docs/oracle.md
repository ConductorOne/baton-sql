# Oracle

baton-sql talks to Oracle through the pure-Go `go-ora` driver, so no native client is needed
(unlike Db2). Connect with an `oracle://` DSN:

```yaml
connect:
  dsn: "oracle://${DB_HOST}:${DB_PORT}/${DB_SERVICE}"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
```

See [examples/oracle-test.yml](../examples/oracle-test.yml) for a full config (users, roles,
privileges, account provisioning, enable/disable/update actions).

## Idempotent grant / revoke: `validation_queries_signal_idempotency`

Oracle applies `GRANT`/`REVOKE` of roles and privileges as DDL that does not report
rows-affected, and re-running an already-applied statement raises an error. A repeat revoke of a
role the user no longer has fails with `ORA-01951: ROLE '...' not granted to '...'`. So a resync
that re-issues a revoke, or a grant of a role the user already holds, would surface as a failure
even though nothing needs to change.

To make grant and revoke idempotent, set `validation_queries_signal_idempotency: true` on the
grant or revoke and add a `validation_query` that answers **"is there work to do?"**. When the
query returns no rows, the connector reports an idempotent success (`GrantAlreadyExists` on grant,
`GrantAlreadyRevoked` on revoke) instead of running the DDL and hitting the error.

```yaml
grant:
  no_transaction: true
  validation_queries_signal_idempotency: true
  # returns a row only while the role is NOT yet granted (no rows => already granted)
  # exact-match (no UPPER): the GRANT quotes identifiers, so the stored GRANTEE/GRANTED_ROLE
  # are case-sensitive and must equal the bound value
  validation_queries:
    - |
      SELECT 1 FROM dual WHERE NOT EXISTS (
        SELECT 1 FROM DBA_ROLE_PRIVS
        WHERE GRANTEE = ?<principal_name> AND GRANTED_ROLE = ?<role_name>
      )
  queries:
    - GRANT ?<role_name|identifier> TO ?<principal_name|identifier>
revoke:
  no_transaction: true
  validation_queries_signal_idempotency: true
  # returns a row only while the role IS still granted (no rows => already revoked)
  validation_queries:
    - |
      SELECT 1 FROM DBA_ROLE_PRIVS
      WHERE GRANTEE = ?<principal_name> AND GRANTED_ROLE = ?<role_name>
  queries:
    - REVOKE ?<role_name|identifier> FROM ?<principal_name|identifier>
```

Use `|identifier` for role and principal names so they are engine-quoted (safe against
injection and case-sensitive). System-privilege entitlements are different: privilege names are
multiword keywords like `CREATE SESSION`, so their DDL operand must use `|keyword`, not
`|identifier` (quoting would break the clause and `|unquoted` would strip the space):

```yaml
queries:
  - GRANT ?<privilege_name|keyword> TO ?<principal_name|unquoted>
```

The flag is off by default, so without it Oracle keeps failing loudly on the repeat operation.
The same warning as Db2 applies: with the flag on, do **not** use `validation_queries` as
existence preconditions, since a no-rows result is swallowed as an idempotent success. See
[provisioning.md](provisioning.md) for the full explanation.
