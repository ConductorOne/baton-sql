![Baton Logo](./baton-logo.png)

# `baton-sql` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-sql.svg)](https://pkg.go.dev/github.com/conductorone/baton-sql) ![ci](https://github.com/conductorone/baton-sql/actions/workflows/ci.yaml/badge.svg) ![verify](https://github.com/conductorone/baton-sql/actions/workflows/verify.yaml/badge.svg)

`baton-sql` is a connector for built using the [Baton SDK](https://github.com/conductorone/baton-sdk).

## Overview

`baton-sql` is a flexible connector that enables you to sync identities, resources, and permissions from SQL databases. It provides a powerful configuration system that allows you to map database queries to resources and entitlements, with full support for account provisioning and automated password management.

## Key Features

- **Multi-Database Support**: Works with a range of SQL engines (see [Supported Database Engines](#supported-database-engines) below)
- **Account Provisioning**: Create user accounts with automatic random password generation
- **Secure Password Management**: Database-appropriate password hashing (SHA2, bcrypt, MD5)
- **Flexible Configuration**: Map any SQL query results to resources and entitlements
- **Role Management**: Sync and manage role assignments and permissions
- **Custom Schemas**: Support for any database schema through configurable SQL queries

## Supported Database Engines

- MySQL
- Microsoft SQL Server
- Oracle
- PostgreSQL
- SAP HANA
- Vertica
- Amazon Redshift
- WordPress (MySQL-based)
- IBM DB2 — opt-in: requires a binary built with the `db2` tag and IBM's native CLI driver; see [docs/db2.md](docs/db2.md)

## Configuration

The connector is configured using a YAML file that defines:

- **Database Connection**: Connection details via DSN (Data Source Name) and optional `connect.params`
- **Resource Types**: Map database tables/queries to resources (users, roles, etc.)
- **Account Provisioning**: Define schemas and credential options for user creation
- **Entitlements**: Permissions and roles that can be granted to resources
- **Provisioning Actions**: SQL queries for granting/revoking entitlements

For Postgres behind a transaction-mode pooler (PgBouncer, Supabase pooler on port 6543, etc.), set `default_query_exec_mode` to `simple_protocol` via the DSN query string or `connect.params` to avoid prepared-statement conflicts (SQLSTATE 42P05). When unset, baton-sql leaves the URL unchanged and pgx uses its default (`cache_statement`).

```yaml
connect:
  dsn: "postgres://${HOST}:6543/${DB}"
  params:
    default_query_exec_mode: simple_protocol
```

See examples in the [examples](https://github.com/ConductorOne/baton-sql/tree/main/examples) directory.

### Query placeholders

SQL queries reference variables with `?<name>` placeholders. Three modifiers are supported:

- `?<name>`: parameterized value. Bound through the driver (`$1`, `?`, `@p1`, `:1` depending on engine). Safe against SQL injection.
- `?<name|unquoted>`: strips non-alphanumeric characters from the value and inlines it directly. Intended for numeric pagination knobs (LIMIT/OFFSET) and trusted identifier-shaped values. Not safe for arbitrary user input (the sanitizer silently drops characters rather than escaping).
- `?<name|identifier>`: inlines the value as a properly-quoted SQL identifier. Engine-aware (backticks for MySQL, ANSI double-quotes elsewhere) with embedded quote characters doubled. Use this for identifier substitution in GRANT / REVOKE / DDL where parameter binding is not supported by the SQL grammar, e.g. `GRANT SELECT ON ?<schema|identifier>.?<tbl|identifier> TO ?<grantee|identifier>`.

### Multi-database iteration

A connector can be configured to iterate every database in a cluster instead of operating on a single handle. Add a `databases` block under `connect`:

```yaml
connect:
  dsn: "postgres://${HOST}:${PORT}/${ADMIN_DB}"
  user: "${USER}"
  password: "${PASSWORD}"
  databases:
    # Either a static list…
    static: ["analytics", "reporting"]
    # …or a discovery query whose first column is the list of database names.
    discovery_query: |
      SELECT datname FROM pg_database WHERE datistemplate = false
```

When `databases` is set, each `list:` / `entitlements:` / `grants:` block runs once per database. Add `scope: cluster` to a query that should only run once (against the lexicographically first database), useful for catalogs that return the same data from any database, like `pg_user`. The active database name is injected into every row as the synthetic column `database`, so `map:` blocks and `skip_if` expressions can reference `.database`. Provisioning queries route to the database named by the `database` provisioning var, falling back to the primary handle when unset.

Single-database configurations are unchanged. The `databases` block is opt-in and existing examples (`postgres-test.yml`, `mysql-test.yml`, etc.) continue to work identically.

## `baton-sql` Command Line Usage
```
Usage:
  baton-sql [flags]
  baton-sql [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  help               Help about any command

Flags:
      --client-id string       The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string   The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
      --config-path string     required: The file path to the baton-sql config to use ($BATON_CONFIG_PATH)
  -f, --file string            The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
  -h, --help                   help for baton-sql
      --log-format string      The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string       The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
  -p, --provisioning           This must be set in order for provisioning actions to be enabled ($BATON_PROVISIONING)
      --skip-full-sync         This must be set to skip a full sync ($BATON_SKIP_FULL_SYNC)
      --ticketing              This must be set to enable ticketing support ($BATON_TICKETING)
  -v, --version                version for baton-sql

Use "baton-sql [command] --help" for more information about a command.
```

# Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually
building spreadsheets. We welcome contributions, and ideas, no matter how
small&mdash;our goal is to make identity and permissions sprawl less painful for
everyone. If you have questions, problems, or ideas: Please open a GitHub Issue!

Check out [Baton](https://github.com/conductorone/baton) to learn more the project in general.

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.
