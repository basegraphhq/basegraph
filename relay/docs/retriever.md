# CodeGraph Retriever Architecture

Design document for the CodeGraph Retriever sub-agent. This document captures architectural decisions from design discussions.

## Overview

The Retriever is a sub-agent spawned by the Planner to explore the codebase and gather context for issue scoping. It operates autonomously, using filesystem search tools and graph queries to understand code structure and relationships.

### Core Philosophy

1. **Trust the model** — No arbitrary limits on iterations, tool calls, or output size
2. **Deterministic over probabilistic** — Graph queries for relationships, filesystem tools for discovery
3. **LLM-to-LLM communication** — Prose output with code snippets, not structured JSON
4. **Use proven tools** — ripgrep, fd, and direct file reading instead of custom search infrastructure

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  PLANNER (Main Agent)                                           │
│                                                                  │
│  - Receives issue context                                        │
│  - Spawns retrieval tasks                                        │
│  - Accumulates context from retrievers                           │
│  - Decides when context is sufficient                            │
│  - Passes accumulated context to Gap Detector                    │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            │ Natural language query
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  RETRIEVER (Sub-Agent)                                          │
│                                                                  │
│  - Fresh context per task (disposable)                           │
│  - Explores using grep, glob, read, graph tools                  │
│  - Returns prose report with code snippets                       │
│  - Context is discarded after task completes                     │
└─────────────────────────────────────────────────────────────────┘
```

### Why Sub-Agent Pattern?

**Context window preservation.** Models degrade after ~40% context window usage. If the Planner reads code directly, its context fills with raw code and planning quality tanks.

The sub-agent pattern solves this:
- Retriever fills ITS context with code exploration (disposable)
- Retriever returns a curated report (compressed)
- Planner's context stays lean (issue + reports + reasoning)

### Data Flow

```
1. Planner receives issue
2. Planner spawns Retriever with query: "How does notification sending work?"
3. Retriever explores (grep → read → graph → ...)
4. Retriever writes prose report with relevant code snippets
5. Planner receives report, decides:
   - Need more context? → Spawn another retrieval task
   - Sufficient? → Pass to Gap Detector
6. Gap Detector sees: Issue + All retrieval reports (with actual code)
```

## Tools

Four filesystem and graph tools. No custom search infrastructure.

### 1. grep(pattern, include?, limit?)

**Purpose:** Search file contents with regex patterns.

**Backend:** ripgrep (rg)

**Input:**
```go
type GrepParams struct {
    Pattern string `json:"pattern"` // Regex pattern: "func.*Notification", "type.*Service"
    Include string `json:"include"` // Optional glob filter: "*.go", "**/*.sql"
    Limit   int    `json:"limit"`   // Optional result limit (default: 50)
}
```

**Returns:** List of matching locations with:
- filepath
- line number
- matching line content
- context lines (configurable)

**Coverage:** Searches all files in the repository:
- Code files (Go, Python, etc.)
- SQL migrations
- Config files (YAML, JSON, TOML)
- Documentation

### 2. glob(pattern)

**Purpose:** Find files by path patterns.

**Backend:** fd (fast alternative to find)

**Input:**
```go
type GlobParams struct {
    Pattern string `json:"pattern"` // "**/notification/*.go", "migrations/*.sql", "*.yaml"
}
```

**Returns:** List of matching file paths sorted by modification time (most recent first).

**Use case:** Discover files before reading their contents or understanding project structure.

### 3. read(file, start_line?, num_lines?)

**Purpose:** Read file contents with optional line range.

**Backend:** Direct filesystem read

**Input:**
```go
type ReadParams struct {
    File      string `json:"file"`       // Path to file
    StartLine int    `json:"start_line"` // Optional (default: 1)
    NumLines  int    `json:"num_lines"`  // Optional (default: entire file)
}
```

**Returns:** File contents with line numbers.

**Use case:** After grep or glob finds relevant files, read them to understand implementation.

### 4. graph(operation, target, depth?)

**Purpose:** Query code relationships deterministically.

**Backend:** ArangoDB graph traversal

**Input:**
```go
type GraphParams struct {
    Operation string `json:"operation"` // see operations below
    Target    string `json:"target"`    // qname of the entity
    Depth     int    `json:"depth"`     // optional, default 1
}
```

**Operations:**

| Operation | Description | Graph Direction |
|-----------|-------------|-----------------|
| `callers` | Who calls this function? | INBOUND via `calls` |
| `callees` | What does this function call? | OUTBOUND via `calls` |
| `implementations` | What types implement this interface? | INBOUND via `implements` |
| `methods` | What methods does this type have? | INBOUND via `parent` |
| `usages` | Where is this type used (params, returns)? | `param_of` + `returns` |
| `inheritors` | What types inherit from this? | INBOUND via `inherits` |

**Returns:** List of related entities with qname, filepath, line number, name, kind, relationship type.

**Note:** Graph returns lightweight results. Use read to fetch full code for interesting results.

## Output Format

The Retriever returns **prose with embedded code snippets**. Not JSON. Not structured data.

### Why Prose?

1. **LLM-to-LLM communication works best in natural language**
2. **Gap Detector needs to see actual code** to identify edge cases, limitations
3. **The Retriever can editorialize** — "this is a stub", "notice there's no error handling"
4. **Context flows naturally** — the "why" is next to the "what"

### Example Output

```markdown
## How Notifications Work

