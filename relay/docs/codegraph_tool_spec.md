# Codegraph Tool Specification

## Overview

Wire up the existing ArangoDB codegraph client to ExploreAgent, enabling agents to query code relationships and discover symbols for Go and Python files.

**Current State**: ArangoDB client is initialized in worker (line 126-144) and passed to Orchestrator (line 116), but never connected to ExploreTools.

**Target State**: ExploreAgent can use a `codegraph` tool for:
1. **Symbol discovery** - Find symbols by name pattern to get correct qnames
2. **Relationship queries** - Callers, callees, implementations, usages

---

## Implementation Checklist

- [x] Update ExploreTools struct and constructor (`explore_tools.go`)
- [x] Add CodegraphParams struct (`explore_tools.go`)
- [x] Implement executeCodegraph method (`explore_tools.go`)
- [x] Implement executeCodegraphSearch method (`explore_tools.go`)
- [x] Update Execute switch statement (`explore_tools.go`)
- [x] Wire up ArangoDB client in Orchestrator (`orchestrator.go`)
- [x] Update ExploreAgent system prompt (`explore_agent.go`)
- [ ] Write unit tests
- [ ] Manual integration test

---

## Implementation Steps

### 1. Update ExploreTools struct and constructor

**File**: `relay/internal/brain/explore_tools.go`

- Add `arango arangodb.Client` field to struct (can be nil for graceful degradation)
- Modify `NewExploreTools(repoRoot string, arango arangodb.Client)` signature
- Add `codegraph` tool definition to `t.definitions`

```go
type ExploreTools struct {
    repoRoot    string
    arango      arangodb.Client // nil = codegraph unavailable
    definitions []llm.Tool
}
```

### 2. Add CodegraphParams struct

**File**: `relay/internal/brain/explore_tools.go`

```go
type CodegraphParams struct {
    Operation string `json:"operation" jsonschema:"required,enum=search,enum=callers,enum=callees,enum=implementations,enum=usages,description=Operation: search (find symbols), callers/callees/implementations/usages (relationships)"`

    // For search operation
    Name string `json:"name,omitempty" jsonschema:"description=Symbol name pattern (e.g. 'Plan', 'Handler*'). Required for search."`
    Kind string `json:"kind,omitempty" jsonschema:"description=Filter by kind: function, method, struct, interface"`
    File string `json:"file,omitempty" jsonschema:"description=Filter by file path (e.g. 'planner.go')"`

    // For relationship operations
    QName string `json:"qname,omitempty" jsonschema:"description=Fully qualified name. Required for callers/callees/implementations/usages."`
    Depth int    `json:"depth,omitempty" jsonschema:"description=Traversal depth for callers/callees (1-3, default 1)"`
}
```

### 3. Implement executeCodegraph method

**File**: `relay/internal/brain/explore_tools.go`

**Validation:**
- Check if `t.arango == nil` → return "Codegraph unavailable, use grep/read"
- For `search`: require `name` parameter
- For relationship ops: require `qname` parameter
- Validate depth bounds (1-3)

**Operation dispatch:**
```go
switch params.Operation {
case "search":
    return t.executeCodegraphSearch(ctx, params)
case "callers":
    nodes, err = t.arango.GetCallers(ctx, params.QName, depth)
case "callees":
    nodes, err = t.arango.GetCallees(ctx, params.QName, depth)
case "implementations":
    nodes, err = t.arango.GetImplementations(ctx, params.QName)
case "usages":
    nodes, err = t.arango.GetUsages(ctx, params.QName)
}
```

**Search with truncation (max 10 results):**
```go
func (t *ExploreTools) executeCodegraphSearch(ctx context.Context, params CodegraphParams) (string, error) {
    results, total, err := t.arango.SearchSymbols(ctx, arangodb.SearchOptions{
        Name:      params.Name,
        Kind:      params.Kind,
        File:      params.File,
    })
    // Truncate to 10 results
    if len(results) > maxSearchResults {
        results = results[:maxSearchResults]
    }
    return t.formatSearchResults(results, total), nil
}
```

**Constants:**
```go
const (
    maxSearchResults = 10
    defaultGraphDepth = 1
    maxGraphDepth = 3
)
```

**Language support** (hardcoded, for documentation):
```go
var supportedCodegraphLanguages = map[string]bool{
    ".go": true,
    ".py": true,
}
```

### 4. Update Execute switch statement

**File**: `relay/internal/brain/explore_tools.go`

