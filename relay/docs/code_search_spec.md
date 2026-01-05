# Code Search & Codegraph API Specification

**Status:** Active Development  
**Owner:** Relay Brain Team  
**Last Updated:** 2026-01-03  

---

## Problem Statement

The Relay Brain's ExploreAgent needs to understand codebases to answer questions about:
- Symbol locations (types, functions, fields)
- Code relationships (who calls what, what implements what)
- Data flow and dependencies
- Impact analysis (what breaks if I change X)

**Current Challenge:** 
- LLMs are trained on bash/grep/ripgrep → naturally prefer text search
- Text search is verbose (50k+ tokens) but familiar to models
- Codegraph provides deterministic AST-based search but requires adoption
- Need to find the right balance: leverage LLM training + graph superpowers

**Goal:**
Design a code search API that:
1. Works WITH model training, not against it
2. Provides unique value bash/grep cannot (relationships, impact analysis)
3. Is simple, fast, and intuitive for LLMs to use

---

## Current State Analysis

### Tool Usage Pattern (2026-01-03)

**Session 1 (Before optimizations):**
```
bash:  19 calls  → Text search for symbols (inefficient)
read:  20 calls  → Reading files
code:  0 calls   → Not used at all
Tokens: 51-54k per explore (hitting limits)
```

**Session 2 (After making code PRIMARY):**
```
code:  3 calls   → Used for symbol search ✅
bash:  0 calls   → Eliminated redundant searches ✅
read:  12 calls  → Reading specific files
Tokens: 60k per explore (still high, but from legitimate reads)
```

**Session 3 (After reverting to hybrid):**
```
TBD - Let model choose naturally
```

### What Works

✅ **Codegraph infrastructure is solid:**
- 1,297 functions indexed
- 1,144 types indexed
- 3,026 call edges indexed
- Query speed: 28-33ms (excellent!)
- Data quality: Accurate AST-based extraction

✅ **Model CAN use code tool:**
- When prompted correctly, uses code(find, symbol)
- Understands parallel tool execution
- Gets value from structured results

### What Doesn't Work

❌ **Fighting LLM training is expensive:**
- Requires complex prompts ("DO NOT use bash for X")
- Model hesitates, uncertain which tool to use
- Anti-patterns are negative instruction (harder to learn)

❌ **Token consumption still high:**
- Even with code tool, reads 600+ lines (60k tokens)
- Over-reading to understand context
- Hitting medium thoroughness limits

---

## Design Philosophy: Hybrid Approach

**Core Principle:** Work WITH model training, not against it.

### Tool Roles

| Tool | Best For | Why |
|------|----------|-----|
| **bash** | Text search, exploration, finding symbols by name | Model expert, trained on billions of examples |
| **code** | Relationships, graph traversal, impact analysis | Unique capability bash cannot replicate |
| **read** | Understanding specific code sections | Focused reading with line numbers |

**Key Insight:** Let bash do what it's good at (finding things). Use codegraph for what bash CAN'T do (understanding relationships).

### Success Criteria

1. **Model confidence:** No hesitation choosing tools
2. **Unique value:** Code tool provides insights bash cannot
3. **Token efficiency:** 80-120k per explore is acceptable (within 500k-1M budget)
4. **Quality:** Accurate, complete answers to relationship questions

---

## API Design: Current Operations

### Implemented & Exposed

```go
// Symbol Search
code(operation="find", symbol="Issue")
→ SearchSymbols() → Returns locations of all symbols matching pattern
→ Useful but bash(rg "Issue") also works

// Relationship Queries (UNIQUE VALUE)
code(operation="callers", symbol="Plan")
→ GetCallers() → Who calls this function?
→ bash CANNOT do this efficiently

code(operation="callees", symbol="Plan")
→ GetCallees() → What does this call?
→ bash CANNOT do this efficiently

code(operation="implementations", symbol="Store")
→ GetImplementations() → What implements this interface?
→ bash would need complex regex + AST understanding

code(operation="methods", symbol="IssueStore")
→ GetMethods() → What methods does this type have?
→ bash would need parsing struct definitions
```

### Implemented But NOT Exposed