NotificationService is the central dispatcher. When Send() is called,
it routes to the appropriate provider based on the channel type:

```go
// internal/notification/service.go:42-58
func (s *Service) Send(ctx context.Context, n Notification) error {
    switch n.Channel {
    case ChannelEmail:
        return s.emailProvider.Send(ctx, n.To, n.Message)
    case ChannelSMS:
        return s.smsProvider.Send(ctx, n.To, n.Message)
    default:
        return fmt.Errorf("unknown channel: %s", n.Channel)
    }
}
```

The SMS provider is currently a stub — it just logs and returns nil:

```go
// internal/notification/sms.go:15-22
func (p *SMSProvider) Send(ctx context.Context, to string, msg Message) error {
    // TODO: implement actual SMS sending
    log.Info("SMS stub called", "to", to)
    return nil
}
```

### Call Chain

API Handler → NotificationService.Send → EmailProvider.Send / SMSProvider.Send

### Observations

- SMSProvider is not implemented (stub)
- No rate limiting visible in either provider
- Error handling in EmailProvider uses retry with exponential backoff (max 3 attempts)
- No circuit breaker pattern — if provider is down, requests will queue up
```

## System Prompt

```markdown
You are a code exploration agent. Your job is to investigate a codebase
and answer questions about it.

## Your Tools

- `grep(pattern, include?, limit?)` — Search file contents with regex
- `glob(pattern)` — Find files by path patterns
- `read(file, start_line?, num_lines?)` — Read file contents
- `graph(operation, target, depth?)` — Query code relationships
  - `callers` — Who calls this function?
  - `callees` — What does this function call?
  - `implementations` — What types implement this interface?
  - `methods` — What methods does this type have?
  - `usages` — Where is this type used?
  - `inheritors` — What types inherit from this?

## How to Work

1. Start with grep or glob to find relevant code
2. Use read to examine files you've discovered
3. Use graph to trace relationships (callers, callees, implementations)
4. When you understand enough to answer, write your report

## Your Output

Write a natural report for another AI agent. Include:

- What you found (prose explanation)
- Key code snippets with file:line references
- Relationships you discovered (call chains, implementations)
- Observations (edge cases, limitations, patterns, potential issues)
- What you didn't explore (if relevant for follow-up)

