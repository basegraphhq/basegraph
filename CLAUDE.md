# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Build & Run Commands

### Relay (Go backend) - run from `relay/`

```bash
make install-tools          # Install goose, sqlc, ginkgo, gofumpt, golangci-lint
make build                  # Build server and worker binaries
make run-server             # Run the HTTP server
make run-worker             # Run the background worker

# Database
make sqlc-generate          # Generate Go code from SQL queries
make migrate-up             # Run migrations (requires DATABASE_URL)
make migrate-down           # Rollback last migration
make migrate-create NAME=x  # Create new migration

# Quality
make format                 # Format with gofumpt (ALWAYS use this, not gofmt)
make lint                   # Run golangci-lint

# Testing
make test                   # Run all tests (unit + BDD + integration)
make test-unit              # Quick unit tests
make test-bdd               # Ginkgo BDD tests

# Dev environment
make dev-db                 # Start PostgreSQL + Redis via Docker
make dev-down               # Stop dev containers
```

### Codegraph (Go) - run from `codegraph/`

```bash
make build-codegraph        # Build the code extraction engine
make test                   # Run tests
make tidy                   # go mod tidy && go mod vendor
```

### Dashboard (Next.js) - run from `dashboard/`

```bash
bun install                 # Install dependencies
bun dev                     # Start dev server
bun build                   # Production build
bun lint                    # ESLint
npx biome lint .            # Biome linting
```

## Engineering Philosophy

**Code Quality Standard:** All code must be well-written, testable, readable, debuggable, traceable, and simple. No exceptions.

### Principles

1. **Composition over inheritance** - Use interfaces and dependency injection
2. **Testability first** - Accept interfaces, inject dependencies, avoid global state
3. **Deterministic over probabilistic** - Graph queries for code relationships, not vector search
4. **Human judgment in planning** - AI assists, humans decide

### Traceability

Logging and observability are core to this project.

- **Errors:** Always wrap with context (`fmt.Errorf("doing X: %w", err)`) - never naked `return err`
- **Logging:** Structured logs at meaningful points, not just errors
- **Observability:** Requests must be traceable through the system
- **Git:** Commits explain "why", not just "what"

## Critical Conventions

1. **Format:** Always `make format` (gofumpt, not gofmt) before committing
2. **Vendor:** Run `go mod vendor` after changing dependencies
3. **IDs:** Use `common/id.New()` for Snowflake IDs (primary keys)
4. **Interfaces:** Describe behavior (`CodeGraphReader`, not `ICodeGraph`)
5. **Comments:** Default to none. Only add for "why" explanations, not "what"
6. **Domain models:** Use `time.Time`, not `pgtype.Timestamptz` - keep service layer clean

## AI Agent Guidelines

1. **Understand before fixing** - Don't silence errors with type gymnastics. The error is telling you something.
2. **No `any` or clever casts** - Fix the root cause, not the symptom
3. **Ask when uncertain** - Multiple valid approaches? Ask the developer
4. **Think about edge cases** - Highlight business and code edge cases early
5. **Trace the data flow** - Understand the "why" before proposing the "what"
6. **Errors and UX** - When handling errors and concurrency, think from UX perspective. Relay is built to behave like a human teammate.
