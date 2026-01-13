# Token Consumption (Per-Issue Token Spend)

**Issue:** (internal) | **Complexity:** L2 | **Author:** Relay | **Reviewers:** TBD

## TL;DR
- Persist a **lifetime per-issue `token_total`** (prompt + completion only) to the `issues` record.
- Add an **atomic DB increment** query (`UPDATE issues SET token_total = token_total + $delta`) to handle **parallel explores** safely.
- Instrument both **Planner** and **ExploreAgent** to record token deltas on every successful LLM response (**includes retries/duplicates**).
- Make accounting **best-effort** (logs warnings on DB failure; does not block issue processing).
- Validate via **unit tests** (delta computation + call counts) and an **integration/concurrency test** for atomic increments.

## What We're Building
We need internal observability for “how many tokens did we spend to process this issue over its lifetime?”. Today we only compute token usage in-process and log it.

From findings:
- `brain/planner.go` accumulates `resp.PromptTokens`/`resp.CompletionTokens` and logs totals, but does **not** persist token usage. (Finding F1)
- `brain/explore_agent.go` also accumulates and logs tokens only. (Finding F1)
- The processing context already has `issue_id` available in orchestration, and the same `ctx` is passed into Planner and ExploreAgent. (Context Summary)

**Required semantics (resolved gaps, inlined):**
- Gap context: "What’s the definition of token spend?" → **prompt + completion tokens only**.
- Gap context: "What aggregation scope is intended?" → **lifetime aggregate** per issue.
- Gap context: "Retry/dedupe semantics?" → **include retries/duplicate processing** (count actual spend).
- Gap context: "Breakdown required?" → **single total only**.
- Gap context: "Usage surface?" → **internal observability only**.

## Code Changes

### Current State

**Planner logs tokens but does not persist (brain/planner.go:65,98-131):**
```go
func (p *Planner) Plan(ctx context.Context, messages []llm.Message) (PlannerOutput, error) {
    ...
    totalPromptTokens := 0
    totalCompletionTokens := 0

    defer func() {
        slog.InfoContext(ctx, "planner completed",
            "total_prompt_tokens", totalPromptTokens,
            "total_completion_tokens", totalCompletionTokens,
            "total_tokens", totalPromptTokens+totalCompletionTokens)
    }()

    for {
        resp, err := p.llm.ChatWithTools(ctx, llm.AgentRequest{Messages: messages, Tools: p.tools()})
        if err != nil { ... }

        totalPromptTokens += resp.PromptTokens
        totalCompletionTokens += resp.CompletionTokens

        slog.DebugContext(ctx, "planner iteration LLM response received",
            "prompt_tokens", resp.PromptTokens,
            "completion_tokens", resp.CompletionTokens)
        ...
    }
}
```

**ExploreAgent logs tokens but does not persist (brain/explore_agent.go:87-138):**
```go
tracks totalPromptTokens/totalCompletionTokens; increments from resp token fields; logs totals
```

**Issue model/store has no token field (model/issue.go:83-109, store/issue.go:21-61):**
```go
// model/issue.go
type Issue struct {
    ...
    Spec *string `json:"spec,omitempty"`

    ProcessingStatus    ProcessingStatus `json:"processing_status"`
    ...
}

// store/issue.go (Upsert params)
row, err := s.queries.UpsertIssue(ctx, sqlc.UpsertIssueParams{
    ID: issue.ID,
    ...
    Spec: issue.Spec,
})
```

### Proposed Changes

#### 1) DB schema: add `issues.token_total`

> This repo does **not** contain migrations/SQLC sources (Finding F2 tool output). You must apply the migration + sqlc query change in the repository that owns `basegraph.app/relay/core/db/sqlc`.

**Migration SQL (copy-paste):**
```sql
ALTER TABLE issues
  ADD COLUMN IF NOT EXISTS token_total BIGINT NOT NULL DEFAULT 0;

-- Optional: If you want to keep updated_at consistent on increment updates, no backfill is needed.
```