Write naturally. No JSON. Include actual code that matters.
```

## Design Decisions

### Why Filesystem Tools?

**Decision:** Use proven tools (ripgrep, fd, direct file reading) instead of custom search infrastructure.

**Rationale:**
- ripgrep is faster than any custom solution we could build
- fd provides instant file discovery
- Direct file reading is simple and reliable
- No infrastructure to maintain (no Typesense, no Elasticsearch)
- Works offline and in any environment
- Simpler mental model (4 tools: grep, glob, read, graph)

### No Artificial Limits

**Decision:** No caps on iterations, tool calls, or output size.

**Rationale:**
- Models are trained to know when they have enough information
- Arbitrary limits (e.g., "max 5 tool calls") force premature stopping
- Trust the model to explore and conclude naturally
- Only constraint: soft limit at 20 iterations to encourage synthesis (not a hard stop)

### No Structured Response Format

**Decision:** Prose output, not JSON schema.

**Rationale:**
- LLMs communicate better in natural language
- JSON forces the model to fit findings into a rigid structure
- Prose allows editorializing ("this looks like a stub")
- Gap Detector can read prose + code naturally

### Fresh Context Per Retrieval

**Decision:** Each retrieval task gets a fresh context window.

**Rationale:**
- Prevents context pollution across tasks
- Retriever can use full context for exploration (up to 60-70%)
- Context is discarded after report is returned
- Planner only accumulates compressed reports

## Indexing Strategy

### What Gets Indexed (ArangoDB Graph)

- **Code entities:** functions, types, members, modules, files
- **Relationships:** calls, implements, inherits, returns, param_of, parent, imports, decorated_by

### When Indexing Happens

- On main branch push via webhook
- Incremental updates for changed files only
- Acceptable for planning use case (work happens during office hours)

### Graph Schema

See `relay/docs/graph_schema.md` for full ArangoDB schema:
- **Nodes:** functions, types, members, files, modules
- **Edges:** calls, implements, inherits, returns, param_of, parent, imports, decorated_by

## Open Questions

### Planner → Retriever Interface

What does the Planner send to the Retriever?

**Options:**
1. Natural language query only
2. Query + hints (e.g., "look in internal/notification/")
3. Query + focus area (e.g., "call_chain", "implementations")

**Recommendation:** Start with natural language only. Add structure if needed.

### Stopping Signal

What tells the Retriever it's done?

**Answer:** The model's judgment. If the Planner sends a clear, answerable query, the Retriever knows when it has enough to answer. Vague queries ("understand everything about notifications") risk infinite exploration. Specific queries ("Find how SMS notifications are sent and what calls that code path") have natural stopping points.

Soft limit at 20 iterations encourages synthesis by injecting a user message asking for final report.

### Parallel Retrieval

Can the Planner spawn multiple Retrievers in parallel?

**Recommendation:** Support it. Each Retriever is independent (fresh context). Planner can spawn:
- "How do notifications work?"
- "How is config/secrets managed?"
- "What integrations exist?"

All three run in parallel, return reports, Planner accumulates.

## Implementation Notes

### Retriever Loop

```go
func (r *Retriever) Query(ctx context.Context, query string) (string, error) {
    messages := []Message{
        {Role: "system", Content: systemPrompt},
        {Role: "user", Content: query},
    }

    iterations := 0
    for {
        iterations++

        // Soft limit: encourage synthesis
        tools := r.tools
        if iterations > 20 {
            // Inject message asking for final report
            messages = append(messages, Message{
                Role: "user",
                Content: "You've explored extensively. Please synthesize your findings into a final report now.",
            })
            tools = nil // Remove tools to force text response
        }

        resp, err := r.llm.Chat(ctx, messages, tools)
        if err != nil {
            return "", err
        }

        // No tool calls = agent is done
        if len(resp.ToolCalls) == 0 {
            return resp.Content, nil
        }

        // Execute tools, continue
        messages = append(messages, assistantMessage(resp))
        for _, tc := range resp.ToolCalls {
            result := r.executeTool(ctx, tc)
            messages = append(messages, toolResultMessage(tc.ID, result))
        }
    }
}
```

### Timeout

Only hard constraint — safety net for stuck calls:

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
defer cancel()
```

### Tool Results

Return prose, not JSON. Example for graph:

```
Callers of NotificationService.Send (depth 1):

1. APIHandler.sendNotification (internal/http/handler.go:45)
   qname: github.com/org/repo/internal/http.APIHandler.sendNotification

2. Worker.processJob (internal/worker/processor.go:123)
   qname: github.com/org/repo/internal/worker.Worker.processJob

Use read to fetch full code for any of these files.
```

## Migration Path

### Removed

- Typesense search infrastructure
- `relay/common/typesense/` (client code)
- Complex search → get → graph pipeline
- JSON response parsing for search results

### Kept

- `relay/common/arangodb/` (graph client)
- `relay/docs/graph_schema.md` (schema reference)
- Agent-based exploration approach
- Prose output format

### Added

- Filesystem tools: grep (ripgrep), glob (fd), read (direct filesystem)
- Simpler tool set (4 tools instead of 3 specialized ones)
- Works entirely offline with just ArangoDB for graph relationships
