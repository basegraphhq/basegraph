# Graph Symbols & Search Implementation

## Status: Complete ✅

**Date**: 2026-01-03  
**Author**: AI Assistant (handoff document)

---

## Overview

Added `symbols` and `search` operations to the graph tool to solve the "qname discovery problem" - explore agents couldn't bridge from file paths to qualified names needed for relationship queries.

## Problem Solved

**Before**: Agents knew file paths from `tree`/`grep` but couldn't use `graph(callers, ...)` without knowing qnames.

**After**: 
```
graph(symbols, file="internal/brain/planner.go")  → Get qnames for all symbols
graph(search, name="*Issue*")                     → Find symbols by name pattern
graph(callers, qname="...Planner.Plan")           → Now possible!
```

---

## What's Done

### Phase 1: Codegraph - Signature Extraction ✅

**Files modified:**
- `codegraph/golang/extract/types.go` - Added `Signature string` field to `Function` struct
- `codegraph/golang/extract/golang/functions.go` - Added `buildSignature()` method that creates human-readable signatures like `(p *Planner) Plan(ctx context.Context, issue Issue) ([]Action, error)`

**Bug fixed:** Signature extraction now works for functions with no parameters (e.g., `Execute()` method).

**Note**: Run `go mod vendor` in codegraph/golang after pulling changes.

### Phase 2: ArangoDB - Storage & Queries ✅

**Files modified:**
- `relay/common/arangodb/types.go`:
  - Added `Signature string` to `Node` struct
  - Added `FileSymbol`, `SearchOptions`, `SearchResult` types

- `relay/common/arangodb/client.go`:
  - Updated `IngestNodes()` to store signature
  - Added `GetFileSymbols(ctx, filepath)` - returns all symbols in a file
  - Added `SearchSymbols(ctx, opts)` - searches by name pattern with filters
  - Added `globToLike()` helper for pattern conversion
  - Added `ensureIndexes()` for `idx_filepath` and `idx_name` on functions/types/members

- `codegraph/golang/process/ingest.go`:
  - Updated `ingestFunctionNodes()` to pass `Signature` to arangodb.Node

**Bug fixes:**
- `kind="method"` now works correctly (methods stored as `kind="function"` with `is_method=true`)
- Filepath matching uses suffix matching to handle relative vs absolute paths

### Phase 3: Graph Tool - New Operations ✅

**Files modified:**
- `relay/internal/brain/explore_tools.go`:
  - Updated `GraphParams` struct with new fields:
    - `File` - for symbols operation
    - `Name`, `Kind`, `Namespace` - for search operation  
    - `QName` - for relationship operations (renamed from `Target`)
  - Added `executeGraphSymbols()` handler
  - Added `executeGraphSearch()` handler
  - Refactored `executeGraph()` to route to appropriate handler
  - Added `graphIndexedExtensions` map for language detection
  - Updated tool description with new operations and workflow

### Phase 4: Tests ✅

**Files added/modified:**
- `relay/internal/brain/explore_tools_test.go`:
  - Added `mockArangoClient` implementing `arangodb.Client`
  - Added tests for `graph(symbols, ...)` operation
  - Added tests for `graph(search, ...)` operation  
  - Added tests for relationship operations with qname validation

- `codegraph/golang/extract/golang/extract_test.go`:
  - `TestExtractorCapturesSignatures` - tests regular functions, methods, value receivers
  - `TestFormatType` - tests type name simplification

### Phase 5: ArangoDB Indexes ✅

Indexes added in `ensureIndexes()`:
- `idx_filepath` on `functions`, `types`, `members` collections
- `idx_name` on `functions`, `types`, `members` collections

---

## API Reference

### symbols operation

```
graph(operation="symbols", file="internal/brain/planner.go")
```