#### 2) SQLC query: atomic increment

**Add SQLC query (copy-paste; place in your sqlc query file for issues, e.g. `core/db/queries/issues.sql`):**
```sql
-- name: AddIssueTokens :one
UPDATE issues
SET token_total = token_total + $2,
    updated_at = NOW()
WHERE id = $1
RETURNING token_total;
```

#### 3) Model: expose token total

**File: `model/issue.go` (add field)**
```go
// model/issue.go

type Issue struct {
    ...
    Spec *string `json:"spec,omitempty"`

    // TokenTotal is the lifetime aggregate of prompt+completion tokens spent processing this issue.
    // Internal observability only.
    TokenTotal int64 `json:"token_total"`

    ProcessingStatus    ProcessingStatus `json:"processing_status"`
    ...
}
```

#### 4) Store: implement atomic increment

**File: `store/issue.go` (add method + map TokenTotal)**

Add this method to `issueStore`:
```go
// store/issue.go

// AddTokens atomically increments the lifetime token_total for an issue.
func (s *issueStore) AddTokens(ctx context.Context, issueID int64, delta int64) (int64, error) {
    if delta <= 0 {
        return 0, nil
    }

    row, err := s.queries.AddIssueTokens(ctx, sqlc.AddIssueTokensParams{
        ID:    issueID,
        Delta: delta,
    })
    if err != nil {
        return 0, err
    }

    return row.TokenTotal, nil
}
```

Update the DB→model mapper to include the new field (once sqlc regenerates `sqlc.Issue.TokenTotal`):
```go
// store/issue.go

func toIssueModel(issue sqlc.Issue) (*model.Issue, error) {
    ...
    return &model.Issue{
        ID:            issue.ID,
        ...
        Spec:          issue.Spec,
        TokenTotal:    issue.TokenTotal,
        ...
    }, nil
}
```

> Important: **do not** add `TokenTotal` to the `UpsertIssueParams` payload unless you also update the SQL to preserve existing totals. Otherwise you risk resetting totals during ingest/upsert.

#### 5) Brain: token tracker (best-effort)

**New file: `brain/token_tracker.go`**
```go
package brain

import (
    "context"
    "log/slog"
)

type IssueTokenStore interface {
    // AddTokens increments the issue token total by delta and returns the new total.
    AddTokens(ctx context.Context, issueID int64, delta int64) (int64, error)
}

type TokenTracker struct {
    store IssueTokenStore
}

func NewTokenTracker(store IssueTokenStore) *TokenTracker {
    return &TokenTracker{store: store}
}

// Record adds prompt+completion tokens to the issue's lifetime total.
// Best-effort: logs on failure and continues.
func (t *TokenTracker) Record(ctx context.Context, issueID int64, promptTokens, completionTokens int) {
    if t == nil || t.store == nil {
        return
    }

    delta := int64(promptTokens) + int64(completionTokens)
    if delta <= 0 || issueID == 0 {
        return
    }

    newTotal, err := t.store.AddTokens(ctx, issueID, delta)
    if err != nil {
        slog.WarnContext(ctx, "failed to record issue token usage",
            "issue_id", issueID,
            "delta_tokens", delta,
            "err", err,
        )
        return
    }

    slog.DebugContext(ctx, "recorded issue token usage",
        "issue_id", issueID,
        "delta_tokens", delta,
        "token_total", newTotal,
    )
}
```

#### 6) Planner: plumb `issueID` explicitly + record tokens per iteration

**File: `brain/planner.go`**

Update the `Plan` signature and record tokens:
```go
// brain/planner.go

func (p *Planner) Plan(ctx context.Context, issueID int64, messages []llm.Message) (PlannerOutput, error) {
    ...
    for {
        ...
        resp, err := p.llm.ChatWithTools(ctx, llm.AgentRequest{Messages: messages, Tools: p.tools()})
        if err != nil {
            ...
        }

        // Track token usage (existing in-process totals)
        totalPromptTokens += resp.PromptTokens
        totalCompletionTokens += resp.CompletionTokens

        // NEW: persist per-issue totals (prompt + completion only)
        p.tokenTracker.Record(ctx, issueID, resp.PromptTokens, resp.CompletionTokens)

        ...

        results := p.executeExploresParallel(ctx, issueID, resp.ToolCalls)
        ...
    }
}
```

