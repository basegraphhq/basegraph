# Token Consumption (Track token spend per issue)

**Issue:** (internal) | **Complexity:** L3 | **Author:** Relay | **Reviewers:** TBD

## TL;DR
- Persist **per-LLM-call token usage** (prompt+completion) into DB tied to `IssueID`, then aggregate to a lifetime per-issue total.
- Add new Relay public API endpoint `GET /api/v1/issues/:id/token-usage` returning `{ "total_tokens": int }`.
- Use **API-key auth** (same rules as dashboard); return **404 when no data exists** for that issue.
- Instrument LLM calls by **decorating `llm.AgentClient`** so we don’t need to change Planner/Explore signatures; attribution comes from `logger.LogFields` already attached to `context.Context`.
- Validate with unit tests around attribution + aggregation + 404, and integration test hitting the new endpoint.

## What We're Building
We currently **log** token usage (prompt/completion/total) during Planner/Explore runs but **do not persist** anything that can be queried per issue.

- `brain/planner.go` defers a `slog.InfoContext` with `total_prompt_tokens`, `total_completion_tokens`, and `total_tokens`.
- `brain/explore_agent.go` does the same for explore sessions.
- A DB-backed model exists: `model/llm_eval.go` includes `IssueID` and token fields; `store/llm_eval.go` supports `Create()` → `InsertLLMEval`, but **no call sites exist** (Finding F1).
- There is **no `/api/v1/issues` surface** today; routing is wired in `internal/http/router/router.go` only for users/orgs/gitlab (Finding F3).

We will:
1) Start writing `LLMEval` rows for every LLM call with `IssueID` attribution.
2) Expose an API endpoint that returns lifetime cumulative `total_tokens` per issue.

### Resolved Gaps (inlined)
- **Gap #1: "Response shape OK as just `{ \"total_tokens\": <int> }` and should `0` be returned when there’s no data yet vs `404`/`null`?" → "Return 404 when no data yet; response is just total tokens."**
- **Gap #2: "Authorization rule: who is allowed to read an issue’s token usage?" → "Using API key; same like dashboard."**
- **Gap #3: "Where should this new token-usage endpoint live: public Relay HTTP API vs internal?" → "Relay API (public HTTP API)."**
- **Gap #4: "What breakdown is required?" → "Just total tokens for now."**
- **Gap #5: "Include tokens from failed/retried calls?" → "Include failed/retried calls."**
- **Gap #7: "Lifetime cumulative per issue vs per run?" → "Lifetime cumulative per issue."**
- **Gap #8: "Include all stages?" → "All stages for an issue."**

## Code Changes

### Current State

#### `/api/v1` routes do not include issues
**internal/http/router/router.go:25-35**
```go
v1 := router.Group("/api/v1")
{
    userHandler := handler.NewUserHandler(services.Users())
    UserRouter(v1.Group("/users"), userHandler)

    orgHandler := handler.NewOrganizationHandler(services.Organizations())
    OrganizationRouter(v1.Group("/organizations"), orgHandler)

    gitlabHandler := handler.NewGitLabHandler(services.GitLab(), services.WebhookBaseURL())
    GitLabRouter(v1.Group("/integrations/gitlab"), gitlabHandler)
}
```

#### Auth middleware is session-cookie-based; no API-key middleware
**internal/http/middleware/auth.go:17-48**
```go
const (
    sessionCookieName = "relay_session"
)

func RequireAuth(authService service.AuthService) gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionID, err := getSessionID(c)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
            return
        }
        ...
        c.Next()
    }
}
```

#### LLM calls accumulate tokens from ChatWithTools response; no persistence
**brain/planner.go:113-125**
```go
resp, err := p.llm.ChatWithTools(ctx, llm.AgentRequest{
    Messages: messages,
    Tools:    p.tools(),
})
...
// Track token usage
totalPromptTokens += resp.PromptTokens
totalCompletionTokens += resp.CompletionTokens
```

**brain/explore_agent.go:126-138**
```go
resp, err := e.llm.ChatWithTools(ctx, llm.AgentRequest{
    Messages: messages,
    Tools:    tools,
})
...
totalPromptTokens += resp.PromptTokens
totalCompletionTokens += resp.CompletionTokens
```

