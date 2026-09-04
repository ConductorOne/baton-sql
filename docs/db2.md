# DB2 Support

IBM DB2 support is **opt-in** and built with `make build-db2`. Unlike every other engine in
baton-sql (all pure Go), DB2 uses the cgo driver `github.com/ibmdb/go_ibm_db`, which links
against IBM's native ODBC/CLI driver ("clidriver"). No pure-Go DB2 driver exists — the native
dependency cannot be eliminated, so it is isolated behind the `db2` build tag instead.

What this means in practice:

- `make build` (and CI, releases, cross-compilation) needs **no** IBM software, no CGO flags,
  and no environment variables. DB2 code is excluded from default builds.
- A default binary given a `db2://` DSN fails with a clear error:
  `baton-sql: DB2 support not compiled into this binary; rebuild with -tags db2 (see docs/db2.md)`.
- `make build-db2` produces a DB2-capable binary. The CGO flags are scoped to that one make
  target — **do not export `CGO_CFLAGS`/`CGO_LDFLAGS` in your shell profile**. Global CGO flags
  leak into every cgo link on your machine and break unrelated Go builds.
- The built binary needs no `LD_LIBRARY_PATH`/`DYLD_LIBRARY_PATH` at run time: the Makefile
  bakes the lookup into the binary. It resolves the clidriver **relative to itself first**
  (`./clidriver` next to the executable), then falls back to the build-time `DB2HOME` path —
  so `make package-db2` produces a self-contained archive customers untar and run anywhere.

## Prerequisites

- Go 1.24 or later and a C compiler (clang/gcc)
- IBM DB2 ODBC/CLI driver (clidriver) for your platform — free download, no license key

## Step 1: Install the clidriver

Download the archive for your platform from
`https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/`:

| Platform | Archive |
|----------|---------|
| macOS ARM64 (Apple Silicon) | `macarm64_odbc_cli.tar.gz` |
| macOS x86_64 (Intel) | `macos64_odbc_cli.tar.gz` |
| Linux x86_64 | `linuxx64_odbc_cli.tar.gz` |
| Linux ARM64 | **Not available** — IBM does not ship a Linux ARM64 clidriver |
| Linux ppc64le | `linuxppc64le_odbc_cli.tar.gz` |

```bash
curl -O https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/macarm64_odbc_cli.tar.gz
tar -xzf macarm64_odbc_cli.tar.gz
sudo mv clidriver /usr/local/
rm macarm64_odbc_cli.tar.gz
```

Any location works; `/usr/local/clidriver` is the default the Makefile assumes.

## Step 2: Build

```bash
make build-db2                            # clidriver at /usr/local/clidriver
DB2HOME=/opt/clidriver make build-db2     # clidriver elsewhere
```