Update the parallel explore executor to accept `issueID`:
```go
// brain/planner.go

func (p *Planner) executeExploresParallel(ctx context.Context, issueID int64, toolCalls []llm.ToolCall) []toolResult {
    ...
    // When calling explore agent:
    report, err := p.exploreAgent.Explore(ctx, issueID, query)
    ...
}
```

Ensure `Planner` has a `tokenTracker` field and constructor wiring:
```go
// brain/planner.go

type Planner struct {
    llm          llm.AgentClient
    exploreAgent *ExploreAgent
    tokenTracker *TokenTracker
    ...
}

func NewPlanner(llmClient llm.AgentClient, exploreAgent *ExploreAgent, tokenTracker *TokenTracker) *Planner {
    return &Planner{
        llm:          llmClient,
        exploreAgent: exploreAgent,
        tokenTracker: tokenTracker,
    }
}
```

#### 7) ExploreAgent: plumb `issueID` explicitly + record per call

**File: `brain/explore_agent.go`**

Update signature and record tokens after successful LLM response:
```go
// brain/explore_agent.go

func (a *ExploreAgent) Explore(ctx context.Context, issueID int64, query string) (string, error) {
    ...
    resp, err := a.llm.ChatWithTools(ctx, llm.AgentRequest{Messages: messages, Tools: a.tools()})
    if err != nil {
        return "", err
    }

    totalPromptTokens += resp.PromptTokens
    totalCompletionTokens += resp.CompletionTokens

    // NEW
    a.tokenTracker.Record(ctx, issueID, resp.PromptTokens, resp.CompletionTokens)

    ...
}
```

Ensure `ExploreAgent` has a `tokenTracker` and constructor parameter.

#### 8) Orchestrator: create tracker + pass `issueID` into Planner

**File: `brain/orchestrator.go`**

In `NewOrchestrator`, create a tracker using the issue store:
```go
// brain/orchestrator.go

func NewOrchestrator(..., issueStore store.IssueStore, llmClient llm.AgentClient, ...) *Orchestrator {
    tokenTracker := brain.NewTokenTracker(issueStore)

    explore := brain.NewExploreAgent(llmClient, tokenTracker)
    planner := brain.NewPlanner(llmClient, explore, tokenTracker)

    return &Orchestrator{
        ...
        planner: planner,
        ...
    }
}
```

In `HandleEngagement`, pass `IssueID` explicitly:
```go
// brain/orchestrator.go

out, err := o.planner.Plan(ctx, engagement.IssueID, messages)
```

> You may need minor refactors depending on actual constructor signatures in this file, but the required end-state is: **Planner.Plan(ctx, issueID, ...)** and ExploreAgent.Explore(ctx, issueID, ...)**.

### Key Types/Interfaces

**New internal interface for accounting (brain/token_tracker.go):**
```go
type IssueTokenStore interface {
    AddTokens(ctx context.Context, issueID int64, delta int64) (int64, error)
}
```

