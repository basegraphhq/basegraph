# Relay - agents.md

This document helps AI coding agents (and humans) understand Relay's vision, architecture, engineering philosophy, and codebase conventions.

## Product Vision

### The Problem We Solve

When engineers ship bugs, it's almost never a code mistake. It's because they misunderstood a requirement or missed an edge case while rushing. Vague tickets like "Add Twilio support" don't help, and current AI coding tools ask surface-level questions before jumping to write code.

### What Relay Does

Relay is a planning agent that deeply understands your codebase and asks the right questions to extract the right context from your team's heads—before any code is written.

**Example:**

Ticket: "Add Twilio support"

Relay analyzes the codebase and asks:
- @pm: Calls, SMS? What's actually needed?
- @pm: Outbound, inbound, or both?
- @dev_lead: We don't support rate limiting yet. How should we proceed? (Relay knows to tag the dev lead because it found rate limiting gaps in NotificationService via code graph analysis)

**Output:** A clear technical spec covering decisions, pitfalls, edge cases, and affected services. You can code from it or hand it to your coding agent.

### Core Differentiators

1. **Planning = Human Judgment**: We keep humans in the loop, especially during planning. AI assists, humans decide.

2. **Auto-Routing Questions**: Relay doesn't just ask questions—it figures out *who* to ask by analyzing code ownership and ticket history. Product questions go to PMs, architectural decisions to tech leads, implementation details to the assigned dev.

3. **Deterministic Code Search**: We tried standard RAG for Go, but it wasn't accurate enough. Example: "Adding a new method to X interface, what might be affected?" led to missing implementors. We built a code graph engine for deterministic search, not semantic similarity.

---

## Architecture

### Data Stores

| Store | Purpose |
|-------|---------|
| **PostgreSQL** | Users, organizations, workspaces, integrations, specs, repositories, sessions |
| **ArangoDB** | Code graph storage—nodes (types, functions, interfaces) and edges (calls, implements, returns) |
| **Typesense** | BM25 keyword matching for text search |
| **Vector DB** | Embeddings for semantic search where appropriate |

### Why ArangoDB for Code Graphs?

Code relationships are inherently graph-structured: functions call other functions, types implement interfaces, methods belong to structs. ArangoDB's native graph traversal lets us answer questions like:
- "What structs implement this interface?"
- "What functions call this method?"
- "What's affected if I change this type?"

These queries must be deterministic—we can't afford to "miss" an implementor because of embedding similarity thresholds.

---

## Monorepo Structure

```
basegraph/
├── relay/        # Core product - planning agent service (Go)
├── codegraph/    # Code extraction engine (Go)
├── dashboard/    # Web UI (Next.js + Bun)
```

### relay/

The core Relay service. Handles:
- User authentication and sessions
- Organization and workspace management
- Integration with external services (GitLab, Linear, etc.)
- Webhook processing and event logging
- Spec generation orchestration

**Key directories:**
- `cmd/relay/` - Application entrypoint
- `core/db/queries/` - SQL query definitions (input for sqlc)
- `core/db/sqlc/` - Generated database code (output from sqlc)
- `core/config/` - Configuration management
- `internal/` - Internal packages (http handlers, services, repositories)
- `migrations/` - Database migrations (goose)

### codegraph/

Code extraction engine that parses codebases and builds property graphs.

**Key directories:**
- `extract/golang/` - Go language extractor
- `process/` - Neo4j ingestion and orchestration (being migrated to ArangoDB)
- `assistant/` - CLI coding assistant with code graph tools

**Extracted entities:**
- Functions (with call graphs, params, returns)
- Types (structs, interfaces, aliases)
- Members (methods on types)
- Variables and named types
- Files and namespaces (packages)

### dashboard/

Next.js web application for the Relay UI.

**Key directories:**
- `app/` - Next.js App Router pages and API routes
- `components/` - React components (shadcn/ui based)
- `lib/` - Utilities and auth helpers
- `hooks/` - Custom React hooks

---

## Engineering Philosophy

### 1. Composition Over Inheritance

Design for composition. Use interfaces and dependency injection to make code testable and flexible.