```go
// Available in arangodb.Client but not in code tool

GetUsages(qname) → Where is this type used? (params, returns)
GetInheritors(qname) → What inherits from this class?
GetChildren(qname) → Children of parent entity
GetFileSymbols(filepath) → All symbols defined in a file
TraverseFrom(qnames, opts) → Multi-start graph traversal
```

---

## Proposed API Improvements

### Phase 1: Expose Existing Operations (Quick Wins)

**Priority: HIGH**  
**Effort:** 15-30 minutes  
**Impact:** Immediate value, already implemented

#### 1.1 Add `usages` Operation

**Use Case:** "Where is Issue struct used?"

```go
code(operation="usages", symbol="Issue")
```

**Implementation:**
```go
// In explore_tools.go CodeParams
enum=usages  // Add to operation enum

// In executeCodeRelationship()
case "usages":
    nodes, opErr = t.arango.GetUsages(ctx, resolved.QName)
```

**Why Valuable:**
- Answers: "What functions take Issue as parameter?"
- Answers: "What returns Issue type?"
- bash would need: rg for function signatures, parse params/returns → complex

**Example Output:**
```
code(operation="usages", symbol="Issue")
→ Issue (struct) at model/issue.go:84

Usages:
  GetByID at store/issue.go:45 (returns Issue)
  Create at store/issue.go:89 (param: issue Issue)
  Update at service/issue_tracker.go:123 (param: issue *Issue)
```

#### 1.2 Add `symbols` Operation (File Overview)

**Use Case:** "What symbols are defined in planner.go?"

```go
code(operation="symbols", file="brain/planner.go")
```

**Implementation:**
```go
// In explore_tools.go CodeParams
enum=symbols  // Add to operation enum

// New handler in executeCode()
case "symbols":
    return t.executeCodeSymbols(ctx, params)

func (t *ExploreTools) executeCodeSymbols(ctx context.Context, params CodeParams) (string, error) {
    if params.File == "" {
        return "Error: 'file' parameter required for symbols operation", nil
    }
    
    symbols, err := t.arango.GetFileSymbols(ctx, params.File)
    // Format and return
}
```

**Why Valuable:**
- Answers: "What's in this file?" → structured overview
- More efficient than bash(head file.go) → AST-parsed, organized by type
- Shows function signatures, types, fields in one call

**Example Output:**
```
code(operation="symbols", file="brain/planner.go")
→ Symbols in brain/planner.go:

Types:
  Planner (struct) at line 45
  PlanResult (struct) at line 89

Functions:
  NewPlanner(client, explore) at line 52
  Plan(ctx, issue) at line 112
  executeExploresParallel(ctx, calls) at line 250
```

---

### Phase 2: New High-Value Operations (Design Phase)

**Priority: MEDIUM**  
**Effort:** 2-4 hours  
**Impact:** Unique insights bash cannot provide

#### 2.1 Impact Analysis

**Use Case:** "What would break if I change Plan function?"

```go
code(operation="impact", symbol="Plan")
```

**Returns:**
```
Plan (function) at brain/planner.go:112

Impact Analysis:
  Direct Callers (2):
    - HandleEngagement at orchestrator.go:125
    - ProcessIssue at worker.go:89
  
  Transitive Callers (5 within 3 hops):
    - HandleWebhook at http/webhook.go:45
    - ConsumeMessage at queue/consumer.go:78
    ...
  
  Types Used:
    - Issue (param)
    - PlanResult (return)
  
  Estimated Impact: 7 functions, 3 files
```

**Implementation Approach:**
```go
type ImpactResult struct {
    Symbol GraphNode
    DirectCallers []GraphNode
    TransitiveCallers []GraphNode  // 2-3 hops
    DirectCallees []GraphNode
    TypesUsed []GraphNode
    FilesAffected []string
}

func (c *client) GetImpact(ctx, qname string) (ImpactResult, error) {
    // 1. Get direct callers (1 hop)
    // 2. Get transitive callers (2-3 hops via traversal)
    // 3. Get callees (what this depends on)
    // 4. Extract unique files
    // 5. Count total impact
}
```

#### 2.2 Call Path Analysis