## Implementation
| # | Task | File | Done When | Blocked By |
|---|------|------|-----------|------------|
| 1 | Add DB column `issues.token_total` (BIGINT default 0) | (DB migrations repo) | Migration applied in a dev DB; `\d issues` shows `token_total` | DB migration workflow |
| 2 | Add sqlc query `AddIssueTokens` + regenerate `basegraph.app/relay/core/db/sqlc` | (DB/sqlc repo) | Generated code exposes `AddIssueTokens(ctx, params)` and `Issue.TokenTotal` | Task #1 |
| 3 | Add `TokenTotal int64` to `model.Issue` | `model/issue.go` | `go test ./...` compiles | Task #2 |
| 4 | Implement `issueStore.AddTokens` and map `TokenTotal` in `toIssueModel` | `store/issue.go` | Unit tests compile; store method called successfully in a dev run | Task #2 |
| 5 | Add `brain/TokenTracker` | `brain/token_tracker.go` | Unit tests for delta computation pass | - |
| 6 | Plumb `issueID` into Planner and ExploreAgent; call `Record` per response | `brain/planner.go`, `brain/explore_agent.go` | Running a real engagement increments `issues.token_total` | Task #4 |
| 7 | Wire tracker + new method signatures in Orchestrator | `brain/orchestrator.go` | End-to-end processing works; no signature mismatch | Task #6 |

## Tests

### Unit

- [ ] **Unit: TokenTracker delta**
  - GIVEN: `issueID=42`, `promptTokens=10`, `completionTokens=15`
  - WHEN: `Record(ctx, 42, 10, 15)`
  - THEN: store `AddTokens` called with `delta=25`

**File: `brain/token_tracker_test.go` (copy-paste):**
```go
package brain

import (
    "context"
    "sync"
    "testing"

    "github.com/stretchr/testify/require"
)

type fakeTokenStore struct {
    mu    sync.Mutex
    calls []struct {
        issueID int64
        delta   int64
    }
}

func (f *fakeTokenStore) AddTokens(ctx context.Context, issueID int64, delta int64) (int64, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.calls = append(f.calls, struct {
        issueID int64
        delta   int64
    }{issueID: issueID, delta: delta})
    return 123, nil
}

func TestTokenTracker_Record_ComputesDelta(t *testing.T) {
    store := &fakeTokenStore{}
    tr := NewTokenTracker(store)

    tr.Record(context.Background(), 42, 10, 15)

    require.Len(t, store.calls, 1)
    require.Equal(t, int64(42), store.calls[0].issueID)
    require.Equal(t, int64(25), store.calls[0].delta)
}

func TestTokenTracker_Record_IgnoresZeroDelta(t *testing.T) {
    store := &fakeTokenStore{}
    tr := NewTokenTracker(store)

    tr.Record(context.Background(), 42, 0, 0)

    require.Len(t, store.calls, 0)
}
```

### Integration (DB)

- [ ] **Integration: atomic increment under concurrency**
  - GIVEN: issue row with `token_total=0`
  - WHEN: run 20 goroutines calling `AddTokens(issueID, 5)`
  - THEN: final `token_total == 100`

**Suggested test (place near store DB tests, adjust to your existing DB test harness):**
```go
func TestIssueStore_AddTokens_Atomic(t *testing.T) {
    // GIVEN
    ctx := context.Background()
    s := newIssueStoreForTest(t) // use your existing helper
    issue := mustCreateIssue(t, s, /*integrationID*/ 1, /*externalIssueID*/ "X")

    // WHEN
    const n = 20
    const delta = int64(5)

    var wg sync.WaitGroup
    wg.Add(n)
    for i := 0; i < n; i++ {
        go func() {
            defer wg.Done()
            _, err := s.AddTokens(ctx, issue.ID, delta)
            require.NoError(t, err)
        }()
    }
    wg.Wait()

    // THEN
    got, err := s.GetByID(ctx, issue.ID) // or however issues are fetched
    require.NoError(t, err)
    require.Equal(t, int64(n)*delta, got.TokenTotal)
}
```

### Edge case

- [ ] **Edge: DB error should not fail processing**
  - GIVEN: token store returns an error
  - WHEN: Planner/ExploreAgent records tokens
  - THEN: engagement continues (no returned error); warning log emitted

(Implement via a failing fake store in unit tests for Planner/ExploreAgent, if you already have LLM fakes in tests.)

