# DB2 Support

IBM DB2 support is **opt-in** and built with `make build-db2`. Unlike every other engine in
baton-sql (all pure Go), DB2 uses the cgo driver `github.com/ibmdb/go_ibm_db`, which links
against IBM's native ODBC/CLI driver ("clidriver"). No pure-Go DB2 driver exists — the native
dependency cannot be eliminated, so it is isolated behind the `db2` build tag instead.

What this means in practice:

- `make build` (and CI, releases, cross-compilation) needs **no** IBM software, no CGO flags,
  and no environment variables. DB2 code is excluded from default builds.
- A default binary given a `db2://` DSN fails with a clear error:
  `baton-sql: DB2 support not compiled into this binary; rebuild with -tags db2 (see DB2_SETUP.md)`.
- `make build-db2` produces a DB2-capable binary. The CGO flags are scoped to that one make
  target — **do not export `CGO_CFLAGS`/`CGO_LDFLAGS` in your shell profile**. Global CGO flags
  leak into every cgo link on your machine and break unrelated Go builds.
- The built binary needs no `LD_LIBRARY_PATH`/`DYLD_LIBRARY_PATH` at run time: the Makefile
  bakes an rpath on Linux and rewrites the `libdb2.dylib` load command on macOS. It does need
  the clidriver present at the same path it was built against.

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

## Troubleshooting

**`'sqlcli1.h' file not found`** — clidriver missing or `DB2HOME` wrong. Check that
`$DB2HOME/include/sqlcli1.h` exists. If you see this from `make build` (not `build-db2`),
something reintroduced the driver into the default build — the `db2` tag gate is broken.

**`found architecture 'x86_64', required architecture 'arm64'`** — wrong clidriver archive
for your CPU; see the table above.

**`Library not loaded: libdb2.dylib` / `libdb2.so: cannot open shared object file`** — the
clidriver was moved or deleted after the build, or the binary was built by invoking `go build
-tags db2` directly instead of `make build-db2` (skipping the rpath/install_name step).
Rebuild with `make build-db2`, or set `LD_LIBRARY_PATH`/`DYLD_LIBRARY_PATH` to
`$DB2HOME/lib` as a stopgap. Note macOS SIP strips `DYLD_*` variables across protected
binaries — the baked path is the reliable option.

**`go vet` / `golangci-lint` with `-tags db2` fails** — type-checking the tagged path needs
the clidriver headers too. Default-tag lint and vet need nothing.

## Docker / Distribution

- The default release pipeline (goreleaser, `CGO_ENABLED=0`) is unaffected — DB2 does not
  ride the standard release. Ship DB2 as a separate Docker image.
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