**Output:**
```
Symbols in internal/brain/planner.go [indexed]:

  25 | Planner (struct)
       qname: basegraph.app/relay/internal/brain.Planner

  37 | NewPlanner(cfg Config, arango Client) *Planner (function)
       qname: basegraph.app/relay/internal/brain.NewPlanner

  52 | (p *Planner) Plan(ctx context.Context, issue Issue) ([]Action, error) (method)
       qname: basegraph.app/relay/internal/brain.Planner.Plan

Use graph(callers/callees/methods, qname=<qname>) for relationships.
```

**Non-indexed language:**
```
Symbols not available for .tsx files (not indexed).
Use grep to find definitions, or read the file directly.
```

### search operation

```
graph(operation="search", name="*Issue*", kind="struct")
graph(operation="search", name="Plan*", file="internal/brain/planner.go")
```

**Parameters:**
- `name` (required): Glob pattern (`*Issue*`, `Plan*`, `*Handler`)
- `kind` (optional): `function`, `method`, `struct`, `interface`
- `file` (optional): Filter by filepath (supports relative paths)
- `namespace` (optional): Filter by module path

**Output:**
```
Search results for name="*Issue*", kind="struct" (3 of 3):

- Issue (struct) [internal/model/issue.go:15]
  qname: basegraph.app/relay/internal/model.Issue

- IssueStore (struct) [internal/store/issue.go:22]
  qname: basegraph.app/relay/internal/store.IssueStore

Use graph(callers/methods/implementations, qname=<qname>) for relationships.
```

### Relationship operations (existing, updated)

Now use `qname` parameter instead of `target`:

```
graph(operation="callers", qname="basegraph.app/relay/internal/brain.Planner.Plan", depth=2)
graph(operation="methods", qname="basegraph.app/relay/internal/brain.Planner")
```

---

## Indexed Languages

Currently supported (defined in `graphIndexedExtensions`):
- `.go` (Go)
- `.py` (Python)

To add more languages:
1. Add codegraph extractor for the language
2. Add extension to `graphIndexedExtensions` map in `explore_tools.go`

Planned: `.ts`, `.js`, `.java`, `.cpp`, `.php`, `.rs`, `.rb`

---

## Database Schema Changes

New fields stored in ArangoDB `functions` collection:
- `signature`: Human-readable function signature

Indexes added:
- `idx_filepath` on functions, types, members
- `idx_name` on functions, types, members

AQL queries added:
- `GetFileSymbols`: Union query across functions, types, members filtered by filepath
- `SearchSymbols`: Union query with LIKE pattern matching and optional filters

---

## Testing Locally

1. Rebuild codegraph and re-index a repository:
```bash
cd codegraph/golang
go mod vendor
go build ./...
# Run indexer on test repo
```

2. Build and test relay:
```bash
cd relay
go build ./...
make test-unit
```

3. Test via explore agent or direct graph tool invocation.

---

## Known Limitations

1. **Non-indexed languages**: Return helpful error message directing to grep/read
2. **Large files**: No pagination on symbols output (unlikely to be issue in practice)
3. **Search limit**: Capped at 50 results with total count shown
4. **Signature quality**: Depends on codegraph extraction accuracy
5. **Variadic params**: Show as slice type (`[]int`) instead of `...int`

---

## Files Changed Summary

| File | Changes |
|------|---------|
| `codegraph/golang/extract/types.go` | +1 field |
| `codegraph/golang/extract/golang/functions.go` | +100 lines (signature extraction), bug fix |
| `codegraph/golang/extract/golang/extract_test.go` | +80 lines (signature tests) |
| `codegraph/golang/process/ingest.go` | +1 line |
| `relay/common/arangodb/types.go` | +30 lines (new types) |
| `relay/common/arangodb/client.go` | +150 lines (queries, indexes, bug fixes) |
| `relay/internal/brain/explore_tools.go` | +150 lines (new operations) |
| `relay/internal/brain/explore_tools_test.go` | +150 lines (mock client, tests) |
