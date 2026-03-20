# Vertica Driver for baton-sql

**Date:** 2026-03-20
**Status:** Approved

## Overview

Add Vertica database support to baton-sql, following the existing driver pattern used by MySQL, PostgreSQL, Oracle, SQL Server, and SAP HANA. Uses the official `github.com/vertica/vertica-sql-go` driver.

## Driver Package

**New file:** `pkg/database/vertica/vertica.go`

Thin wrapper matching the HDB/Postgres pattern:

```go
package vertica

import (
    "context"
    "database/sql"

    _ "github.com/vertica/vertica-sql-go"
)

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
    db, err := sql.Open("vertica", dsn)
    if err != nil {
        return nil, err
    }
    return db, nil
}
```

**DSN format:** `vertica://user:pass@host:5433/dbname?param=value`

## Connection Wiring

**File:** `pkg/database/database.go`

1. Add `Vertica` to `DbEngine` enum after `HDB`:

```go
const (
    Unknown DbEngine = iota
    MySQL
    PostgreSQL
    SQLite
    MSSQL
    Oracle
    HDB
    Vertica
)
```

2. Import the new package:

```go
import (
    // ...existing imports...
    "github.com/conductorone/baton-sql/pkg/database/vertica"
)
```

3. Add case to `Connect()` switch:

```go
case "vertica":
    db, err := vertica.Connect(ctx, parsedDsn.String())
    if err != nil {
        return nil, Unknown, err
    }
    return db, Vertica, nil
```

## Dialect Handling

### Placeholder Syntax

**File:** `pkg/bsql/query.go` — `getNextPlaceholder()`

Vertica uses `?` (positional). Add explicit case for clarity:

```go
case database.Vertica:
    return "?"
```

### Time Format Parsing

**File:** `pkg/bsql/helpers.go` — `parseTimeWithEngine()`

Add Vertica case with PostgreSQL-like formats:

```go
case database.Vertica:
    prioritizedFormats = []string{
        "2006-01-02 15:04:05.000000-07:00",
        "2006-01-02 15:04:05.000000",
        "2006-01-02 15:04:05",
        time.RFC3339,
    }
```

### Boolean/Value Normalization

No changes needed. The existing `normalizeValue()` in `query.go` only applies special handling for Oracle (bool → `"1"`/`"0"` strings) and MSSQL (bool → `0`/`1` integers). All other engines, including Vertica, fall through to `return v` which preserves the native Go `bool`. Vertica has native `BOOLEAN` support — the driver scans `'t'`/`'f'` wire values directly to Go `bool`, so no conversion is required.

## Docker Compose Test Infrastructure

**New file:** `docker-compose-vertica-test.yml` (matches existing naming: `docker-compose-<db>-test.yml`)

- Image: `vertica/vertica-ce`
- Port: `5433:5433` (Vertica default port — note: not 5432 which is PostgreSQL)
- Default user: `dbadmin`
- Health check on port 5433

**New file:** `test/vertica-init.sql` (matches existing pattern: `test/<db>-init.sql`)

- Test schema with users, groups, and grants tables
- Matching the pattern of other database init scripts

## Dependencies

**`go.mod`:** Add `github.com/vertica/vertica-sql-go`

Run `go mod tidy` and `go mod vendor` (if vendor directory exists) after adding the dependency.

## Documentation

**`README.md`:** Add Vertica to supported databases list with example DSN.

## Files Changed

| File | Change Type |
|------|-------------|
| `pkg/database/vertica/vertica.go` | New |
| `pkg/database/database.go` | Modified (enum, import, switch) |
| `pkg/bsql/query.go` | Modified (placeholder case) |
| `pkg/bsql/helpers.go` | Modified (time format case) |
| `docker-compose-vertica-test.yml` | New |
| `test/vertica-init.sql` | New |
| `pkg/bsql/query_test.go` | Modified (Vertica placeholder test case) |
| `pkg/bsql/helpers_test.go` | Modified (Vertica time parsing test case) |
| `go.mod` / `go.sum` | Modified |
| `README.md` | Modified |

## Design Decisions

1. **Approach A (Minimal)** chosen over connection validation (B) and abstraction refactor (C) to match existing patterns and minimize risk.
2. **Official driver** (`vertica-sql-go`) chosen for maintenance and `database/sql` compatibility.
3. **`vertica://` scheme** matches driver's native DSN format.
4. **Explicit placeholder case** added for readability even though `default` already returns `"?"`.
5. **No boolean normalization** needed — Vertica has native boolean support unlike Oracle/MSSQL.
