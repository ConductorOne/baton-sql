# Date & time values

Two fields in `traits.secret` accept datetime values from your SQL query results:

| Field | Trait | Description |
|---|---|---|
| `expires_at` | `traits.secret` | When the credential expires |
| `last_used_at` | `traits.secret` | When the credential was last used |

```yaml
traits:
  secret:
    expires_at: ".expires_at"      # column or CEL expression
    last_used_at: ".last_used_at"  # column or CEL expression
```

## Accepted formats

The connector accepts both string timestamps and numeric epoch columns. The formats below are exactly those covered by the test suite — every entry has a passing test.

### RFC 3339 / ISO 8601

| Example | Notes |
|---|---|
| `2025-09-01T00:00:00Z` | UTC, no fractional seconds |
| `2025-09-01T00:00:00+05:30` | With timezone offset |
| `2025-04-17T14:30:45.123Z` | Millisecond precision |
| `2025-04-17T14:30:45.123456789Z` | Nanosecond precision |
| `2025-04-17T14:30:45.123-05:00` | Offset with fractional seconds |

### SQL space-separated (MySQL, PostgreSQL, SQL Server, SQLite)

| Example | Notes |
|---|---|
| `2025-04-17 14:30:45` | Basic DATETIME / TIMESTAMP |
| `2025-04-17 14:30:45.123` | Millisecond precision (MySQL) |
| `2025-04-17 14:30:45.123456` | Microsecond precision (PostgreSQL) |
| `2025-04-17 14:30:45.123456789` | Nanosecond precision |

### Date only

| Example | Notes |
|---|---|
| `2025-04-17` | ISO date — no time component |

### US / European slash notation

| Example | Notes |
|---|---|
| `04/17/2025 14:30:45` | US: MM/DD/YYYY with time |
| `17/04/2025 14:30:45` | European: DD/MM/YYYY with time |
| `04/17/2025` | US short date |
| `17/04/2025` | European short date |

> **Ambiguity note:** when the day and month are both ≤ 12 (e.g. `05/06/2025`), US format is tried first and wins. Use an unambiguous column value or cast it to ISO 8601 in your query if this matters.

### Oracle NLS_DATE_FORMAT variants

| Example | Notes |
|---|---|
| `17-APR-2025 14:30:45` | Uppercase month abbreviation |
| `17-Apr-2025 14:30:45` | Mixed-case month abbreviation |
| `17-APR-25 14:30:45` | Uppercase abbreviation, 2-digit year |
| `17-Apr-25 14:30:45` | Mixed-case abbreviation, 2-digit year |
| `Apr 17, 2025 14:30:45` | Short-name, comma-separated |
| `April 17, 2025 14:30:45` | Full month name |
| `17-04-2025` | Day-month-year, date only |
| `17-04-25` | Day-month-year, 2-digit year |

### DB2 TIMESTAMP

| Example | Notes |
|---|---|
| `2025-04-17-14.30.45.123456` | DB2 native TIMESTAMP string |

### Numeric epoch columns

| Example | Notes |
|---|---|
| `1756684800` | Unix seconds (accepted range: 1970–2100) |
| `1756684800000` | Unix milliseconds (accepted range matching the same window) |

### Vertica TIMESTAMPTZ

Vertica's timezone-aware timestamp format (`2025-04-17 14:30:45.123456-04`) is handled by the engine-aware path and does not require any extra configuration.

## Failure behavior

Datetime parsing is **best-effort**. If a value cannot be parsed in any of the formats above, the connector logs a warning and skips that field — the resource is still synced, just without `expires_at` or `last_used_at` set. Nothing fails.

To diagnose a skipped value, enable debug logging and look for:

```
failed to parse secret expires_at   expires_at=<raw value>
failed to parse secret last_used_at last_used_at=<raw value>
```

If your column format is not in the list above, cast it to ISO 8601 in your query:

```sql
-- PostgreSQL
SELECT id, name,
       TO_CHAR(expires_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS expires_at
FROM api_tokens
```