## Gotchas
- **Parallel explores need atomic increments:** `brain/planner.go` runs explores in parallel (`executeExploresParallel`), so token recording must be safe under concurrency. Use a single SQL `UPDATE ... SET token_total = token_total + delta` (no read-modify-write in Go).
- **Do not reset totals on Upsert:** `store/issue.go:21-61` upserts issues for ingest. If you add `token_total` to upsert params without careful SQL, you can overwrite accumulated totals (e.g., set to 0). Keep token updates isolated to `AddIssueTokens`.
- **Rollout ordering matters with sqlc-generated structs:** If you regenerate sqlc with `token_total` included, any query generated with `SELECT *` (expanded at generation time) may fail against a DB without the new column (scan mismatch). Deploy **migration first**, then code.
- **Errors from LLM calls may still spend tokens:** We only record tokens after successful responses because that’s what we have access to. If the provider charges tokens for failed requests, this will undercount. (Acceptable for internal observability; call out in dashboards.)
- **Best-effort logging:** Since this is internal-only, the spec makes accounting non-blocking. If you need strict correctness later (billing/quotas), revisit this.

## Operations
- **Verify:**
  1. Apply migration.
  2. Process a real issue.
  3. Run `SELECT token_total FROM issues WHERE id = <issue_id>;` and confirm it increased.
  4. Confirm logs include `recorded issue token usage` with `delta_tokens`.
- **Monitor:**
  - Count of log warnings: `failed to record issue token usage` (should be near-zero).
  - Spot-check that `token_total` increases monotonically per issue.
- **Rollback:**
  1. Roll back application deploy (stop writing increments).
  2. Keep the column; it’s harmless. (Preferred.)
  3. Only if required, run a down migration to drop `token_total` **after** rolling back code that expects it.

## Decisions
| Decision | Why | Trade-offs |
|----------|-----|------------|
| Persist a single `issues.token_total` lifetime aggregate | Gap context: "What aggregation scope is intended?" → "lifetime" and "What level of breakdown is required?" → "just total" | No per-run/event attribution; future breakdowns require schema change |
| Count prompt+completion only | Gap context: "What’s the definition of token spend?" → "just promp + completion" | Under-counts if embeddings/tool APIs are added later |
| Include retries/duplicate processing | Gap context: "Retry/dedupe semantics" → "include" | Totals can grow due to failures; that’s intended for “actual spend” |
| Best-effort recording (warn on failure, don’t fail engagement) | Gap context: "Usage surface" → "internal" | Might miss some increments; must monitor warning logs |
| Explicitly plumb `issueID` into Planner/ExploreAgent | Context: relying on extracting from `ctx` is brittle; signatures are small and localized | Requires signature changes and refactors at call sites |

## Alternatives Considered
| Option | Pros | Cons | Why Not |
|--------|------|------|---------|
| Wrap `llm.AgentClient` with an accounting decorator | Centralized; no need to touch call sites | Still needs `issue_id` context; hard to attribute in tool calls; implicit magic | More invasive/opaque for L2; explicit call-site instrumentation is clearer |
| New `issue_token_usage` table keyed by issue_id | Avoids modifying `issues` upsert paths | Still needs migration + queries; adds join for every read | Simpler to store on `issues` for internal observability |
| Store tokens on `EventLog` and roll up | Best detail; per event visibility | More schema, more queries, rollup complexity | Requirement is lifetime total only |

## Assumptions
| Assumption | If Wrong |
|------------|----------|
| The DB schema + sqlc sources live outside this repo (imports `basegraph.app/relay/core/db/sqlc`) | Create a `db/migrations` + `db/queries` structure in this repo, wire sqlc generation into CI, and update imports accordingly |
| `issueStore` is available to Orchestrator construction | If Orchestrator doesn’t have store access today, inject it via constructor params or create a small `TokenUsageStore` dependency passed to Planner/ExploreAgent |
| `llm.AgentClient.ChatWithTools` responses always have integer `PromptTokens`/`CompletionTokens` | If tokens can be missing/nullable, guard with defaults and only record when present |

# Context from Planner

(Provided in issue context; no additional exploration required.)
