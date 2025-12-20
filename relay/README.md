# Relay - Turn Tickets into Production-Ready Specs

Relay analyzes your tickets, gathers context from your team, and maps your codebase to generate specs that actually ship. Hand them to any developer or AI agent.

## What Relay Does

**The Problem**: When engineers ship bugs, it's almost never a code mistake. It's because they misunderstood a requirement or missed an edge case while rushing. Vague tickets like "Add Twilio support" don't help, and current AI coding tools ask surface-level questions before jumping to write code.

**The Solution**: Relay is a planning agent that deeply understands your codebase and asks the right questions to extract the right context from your team's heads—before any code is written.

### Example in Action

**Ticket**: "Add Twilio support"

Relay analyzes the codebase and asks:
- @pm: Calls, SMS? What's actually needed?
- @pm: Outbound, inbound, or both?
- @dev_lead: We don't support rate limiting yet. How should we proceed? (Relay knows to tag the dev lead because it found rate limiting gaps in NotificationService via code graph analysis)

**Output**: A clear technical spec covering decisions, pitfalls, edge cases, and affected services.

## Why Relay is Different

### Context Lives in People, Not Just Code

Edge cases don't live in your codebase—they live in your team's heads. Business logic nuances. Production gotchas. That one integration everyone forgets about.

Relay asks the right questions. It pulls context from the people who know your product, then maps that against your actual codebase constraints. What you get is a spec that accounts for both the business logic edge cases humans catch and the architectural limitations code analysis reveals.

This is the step most AI tools skip. We built Relay because planning is too important to automate away.

### Deterministic Code Understanding vs Semantic Search

**Semantic Search (Cursor, Aider, etc.)**
- ❌ Embeddings miss exact import chains and call graphs
- ❌ Can't verify if a function actually exists or trace its dependencies
- ❌ Guesses at architecture based on similarity, not structure
- ❌ Reads 50+ files hoping to find the right context

**Relay's Codegraph (Compiler-Based)**
- ✅ Traces exact import paths and function call chains
- ✅ Verifies every symbol exists and maps its relationships
- ✅ Understands actual architecture from parse trees, not vibes
- ✅ Fetches only the 3-5 files that actually matter

**Result**: Your AI agent gets a spec with verified imports, actual function signatures, and real architectural constraints—not hallucinated ones. No more "this function doesn't exist" or "wrong number of parameters" errors after 20 minutes of generation.

## Architecture

This repository contains the unified Relay codebase that produces two binaries:

```
relay/
├── cmd/
│   ├── server/    # HTTP server + webhook ingestion
│   └── worker/    # Event processing pipeline
├── internal/
│   ├── service/   # Business logic (shared)
│   ├── store/   # Data access (shared)
│   ├── model/   # Domain models (shared)
│   └── ...
```

### Event Flow

```
Webhook → Server → Redis Stream → Worker → Analysis → Spec
```

1. **Server** receives webhooks from issue trackers (Linear, GitHub, etc.)
2. **Server** converts webhooks to events and publishes to Redis
3. **Worker** consumes events from Redis and processes them
4. **Worker** analyzes issues, asks questions, and generates specs
5. **Worker** posts specs back to the issue tracker

## Quick Start

### Prerequisites
- Go 1.24+
- PostgreSQL 15+
- Redis 7+

### Development Setup

1. **Start infrastructure:**
```bash
make dev-db
```

2. **Set up environment:**
```bash
cp .env.example .env
# Edit .env with your configuration
```

3. **Run migrations:**
```bash
make migrate-up
```

4. **Build binaries:**
```bash
make build
```

5. **Run server:**
```bash
make run-server
```

6. **Run worker (in another terminal):**
```bash
make run-worker
```

## Configuration

Both binaries share configuration but use different subsets:

### Server Configuration
- `PORT`: HTTP server port (default: 8080)
- `WEBHOOK_BASE_URL`: Base URL for webhooks
- `WORKOS_*`: WorkOS authentication settings
- `DATABASE_URL`: PostgreSQL connection
- `REDIS_*`: Redis connection for event queuing

### Worker Configuration
- `DATABASE_URL`: PostgreSQL connection (shared)
- `REDIS_*`: Redis connection for event consumption

## Development

### Building
```bash
make build          # Build both binaries
make build-server   # Build server only
make build-worker   # Build worker only
```

### Running
```bash
make run-server     # Run server
make run-worker     # Run worker
```

### Database
```bash
make migrate-up     # Run migrations
make migrate-down   # Rollback migrations
make sqlc-generate  # Regenerate SQLC code
```

### Code Quality
```bash
make format       # Format code with gofumpt
make lint         # Run golangci-lint
make test         # Run tests
```

## How It Works

### Issue Analysis Process

1. **Webhook Reception**: Server receives webhook from issue tracker
2. **Event Creation**: Server creates canonical event from webhook data
3. **Queue Publishing**: Server publishes event to Redis stream
4. **Event Consumption**: Worker consumes event from Redis
5. **Code Analysis**: Worker analyzes codebase related to the issue
6. **Question Generation**: Worker generates targeted questions based on gaps
7. **Human Input**: Questions are posted back to the issue for team response
8. **Spec Generation**: After gathering context, worker generates technical spec
9. **Spec Delivery**: Final spec is posted as a comment on the issue

### Key Components

**Server (`cmd/server`)**:
- HTTP server with Gin framework
- Webhook handlers for multiple issue trackers
- Event normalization and publishing
- Authentication via WorkOS

**Worker (`cmd/worker`)**:
- Redis stream consumer
- Event processing pipeline (currently empty)
- Integration with issue tracker APIs

**Shared Components**:
- Domain models and business logic
- Database access layer with SQLC
- Configuration management
- Common utilities and helpers

## Current Status

✅ **Server**: Fully functional with webhook endpoints  
✅ **Worker**: Basic structure ready for pipeline implementation  
✅ **Database**: PostgreSQL schema from both services merged  
✅ **Build System**: Unified Makefile for both binaries  

🔜 **Pipeline**: Empty service ready for your Relay logic implementation

## Integration Support

Currently supports:
- ✅ Linear (primary)
- ✅ GitHub 
- ✅ GitLab
- 🔄 Jira (coming Q1 2026)

## Self-Hosting vs Cloud

**Current**: Cloud-hosted with code indexing on our servers  
**Roadmap**: Self-hosted deployments for teams with strict data residency requirements

## Why This Architecture

**Single Codebase Benefits**:
- One git repository to maintain
- Shared business logic eliminates consistency issues  
- Single CI/CD pipeline and deployment process
- Unified monitoring, logging, and alerting

**Binary Separation Benefits**:
- Independent scaling (webhooks vs processing)
- Different resource profiles (I/O vs CPU intensive)
- Separate failure domains
- Clean operational separation

This is exactly how experienced Go teams handle microservices that share heavy domain logic - shared code with operational separation.

## Contributing

This follows standard Go project conventions:
- Domain-driven design in `internal/`
- Interface-based dependencies for testability
- Comprehensive error handling
- Structured logging with context

Run `make install-tools` to install development dependencies.

## Contact

**Early Access**: Relay is in private beta  
**Feedback**: nithinsj@basegraph.app, nithinsudarsan@basegraph.app  
**Discord**: Join our Discord server  

© 2025 Basegraph