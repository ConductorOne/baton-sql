# Add Vertica Driver Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Vertica database support to baton-sql following existing driver patterns.

**Architecture:** Thin driver wrapper in `pkg/database/vertica/`, wired into the central `Connect()` router via `DbEngine` enum. Dialect-specific handling added for placeholders and time formats. Docker Compose test infrastructure included.

**Tech Stack:** Go, `github.com/vertica/vertica-sql-go`, Docker (`vertica/vertica-ce`)

**Spec:** `docs/superpowers/specs/2026-03-20-vertica-driver-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/database/vertica/vertica.go` | Create | Vertica driver Connect() wrapper |
| `pkg/database/database.go` | Modify | Add Vertica enum, import, switch case |
| `pkg/bsql/query.go` | Modify | Add Vertica placeholder case |
| `pkg/bsql/helpers.go` | Modify | Add Vertica time format case |
| `test/vertica-init.sql` | Create | Test database schema and seed data |
| `docker-compose-vertica-test.yml` | Create | Docker Compose for Vertica CE |
| `examples/vertica-test.yml` | Create | Example connector config for Vertica |
| `pkg/bsql/query_test.go` | Modify | Add Vertica placeholder test case |
| `pkg/bsql/helpers_test.go` | Modify | Add Vertica time parsing test case |
| `go.mod` / `go.sum` | Modify | Add vertica-sql-go dependency |
| `README.md` | Modify | Add Vertica to supported databases |

---

## Chunk 1: Driver Package and Connection Wiring

### Task 1: Add Vertica driver dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the vertica-sql-go dependency**

Run:
```bash
cd /Users/ali.falahi/Documents/Github/baton-sql
go get github.com/vertica/vertica-sql-go
```

- [ ] **Step 2: Tidy and vendor the dependency**

Run:
```bash
go mod tidy && go mod vendor
```

- [ ] **Step 3: Verify the dependency is in go.mod**

Run:
```bash
grep vertica go.mod
```
Expected: line containing `github.com/vertica/vertica-sql-go`

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum vendor/
git commit -m "chore: add vertica-sql-go dependency"
```

---

### Task 2: Create Vertica driver package

**Files:**
- Create: `pkg/database/vertica/vertica.go`

- [ ] **Step 1: Create the driver package**

Create `pkg/database/vertica/vertica.go`:

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

This mirrors `pkg/database/hdb/hdb.go` exactly.

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build ./pkg/database/vertica/
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/database/vertica/vertica.go
git commit -m "feat: add vertica driver package"
```

---

### Task 3: Wire Vertica into the connection router

**Files:**
- Modify: `pkg/database/database.go:25-33` (DbEngine enum)
- Modify: `pkg/database/database.go:3-19` (imports)
- Modify: `pkg/database/database.go:333-371` (Connect switch)

- [ ] **Step 1: Add Vertica to the DbEngine enum**

In `pkg/database/database.go`, add `Vertica` after `HDB` in the const block:

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

- [ ] **Step 2: Add the import**

Add to the import block in `pkg/database/database.go`:

```go
"github.com/conductorone/baton-sql/pkg/database/vertica"
```

- [ ] **Step 3: Add the switch case**

Add a new case in the `Connect()` function's switch statement (after the `"hdb"` case, before `default`):

```go
	case "vertica":
		db, err := vertica.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, Vertica, nil
```

- [ ] **Step 4: Verify it compiles**