```go
// Good: Accept interface, return struct
type SpecGenerator struct {
    codeGraph  CodeGraphReader
    llmClient  LLMClient
    specStore  SpecRepository
}

func NewSpecGenerator(cg CodeGraphReader, llm LLMClient, ss SpecRepository) *SpecGenerator {
    return &SpecGenerator{codeGraph: cg, llmClient: llm, specStore: ss}
}
```

### 2. Testability First

Code must be testable. If you can't test it, redesign it.

- Accept interfaces, not concrete types
- Avoid global state
- Keep functions pure where possible
- Use dependency injection for external services

### 3. Deterministic Over Probabilistic

For code understanding, prefer deterministic graph queries over probabilistic embeddings.

```
// Bad: "Find similar code to interface X"
// Vector search might miss exact implementors

// Good: "Find all types that implement interface X"
// Graph query returns deterministic, complete results
```

Use semantic search for natural language queries (e.g., "how does auth work?"), but use graph queries for structural relationships.

### 4. Human Judgment in Planning

AI assists, humans decide. Relay surfaces questions and context—humans make the calls on ambiguous requirements, architectural trade-offs, and edge case handling.

---

## Rules for AI Agents

These are explicit rules for AI coding agents working on this codebase.

### 1. Understand Before You Fix

When you hit a type error, compiler complaint, or unexpected behavior—**stop**. Don't do type gymnastics or clever workarounds to make the error go away. The error is telling you something.

Step back and ask:
- Why does this type mismatch exist?
- Is the design wrong, or is the usage wrong?
- What was the original intent?

If you're unsure, **ask the developer**. Don't rush toward a solution. A quick fix that silences the compiler often hides a deeper design issue that will surface later as a bug.

### 2. No Type Gymnastics

Don't cast, use `any`, add wrapper types, or do clever tricks to make a type error disappear. You're hiding the problem, not solving it. Fix the root cause.

### 3. Don't Rush Toward the End Goal

Take time to understand the context. Read the relevant code. Trace the data flow. Understand the *why* before proposing the *what*. Fast but wrong is worse than slow but right.

### 4. Ask When Uncertain

If something doesn't make sense, ask. If there are multiple valid approaches, ask which one the developer prefers. Don't guess. Don't assume. The goal is to get it right, not to get it done quickly.

---

## Codebase Conventions

### Go Services (relay, codegraph)

**Dependencies:**
- Vendor all dependencies: `go mod vendor`
- Use Go 1.24+ (for tool directive support)

**Database:**
- **sqlc** for type-safe query generation
- **goose** for migrations
- Queries live in `core/db/queries/*.sql` (input)
- Generated code in `core/db/sqlc/*.go` (output)

**Project structure:**
```
service/
├── cmd/service/main.go    # Entrypoint
├── core/                   # Core domain logic
│   ├── config/            # Configuration (env vars)
│   └── db/
│       ├── db.go          # Pool management, transactions
│       ├── queries/       # SQL query definitions (input)
│       └── sqlc/          # Generated Go code (output)
├── internal/              # Internal packages
│   ├── http/              # HTTP handlers
│   ├── model/             # Domain models (clean types)
│   ├── service/           # Business logic
│   └── repository/        # Data access (sqlc ↔ model conversion)
├── migrations/            # SQL migrations
└── vendor/                # Vendored dependencies
```

**Naming:**
- Use descriptive names; avoid abbreviations
- Interfaces: describe behavior (e.g., `CodeGraphReader`, not `ICodeGraph`)
- Package names: short, lowercase, singular

**Error handling:**
- Return errors, don't panic
- Wrap errors with context: `fmt.Errorf("creating spec: %w", err)`

### Relay Data Layer

Relay uses a layered architecture for data access:

```
HTTP Handler → Service → Repository → sqlc → PostgreSQL
```

**Layer responsibilities:**

| Layer | Location | Responsibility |
|-------|----------|----------------|
| **sqlc** | `core/db/sqlc/` | Generated code, uses `pgtype.*` |
| **Repository** | `internal/repository/` | Converts sqlc ↔ domain models, implements interfaces |
| **Domain Models** | `internal/model/` | Clean types with `time.Time`, JSON tags for API |
| **Service** | `internal/service/` | Business logic, accepts repository interfaces |

**Domain models** (`internal/model/`):

Domain models use standard Go types (`time.Time`) instead of database-specific types (`pgtype.Timestamptz`). This keeps the service layer clean and decoupled from the database driver.

