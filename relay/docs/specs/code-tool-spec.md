# Code Tool Specification

## Problem

The current `graph` tool is underutilized (1 call out of 34 in logs) because:
1. Requires qnames which the model doesn't have upfront
2. Search returns too much data (30+ results)
3. Two-step workflow: search → get qname → query relationships

## Solution

Replace `graph` with a simpler `code` tool that:
- Accepts symbol name + optional filters (no qnames exposed)
- Resolves to qname internally
- Returns compact, one-line-per-result output
- Handles ambiguity with helpful error messages

## Tool Schema

```json
{
  "name": "code",
  "parameters": {
    "operation": "find | callers | callees | implementations | methods",
    "symbol": "string (required) - name or pattern like 'Plan', '*Issue*', 'Planner.Plan'",
    "kind": "string (optional) - 'method', 'struct', 'interface', 'function'",
    "file": "string (optional) - file path filter"
  }
}
```

## Operations

| Operation | Description | Returns |
|-----------|-------------|---------|
| find | Find symbols by name/pattern | Symbol locations |
| callers | Who calls this symbol | Calling functions |
| callees | What does this symbol call | Called functions |
| implementations | What implements this interface | Implementing types |
| methods | What methods does this type have | Methods |

## Output Format

**Successful find (single result):**
```
Issue (struct) at model/issue.go:15
```

**Successful find (multiple results):**
```
Found 3 symbols matching "Plan":
  Planner.Plan (method) at internal/brain/planner.go:45
  TaskPlanner.Plan (method) at internal/task/planner.go:23
  PlanConfig (struct) at internal/config/plan.go:10
```

**Successful callers:**
```
Planner.Plan (method) at internal/brain/planner.go:45

Callers:
  Orchestrator.Run at internal/orchestrator/orchestrator.go:89
  Worker.Execute at internal/worker/worker.go:112
```

**Ambiguous match for relationship query:**
```
Multiple methods named "Plan":
  Planner.Plan at internal/brain/planner.go:45
  TaskPlanner.Plan at internal/task/planner.go:23

Add file="brain" or symbol="Planner.Plan" to specify.
```

**Not found:**
```
No symbol "FooBar" found.
Try: rg -n "FooBar" for text search.
```

## Limits

- Max 15 results per query
- One line per result (compact output)

## Implementation Tasks

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Add `ResolveSymbol` method to arangodb client | `common/arangodb/client.go` | Done |
| 2 | Add `ResolvedSymbol` and `AmbiguousSymbolError` types | `common/arangodb/types.go` | Done |
| 3 | Create `CodeParams` struct | `internal/brain/explore_tools.go` | Done |
| 4 | Implement `executeCode` function | `internal/brain/explore_tools.go` | Done |
| 5 | Update tool definitions (remove graph, add code) | `internal/brain/explore_tools.go` | Done |
| 6 | Update system prompt | `internal/brain/explore_agent.go` | Done |
| 7 | Add code tool tests | `internal/brain/explore_tools_test.go` | TODO |
| 8 | Remove old graph code | `internal/brain/explore_tools.go` | Done |

## API Changes

### New: `ResolveSymbol`

```go
// ResolveSymbol finds a single symbol matching the query.
// Returns AmbiguousSymbolError if multiple matches found.
// Returns ErrNotFound if no matches found.
func (c *client) ResolveSymbol(ctx context.Context, opts SearchOptions) (ResolvedSymbol, error)
```

### New Types

```go
// ResolvedSymbol contains the qname and location of a uniquely resolved symbol.
type ResolvedSymbol struct {
    QName    string
    Name     string
    Kind     string
    Filepath string
    Pos      int
}

// AmbiguousSymbolError is returned when multiple symbols match the query.
type AmbiguousSymbolError struct {
    Query      string
    Candidates []SearchResult
}
```

## Success Criteria

1. Model uses `code` tool naturally (no qname friction)
2. Single call for relationship queries (no two-step workflow)
3. Compact output (< 1KB per typical query)
4. Token usage drops significantly (target: 50% reduction)