**Use Case:** "How does HTTP request reach the database?"

```go
code(operation="call_path", from="HandleWebhook", to="ExecuteQuery")
```

**Returns:**
```
Call Path from HandleWebhook to ExecuteQuery:

Path 1 (4 hops):
  HandleWebhook (http/webhook.go:45)
  → ProcessIssue (service/event_ingest.go:89)
  → CreateIssue (store/issue.go:112)
  → ExecuteQuery (db/connection.go:234)

Path 2 (5 hops):
  HandleWebhook → ... (alternate route)
```

**Implementation Approach:**
```go
type CallPath struct {
    Hops []GraphNode
    Depth int
}

func (c *client) GetCallPath(ctx, fromQName, toQName string, maxDepth int) ([]CallPath, error) {
    // Use AQL shortest path or k-shortest paths
    // Limit to 3-5 paths to avoid explosion
}
```

#### 2.3 File Dependencies

**Use Case:** "What files does planner.go depend on?"

```go
code(operation="file_deps", file="brain/planner.go")
```

**Returns:**
```
brain/planner.go dependencies:

Imports (3):
  - common/llm
  - internal/brain/explore_agent
  - model/issue

Imported By (2):
  - internal/brain/orchestrator.go
  - cmd/worker/main.go

Transitively Affects (8 files):
  [list of files that import planner or its importers]
```

**Implementation Approach:**
```go
func (c *client) GetFileDependencies(ctx, filepath string) (FileDeps, error) {
    // Query imports edges for this file
    // Query reverse imports edges
    // Return import graph
}
```

---

## Tool Description Strategy

### Current Approach (After Testing)

Make code tool complementary to bash, not competitive:

```markdown
## code tool - Relationship & Structure Analysis

Use for queries bash CANNOT answer:
  • callers - Who calls this function? (graph traversal)
  • callees - What does this call? (call graph)
  • implementations - What implements this interface? (type hierarchy)
  • methods - What methods does this type have? (AST structure)
  • usages - Where is this type used? (semantic search)
  • symbols - All symbols in a file (AST overview)

For finding symbols by name: bash works great, use what you prefer.

Examples:
  code(operation="callers", symbol="Plan")
  → Shows call graph (bash can't do this!)
  
  code(operation="usages", symbol="Issue")
  → Where Issue is used as param/return (bash would need complex parsing)
  
  code(operation="symbols", file="planner.go")
  → Structured overview (faster than reading file)
```

### bash tool Description

```markdown
## bash - Your Familiar Exploration Tool

Use freely for:
  • Finding symbols: rg -n "type Issue struct"
  • Text patterns: rg -n "TODO"
  • File exploration: head -30 file.go
  • Git commands: git log --oneline

For relationship queries (callers, implementations), use code tool.
```

---

## Token Budget Strategy

### Revised Approach: Accept Higher Token Usage

**Old thinking:** Fight to reduce tokens via tool forcing
**New thinking:** Accept natural exploration, set appropriate budget

**Budget Tiers:**
```go
ThoroughnessQuick: {
    MaxIterations:   20,
    SoftTokenTarget: 100000,   // Increased from 20k
    HardTokenLimit:  150000,   // Safety ceiling
}

ThoughnessMedium: {
    MaxIterations:   30,
    SoftTokenTarget: 300000,   // Increased from 60k
    HardTokenLimit:  400000,
}

ThoughnessThorough: {
    MaxIterations:   50,
    SoftTokenTarget: 500000,
    HardTokenLimit:  800000,   // Increased from 150k
}
```

**Rationale:**
- You have 500k-1M token budget per session
- Quality answers require understanding code (legitimate reads)
- Fighting model training wastes time/effort
- Better to accept 100-200k per explore and get great answers

---

## Success Metrics

### Tool Usage (Target)

```
code tool: 5-10 calls per explore
  - 80%+ for relationship queries (callers, implementations, usages)
  - 20% for find/symbols (when convenient)

bash tool: 10-15 calls per explore
  - Finding symbols, text search, exploration
  - Model comfortable, no hesitation

read tool: 10-15 calls per explore
  - Understanding code after finding locations
```

### Token Consumption (Target)