That's it — no shell configuration. The target runs
`go build -tags db2` with `CGO_CFLAGS`/`CGO_LDFLAGS` set inline for that command only
(`-ldb2` is deliberately omitted; the driver's own cgo directives add it), then on macOS
rewrites the dylib load command with `install_name_tool` and re-signs the binary.

```bash
# Verify — runs without any library-path environment variables:
./dist/darwin_arm64/baton-sql -h
```

## Distributing to customers (binary bundle)

Most deployments run the bare binary, not Docker. `make package-db2` produces a single
self-contained archive:

```bash
make package-db2
# -> dist/baton-sql-db2-<os>-<arch>.tar.gz  (~55 MB)
```

The archive holds the binary with the clidriver beside it (including the `license/`
directory IBM's redistribution terms require):

```
baton-sql-db2-linux-amd64/
├── baton-sql
└── clidriver/
```

The customer untars it anywhere and runs `./baton-sql` — no installation, no root, no
environment variables. The binary finds the driver via its baked-in relative path
(`$ORIGIN/clidriver/lib` on Linux, `@executable_path/clidriver/lib` on macOS); if a
system-wide clidriver exists at the build-time `DB2HOME` it serves as fallback. The two
directories must stay side by side — moving the binary out of the bundle alone breaks the
relative lookup (the fallback still applies if present).

Platform notes: Linux bundles are glibc-only (no Alpine/musl) and x86_64 only — IBM ships
no Linux ARM64 clidriver. The clidriver needs `libxml2` from the OS (`apt-get install
libxml2` / `yum install libxml2`) — preinstalled on virtually every full server distro,
absent in minimal container images. A Linux bundle must be built on Linux (cgo — use the
Docker builder below or any amd64 Linux host).

## DSN Format

Standard URL form (recommended) — converted to DB2's native format automatically:

```
db2://username:password@hostname:port/database
db2://db2inst1:pass123@localhost:50000/TESTDB
```

Query parameters are forwarded as additional DB2 connection keywords
(e.g. `db2://user:pass@host:50000/DB?Security=SSL` adds `SECURITY=SSL`). Values containing
`;` are brace-quoted automatically; parameters that would override the URL-derived keywords
(`HOSTNAME`, `DATABASE`, `PORT`, `PROTOCOL`, `UID`, `PWD`) are rejected — use the native
form below for full control.

DB2's native form is also accepted as-is:

```
HOSTNAME=localhost;PORT=50000;DATABASE=TESTDB;UID=db2inst1;PWD=pass123;PROTOCOL=TCPIP
```

## Writing a Db2 spec

Db2 needs two things in every spec. Other engines need them only in spots (Oracle folds
unquoted identifiers to uppercase; Redshift needs `string()` around columns in CEL
concatenations), but Db2 needs both everywhere. Both fail loudly, but the error names neither
the column nor the query, so they're easy to miss when adapting another engine's spec.

### Wrap every column reference in `string()`

A bare column reference inside a CEL concatenation or comparison aborts the whole sync at
`list-resources`, before any resource is emitted:

```
error: listing resources failed: no such overload
```

The error names neither the column nor the expression. Wrap each reference in `string()`:

```yaml
id: "string(.database_name) + '.' + string(.schema_name)"
```

`examples/redshift-test.yml` uses this form throughout — copy it when adapting a spec to Db2.

### Alias columns to double-quoted lowercase names

Db2 returns column names uppercase, and the engine keys each row on the driver's names, so
`SELECT GRANTEE` yields `.GRANTEE`, not `.grantee`. Alias every selected column to the
lowercase name the CEL expressions reference:

```sql
SELECT RTRIM(GRANTEE) AS "grantee" FROM SYSCAT.DBAUTH
```

The double quotes are required; without them Db2 folds the alias back to uppercase.

## Provisioning support

Entitlement provisioning works normally: `GRANT`/`REVOKE` of roles, authorities and object
privileges behave the same as on any other engine.

**Account creation and deletion are not available for Db2.** Db2 LUW has no `CREATE USER`
statement — authorization IDs are operating-system, LDAP or Kerberos identities managed
outside the database, and `GRANT ... TO USER <authid>` succeeds even for names Db2 has never
seen. A create-account request against a `db2://` DSN has nothing to call.

**Group principals can't be synced or provisioned.** Db2 has no in-database group table;
group membership is reachable only through the per-authorization-ID function
`SYSPROC.AUTH_LIST_GROUPS_FOR_AUTHID`, which a YAML resource list can't express, so a Db2
spec should not declare `group` in an entitlement's `grantable_to`.

Two caveats on how this surfaces. `grantableTo` is spec-driven: the connector copies whatever
the spec declares and does not filter by engine, so a spec that still lists `group` will
advertise it as grantable even on Db2. Enforcement happens at ingest instead. A grant emitted
with `principal_type: group` is dropped (visible as `grants_dropped` and
`ingest_quality.reason_flags` in the sync token).

## Running the Db2 tests

Like the build, the test and vet targets carry the CGO flags inline:

```bash
make test-db2                          # go test -tags db2 ./...
make vet-db2                           # go vet  -tags db2 ./...
DB2HOME=/opt/clidriver make test-db2   # clidriver elsewhere
```

Running the raw commands instead of the make targets needs the same two CGO variables the
`build-db2` target sets, plus a library path — `go test` builds and runs its own binary,
which the build-time install-name/rpath rewrite never touches:

```bash
CGO_CFLAGS="-I$DB2HOME/include" CGO_LDFLAGS="-L$DB2HOME/lib" \
  DYLD_LIBRARY_PATH="$DB2HOME/lib" \
  go test -tags db2 ./...
```

Use `LD_LIBRARY_PATH` instead of `DYLD_LIBRARY_PATH` on Linux. Without the CGO flags you get
`fatal error: 'sqlcli1.h' file not found`; without the library path on macOS,
`Library not loaded: libdb2.dylib` (both covered under Troubleshooting).

## Troubleshooting

**`'sqlcli1.h' file not found`** — clidriver missing or `DB2HOME` wrong. Check that
`$DB2HOME/include/sqlcli1.h` exists. If you see this from `make build` (not `build-db2`),
something reintroduced the driver into the default build — the `db2` tag gate is broken.

**`found architecture 'x86_64', required architecture 'arm64'`** — wrong clidriver archive
for your CPU; see the table above.

**`Library not loaded: libdb2.dylib` / `libdb2.so: cannot open shared object file`** — the
binary can't find the clidriver in either of its baked-in locations: `./clidriver/lib` next
to the executable, then the build-time `DB2HOME` path. Restore the bundle layout (binary and
`clidriver/` side by side), rebuild with `make build-db2`, or set
`LD_LIBRARY_PATH`/`DYLD_LIBRARY_PATH` to the clidriver `lib` dir as a stopgap. This also
happens when `go build -tags db2` is invoked directly, skipping the Makefile's
rpath/install_name steps. Note macOS SIP strips `DYLD_*` variables across protected
binaries — the baked paths are the reliable option.

**`error while loading shared libraries: libxml2.so.2`** (Linux) — the clidriver depends on
the OS libxml2 package: `apt-get install libxml2` / `yum install libxml2`.

**`go vet` / `golangci-lint` with `-tags db2` fails** — type-checking the tagged path needs
the clidriver headers too. Default-tag lint and vet need nothing.

## Provisioning: `validation_queries` semantics

Db2 is DDL-based: its `GRANT`/`REVOKE` don't report rows-affected, so a `validation_query`
returning no rows is treated as an idempotent success, not a failed precondition. Db2 is the
only engine with this behavior today, and it ships opt-in behind the `db2` build tag. It means
you must not use `validation_queries` as existence preconditions on Db2. See
[Provisioning: `validation_queries` semantics](provisioning.md) for the full explanation and
examples.

## Docker

- The default release pipeline (goreleaser, `CGO_ENABLED=0`) is unaffected — DB2 does not
  ride the standard release. Ship DB2 as the binary bundle above or as a separate Docker
  image.
- Use a **glibc** base (Debian/Ubuntu/UBI). The clidriver does not support musl, so Alpine
  is out. **linux/amd64 only** (no ARM64 clidriver).
- Bake the rpath at build time and copy the clidriver into the runtime image at the same
  path — no `LD_LIBRARY_PATH` needed.

```dockerfile
FROM golang:1.24-bookworm AS builder
RUN curl -sO https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/linuxx64_odbc_cli.tar.gz && \
    tar -xzf linuxx64_odbc_cli.tar.gz && mv clidriver /opt/clidriver
WORKDIR /src
COPY . .
RUN CGO_CFLAGS="-I/opt/clidriver/include" \
    CGO_LDFLAGS="-L/opt/clidriver/lib -Wl,-rpath,/opt/clidriver/lib" \
    go build -tags db2 -o /baton-sql ./cmd/baton-sql

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends libxml2 && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /opt/clidriver /opt/clidriver
COPY --from=builder /baton-sql /baton-sql
ENTRYPOINT ["/baton-sql"]
```

### Redistribution licensing

Bundling the clidriver in a distributed image is expressly permitted by the IBM license's
"Redistributables" clause (IPLA + License Information document, shipped inside the tarball
at `clidriver/license/`), with conditions: distribute only the files enumerated in
`odbc_REDIST.txt`, keep the `license/` directory and copyright notices, and your end-user
terms must be at least as protective of IBM as IBM's own. Two caveats before shipping to
customers: the GSKit TLS libraries (`libgsk8*`) are not in the enumerated redistributables
list — get legal/IBM sign-off if TLS connections to DB2 are needed — and the Db2 Connect
license file (`db2consv_*.lic`, required for direct z/OS or IBM i connections) is **not**
redistributable; customers must supply their own.
