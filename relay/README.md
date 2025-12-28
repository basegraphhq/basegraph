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

### High-Level Flow

```
Webhook → Server → Redis Stream → Worker → Thinking Pipeline → Spec
```

1. **Server** receives webhooks from issue trackers
2. **Server** converts webhooks to canonical events and publishes to Redis
3. **Worker** consumes events from Redis stream
4. **Worker** passes events through the Thinking Pipeline
5. **Thinking Pipeline** analyzes, asks questions, and generates specs
6. **Worker** posts specs back to the issue tracker

### System Components

**Server (`cmd/server`)**:
- HTTP server with Gin framework
- Webhook handlers for Linear, GitHub, GitLab
- Event normalization and publishing to Redis
- Authentication via WorkOS

**Worker (`cmd/worker`)**:
- Redis stream consumer
- Thinking Pipeline orchestration
- Integration with issue tracker APIs for posting questions/specs

**Shared Components**:
- Domain models and business logic
- Database access layer with SQLC
- Configuration management
- Common utilities and helpers

## Thinking Pipeline

The Thinking Pipeline is Relay's core intelligence layer. It transforms vague tickets into production-ready specs by understanding context gaps, retrieving relevant code information, and extracting tribal knowledge from your team.

### Pipeline Architecture

```
Event → Fetch Discussions → Upsert Issue → Extract Keywords → Planner → Retriever → Gap Detector ─┬─→ Questions
                                                                                                   └─→ Spec Generator
```

### Event Processing Phase

Before the thinking loop begins, incoming events are preprocessed:

1. **Fetch Discussions**: Retrieve full issue context from tracker API (title, description, all comments, metadata)
2. **Upsert Issue**: Store/update in `issues` table with complete discussion history
3. **Extract Keywords**: Lightweight LLM (Haiku) extracts search terms for retriever filtering
4. **Pass to Planner**: Prepared context moves to the thinking loop

### Thinking Loop

The core intelligence cycle that runs until all gaps are resolved:

#### 1. Planner

**Goal**: Orchestrate context gathering by identifying what's missing.

**Responsibilities**:
- Evaluate if current context is sufficient
- Identify missing information needed for a complete spec
- Plan retrieval tasks with focused queries
- Coordinate the Executor to fetch additional context

**Inputs**: Issue context from database (title, description, discussions, keywords, metadata)

**Flow**:
```
Read issue → Check sufficiency → {Sufficient: Call Gap Detector}
                                 {Insufficient: Plan retrieval → Execute → Loop}
```

#### 2. Retrievers (Executor)

Specialized providers that fetch context based on Planner queries:

**Code Context Retriever**:
- **Powered by**: ArangoDB (code graph) + filesystem tools (ripgrep, fd, direct file reading)
- **How it works**: Keywords → filesystem search → LLM-guided graph exploration → Relevant code context
- **What it provides**: Verified imports, call chains, architectural patterns, exact file locations

**Learnings Retriever**:
- **Powered by**: PostgreSQL + embeddings
- **What it contains**: Tribal knowledge extracted from past gap resolutions
- **Categories**: Business rules, architectural decisions, team preferences, edge cases, past incidents

**Why deterministic code understanding matters**:
- ✅ Traces exact import paths and function call chains
- ✅ Verifies every symbol exists and maps relationships
- ✅ Understands actual architecture from parse trees
- ✅ Fetches only the 3-5 files that matter (not 50+ files via semantic search)

#### 3. Gap Detector

**Goal**: Identify gaps in requirements, limitations, and edge cases by analyzing enriched context.

**What it detects**:
- Requirement gaps (missing/ambiguous specifications)
- Code limitations (architectural constraints)
- Business edge cases (product scenarios not covered)
- Technical edge cases (error scenarios not handled)
- Implied assumptions (unstated expectations)

**Question Philosophy**:
- Target the right stakeholder (@pm vs @dev_lead)
- Provide source/evidence from code analysis
- Include a suggestion to anchor discussion
- Explain severity and impact

**Example Questions** (for "Add Twilio Support"):

*To PM*:
> **Question**: Is this outbound only, or are inbound calls also expected?
>
> **Source**: Ticket mentions "international calling" but doesn't specify direction
>
> **Suggestion**: Inbound immediately brings in number provisioning, webhooks, and lifecycle handling
>
> **Why this matters (Severity: High)**: This decision can 2–3× the scope if misunderstood

*To Developer*:
> **Question**: Given `core_service` has no DB access, how do we handle provider config and credentials?
>
> **Source**: `core_service` is explicitly stateless; Twilio requires account-level configuration
>
> **Suggestion**: Worth deciding early whether core stays "dumb" or gets minimal config awareness
>
> **Why this matters (Severity: High)**: Misplacing this logic leads to awkward cross-service coupling

**Reply Analysis**:

When humans respond, Gap Detector takes one or more actions:

| Action | When | Result |
|--------|------|--------|
| **Resolve Gap** | Answer is complete | Mark gap closed, continue to next gap or Spec Generator |
| **Update Learnings** | Reply contains reusable knowledge | Extract insight and store in learnings database |
| **Follow-up** | Answer is partial or raises new questions | Post follow-up question, wait for next reply |