Add case for "codegraph" in the Execute method.

### 5. Wire up ArangoDB client in Orchestrator

**File**: `relay/internal/brain/orchestrator.go`

Change line 128:
```go
// Before:
tools := NewExploreTools(cfg.RepoRoot)

// After:
tools := NewExploreTools(cfg.RepoRoot, arango)
```

### 6. Update ExploreAgent system prompt (optional enhancement)

**File**: `relay/internal/brain/explore_agent.go`

Add guidance about when to use codegraph vs grep:
- `codegraph` for structural queries (call chains, interface implementations)
- `grep/read` for text patterns, comments, unsupported languages

---

## Critical Files

| File | Change |
|------|--------|
| `relay/internal/brain/explore_tools.go` | Add codegraph tool, params, execute method |
| `relay/internal/brain/orchestrator.go` | Pass arango to NewExploreTools (line 128) |
| `relay/internal/brain/explore_agent.go` | Update system prompt with codegraph guidance |
| `relay/common/arangodb/client.go` | Reference only (no changes needed) |

---

## Tool Definition

```go
{
    Name: "codegraph",
    Description: `Query code structure graph. SUPPORTED: Go (.go) and Python (.py) only.

OPERATIONS:

1. search - Find symbols by name to get their qnames
   codegraph(operation="search", name="Plan", kind="method")
   codegraph(operation="search", name="Handler*", file="api.go")

2. callers - Find functions that call this function
   codegraph(operation="callers", qname="basegraph.app/relay/internal/brain.Planner.Plan")

3. callees - Find functions called by this function
   codegraph(operation="callees", qname="...", depth=2)

4. implementations - Find types that implement this interface
   codegraph(operation="implementations", qname="basegraph.app/relay/store.IssueStore")

5. usages - Find where this type is used (params, returns)
   codegraph(operation="usages", qname="basegraph.app/relay/internal/model.Issue")

WORKFLOW: Use search first to find the correct qname, then use relationship operations.

For unsupported languages, use grep/read instead.`,
    Parameters: llm.GenerateSchemaFrom(CodegraphParams{}),
}
```

---

## Output Formatting

**Search results (with truncation)**:
```
Found 3 symbols matching "Plan" (kind=method):

  basegraph.app/relay/internal/brain.Planner.Plan (internal/brain/planner.go:445)
  basegraph.app/relay/internal/brain.ExploreAgent.Run (internal/brain/explore_agent.go:145)
  basegraph.app/relay/internal/model.PlanAction (internal/model/action.go:12)

Use qname with callers/callees/implementations/usages operations.
```

**Search results (truncated)**:
```
Showing 10 of 47 symbols matching "Handle". Refine with kind or file filter.

  basegraph.app/relay/internal/brain.Orchestrator.HandleEngagement (internal/brain/orchestrator.go:153)
  ...
```

**Relationship results**:
```
Callers of basegraph.app/relay/internal/brain.Planner.Plan (depth 1) - 3 results:

- HandleEngagement (function)
  qname: basegraph.app/relay/internal/brain.Orchestrator.HandleEngagement
  file: internal/brain/orchestrator.go:307
  signature: func (o *Orchestrator) HandleEngagement(ctx context.Context, input EngagementInput) error

Use read(file_path, offset) to see the code.
```

**No results**:
```
No symbols found matching "FooBar".

Possible reasons:
- Symbol may not exist in indexed codebase (Go/Python only)
- Try a different name pattern or check spelling
- Codebase may need re-indexing
```

**Codegraph unavailable**:
```
Codegraph is not available. Use grep and read tools instead.
```

---

## Verification

1. **Unit test**: Mock arangodb.Client, verify all 5 operations work
2. **Nil client test**: Verify graceful degradation message
3. **Search truncation test**: Verify truncation message when results > 10
4. **Integration test**: With real ArangoDB data:
   ```
   codegraph(operation="search", name="Plan", kind="method")
   codegraph(operation="callers", qname="basegraph.app/relay/internal/brain.Planner.Plan")
   ```
5. **Manual test**: Run worker, trigger ExploreAgent, observe codegraph tool usage

---

## Edge Cases Handled

| Case | Behavior |
|------|----------|
| `arango == nil` | "Codegraph unavailable, use grep/read" |
| Search without `name` | Error: "name parameter required for search" |
| Relationship op without `qname` | Error: "qname parameter required for [op]" |
| No results | Helpful message with possible reasons |
| Results > 10 (search) | Truncate + "Showing 10 of N. Refine with kind/file." |
| Invalid operation | List valid operations |
| Depth < 1 or > 3 | Clamp to bounds |
| Symbol not in graph | Same as no results |