#### Context already includes IssueID/WorkspaceID/IntegrationID for attribution
**brain/orchestrator.go:94-115**
```go
ctx = logger.WithLogFields(ctx, logger.LogFields{ IssueID: &input.IssueID, ... })
...
ctx = logger.WithLogFields(ctx, logger.LogFields{ IntegrationID: &issue.IntegrationID })
```

### Proposed Changes

> Note: file paths below use the repo’s existing conventions (`internal/http/...`, `store/...`, `model/...`). If your repo uses `http/...` instead of `internal/http/...` in practice, adjust accordingly.

#### 1) Add a persisted token usage record per LLM call (LLMEval)

**File: `service/token_usage_recorder.go` (new)**
```go
package service

import (
    "context"
    "log/slog"
    "time"

    "basegraph.co/relay/internal/logger"
    "basegraph.co/relay/model"
    "basegraph.co/relay/store"
)

type TokenUsageRecorder interface {
    RecordLLMCall(ctx context.Context, stage string, promptTokens, completionTokens int, err error)
}

type tokenUsageRecorder struct {
    llmEvals store.LLMEvalStore
}

func NewTokenUsageRecorder(llmEvals store.LLMEvalStore) TokenUsageRecorder {
    return &tokenUsageRecorder{llmEvals: llmEvals}
}

func (r *tokenUsageRecorder) RecordLLMCall(ctx context.Context, stage string, promptTokens, completionTokens int, callErr error) {
    fields := logger.GetLogFields(ctx) // must exist; see Gotchas for fallback

    if fields.IssueID == nil {
        // Not issue-scoped; skip.
        return
    }

    eval := model.LLMEval{
        IssueID:          fields.IssueID,
        IntegrationID:    fields.IntegrationID,
        WorkspaceID:      fields.WorkspaceID,
        Stage:            &stage,
        PromptTokens:     int32(promptTokens),
        CompletionTokens: int32(completionTokens),
        TotalTokens:      int32(promptTokens + completionTokens),
        CreatedAt:        time.Now().UTC(),
    }

    if callErr != nil {
        msg := callErr.Error()
        eval.Error = &msg
    }

    if err := r.llmEvals.Create(ctx, &eval); err != nil {
        // Never fail the request/run because accounting failed.
        slog.WarnContext(ctx, "failed to persist token usage", "err", err)
    }
}
```

**File: `model/llm_eval.go` (update)**

Add `Stage`, `IntegrationID`, `WorkspaceID`, and `Error` if they don’t already exist. (Finding F1 indicates token + IssueID fields exist; stage/error are needed for "all stages" and debugging failed calls.)

Copy-paste-ready struct fields (keep existing fields; insert these if missing):
```go
// model/llm_eval.go

type LLMEval struct {
    ID               string     `db:"id" json:"id"`
    IssueID          *string    `db:"issue_id" json:"issue_id"`
    WorkspaceID      *string    `db:"workspace_id" json:"workspace_id"`
    IntegrationID    *string    `db:"integration_id" json:"integration_id"`

    Stage            *string    `db:"stage" json:"stage"`
    PromptTokens     int32      `db:"prompt_tokens" json:"prompt_tokens"`
    CompletionTokens int32      `db:"completion_tokens" json:"completion_tokens"`
    TotalTokens      int32      `db:"total_tokens" json:"total_tokens"`

    Error            *string    `db:"error" json:"error"`
    CreatedAt        time.Time  `db:"created_at" json:"created_at"`
}
```

**File: `store/llm_eval.go` (update)**

Add aggregation query for per-issue total.
```go
package store

import (
    "context"
    "database/sql"
)

type LLMEvalStore interface {
    Create(ctx context.Context, eval *model.LLMEval) error
    SumTotalTokensByIssueID(ctx context.Context, issueID string) (int64, error)
}

func (s *llmEvalStore) SumTotalTokensByIssueID(ctx context.Context, issueID string) (int64, error) {
    var total sql.NullInt64
    err := s.db.GetContext(ctx, &total, `
        SELECT SUM(total_tokens) AS total
        FROM llm_evals
        WHERE issue_id = $1
    `, issueID)
    if err != nil {
        return 0, err
    }
    if !total.Valid {
        return 0, sql.ErrNoRows
    }
    return total.Int64, nil
}
```