**Learnings Extraction Example**:
```
Question: "How should we handle Twilio rate limits?"

Human Reply: "We've been burned by this before with Exotel. Always use
exponential backoff with jitter, max 3 retries, then fail the call and
alert ops. Never silently retry forever - it masks problems."

Gap Detector Actions:
✓ Resolve Gap - Question answered
✓ Update Learnings - Store: "Rate limiting policy: exponential backoff
  with jitter, max 3 retries, then fail + alert. Never silent retry."
```

This captured learning will surface in future issues involving rate limiting or retry logic.

#### 4. Spec Generator

**Goal**: When all critical gaps are resolved, produce a detailed implementation plan.

**What it produces**:

```
┌─────────────────────────────────────────────────────────────────┐
│                      IMPLEMENTATION SPEC                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Summary & Context                                            │
│     - What we're building and why                                │
│     - Key decisions from gap resolution                          │
│                                                                  │
│  2. Affected Services & Files                                    │
│     - Exact files to modify/create                               │
│     - Import chains and dependencies (verified, not hallucinated)│
│                                                                  │
│  3. Implementation Steps                                         │
│     - Ordered tasks with clear boundaries                        │
│     - Code snippets and signatures where helpful                 │
│                                                                  │
│  4. Edge Cases & Error Handling                                  │
│     - Known edge cases from gap analysis                         │
│     - Expected error scenarios and responses                     │
│                                                                  │
│  5. Testing Requirements                                         │
│     - Critical paths to test                                     │
│     - Edge cases requiring coverage                              │
│                                                                  │
│  6. Out of Scope                                                 │
│     - Explicitly excluded items                                  │
│     - Future considerations                                      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**Inputs**: Issue context, code context, resolved gaps, learnings, architectural constraints

**For Human Developers**:
- Clear, scannable structure with context on "why" for each decision
- Links to relevant code locations
- Explicit scope boundaries

**For AI Coding Agents**:
- Precise file paths and function signatures (verified)
- Ordered steps with dependencies marked
- Concrete acceptance criteria

**Why this matters**: Traditional AI tools jump to code generation with minimal context, resulting in hallucinated imports, wrong signatures, and missed edge cases. Relay's spec ensures verified context, baked-in human decisions, and clear boundaries.

### Pipeline Lifecycle

```
1. Event arrives (webhook from issue tracker)
       ↓
2. Fetch Discussions + Upsert Issue + Extract Keywords
       ↓
3. Planner reads issue context
       ↓
4. [LOOP] Planner → Executor → Retrievers → Planner
   (until context is sufficient)
       ↓
5. Gap Detector analyzes enriched context
       ├─ Gaps Found → Generate questions → Post to issue → Wait for human response → [back to step 1]
       └─ No Gaps → Continue
       ↓
6. Spec Generator creates implementation plan
       ↓
7. Post spec as issue comment
       ↓
8. Done ✓ (Ready for developer or coding agent)
```

### LLM Configuration

Each pipeline component uses the right model for the job:

| Component | Model | Rationale |
|-----------|-------|-----------|
| **Keywords Extractor** | Haiku / GPT-4o-mini | Simple extraction, not reasoning (~$0.25/1M tokens) |
| **Planner** | Sonnet | Context sufficiency requires judgment (~$3/1M tokens) |
| **Code Graph Explorer** | Sonnet | Navigate graph, follow call chains (~$3/1M tokens) |
| **Gap Detector** | Sonnet (or Opus) | Critical — question quality matters most (~$3-15/1M tokens) |
| **Spec Generator** | Sonnet | Technical writing, structured output (~$3/1M tokens) |

**Cost estimate**: ~$0.12-0.20 per issue with 2-3 gap resolution cycles

### Database Schema

```sql
-- Core issue tracking
issues {
    id              bigint PK
    project_id      bigint FK
    external_id     string
    provider        enum('linear', 'github', 'gitlab')
    title           string
    description     text
    discussions     jsonb      -- all comments
    keywords        text[]     -- extracted search terms
    metadata        jsonb      -- labels, assignee, priority
    created_at      timestamp
    updated_at      timestamp
}

-- Captured tribal knowledge
learnings {
    id              bigint PK
    project_id      bigint FK
    issue_id        bigint FK (nullable)
    category        enum('business_rule', 'architecture', 'preference', 'edge_case')
    content         text
    source_context  text
    embedding       float[]    -- for similarity search
    created_at      timestamp
}

-- Gap tracking and resolution
gaps {
    id              bigint PK
    issue_id        bigint FK
    status          enum('open', 'resolved', 'skipped')
    question        text
    evidence        text       -- code reference or observation
    severity        enum('high', 'medium', 'low')
    target          enum('pm', 'developer')
    answer          text (nullable)
    learning_id     bigint FK (nullable)
    created_at      timestamp
    resolved_at     timestamp
}
```

### Design Principles

1. **Humans Decide, AI Assists**: Gap Detector surfaces decisions but never makes them
2. **Deterministic Over Probabilistic**: Code graph traversal over semantic search
3. **Right Question, Right Person**: PMs get product questions, developers get technical questions
4. **Tribal Knowledge Extraction**: Every human interaction captures reusable learnings
5. **Graceful Degradation**: Missing data should inform, not block

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