---

## Manual Test Cases

These issues are designed to exercise specific codegraph operations. Use them against an indexed codebase to verify the tool works end-to-end with the Planner.

### Test Setup

1. Index the target codebase into ArangoDB
2. Start the worker with `BRAIN_DEBUG_DIR` set
3. Create these issues in the issue tracker
4. Trigger Relay and observe debug logs for codegraph tool usage

### Test Issues

#### Issue 1: Symbol Search + Lookup
**Title:** Add retry logic to gap closing in ActionExecutor

**Description:**
When closing a gap via `update_gaps`, if the database call fails, we should retry once before giving up. Currently it just fails immediately.

Can you check how gap closing is implemented and add a simple retry?

**Expected codegraph usage:**
- `search` for "gap" or "close" symbols
- `lookup` on the executor method to see implementation

**Validates:** Basic symbol discovery workflow

---

#### Issue 2: Incoming Calls
**Title:** Audit all callers of `FormatValidationErrorForLLM`

**Description:**
We just added `FormatValidationErrorForLLM` for validation error feedback. Before we ship, can you verify:
1. Where is it being called from?
2. Are there other places that should use it but don't?

**Expected codegraph usage:**
- `search` for "FormatValidationErrorForLLM"
- `callers` on the found qname

**Validates:** `callers` operation, call hierarchy traversal

---

#### Issue 3: Search with Kind Filter
**Title:** List all action types and their validators

**Description:**
I'm trying to understand the action system. Can you give me a quick overview of:
- All the action types we support
- Which validator function handles each one

**Expected codegraph usage:**
- `search` with `kind=const` for action types
- `search` with `kind=function` for validators
- Possibly `callees` to trace validator dispatch

**Validates:** Kind filtering, constant discovery

---

#### Issue 4: Outgoing Calls Chain
**Title:** Trace the flow from webhook to planner

**Description:**
When a GitHub comment webhook comes in, how does it eventually reach the Planner? I want to understand the full path:
- Entry point (HTTP handler)
- Queue/worker handoff
- Orchestrator
- Planner invocation

**Expected codegraph usage:**
- `search` for webhook handler
- `callees` with depth=2 or 3 to trace the call chain
- Multiple `callees` calls following the path

**Validates:** `callees` operation, multi-hop traversal

---

#### Issue 5: Find Usages
**Title:** Where do we use `GapSeverityBlocking`?

**Description:**
We're considering renaming severity levels. Before we do, can you find all places where `GapSeverityBlocking` is used? I want to understand the blast radius.

**Expected codegraph usage:**
- `search` for "GapSeverityBlocking"
- `usages` on the constant qname

**Validates:** `usages` operation for constants/types

---

#### Issue 6: Interface + Implementations
**Title:** What methods does `ActionValidator` interface require?

**Description:**
I want to add a new validation hook. What's the current interface look like and where is it implemented?

**Expected codegraph usage:**
- `search` for "ActionValidator" with `kind=interface`
- `implementations` to find concrete types

**Validates:** `implementations` operation, interface discovery

---

### Expected Outcomes

| Issue | Primary Op | Success Criteria |
|-------|------------|------------------|
| 1 | search + lookup | Agent finds executor code without grep fallback |
| 2 | callers | Agent correctly identifies all call sites |
| 3 | search (kind) | Agent finds constants and functions by kind filter |
| 4 | callees (depth) | Agent traces multi-file call path |
| 5 | usages | Agent finds all references to constant |
| 6 | implementations | Agent finds interface implementors |

### Failure Modes to Watch

- Agent falls back to grep when codegraph should work
- Agent uses wrong qname format (missing package path)
- Agent doesn't use search first to discover qnames
- Depth parameter ignored or incorrectly applied
- Results truncated but agent doesn't refine search

---

## Realistic Test Scenarios

These are natural issues a developer would file. The agent should organically discover it needs codegraph to solve them.

### Scenario 1: Bug Fix Requiring Call Tracing
**Title:** Validation errors crash the worker instead of returning to user

**Description:**
When I submit an action with an invalid gap ID, the whole engagement fails with a fatal error. I expected it to tell me what went wrong so I can fix it.

