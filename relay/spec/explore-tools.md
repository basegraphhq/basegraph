# Explore Tools Specification

## Overview

This spec defines high-impact tools for the ExploreAgent to reduce token consumption and improve exploration quality. Based on analysis of production logs (Jan 3, 2026), explore agents hit hard token limits (~120k tokens) primarily due to:

1. **Lack of structural awareness** - blind glob/grep without knowing project layout
2. **Reading full files for structure** - consuming thousands of tokens just to understand what's in a file
3. **Grep returning too many results** - broad patterns like `issue` returning 344 matches

## Current Tools

| Tool | Purpose | Token Cost |
|------|---------|------------|
| `grep` | Search file contents by regex | Medium (depends on matches) |
| `glob` | Find files by path pattern | Low |
| `read` | Read file contents with line range | High (up to 300 lines) |
| `graph` | Query code relationships from ArangoDB | Low |

## Proposed Tools

### 1. `tree` - Directory Structure

**Problem:** Agent runs blind globs (`**/*issue*`) because it doesn't know where things are.

**Solution:** Show directory structure up to a configurable depth.

#### Parameters

```go
type TreeParams struct {
    Path  string `json:"path,omitempty" jsonschema:"description=Directory to list (default: repo root)"`
    Depth int    `json:"depth,omitempty" jsonschema:"description=Max depth (default: 2, max: 4)"`
}
```

#### Output Format

```
internal/
  brain/
    explore_agent.go
    explore_tools.go
    orchestrator.go
    planner.go
  model/
    event_log.go
    issue.go
    llm_eval.go
  service/
    event_ingest.go
  store/
    event_log.go
    issue.go
    llm_eval.go
cmd/
  server/
    main.go
  worker/
    main.go
```

#### Implementation Details

**Status:** ✅ Implemented in `relay/internal/brain/explore_tools.go`

**Behavior:**
- Default depth: 2 levels
- Max depth: 4 levels (prevents token explosion)
- Excludes: `.git`, `node_modules`, `vendor`, `__pycache__`, `.next`, `dist`, `build`, `.idea`, `.vscode`, `.cache`, `coverage`, `.turbo`, `target`
- Tree-style output with `├──` and `└──` connectors
- Directories shown first (with trailing `/`), then files
- Max entries: 200 (prevents token explosion on large repos)
- Truncation message shown if limit hit

**Security:**
- Rejects absolute paths (`/etc/passwd`)
- Rejects path traversal (`../../../etc`)
- Uses `filepath.Rel` to prevent `/repo` vs `/repo-evil` bypass
- All paths validated to be within repo root

#### Token Cost

~50-150 tokens for typical project structure at depth 2.

#### Tool Description (for LLM)

```
Show directory structure.

WHEN TO USE:
- First step when exploring unfamiliar area
- Before grep/glob to understand where to look
- When you need to find the right directory

PARAMETERS:
* path: Directory to list (default: repo root)
* depth: How deep to go (default 2, max 4)

RETURNS: Tree view of directories and files.
Limited to 200 entries. Use path param to focus on specific areas.

EXAMPLE: After tree(path="internal"), you know to grep in "internal/brain/" not "**/*brain*"
```

---

### 2. `symbols` - File Outline

**Problem:** Agent reads entire files (188 lines, ~3000 tokens) just to understand what's defined there.

**Solution:** Return structural outline with declarations and line numbers.

#### Parameters

```go
type SymbolsParams struct {
    File string `json:"file" jsonschema:"required,description=File path to outline"`
}
```

#### Output Format

```
package model

const (
    IssueStatusOpen      line 12
    IssueStatusClosed    line 13
    IssueStatusResolved  line 14
)

type IssueID int64                                   line 17

type Issue struct                                    line 20
    func NewIssue(extID int, title string) Issue     line 35
    func (i Issue) IsOpen() bool                     line 42
    func (i Issue) Close() Issue                     line 48
    func (i *Issue) AddComment(c Comment)            line 55

type IssueStore interface                            line 62
    Get(ctx, id) (Issue, error)                      line 63
    Upsert(ctx, issue) error                         line 64
    List(ctx, filter) ([]Issue, error)               line 65
```

