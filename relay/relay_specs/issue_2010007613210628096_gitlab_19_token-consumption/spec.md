# Spec: Token Consumption

**Status:** Draft  
**Issue:** N/A (internal)  
**Last updated:** 2026-01-11  
**Complexity:** L2

---

## TL;DR
- Provide a **DB-level, SQL-queryable** way to get **lifetime token consumption per issue** (Gap #2, Gap #3).
- Token spend is **SUM(prompt_tokens + completion_tokens)** over all persisted LLM calls (`llm_evals`) **tied to the issue** (Gap #4).
- Attribution is **best-effort**: only rows with `issue_id` set are counted; missing links are acceptable (Gap #1).
- Deliver as **persistence only** (no UI/product API); optionally add an internal store accessor for convenience/tests (Gap #5).
- Biggest risk: schema/sqlc changes live in an **external DB/sqlc module** (not in this repo), so delivery requires an upstream PR and coordination.

## Problem Statement
We already persist per-LLM-call token counts on `LLMEval` records and can optionally associate an eval with an `issue_id`. Specifically:
- `LLMEval` includes `IssueID *int64`, `PromptTokens *int`, and `CompletionTokens *int` (Finding F1).
- The store insert path forwards `IssueID`, `PromptTokens`, and `CompletionTokens` into the generated sqlc insert (Finding F3).

However, we don’t have a standardized, first-class SQL surface to query **lifetime token totals per issue** (e.g., “How many tokens have we spent on issue 123 in workspace 9?”). Today, operators must manually aggregate raw rows.

We need a stable and documented SQL surface (view and/or named query) to compute per-issue totals.

## Success Criteria (OpenSpec-style scenarios)

### Requirement: Lifetime token aggregation per issue
The system SHALL provide an SQL-queryable aggregation of token consumption per issue, defined as the sum of all LLM calls tied to that issue.

#### Scenario: Happy path (issue has LLM calls)
- **WHEN** an issue has one or more persisted `llm_evals` rows with `workspace_id = W`, `issue_id = I`, and token fields present
- **THEN** querying the aggregate returns:
  - `llm_call_count = COUNT(*)`
  - `prompt_tokens_sum = SUM(prompt_tokens)`
  - `completion_tokens_sum = SUM(completion_tokens)`
  - `total_tokens_sum = SUM(prompt_tokens + completion_tokens)`

#### Scenario: NULL token fields (best-effort)
- **WHEN** some `llm_evals` rows for `(W, I)` have `prompt_tokens` and/or `completion_tokens` as NULL (token fields are pointers in Go, so they may be unset) (Finding F1)
- **THEN** the aggregate SHALL treat NULL as 0 for summation (via `COALESCE`) and SHALL still count the row in `llm_call_count`.

#### Scenario: No linked calls
- **WHEN** an issue has no `llm_evals` rows with `workspace_id = W` and `issue_id = I` (best-effort attribution; Gap #1)
- **THEN** the aggregate query returns **no row** for that `(W, I)` (and consumers may `COALESCE` to 0 if they need explicit zeros).

#### Scenario: Issue linkage missing
- **WHEN** an `llm_evals` row has `issue_id = NULL`
- **THEN** that row SHALL NOT contribute to any issue’s totals (Gap #1).

### Requirement: Persistence-only delivery
The system SHALL NOT introduce UI/product API surfaces for displaying token spend.

#### Scenario: Operator validation
- **WHEN** the feature is shipped
- **THEN** operators can validate results via a canonical SQL query (Gap #2) without UI/API changes (Gap #5).

## Goals / Non-goals

### Goals
- Enable **lifetime** per-issue token spend queries (Gap #3).
- Standardize calculation semantics: `prompt_tokens + completion_tokens` (Gap #4).
- Keep scope minimal: persistence + SQL queryability (Gap #5).

### Non-goals
- Perfect attribution of every LLM call to an issue (Gap #1).
- UI, dashboards, alerts, billing workflows.
- Backfilling or inferring missing `issue_id` for existing/orphaned LLM evals.
- Deduplicating retried LLM calls (aggregation will reflect what’s stored).

## Decision Log (ADR-lite)

| # | Decision | Context (Gap/Finding) | Consequences |
|---|----------|------------------------|--------------|
| 1 | Define “token spend” as `prompt_tokens + completion_tokens` summed over all LLMEvals for the issue. | Gap #4: “sum of all llm calls tied to the issue”; Finding F1: token fields exist on `LLMEval`. | Consistent accounting; excludes anything not recorded as an `LLMEval` token field. |
| 2 | Aggregate window is **lifetime per issue** (no run scoping). | Gap #3: “lifetime” | Totals monotonically increase as more LLMEvals are stored. |
| 3 | Use **best-effort attribution**: only `llm_evals` rows with `issue_id` set are counted. | Gap #1: “it’s fine”; Finding F3: store supports optional `IssueID`. | Under-counting is acceptable if some calls aren’t linked to an issue. |
| 4 | Expose the aggregation as a **DB view + (optional) sqlc query**. | Gap #2: success = SQL query; Workspace learning: DB/sqlc is external; Finding F3/F4: store depends on generated `sqlc`. | View provides stable SQL surface; sqlc query enables typed access/testing if needed. |
| 5 | Treat NULL tokens as 0 using `COALESCE`. | Finding F1: `PromptTokens`/`CompletionTokens` are `*int` and can be nil. | Prevents NULL sums; makes totals robust to partial token capture. |

## Assumptions

| # | Assumption | If Wrong |
|---|------------|----------|
| 1 | The DB has an `llm_evals` (or equivalent) table with `workspace_id`, `issue_id`, `prompt_tokens`, `completion_tokens`. | Adjust view/query to match the actual table/column names in the external DB/sqlc module. |
| 2 | `workspace_id` is available and should be part of the grouping key to avoid cross-workspace collisions. | If issues are globally unique, grouping can omit workspace_id; otherwise keep it to prevent incorrect totals. |
| 3 | We can land schema/sqlc changes in the external `basegraph.co/relay/core/db/sqlc` module used by this repo. | If upstream changes aren’t possible, document the raw SQL query only and skip sqlc/store additions in this repo. |

## Design

### API / Data Model

**Existing source of truth in this repo**
- `model/llm_eval.go` defines:
  - `IssueID *int64`
  - `PromptTokens *int`
  - `CompletionTokens *int` (Finding F1)
- `store/llm_eval.go` creates eval rows using `s.queries.InsertLLMEval(...)` and passes through `IssueID`, `PromptTokens`, `CompletionTokens` (Finding F3).
- `store/interfaces.go` defines `LLMEvalStore` and currently supports `ListByIssue(ctx, issueID int64)` but no aggregation API (Finding F2).

**New DB surface (primary deliverable)**
- Add a DB VIEW (recommended name): `issue_token_consumption`.
- Keyed by `(workspace_id, issue_id)`.
- Columns:
  - `workspace_id BIGINT NOT NULL`
  - `issue_id BIGINT NOT NULL`
  - `llm_call_count BIGINT NOT NULL`
  - `prompt_tokens_sum BIGINT NOT NULL`
  - `completion_tokens_sum BIGINT NOT NULL`
  - `total_tokens_sum BIGINT NOT NULL`

**Conceptual view definition (final SQL must match upstream schema/table names):**
```sql
CREATE VIEW issue_token_consumption AS
SELECT
  workspace_id,
  issue_id,
  COUNT(*) AS llm_call_count,
  SUM(COALESCE(prompt_tokens, 0)) AS prompt_tokens_sum,
  SUM(COALESCE(completion_tokens, 0)) AS completion_tokens_sum,
  SUM(COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0)) AS total_tokens_sum
FROM llm_evals
WHERE issue_id IS NOT NULL
GROUP BY workspace_id, issue_id;
```

**Optional typed access in this repo (secondary deliverable)**
- Add a model for typed results:
  - `model/issue_token_consumption.go` (new)
  - `type IssueTokenConsumption struct { WorkspaceID, IssueID, LLMCallCount, PromptTokensSum, CompletionTokensSum, TotalTokensSum int64 }`
- Extend `LLMEvalStore` with:
  - `GetTokenConsumptionByIssue(ctx context.Context, workspaceID, issueID int64) (*model.IssueTokenConsumption, error)`
- Implement in `store/llm_eval.go` or new `store/issue_token_consumption.go`, backed by a new sqlc query reading from the view.

### Flow / Sequence
1. An LLM call produces an `LLMEval` with optional `IssueID` and optional token counts (Finding F1).
2. Persistence path inserts the eval via `(*llmEvalStore).Create()`, which forwards `IssueID` + token fields to sqlc insert (Finding F3).
3. Operators (and optionally internal code) query `issue_token_consumption` to compute totals on demand.

### Concurrency / Idempotency / Retry behavior
- The view is derived from stored rows; reads are concurrency-safe.
- If upstream code retries inserts and duplicates `llm_evals`, totals will include duplicates. This feature does not change deduplication.
- The view should be non-blocking and computed at query-time; no background job needed.

## Implementation Plan

> Important constraint: this repo imports generated queries from `basegraph.co/relay/core/db/sqlc` (Finding F3, F4), and workspace learnings confirm migrations/sqlc live outside this repo. Therefore, schema/view + sqlc query work must land in that upstream module first.

| # | Task | Touch Points | Done When | Blocked By |
|---|------|--------------|-----------|------------|
| 1.1 | Add DB migration to create view `issue_token_consumption` (definition above, adjusted to real table/columns). | **External module** that provides `basegraph.co/relay/core/db/sqlc` (workspace learning; Finding F4 shows dependency) | In a dev DB, `SELECT * FROM issue_token_consumption ...` works and matches manual aggregation from raw eval rows. | Upstream repo access/ownership |
| 1.2 | Add sqlc query `GetIssueTokenConsumption(workspace_id, issue_id)` (and optionally `ListIssueTokenConsumptionByWorkspace(workspace_id)`) reading from the view. | **External module** that generates `basegraph.co/relay/core/db/sqlc` | Generated `sqlc.Queries` exposes the new method(s). | 1.1 |
| 2.1 | (Optional) Add model type for the aggregate result. | `model/issue_token_consumption.go` (new) | Model compiles; used by store method/tests. | - |
| 2.2 | (Optional) Extend `LLMEvalStore` interface with `GetTokenConsumptionByIssue(...)`. | `store/interfaces.go` (Finding F2) | Interface updated; callers compile. | 1.2 (if implementation uses sqlc) |
| 2.3 | (Optional) Implement store method that calls the new sqlc query and returns `nil` on no-row. | `store/llm_eval.go` (Finding F3) or `store/issue_token_consumption.go` (new) | Method returns correct aggregates and handles “no rows” deterministically. | 1.2, 2.1 |
| 2.4 | Document canonical SQL for operators. | `README.md` or runbook doc in this repo (choose existing ops doc location) | Docs include example query + semantics (best-effort, NULL→0). | 1.1 |

### PR Sequence
1. **PR (Upstream DB/sqlc module):** migration creates `issue_token_consumption` view + sqlc query definitions + regenerated code.
2. **PR (This repo):** (optional) model + store method + docs updates; bump dependency on upstream module so the new sqlc query is available.

## Test Plan

### Unit Tests (this repo; only if optional accessor is implemented)
- [ ] `GetTokenConsumptionByIssue` maps sqlc row → `model.IssueTokenConsumption` correctly (sums and counts).
- [ ] `GetTokenConsumptionByIssue` returns `nil` (or a well-defined zero struct—pick one and document) when no row exists.

### Integration Tests
- [ ] In a DB seeded with multiple eval rows for the same `(workspace_id, issue_id)`, verify:
  - `llm_call_count` equals inserted row count
  - token sums match expected `COALESCE` behavior
- [ ] Insert eval rows where `issue_id IS NULL`; verify they do not affect any issue’s totals.

### Failure-mode Tests
- [ ] Rows with `prompt_tokens` NULL and/or `completion_tokens` NULL still contribute to `llm_call_count` and do not break summation.

### Manual Validation (meets Gap #2 success signal)
- Run (via view):
  ```sql
  SELECT *
  FROM issue_token_consumption
  WHERE workspace_id = $1 AND issue_id = $2;
  ```
- Cross-check (raw aggregation):
  ```sql
  SELECT
    COUNT(*) AS llm_call_count,
    SUM(COALESCE(prompt_tokens, 0)) AS prompt_tokens_sum,
    SUM(COALESCE(completion_tokens, 0)) AS completion_tokens_sum,
    SUM(COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0)) AS total_tokens_sum
  FROM llm_evals
  WHERE workspace_id = $1 AND issue_id = $2;
  ```

## Observability + Rollout
- **Logging:** None required; this is a derived aggregate.
- **Metrics:** Not required.
- **Safe deploy:**
  1) apply upstream migration (view creation is additive/non-destructive),
  2) deploy app changes (if any) that reference new sqlc query.
- **Backout plan:** Drop the view (or revert migration) and revert any sqlc/store changes; source `llm_evals` data remains intact.
- **Watch in prod:** Spot-check a few known issues by comparing view output vs raw aggregation query.

## Gotchas / Best Practices
- Token fields are optional (`*int`) on `LLMEval` (Finding F1); always `COALESCE` to 0 in aggregates.
- Keep grouping scoped by `workspace_id` to avoid collisions across workspaces (Assumption #2).
- Use BIGINT for sums/counts to reduce overflow risk.
- This repo does **not** contain the sqlc/migrations; it imports `basegraph.co/relay/core/db/sqlc` (Finding F3, F4). Plan work/PRs accordingly.

---

## Changelog
- Updated spec to reflect confirmed constraints: persistence-only, lifetime totals, SQL query as success signal, best-effort attribution (Gaps #1–#5).
- Replaced previously unverified “Finding” references with verified repo touch points: `model/llm_eval.go`, `store/interfaces.go`, `store/llm_eval.go`, `service/txrunner.go`.
- Made the external-DB-module dependency explicit: migrations/sqlc live in `basegraph.co/relay/core/db/sqlc` (imported, not in this repo), so DB/view + sqlc query changes must land upstream.
- Clarified PR sequencing (upstream DB/sqlc first, then this repo) and removed ambiguous “repo-specific migration path” placeholders.
- Tightened scenarios around NULL token handling and workspace scoping; added clear “no rows” semantics.
- Expanded test plan to include manual SQL verification plus optional store-level unit/integration tests if we add an accessor method.