```go
// internal/model/user.go
type User struct {
    ID        int64     `json:"id"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`  // Not pgtype.Timestamptz
}
```

**Repository pattern** (`internal/repository/`):

Repositories implement interfaces for testability. Since we use PostgreSQL exclusively (no plans to switch), repositories return interfaces rather than concrete types—both work equivalently for composition.

```go
// interfaces.go - contracts for DI/mocking
type UserRepository interface {
    GetByID(ctx context.Context, id int64) (*model.User, error)
    Create(ctx context.Context, user *model.User) error
}

// user.go - implementation
func NewUserRepository(q *sqlc.Queries) UserRepository {
    return &userRepo{queries: q}
}
```

**Repository factory** (`internal/repository/factory.go`):

Creates all repositories from a single `*sqlc.Queries` instance. Works with both pool and transaction contexts.

```go
repos := repository.NewRepositories(db.Queries())
user, err := repos.Users().GetByID(ctx, 123)
```

**Transaction support** (`core/db/db.go`):

Use `db.WithTx()` when operations must be atomic. The transaction auto-commits on success, auto-rollbacks on error.

```go
err := db.WithTx(ctx, func(q *sqlc.Queries) error {
    repos := repository.NewRepositories(q)
    
    if err := repos.Organizations().Create(ctx, org); err != nil {
        return err  // auto-rollback
    }
    return repos.Workspaces().Create(ctx, ws)
    // auto-commit on success
})
```

**Services accept interfaces** (testability):

```go
type UserService struct {
    users repository.UserRepository  // interface, not concrete
}

func NewUserService(users repository.UserRepository) *UserService {
    return &UserService{users: users}
}
```

### Dashboard (Next.js)

**Package manager:** Bun

**Routing:** App Router (app/ directory)

**Components:**
- shadcn/ui as component foundation
- Components in `components/`
- UI primitives in `components/ui/`

**Styling:**
- Tailwind CSS
- CSS variables for theming (see `globals.css`)

**Auth:**
- Better Auth for authentication
- Auth helpers in `lib/auth.ts`

---

## Key Domain Concepts

### Organization
A company or team using Relay. Contains users and workspaces.

### Workspace
A container within an organization. Groups related repositories and integrations. Think of it as a "project" or "team space."

### Integration
A connection to an external service:
- **Git providers**: GitLab, GitHub
- **Project management**: Linear, Jira
- **Communication**: Slack (future)

Each integration stores OAuth tokens and provider-specific configuration.

### Repository
A codebase connected through an integration. Relay indexes repositories to build code graphs.

### Code Graph
A structured representation of code:
- **Nodes**: Types, functions, interfaces, methods, variables
- **Edges**: CALLS, IMPLEMENTS, RETURNS, HAS_PARAM, MEMBER_OF

Enables deterministic queries about code structure and relationships.

### Spec
The output of Relay's planning process. A technical specification that covers:
- Decisions made
- Edge cases identified
- Affected services/components
- Implementation guidance

---

## Common Tasks

### Adding a new database table

1. Create migration: `make migrate-create NAME=create_foo_table`
2. Write SQL in `migrations/TIMESTAMP_create_foo_table.sql`
3. Add queries in `core/db/queries/foo.sql`
4. Generate Go code: `make sqlc-generate`
5. Create domain model in `internal/model/foo.go`
6. Add repository interface to `internal/repository/interfaces.go`
7. Implement repository in `internal/repository/foo.go`
8. Add to factory in `internal/repository/factory.go`

### Adding a new code graph entity type

1. Add type definition in `codegraph/extract/types.go`
2. Implement extraction in `codegraph/extract/golang/`
3. Add export logic in `codegraph/process/export_nodes.go`
4. Update ingestion in `codegraph/process/`

### Running the relay service locally

```bash
cd relay
make install-tools    # Install goose and sqlc
make migrate-up DB_STRING="postgres://..."
make run
```

---

## What Not To Do

1. **Don't use vector search for code relationships**—use graph queries. Embeddings miss structural relationships.

2. **Don't auto-generate code without human review**—Relay produces specs, not code. The human (or their coding agent) writes the code.

3. **Don't bypass composition**—if you're reaching for global state or singletons, redesign.

4. **Don't add features without understanding ownership**—Relay's value is asking the *right person*. Unclear ownership is an open problem we're solving.