```
Per explore: 80-150k tokens
Per issue (2-3 explores): 200-400k tokens
Session (500k budget): 1-2 issues comfortably
```

### Quality Metrics

```
Relationship queries answered: 100% (using code tool)
Symbol finding: Fast (bash or code, both work)
Model confidence: High (no tool selection hesitation)
User satisfaction: "Complete, accurate answers"
```

---

## Implementation Roadmap

### ✅ Completed

- [x] Codegraph infrastructure (ArangoDB, schema, ingestion)
- [x] Basic operations (find, callers, callees, implementations, methods)
- [x] ExploreAgent integration
- [x] Initial testing (3 sessions, iterated on prompts)

### ✅ Phase 1: Expose Existing Operations (Complete)

**Timeline:** 30 minutes  
**Completed:** 2026-01-03

- [x] Add `usages` operation to code tool
- [x] Add `symbols` operation to code tool
- [x] Update tool descriptions (hybrid approach)
- [x] Update system prompt (code for relationships, bash for exploration)
- [ ] Test with 3-5 queries

### 📋 Phase 2: New Operations (Future)

**Timeline:** 2-4 hours  
**Owner:** TBD

- [ ] Design `impact` operation API
- [ ] Implement GetImpact() in arangodb client
- [ ] Design `call_path` operation API
- [ ] Implement GetCallPath() in arangodb client
- [ ] Design `file_deps` operation API
- [ ] Implement GetFileDependencies() in arangodb client
- [ ] Add all to code tool
- [ ] Test impact analysis queries

### 🔮 Phase 3: Advanced Features (Ideas)

- [ ] Call chain visualization (Mermaid diagrams)
- [ ] Diff impact analysis ("what changed between commits?")
- [ ] Cross-language support (Go + TypeScript graphs)
- [ ] Semantic similarity search (find similar functions)
- [ ] Code pattern detection (find all auth checks)

---

## Testing Plan

### Test Queries for Phase 1

**Relationship Queries (code tool should shine):**
1. "Who calls the Plan function?" → `code(callers, "Plan")`
2. "What does HandleEngagement call?" → `code(callees, "HandleEngagement")`
3. "What implements the Store interface?" → `code(implementations, "Store")`
4. "Where is Issue struct used?" → `code(usages, "Issue")` [NEW]
5. "What's defined in planner.go?" → `code(symbols, file="planner.go")` [NEW]

**Exploration Queries (bash should work fine):**
1. "Find all TODO comments" → bash should use rg
2. "Find Issue type definition" → bash or code, both work
3. "What's in brain directory?" → bash or tree

**Expected Results:**
- Queries 1-5: code tool used (relationship/structure)
- Queries 6-8: bash or code (model chooses naturally)
- No hesitation, fast answers
- Token usage: 80-150k per explore (acceptable)

---

## Open Questions

1. **Token budget philosophy:**
   - Accept 100-200k per explore as normal?
   - Or continue optimizing toward 50k?
   - **Current thinking:** Accept higher usage for quality

2. **Tool forcing vs. natural selection:**
   - Force code tool for symbols via prompts?
   - Or let model choose (bash works fine)?
   - **Current thinking:** Let model choose, code for relationships only

3. **Phase 2 priority:**
   - Which operation most valuable: impact, call_path, or file_deps?
   - Should we build all or wait for user feedback?
   - **Current thinking:** Wait, see what questions users actually ask

4. **Performance at scale:**
   - 30ms queries are great now (1.3k functions)
   - What happens at 10k, 100k functions?
   - Need caching? Indexing improvements?

---

## References

- [Graph Schema Documentation](./graph_schema.md)
- [ExploreAgent Implementation](../internal/brain/explore_agent.go)
- [ArangoDB Client](../common/arangodb/client.go)
- [Tool Definitions](../internal/brain/explore_tools.go)

---

## Change Log

**2026-01-03:**
- Initial spec created
- Documented current state (3 test sessions)
- Defined hybrid approach (bash + code complementary)
- Designed Phase 1 (usages, symbols operations)
- Outlined Phase 2 (impact, call_path, file_deps)
- Revised token budget philosophy
