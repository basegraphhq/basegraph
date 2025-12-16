# Relay Development Commands for AI Agents

## Essential Commands

### Building
```bash
# Build both binaries (from relay/ directory)
make build

# Build individual components
make build-server    # → bin/server
make build-worker    # → bin/worker

# Run without building
make run-server
make run-worker
```

### Testing
```bash
# Run all tests
make test          # go test ./...

# Run specific package
go test ./internal/store/...
go test ./internal/service/...

# Run single test (Ginkgo v2)
go test -v ./internal/http/handler -run "TestUserHandler"
```

### Code Quality (REQUIRED before commits)
```bash
# Format code - ALWAYS run this before committing
make format        # gofumpt -w . (stricter than gofmt)

# Lint code
make lint          # golangci-lint run

# Database operations
make sqlc-generate  # After changing queries in core/db/queries/
make migrate-up      # Run migrations
```

## Code Style Rules

### Architecture Pattern
**Clean Architecture**: `domain` (business logic) vs `model` (clean data)
```
Database → Model → Service → Domain → Business Logic
```

### Key Conventions
- **No comments by default** - code should be self-documenting
- **Always use `make format`** before committing (never `gofmt` directly)
- **Error wrapping**: `fmt.Errorf("operation: %w", err)`
- **No `is_active` flags** - hard delete for operational data
- **Snowflake IDs**: All primary keys use `bigint` with `common/id.New()`

### Package Responsibilities
- **`internal/domain/`**: Business logic with enriched data (`[]domain.Keyword`)
- **`internal/model/`**: Clean data for APIs (`[]string` keywords)
- **`internal/store/`**: Database access with SQLC integration
- **`core/db/sqlc/`**: Generated code (never edit manually)

### Database Patterns
```sql
-- JSONB for enrichment snapshots
code_findings jsonb,
learnings jsonb,
discussions jsonb

-- Core entities in separate tables
CREATE TABLE learnings (...)
```

### Error Handling
```go
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    return nil, fmt.Errorf("operation failed: %w", err)
}
```

### Testing Approach
- **Ginkgo v2 + Gomega** for BDD-style tests
- **Hand-written mocks** for simplicity
- **Test files colocated** with source: `user.go` + `user_test.go`
- **Quality over coverage** - test business logic, not framework behavior