Run:
```bash
go build ./pkg/database/...
```
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add pkg/database/database.go
git commit -m "feat: wire vertica into connection router"
```

---

## Chunk 2: Dialect Handling

### Task 4: Add Vertica placeholder case

**Files:**
- Modify: `pkg/bsql/query.go:56-71` (getNextPlaceholder)

- [ ] **Step 1: Add explicit Vertica case to getNextPlaceholder**

In `pkg/bsql/query.go`, in the `getNextPlaceholder` method, add a case for Vertica alongside MySQL and SQLite (since they all use `?`):

```go
func (s *SQLSyncer) getNextPlaceholder(qArgs []interface{}) string {
	switch s.dbEngine {
	case database.MySQL:
		return "?"
	case database.PostgreSQL:
		return fmt.Sprintf("$%d", len(qArgs))
	case database.SQLite:
		return "?"
	case database.MSSQL:
		return fmt.Sprintf("@p%d", len(qArgs))
	case database.Oracle:
		return fmt.Sprintf(":%d", len(qArgs))
	case database.Vertica:
		return "?"
	default:
		return "?"
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build ./pkg/bsql/
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/bsql/query.go
git commit -m "feat: add vertica placeholder case"
```

---

### Task 5: Add Vertica time format parsing

**Files:**
- Modify: `pkg/bsql/helpers.go:88-136` (parseTimeWithEngine)

- [ ] **Step 1: Add Vertica case to parseTimeWithEngine**

In `pkg/bsql/helpers.go`, add a `case database.Vertica:` block in the `parseTimeWithEngine` switch, after the Oracle case and before the `default`:

```go
	case database.Vertica:
		// Vertica common formats (similar to PostgreSQL with timezone support)
		prioritizedFormats = []string{
			"2006-01-02 15:04:05.000000-07:00",
			"2006-01-02 15:04:05.000000",
			"2006-01-02 15:04:05",
			time.RFC3339,
		}
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build ./pkg/bsql/
```
Expected: no errors

- [ ] **Step 3: Run existing tests to ensure no regressions**

Run:
```bash
go test ./pkg/bsql/ -v -count=1
```
Expected: all existing tests pass

- [ ] **Step 4: Commit**

```bash
git add pkg/bsql/helpers.go
git commit -m "feat: add vertica time format parsing"
```

---

### Task 6: Add Vertica test cases to query_test.go

**Files:**
- Modify: `pkg/bsql/query_test.go:228` (after Oracle test case in Test_parseQueryOpts)

- [ ] **Step 1: Add Vertica placeholder test case**

In `pkg/bsql/query_test.go`, add a new test case in `Test_parseQueryOpts` after the Oracle test case (after line 228):

```go
		{
			"Test valid query with multiple replacements (Vertica)",
			database.Vertica,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table LIMIT ? OFFSET ?",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
```

- [ ] **Step 2: Run the test to verify it passes**

Run:
```bash
go test ./pkg/bsql/ -run Test_parseQueryOpts -v -count=1
```
Expected: all test cases pass, including the new Vertica case

- [ ] **Step 3: Commit**

```bash
git add pkg/bsql/query_test.go
git commit -m "test: add vertica placeholder test case"
```

---

### Task 7: Add Vertica test cases to helpers_test.go

**Files:**
- Modify: `pkg/bsql/helpers_test.go:151` (before closing brace of TestParseTimeWithEngine test cases)

- [ ] **Step 1: Add Vertica time parsing test cases**

In `pkg/bsql/helpers_test.go`, add new test cases in `TestParseTimeWithEngine` after the existing cases (before line 152):

```go
		{
			name:          "Vertica timestamp with timezone",
			input:         "2025-04-17 14:30:45.123456",
			dbEngine:      database.Vertica,
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 123456000, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Vertica timestamp without microseconds",
			input:         "2025-04-17 14:30:45",
			dbEngine:      database.Vertica,
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
```

- [ ] **Step 2: Run the test to verify it passes**

Run:
```bash
go test ./pkg/bsql/ -run TestParseTimeWithEngine -v -count=1
```
Expected: all test cases pass, including the new Vertica cases

- [ ] **Step 3: Commit**

```bash
git add pkg/bsql/helpers_test.go
git commit -m "test: add vertica time parsing test cases"
```

---

## Chunk 3: Test Infrastructure

### Task 8: Create Vertica test init SQL

**Files:**
- Create: `test/vertica-init.sql`

- [ ] **Step 1: Create the init SQL file**

Create `test/vertica-init.sql` with the same schema pattern as `test/postgres-init.sql`, adapted for Vertica syntax:

```sql
-- Create users table
CREATE TABLE IF NOT EXISTS users (
  id AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(100) NOT NULL,
  email VARCHAR(255) NOT NULL,
  employee_id VARCHAR(50),
  status VARCHAR(20) DEFAULT 'active',
  account_type VARCHAR(20) DEFAULT 'human',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  last_login TIMESTAMP NULL,
  manager_id INTEGER
);

-- Insert sample users
INSERT INTO users (username, email, employee_id, status, account_type, created_at, last_login) VALUES
('admin', 'admin@example.com', 'EMP001', 'active', 'human', '2025-01-01 12:00:00', '2025-04-15 09:30:00'),
('jane.doe', 'jane.doe@example.com', 'EMP002', 'active', 'human', '2025-01-05 14:30:00', '2025-04-17 08:45:00'),
('john.smith', 'john.smith@example.com', 'EMP003', 'active', 'human', '2025-01-10 09:45:00', '2025-04-16 16:20:00'),
('service.acct', 'service@example.com', 'SVC001', 'active', 'service', '2025-02-01 08:00:00', NULL),
('disabled.user', 'disabled@example.com', 'EMP004', 'disabled', 'human', '2025-02-15 10:15:00', '2025-03-01 11:10:00');

-- Update manager relationships
UPDATE users SET manager_id = 1 WHERE username IN ('jane.doe', 'john.smith');
UPDATE users SET manager_id = 2 WHERE username = 'service.acct';
UPDATE users SET manager_id = 3 WHERE username = 'disabled.user';

-- Create roles table
CREATE TABLE IF NOT EXISTS roles (
  id AUTO_INCREMENT PRIMARY KEY,
  role_name VARCHAR(100) NOT NULL
);

-- Insert sample roles
INSERT INTO roles (role_name) VALUES ('admin');
INSERT INTO roles (role_name) VALUES ('user');
INSERT INTO roles (role_name) VALUES ('reader');

-- Create user_roles table
CREATE TABLE IF NOT EXISTS user_roles (
  user_id INTEGER,
  role_id INTEGER,
  PRIMARY KEY (user_id, role_id)
);

-- Assign roles to users
INSERT INTO user_roles (user_id, role_id) VALUES (1, 1);
INSERT INTO user_roles (user_id, role_id) VALUES (2, 2);
INSERT INTO user_roles (user_id, role_id) VALUES (3, 2);
INSERT INTO user_roles (user_id, role_id) VALUES (3, 3);
INSERT INTO user_roles (user_id, role_id) VALUES (4, 2);

SELECT 'Vertica test database initialized successfully' AS message;
```

Note: Vertica uses `AUTO_INCREMENT` instead of `SERIAL` for auto-increment columns. Vertica does not support `FOREIGN KEY` constraints, so they are omitted. Multi-row `INSERT INTO ... VALUES (...), (...)` syntax is supported in Vertica, but individual inserts are used for roles/user_roles for clarity.

- [ ] **Step 2: Commit**

```bash
git add test/vertica-init.sql
git commit -m "test: add vertica init sql schema"
```

---

### Task 9: Create Docker Compose file for Vertica

**Files:**
- Create: `docker-compose-vertica-test.yml`

- [ ] **Step 1: Create the Docker Compose file**

Create `docker-compose-vertica-test.yml`:

```yaml
services:
  # Vertica CE Database for testing
  vertica:
    image: vertica/vertica-ce:latest
    container_name: baton-vertica-test
    ports:
      - "5433:5433"
    environment:
      VERTICA_DB_NAME: batondb
    volumes:
      - ./test/vertica-init.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD", "/opt/vertica/bin/vsql", "-U", "dbadmin", "-d", "batondb", "-c", "SELECT 1"]
      interval: 10s
      timeout: 10s
      retries: 30
      start_period: 120s
```

Note: Vertica CE takes longer to start than most databases (1-2 minutes), so `start_period` is set to 120s and retries to 30. The `vertica/vertica-ce` image supports `/docker-entrypoint-initdb.d/` for init scripts — verify this works when testing the Docker setup. If the init script is not automatically executed, run it manually via `vsql` after the container starts.

- [ ] **Step 2: Commit**

```bash
git add docker-compose-vertica-test.yml
git commit -m "test: add docker compose for vertica"
```

---

### Task 10: Create example Vertica connector config

**Files:**
- Create: `examples/vertica-test.yml`

- [ ] **Step 1: Create the example config**

Create `examples/vertica-test.yml`:

```yaml
---
app_name: Vertica Test
app_description: Test configuration for Vertica database
connect:
  dsn: "vertica://${DB_HOST}:${DB_PORT}/${DB_DATABASE}?tlsmode=none"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"

resource_types:
  user:
    name: "User"
    description: "A user within the Vertica system"
    list:
      query: |
        SELECT
          u.id,
          u.username,
          u.email,
          u.employee_id,
          u.status,
          u.account_type,
          u.created_at,
          u.last_login,
          u.manager_id,
          m.username as manager_username
        FROM
          users u
        LEFT JOIN
          users m ON u.manager_id = m.id
      pagination:
        strategy: "offset"
        primary_key: "id"
      map:
        id: ".username"
        display_name: ".username"
        description: ".username"
        traits:
          user:
            status: ".status"
            login: ".username"
            emails:
              - ".email"
            account_type: ".account_type == 'employee' ? 'human' : .account_type"
            employee_ids:
              - ".employee_id"
            last_login: ".last_login != null ? string(.last_login) : ''"
            profile:
              user_id: ".id"
              created_at: ".created_at"
              last_login: ".last_login != null ? string(.last_login) : ''"
              manager_id: ".manager_id"
              manager_username: ".manager_username"

  role:
    name: "Role"
    description: "A role within the Vertica system"
    list:
      query: |
        SELECT
          id,
          role_name
        FROM
          roles
      pagination:
        strategy: "offset"
        primary_key: "id"
      map:
        id: ".role_name"
        display_name: ".role_name"
        description: "'Vertica role: ' + .role_name"
        traits:
          role:
            profile:
              role_id: ".id"

    static_entitlements:
      - id: "member"
        display_name: "'Member'"
        description: "'Role member'"
        purpose: "assignment"
        grantable_to:
          - "user"

    grants:
      - query: |
          SELECT
            u.username,
            r.role_name
          FROM
            users u
          JOIN
            user_roles ur ON u.id = ur.user_id
          JOIN
            roles r ON r.id = ur.role_id
        pagination:
          strategy: "offset"
          primary_key: "username"
        map:
          - skip_if: ".role_name != resource.ID"
            principal_id: ".username"
            principal_type: "user"
            entitlement_id: "member"
```

- [ ] **Step 2: Commit**

```bash
git add examples/vertica-test.yml
git commit -m "docs: add example vertica connector config"
```

---

## Chunk 4: Documentation

### Task 11: Update README

**Files:**
- Modify: `README.md:13` (Key Features multi-database line)
- Modify: `README.md:20-26` (Supported Database Engines list)

- [ ] **Step 1: Update the Key Features line**

In `README.md` line 13, update:

From:
```
- **Multi-Database Support**: Works with MySQL, PostgreSQL, Oracle, SQL Server, SQLite, and WordPress
```

To:
```
- **Multi-Database Support**: Works with MySQL, PostgreSQL, Oracle, SQL Server, Vertica, SQLite, and WordPress
```

- [ ] **Step 2: Update the Supported Database Engines list**

In `README.md` lines 20-26, update:

From:
```markdown
## Supported Database Engines

- MySQL
- Microsoft SQL Server
- Oracle
- PostgreSQL
```

To:
```markdown
## Supported Database Engines

- MySQL
- Microsoft SQL Server
- Oracle
- PostgreSQL
- Vertica
```

- [ ] **Step 3: Verify build still passes**

Run:
```bash
go build ./cmd/baton-sql/
```
Expected: no errors

- [ ] **Step 4: Run all tests**

Run:
```bash
go test ./... -count=1
```
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: add vertica to supported databases"
```