#### Behavior

- Shows: package, imports (collapsed), const blocks, var blocks, types, functions, methods
- For types: shows receiver methods indented under the type
- Signatures: abbreviated (param names + types, return types)
- Line numbers: right-aligned for easy reading
- Max output: 100 symbols (truncate with "... and N more")

#### Language Support

**Language agnostic from day 1** via tree-sitter:
- Go, TypeScript, Python, Rust, Java, C, C++, etc.
- Use `github.com/smacker/go-tree-sitter` with language-specific grammars
- Graceful fallback: if language not supported, return error suggesting `read` tool

#### Token Cost

~100-300 tokens for typical file (vs 2000-4000 for full read).

#### Tool Description (for LLM)

```
Get structural outline of a file (declarations with line numbers).

WHEN TO USE:
- After grep/glob found a relevant file
- Before reading — understand what's in the file first
- When you need to find a specific function/type

RETURNS: Package, types, functions, methods with line numbers.
Use this BEFORE read() — then read only the lines you need.

EXAMPLE WORKFLOW:
1. grep finds "IssueStore" in "store/issue.go:45"
2. symbols("store/issue.go") → see all declarations
3. read("store/issue.go", start_line=45, num_lines=30) → get just that function

This saves thousands of tokens vs reading the whole file.
```

---

### 3. `definition` - Find Symbol Definition

**Problem:** Agent greps for symbol name, gets all usages (69 matches for `IssueID`), then reads multiple files to find the definition.

**Solution:** Jump directly to where a symbol is defined.

#### Parameters

```go
type DefinitionParams struct {
    Symbol string `json:"symbol" jsonschema:"required,description=Symbol name to find (e.g., 'IssueStore', 'NewIssue', 'IssueID')"`
    Kind   string `json:"kind,omitempty" jsonschema:"description=Filter by kind: type, func, method, const, var, interface (optional)"`
}
```

#### Output Format

```
Found 2 definitions for "IssueStore":

1. interface IssueStore
   Location: internal/model/issue.go:62
   
2. type issueStore struct (implements IssueStore)
   Location: internal/store/issue.go:15
```

If single match:
```
type IssueID int64
Location: internal/model/issue.go:17
```

#### Behavior

- Searches indexed symbols from codegraph (ArangoDB)
- Falls back to AST grep if not in graph
- Partial matching: `IssueStore` matches `IssueStore`, `issueStore`, `MockIssueStore`
- Kind filter: narrows to specific declaration type
- Max results: 10 (if more, suggest narrowing with `kind`)

#### Token Cost

~20-50 tokens per result.

#### Tool Description (for LLM)

```
Find where a symbol is defined.

WHEN TO USE:
- You want the definition, not usages
- grep returns too many results (all references, not definition)
- You know the name but not the file

PARAMETERS:
* symbol: Name to find (e.g., "IssueStore", "processRefund")
* kind: Optional filter — "type", "func", "interface", "const", "var"

RETURNS: Definition location(s) with signature preview.

BETTER THAN GREP for finding definitions:
- grep("IssueID") → 69 matches (all usages)
- definition("IssueID") → 1 match (the type declaration)
```

---

## Implementation Status

| Tool | Status | Token Impact | Complexity | Dependencies |
|------|--------|--------------|------------|--------------|
| `tree` | ✅ **Implemented** | -30-40% exploration | Low | None (filesystem only) |
| `symbols` | 🔜 Planned | -50-60% on reads | Medium | tree-sitter (language agnostic) |
| `definition` | 🔜 Planned | -20% | Low | ArangoDB graph (already indexed) |

## Expected Impact