> If your DB driver returns one row with NULL for `SUM` instead of no rows: treat NULL as "no data". The handler will map it to 404.

**DB migration: `migrations/xxxx_add_llm_eval_fields.sql` (new)**
```sql
-- +migrate Up
ALTER TABLE llm_evals
    ADD COLUMN IF NOT EXISTS workspace_id TEXT,
    ADD COLUMN IF NOT EXISTS integration_id TEXT,
    ADD COLUMN IF NOT EXISTS stage TEXT,
    ADD COLUMN IF NOT EXISTS total_tokens INT,
    ADD COLUMN IF NOT EXISTS error TEXT;

-- Backfill total_tokens if prompt/completion already exist
UPDATE llm_evals
SET total_tokens = COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0)
WHERE total_tokens IS NULL;

-- +migrate Down
ALTER TABLE llm_evals
    DROP COLUMN IF EXISTS workspace_id,
    DROP COLUMN IF EXISTS integration_id,
    DROP COLUMN IF EXISTS stage,
    DROP COLUMN IF EXISTS total_tokens,
    DROP COLUMN IF EXISTS error;
```

#### 2) Instrument all LLM calls (planner/explore/any future stage) with a decorator

**File: `internal/llm/instrumented_agent_client.go` (new)**
```go
package llm

import (
    "context"

    commonllm "basegraph.co/relay/common/llm"
    "basegraph.co/relay/service"
)

type InstrumentedAgentClient struct {
    inner    commonllm.AgentClient
    recorder service.TokenUsageRecorder
    stage    string
}

func NewInstrumentedAgentClient(inner commonllm.AgentClient, recorder service.TokenUsageRecorder, stage string) commonllm.AgentClient {
    return &InstrumentedAgentClient{inner: inner, recorder: recorder, stage: stage}
}

func (c *InstrumentedAgentClient) ChatWithTools(ctx context.Context, req commonllm.AgentRequest) (commonllm.AgentResponse, error) {
    resp, err := c.inner.ChatWithTools(ctx, req)

    // Record even on error; include what we have.
    c.recorder.RecordLLMCall(ctx, c.stage, resp.PromptTokens, resp.CompletionTokens, err)

    return resp, err
}
```

**File: `brain/planner.go` (update)**

