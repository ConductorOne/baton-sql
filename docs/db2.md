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

DB2's native form is also accepted as-is (ODBC keywords are case-insensitive and may carry
spaces after each `;`):

```
HOSTNAME=localhost;PORT=50000;DATABASE=TESTDB;UID=db2inst1;PWD=pass123;PROTOCOL=TCPIP
```

The native form is self-contained: it already carries the host, port, credentials, params and
target database. It is therefore mutually exclusive with the structured `connect` fields
(`host`, `port`, `user`, `password`, `params`) and with a per-database override (`connect.database`
or the `databases` block for multi-database sync). Combining them is rejected with an explicit
error rather than silently ignoring the extra settings, so use the `db2://` URL form when you
need multi-database discovery or want to supply fields separately.

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