Based on Jan 3 logs where explore agents consumed 121k and 110k tokens:

| Metric | Before | After (estimated) |
|--------|--------|-------------------|
| Avg tokens per exploration | 115k | 50-70k |
| Hard limit hits | Frequent | Rare |
| Iterations to find code | 8-14 | 4-6 |
| Full file reads | Many | Targeted only |

## Integration Notes

### Tool Registration

Add to `ExploreTools.All()` in `explore_tools.go`:

```go
func (t *ExploreTools) All() []llm.Tool {
    return []llm.Tool{
        t.grepTool(),
        t.globTool(),
        t.readTool(),
        t.graphTool(),
        t.treeTool(),     // NEW
        t.symbolsTool(),  // NEW
        t.definitionTool(), // NEW
    }
}
```

### Execution Router

Add cases in `Execute()`:

```go
case "tree":
    return t.executeTree(ctx, arguments)
case "symbols":
    return t.executeSymbols(ctx, arguments)
case "definition":
    return t.executeDefinition(ctx, arguments)
```

### Security

**Critical:** Path validation prevents directory traversal attacks.

**Implementation (tree tool):**
```go
// 1. Reject absolute paths immediately
if params.Path != "" && filepath.IsAbs(params.Path) {
    return "Error: path outside repository", nil
}

// 2. Resolve path within repo
rootPath := filepath.Join(t.repoRoot, params.Path)

// 3. Use filepath.Rel to check containment (handles /repo vs /repo-evil)
absPath, _ := filepath.Abs(rootPath)
absRoot, _ := filepath.Abs(t.repoRoot)
relPath, err := filepath.Rel(absRoot, absPath)
if err != nil || strings.HasPrefix(relPath, "..") {
    return "Error: path outside repository", nil
}
```

**Why this approach:**
- `filepath.IsAbs()` catches `/etc/passwd` immediately
- `filepath.Rel()` properly detects `../../../etc` traversal
- Prevents `/repo` vs `/repo-evil` bypass (where `strings.HasPrefix` would fail)

**Tested attack vectors:**
- Absolute paths: `/etc/passwd` ✅ Blocked
- Path traversal: `../../../etc` ✅ Blocked
- Sibling escape: `../repo-evil` ✅ Blocked
- Symlinks: ⚠️ Followed (known limitation, documented in tests)

## Future Tools (Not in MVP)

| Tool | Purpose | When to Add |
|------|---------|-------------|
| `references` | Find all usages of a symbol | If `graph(usages)` proves insufficient |
| `batch_read` | Read multiple files in one call | If message overhead becomes significant |
| `search_types` | Find types by pattern | If `definition` with kind filter is clunky |

## Testing

### Tree Tool Tests (✅ Implemented)

**Security tests (18 test cases):**
- Absolute path rejection (`/etc/passwd`)
- Path traversal with `..` (`../../../etc`)
- Encoded traversal (`src/../../..`)
- Sibling directory access (`../repo-evil`)
- Symlink behavior (documented limitation)

**Functionality tests:**
- Default depth behavior
- Path parameter handling
- Directory exclusions (`.git`, `node_modules`, etc.)
- Depth limits and capping
- Non-existent paths
- File vs directory validation
- Directory-first sorting
- Empty directory handling

**Edge case tests:**
- Paths with spaces
- Paths with special characters
- Deep nesting within depth limits

**Test location:** `relay/internal/brain/explore_tools_test.go`

### Symbols Tool Tests (Planned)

- All declaration types (const, var, type, func, method)
- Empty files, syntax errors
- Tree-sitter language support

### Definition Tool Tests (Planned)

- Exact match, partial match
- Kind filter
- Not found scenarios

### Integration Tests (Future)

- Real repo exploration with token counting
- Compare token usage before/after on same queries
- Verify doom loop detection still works with new tools

### Eval Criteria

Success = explore agent completes queries under 60k tokens average (vs current 110k+).