Wherever `Planner` is constructed, wrap its `llm.AgentClient` with stage `"planner"`. If the constructor is in this file, update it; if it’s elsewhere, do it at the wiring site (see Implementation step #2).

Example constructor change (adjust to your actual constructor signature):
```go
// BEFORE
func NewPlanner(llmClient llm.AgentClient, ...) *Planner {
    return &Planner{llm: llmClient, ...}
}

// AFTER
func NewPlanner(llmClient llm.AgentClient, recorder service.TokenUsageRecorder, ...) *Planner {
    return &Planner{llm: internalllm.NewInstrumentedAgentClient(llmClient, recorder, "planner"), ...}
}
```

**File: `brain/explore_agent.go` (update)**
Same pattern with stage `"explore"`.

```go
func NewExploreAgent(llmClient llm.AgentClient, recorder service.TokenUsageRecorder, ...) *ExploreAgent {
    return &ExploreAgent{llm: internalllm.NewInstrumentedAgentClient(llmClient, recorder, "explore"), ...}
}
```

> Add additional stages similarly (e.g., orchestrator-level summarizers) by wrapping with the correct stage name.

#### 3) Add API-key auth middleware (consistent with dashboard) for the new endpoint

Because there is **no existing API-key middleware** (Current State snippet from `internal/http/middleware/auth.go` is session-cookie), we add a new middleware that:
- Reads `Authorization: Bearer <api_key>` OR `X-API-Key: <api_key>`.
- Validates it using the same backing store/service the dashboard uses.

**File: `internal/http/middleware/api_key_auth.go` (new)**
```go
package middleware

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"

    "basegraph.co/relay/service"
)

type apiKeyContextKey string

const apiKeyOrgIDKey apiKeyContextKey = "api_key_org_id"

func RequireAPIKey(authz service.APIKeyAuthzService) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.GetHeader("X-API-Key")
        if key == "" {
            auth := c.GetHeader("Authorization")
            if strings.HasPrefix(auth, "Bearer ") {
                key = strings.TrimPrefix(auth, "Bearer ")
            }
        }

        if key == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing api key"})
            return
        }

        principal, err := authz.AuthorizeAPIKey(c.Request.Context(), key)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
            return
        }

        // Make principal available to handlers (org/workspace scoping).
        c.Set(string(apiKeyOrgIDKey), principal.OrganizationID)
        c.Next()
    }
}
```

**File: `service/api_key_authz.go` (new)**
```go
package service

import "context"

type APIKeyPrincipal struct {
    OrganizationID string
}

type APIKeyAuthzService interface {
    AuthorizeAPIKey(ctx context.Context, apiKey string) (*APIKeyPrincipal, error)
}

// NOTE: Wire this to the same implementation the dashboard uses.
// If none exists in this repo, implement it here backed by the API keys table.
```

> This spec intentionally leaves the backing store wiring explicit (see Assumptions) because the findings show no existing API-key middleware. The teammate implementing should locate the dashboard API-key verification logic and reuse it.

#### 4) Add Issues token-usage endpoint

**File: `internal/http/router/issues.go` (new)**
```go
package router

import (
    "github.com/gin-gonic/gin"

    "basegraph.co/relay/internal/http/handler"
)

func IssuesRouter(rg *gin.RouterGroup, h *handler.IssuesHandler) {
    rg.GET("/:id/token-usage", h.GetTokenUsage)
}
```

**File: `internal/http/handler/issues.go` (new)**
```go
package handler

import (
    "database/sql"
    "net/http"

    "github.com/gin-gonic/gin"

    "basegraph.co/relay/service"
)

type IssuesHandler struct {
    issues service.IssuesService
}

func NewIssuesHandler(issues service.IssuesService) *IssuesHandler {
    return &IssuesHandler{issues: issues}
}

type tokenUsageResponse struct {
    TotalTokens int64 `json:"total_tokens"`
}

func (h *IssuesHandler) GetTokenUsage(c *gin.Context) {
    issueID := c.Param("id")

    total, err := h.issues.GetTotalTokens(c.Request.Context(), issueID)
    if err != nil {
        if err == sql.ErrNoRows {
            c.JSON(http.StatusNotFound, gin.H{"error": "no token usage"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
        return
    }

    c.JSON(http.StatusOK, tokenUsageResponse{TotalTokens: total})
}
```

**File: `service/issues.go` (new or update existing service)**
```go
package service

import (
    "context"
    "database/sql"

    "basegraph.co/relay/store"
)

type IssuesService interface {
    GetTotalTokens(ctx context.Context, issueID string) (int64, error)
}

type issuesService struct {
    llmEvals store.LLMEvalStore
    // TODO: add IssueStore lookup for authz/scoping.
}

func NewIssuesService(llmEvals store.LLMEvalStore) IssuesService {
    return &issuesService{llmEvals: llmEvals}
}

func (s *issuesService) GetTotalTokens(ctx context.Context, issueID string) (int64, error) {
    total, err := s.llmEvals.SumTotalTokensByIssueID(ctx, issueID)
    if err != nil {
        return 0, err
    }
    if total == 0 {
        // NOTE: requirement is 404 when no data; zero may be valid if we persisted zeros.
        // We only return 404 when there are no rows/NULL SUM (handled in store).
        // If you can’t distinguish, remove this block.
        return 0, sql.ErrNoRows
    }
    return total, nil
}
```

**File: `internal/http/router/router.go` (update)**

Add issues handler/router and apply API-key auth middleware to it.
```go
// internal/http/router/router.go

v1 := router.Group("/api/v1")
{
    ... existing routers ...

    // Issues (API-key auth)
    issuesHandler := handler.NewIssuesHandler(services.Issues())
    issuesGroup := v1.Group("/issues")
    issuesGroup.Use(middleware.RequireAPIKey(services.APIKeyAuthz()))
    IssuesRouter(issuesGroup, issuesHandler)
}
```

#### 5) Wire services/stores

**File: `service/services.go` (or wherever `services.*()` accessors are defined) (update)**

Add:
- `Stores.LLMEvals()` accessor if missing (Finding F1 indicates none today)
- `services.TokenUsageRecorder()`
- `services.Issues()`
- `services.APIKeyAuthz()`

Copy-paste example (adapt to actual structure):
```go
func (s *Services) TokenUsageRecorder() service.TokenUsageRecorder {
    return service.NewTokenUsageRecorder(s.Stores.LLMEvals())
}

func (s *Services) Issues() service.IssuesService {
    return service.NewIssuesService(s.Stores.LLMEvals())
}
```

### Key Types/Interfaces
- `service.TokenUsageRecorder`: single responsibility: persist one call’s token usage, never fail the caller.
- `store.LLMEvalStore.SumTotalTokensByIssueID`: aggregates lifetime total.
- `service.APIKeyAuthzService`: validates API key (same rules as dashboard).

## Implementation
| # | Task | File | Done When | Blocked By |
|---|------|------|-----------|------------|
| 1 | Add DB fields + migration for per-call usage rows (`stage`, `total_tokens`, `error`, `workspace_id`, `integration_id`) | `migrations/xxxx_add_llm_eval_fields.sql`, `model/llm_eval.go` | Migration applies cleanly; model compiles; existing rows backfilled with `total_tokens` | - |
| 2 | Implement `TokenUsageRecorder` and `InstrumentedAgentClient` decorator | `service/token_usage_recorder.go`, `internal/llm/instrumented_agent_client.go` | Any `ChatWithTools` call results in an `llm_evals` row when `IssueID` is present on ctx; failures don’t break flow | - |
| 3 | Wire the instrumented client into Planner/Explore construction with explicit stage names (`planner`, `explore`) | `brain/planner.go`, `brain/explore_agent.go`, plus whichever file constructs them | Planner + Explore produce `llm_evals.stage` values correctly | 2 |
| 4 | Add store aggregation method `SumTotalTokensByIssueID` | `store/llm_eval.go` | Returns SUM for issue; returns `sql.ErrNoRows` (or equivalent) when no data | 1 |
| 5 | Implement Issues service method `GetTotalTokens` using LLMEvals store | `service/issues.go` | Unit test passes for SUM and 404 behavior | 4 |
| 6 | Implement API-key middleware + service interface and wire to existing dashboard logic | `internal/http/middleware/api_key_auth.go`, `service/api_key_authz.go`, service wiring | Endpoint rejects missing/invalid key; accepts valid key | - |
| 7 | Add `/api/v1/issues/:id/token-usage` route, handler, and router; apply API-key middleware | `internal/http/router/issues.go`, `internal/http/handler/issues.go`, `internal/http/router/router.go` | `GET /api/v1/issues/<id>/token-usage` returns `{total_tokens}` or 404 | 5, 6 |
| 8 | Add metrics/logging around persistence failures and endpoint usage | `service/token_usage_recorder.go`, handler | Warnings emitted on persist failures; request logs include issueID | - |

## Tests

### Unit

1) **Token recorder persists with IssueID + stage**
- GIVEN: ctx with `logger.WithLogFields(ctx, logger.LogFields{IssueID: ptr("ISSUE_1"), WorkspaceID: ptr("WS_1"), IntegrationID: ptr("INT_1")})`
- WHEN: `RecordLLMCall(ctx, "planner", 10, 5, nil)`
- THEN: store `Create()` called with `IssueID=ISSUE_1`, `Stage="planner"`, `TotalTokens=15`