Logs show:
```
level=ERROR msg="action validation failed" error="action[0] update_gaps: close[0]: gap not found: 999"
```

**Why codegraph helps:**
Agent needs to trace error handling flow from validator → orchestrator → planner to understand where the error gets swallowed vs surfaced.

**Expected agent behavior:**
1. Find where validation errors originate
2. Trace callers to see how errors propagate
3. Identify the gap in error feedback

---

### Scenario 2: Feature Addition with Impact Analysis
**Title:** Add "wontfix" as a gap close reason

**Description:**
Sometimes we realize a gap isn't worth pursuing. We need a `wontfix` close reason alongside `answered`, `inferred`, and `not_relevant`.

**Why codegraph helps:**
Agent needs to find all places where close reasons are defined, validated, and handled to ensure complete implementation.

**Expected agent behavior:**
1. Search for existing close reason constants
2. Find the validation function
3. Trace usages to find switch statements or conditionals that need updating

---

### Scenario 3: Refactoring with Dependency Discovery
**Title:** Split ActionExecutor into separate executors per action type

**Description:**
`ActionExecutor` is getting too big. Each action type (post_comment, update_gaps, etc.) should have its own executor struct. Need to understand current structure first.

**Why codegraph helps:**
Agent needs to understand the full interface, all implementations, and what calls what before proposing a refactor.

**Expected agent behavior:**
1. Find ActionExecutor interface and struct
2. List all methods and their callees
3. Identify shared vs action-specific logic
4. Check what depends on ActionExecutor

---

### Scenario 4: Performance Investigation
**Title:** Planner takes 30+ seconds on large issues

**Description:**
When an issue has many discussions, the planner is super slow. Need to figure out what's happening - is it LLM calls? Database queries? Context building?

**Why codegraph helps:**
Agent needs to trace the hot path from entry point through all function calls to identify bottlenecks.

**Expected agent behavior:**
1. Find Planner.Plan entry point
2. Use callees with depth to map the execution graph
3. Identify loops, database calls, and LLM invocations
4. Pinpoint likely bottlenecks

---

### Scenario 5: Understanding Unfamiliar Code
**Title:** How does the gap system work?

**Description:**
I'm new to the codebase. I need to understand:
- How gaps are created
- How they're stored
- How they're closed
- How they appear in the UI context

Just need an overview before I start working on gap-related features.

**Why codegraph helps:**
Agent needs to explore the gap domain across multiple packages - model, store, brain, actions.

**Expected agent behavior:**
1. Search for Gap-related types and functions
2. Find the store interface and implementations
3. Trace how gaps flow through the system
4. Build a mental model from the code structure

---

### Scenario 6: Bug in Specific Edge Case
**Title:** Closing a gap with short ID doesn't work in batch with spec generation

**Description:**
When I try to close gap `3` (short ID) in the same batch as `ready_for_spec_generation`, it fails saying the gap is still open. But closing it separately works fine.

**Why codegraph helps:**
Agent needs to understand the batch validation logic, specifically how `pendingClosures` tracking works with short IDs vs full IDs.

**Expected agent behavior:**
1. Find the spec generation validator
2. Understand how pendingClosures is built
3. Trace the gap ID lookup logic
4. Identify the short ID handling bug

---

### Scenario 7: Integration Between Systems
**Title:** Webhook events aren't triggering the planner

**Description:**
GitHub webhooks are being received (I see them in logs) but the planner never runs. Events seem to get stuck somewhere between the webhook handler and the worker.

**Why codegraph helps:**
Agent needs to trace the full async flow: webhook → queue → worker → orchestrator → planner.

**Expected agent behavior:**
1. Find webhook handler entry point
2. Trace callees to queue producer
3. Find worker consumer
4. Trace to orchestrator and planner
5. Identify where the chain breaks

---

### Expected Codegraph Usage by Scenario

| Scenario | Primary Operations | Depth Needed |
|----------|-------------------|--------------|
| 1 | callers, callees | 2 |
| 2 | search, usages | 1 |
| 3 | search, callees, callers | 2-3 |
| 4 | callees | 3 |
| 5 | search (multiple), implementations | 1-2 |
| 6 | search, callees | 2 |
| 7 | callees | 3 |

### Success Indicators

- Agent uses codegraph proactively without being asked
- Agent combines multiple operations (search → callers → callees)
- Agent refines searches when results are too broad
- Agent falls back to grep/read only for non-Go files or text content
- Agent builds accurate mental model from graph traversal
