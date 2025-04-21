# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build, Test, and Lint Commands
- Build: `make build` - Builds the baton-sql binary
- Run tests: `go test ./...` - Run all tests
- Run specific test: `go test ./pkg/path/to/package -run TestName` 
- Run tests with verbose output: `go test -v ./...`
- Lint: `make lint` - Runs golangci-lint

## Code Style Guidelines
- Error handling: Use `fmt.Errorf` with context; check specific errors with `errors.Is`
- Imports: Standard lib first, then third-party, then project imports, alphabetized within groups
- Naming: CamelCase for exported, camelCase for unexported; special handling for ID, URL, HTTP, API
- Tests: Table-driven tests with testify/require; format TestStructName_methodName
- Line length: Maximum 200 characters
- Comments: Complete sentences ending with periods for exported items
- Absolutely no usage of log.Fatal or log.Panic (enforced by ruleguard)