Fixture + test (use gomock/testify as used in repo; example uses a minimal fake):

**File: `service/token_usage_recorder_test.go` (new)**
```go
package service_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"

    "basegraph.co/relay/internal/logger"
    "basegraph.co/relay/model"
    "basegraph.co/relay/service"
)

type fakeLLMEvalStore struct{ created *model.LLMEval }

func (f *fakeLLMEvalStore) Create(ctx context.Context, eval *model.LLMEval) error {
    f.created = eval
    return nil
}

func TestTokenUsageRecorder_RecordLLMCall_Persists(t *testing.T) {
    st := &fakeLLMEvalStore{}
    rec := service.NewTokenUsageRecorder(st)

    issueID := "ISSUE_1"
    wsID := "WS_1"
    intID := "INT_1"

    ctx := logger.WithLogFields(context.Background(), logger.LogFields{
        IssueID:       &issueID,
        WorkspaceID:   &wsID,
        IntegrationID: &intID,
    })

    rec.RecordLLMCall(ctx, "planner", 10, 5, nil)

    require.NotNil(t, st.created)
    require.NotNil(t, st.created.IssueID)
    require.Equal(t, "ISSUE_1", *st.created.IssueID)
    require.NotNil(t, st.created.Stage)
    require.Equal(t, "planner", *st.created.Stage)
    require.Equal(t, int32(15), st.created.TotalTokens)
}
```

