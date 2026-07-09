GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)
BUILD_DIR = dist/${GOOS}_${GOARCH}

ifeq ($(GOOS),windows)
OUTPUT_PATH = ${BUILD_DIR}/baton-sql.exe
else
OUTPUT_PATH = ${BUILD_DIR}/baton-sql
endif

.PHONY: build
build:
	go build -o ${OUTPUT_PATH} ./cmd/baton-sql

# DB2 support requires the native IBM CLI driver at build and run time (see DB2_SETUP.md).
# Flags are scoped to this target only — do not export CGO_CFLAGS/CGO_LDFLAGS in your shell.
# -ldb2 is omitted on purpose: go_ibm_db's cgo directives already add it. On macOS,
# libdb2.dylib's install name is the bare filename, so rpath alone can't resolve it; the
# darwin steps point the load command at DB2HOME and re-sign (ad-hoc) so no
# DYLD_LIBRARY_PATH is needed at run time.
DB2HOME ?= /usr/local/clidriver

.PHONY: build-db2
build-db2:
	CGO_CFLAGS="-I$(DB2HOME)/include" CGO_LDFLAGS="-L$(DB2HOME)/lib -Wl,-rpath,$(DB2HOME)/lib" \
		go build -tags db2 -o ${OUTPUT_PATH} ./cmd/baton-sql
ifeq ($(GOOS),darwin)
	install_name_tool -change libdb2.dylib $(DB2HOME)/lib/libdb2.dylib ${OUTPUT_PATH}
	codesign -f -s - ${OUTPUT_PATH}
endif

.PHONY: update-deps
update-deps:
	go get -d -u ./...
	go mod tidy -v
	go mod vendor

.PHONY: add-dep
add-dep:
	go mod tidy -v
	go mod vendor

.PHONY: lint
lint:
	golangci-lint run

.PHONY: test
test:
	go test ./...
