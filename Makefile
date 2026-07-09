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

# DB2 support requires the native IBM CLI driver at build and run time (see docs/db2.md).
# Flags are scoped to this target only — do not export CGO_CFLAGS/CGO_LDFLAGS in your shell.
# -ldb2 is omitted on purpose: go_ibm_db's cgo directives already add it.
#
# The binary resolves libdb2 relative to itself first (./clidriver/lib next to the
# executable), then at the build-time DB2HOME, so one artifact works both untarred
# alongside a bundled driver (see package-db2) and on hosts with a system-wide install.
# On macOS, libdb2.dylib's install name is the bare filename, so the darwin steps rewrite
# the load command to @rpath and re-sign (ad-hoc). No LD_LIBRARY_PATH/DYLD_LIBRARY_PATH
# is needed at run time in either layout.
DB2HOME ?= /usr/local/clidriver

ifeq ($(GOOS),darwin)
DB2_RPATH_FLAGS = -Wl,-rpath,@executable_path/clidriver/lib -Wl,-rpath,$(DB2HOME)/lib
else
DB2_RPATH_FLAGS = -Wl,-rpath,$$ORIGIN/clidriver/lib -Wl,-z,origin -Wl,-rpath,$(DB2HOME)/lib
endif

.PHONY: build-db2
build-db2:
	CGO_CFLAGS='-I$(DB2HOME)/include' CGO_LDFLAGS='-L$(DB2HOME)/lib $(DB2_RPATH_FLAGS)' \
		go build -tags db2 -o ${OUTPUT_PATH} ./cmd/baton-sql
ifeq ($(GOOS),darwin)
	install_name_tool -change libdb2.dylib @rpath/libdb2.dylib ${OUTPUT_PATH}
	codesign -f -s - ${OUTPUT_PATH}
endif

# Self-contained DB2 distribution: binary + clidriver (including its license/ directory,
# which the IBM redistribution terms require shipping) in one archive. Untar and run —
# no installation, no environment variables.
BUNDLE_NAME = baton-sql-db2-${GOOS}-${GOARCH}
BUNDLE_DIR = dist/${BUNDLE_NAME}

.PHONY: package-db2
package-db2: build-db2
	rm -rf ${BUNDLE_DIR} ${BUNDLE_DIR}.tar.gz
	mkdir -p ${BUNDLE_DIR}
	cp ${OUTPUT_PATH} ${BUNDLE_DIR}/
	cp -R $(DB2HOME) ${BUNDLE_DIR}/clidriver
	tar -czf ${BUNDLE_DIR}.tar.gz -C dist ${BUNDLE_NAME}
	@echo "Created ${BUNDLE_DIR}.tar.gz"

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