2) **Aggregation returns 404 when no rows**
- GIVEN: store returns `sql.ErrNoRows`
- WHEN: handler calls `GET /api/v1/issues/ISSUE_404/token-usage`
- THEN: HTTP 404

### Integration

3) **Endpoint returns total tokens**
- GIVEN: DB contains two `llm_evals` rows for `issue_id=ISSUE_1` with `total_tokens=15` and `total_tokens=5`
- WHEN: `GET /api/v1/issues/ISSUE_1/token-usage` with valid API key
- THEN: `200` with body `{ "total_tokens": 20 }`

SQL fixture:
```sql
INSERT INTO llm_evals (id, issue_id, total_tokens, prompt_tokens, completion_tokens, created_at)
VALUES
  ('E1', 'ISSUE_1', 15, 10, 5, NOW()),
  ('E2', 'ISSUE_1', 5, 3, 2, NOW());
```

### Edge case

4) **Failed LLM call still records tokens when response contains usage**
- GIVEN: instrumented client inner returns `(resp{PromptTokens:7, CompletionTokens:1}, error("rate limit"))`
- WHEN: `ChatWithTools` is called
- THEN: recorder is invoked with `err!=nil` and `TotalTokens=8`, `Error` set

**File: `internal/llm/instrumented_agent_client_test.go` (new)**
```go
package llm_test

import (
    "context"
    "errors"
    "testing"

    "github.com/stretchr/testify/require"

    commonllm "basegraph.co/relay/common/llm"
    internalllm "basegraph.co/relay/internal/llm"
)

type fakeRecorder struct{ prompt, completion int; gotErr bool }
func (r *fakeRecorder) RecordLLMCall(ctx context.Context, stage string, p, c int, err error) {
    r.prompt, r.completion = p, c
    r.gotErr = err != nil
}

type fakeAgentClient struct{}
func (f *fakeAgentClient) ChatWithTools(ctx context.Context, req commonllm.AgentRequest) (commonllm.AgentResponse, error) {
    return commonllm.AgentResponse{PromptTokens: 7, CompletionTokens: 1}, errors.New("rate limit")
}

func TestInstrumentedAgentClient_RecordsOnError(t *testing.T) {
    rec := &fakeRecorder{}
    c := internalllm.NewInstrumentedAgentClient(&fakeAgentClient{}, rec, "planner")

    _, _ = c.ChatWithTools(context.Background(), commonllm.AgentRequest{})

    require.True(t, rec.gotErr)
    require.Equal(t, 7, rec.prompt)
    require.Equal(t, 1, rec.completion)
}
```

## Gotchas
- **No API-key middleware exists today**: current `RequireAuth` uses `relay_session` cookie (`internal/http/middleware/auth.go`). Don’t accidentally ship the endpoint using session auth; it must be API-key auth per Gap #2.
- **Attribution relies on context**: we’re extracting IssueID via `logger.GetLogFields(ctx)` (Finding F4). Ensure all issue runs attach IssueID to context early (orchestrator does: `brain/orchestrator.go:94-115`). If some stage runs without IssueID, token rows will be skipped.
- **Failed/retried accounting limitations**: the decorator only sees one `ChatWithTools` invocation. If the underlying `common/llm` client retries internally and doesn’t expose per-attempt usage, we can’t count intermediate attempts. If this becomes a problem, we must move instrumentation into `basegraph.co/relay/common/llm`.
- **404 vs 0**: requirement is **404 when no data yet** (Gap #1). But a real total can be `0` if we store rows with 0 tokens (unlikely). Prefer distinguishing “no rows / NULL SUM” vs “sum is 0”.
- **Don’t break production on accounting failures**: persistence should be best-effort; errors should be logged/metrics but never fail planning/explore.

## Operations
- **Verify:**
  - Run migrations; execute an issue run; confirm `llm_evals` has rows with `issue_id` populated and `total_tokens > 0`.
  - Call `GET /api/v1/issues/<issue_id>/token-usage` with a valid API key; confirm JSON `{ "total_tokens": <sum> }`.
  - Call same endpoint for an issue with no rows; confirm **404**.
- **Monitor:**
  - Add log-based alert: count of `"failed to persist token usage"` warnings.
  - Add dashboard chart: `llm_evals` inserts/min, and endpoint 404/500 rates.
- **Rollback:**
  - Feature rollback: disable route registration for issues token usage in `internal/http/router/router.go`.
  - Data rollback: migration `Down` drops new columns; safe because feature is additive. (If rows are needed later, avoid dropping; instead keep columns and stop writing.)

## Decisions
| Decision | Why | Trade-offs |
|----------|-----|------------|
| Return `404` when no token data exists | Gap #1: "Response shape OK ... `0` vs `404`?" → "404" | Some clients prefer `0`; they must handle 404 as "not computed yet" |
| Separate endpoint `GET /api/v1/issues/:id/token-usage` returning `{total_tokens}` | Gap #3: "Issue payload vs separate endpoint?" → "separate endpoint" | Extra HTTP call; avoids bloating issue payload and avoids breaking existing issue schemas |
| Persist per-call token usage into `llm_evals` and aggregate by SUM | Finding F1 shows `LLMEval` exists; simplest path to lifetime aggregation | More rows; requires migration and indexing (add index on `issue_id` if table grows) |
| LLM instrumentation via context-attributing decorator around `llm.AgentClient` | Finding F4: IssueID already on ctx; avoids changing Planner/Explore signatures | Won’t see internal retries if common client retries; may undercount in rare cases |
| API-key auth for endpoint | Gap #2: "using api key. same like dashboard" | Requires implementing missing middleware/service; must align with existing dashboard key semantics |

## Alternatives Considered
| Option | Pros | Cons | Why Not |
|--------|------|------|---------|
| Only log tokens (status quo) and build totals from logs | No DB changes | Not queryable/accurate; hard to aggregate; no API | Doesn’t satisfy “API endpoint returns lifetime total” |
| Store per-issue totals in a dedicated `issue_token_usage` table and increment | Fast reads | Requires atomic increments + backfill; harder to include per-call details/debugging | `LLMEval` already exists and supports later breakdowns |
| Instrument inside `basegraph.co/relay/common/llm` | Captures retries accurately | Out-of-repo change; larger blast radius | Start with in-repo decorator; revisit if undercount observed |

## Assumptions
| Assumption | If Wrong |
|------------|----------|
| There is an existing API-key verification mechanism used by the dashboard that we can reuse in this service | If not found, implement `APIKeyAuthzService` backed by DB (create `api_keys` store/model) and coordinate key issuance with dashboard team |
| `logger.GetLogFields(ctx)` exists and can read fields set by `logger.WithLogFields` | If not present, add it to `internal/logger` package (or store fields in context key) and update recorder to safely no-op when missing |
| `llm.AgentResponse` includes `PromptTokens`/`CompletionTokens` even when `err != nil` for provider errors | If response is empty on error, we will record `0`; to satisfy “include failed/retried”, move instrumentation closer to provider layer (common/llm) |
| `llm_evals` table exists today (Finding F1) | If not, create the table in a new migration and adjust `store/llm_eval.go` accordingly |
