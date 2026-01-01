# Action Executor Implementation Spec

## Overview

The Action Executor receives actions from the Planner and executes them against external systems (issue tracker, database). It handles retries, error classification, and reports failures back for potential Planner re-invocation.

## Design Principles

1. **Context window efficiency**: Error messages must be terse. Every token sent back to Planner matters for reasoning quality.
2. **Provider-agnostic interfaces**: Use `ExternalIssueID` not `IID`. Pass `model.Issue` to let providers extract what they need.
3. **Clear separation**: Validator checks preconditions, Executor performs actions.

---

## Phase 1: Scaffolding (Interfaces & Types)

### 1.1 IssueTrackerService Extension

**File**: `relay/internal/service/issue_tracker/issue_tracker.go`

Add types:
```go
type CreateDiscussionParams struct {
    Issue   model.Issue
    Content string
}

type CreateDiscussionResult struct {
    DiscussionID string // Thread ID
    NoteID       string // Comment ID within thread
}

type ReplyToThreadParams struct {
    Issue        model.Issue
    DiscussionID string // Thread to reply to
    Content      string
}

type ReplyToThreadResult struct {
    NoteID string
}
```

Extend interface:
```go
type IssueTrackerService interface {
    // ... existing methods ...
    CreateDiscussion(ctx context.Context, params CreateDiscussionParams) (*CreateDiscussionResult, error)
    ReplyToThread(ctx context.Context, params ReplyToThreadParams) (*ReplyToThreadResult, error)
}
```

**Status**: [ ] Not started

### 1.2 GitLab Stubs

**File**: `relay/internal/service/issue_tracker/gitlab.go`

Add stub implementations:
```go
func (s *gitLabIssueTrackerService) CreateDiscussion(ctx context.Context, params CreateDiscussionParams) (*CreateDiscussionResult, error) {
    panic("not implemented")
}

func (s *gitLabIssueTrackerService) ReplyToThread(ctx context.Context, params ReplyToThreadParams) (*ReplyToThreadResult, error) {
    panic("not implemented")
}
```

**Status**: [ ] Not started

### 1.3 ActionExecutor

**File**: `relay/internal/brain/action_executor.go` (NEW)

```go
package brain

type actionExecutor struct {
    issueTracker issue_tracker.IssueTrackerService
    issues       store.IssueStore
    gaps         store.GapStore
}

func NewActionExecutor(
    issueTracker issue_tracker.IssueTrackerService,
    issues store.IssueStore,
    gaps store.GapStore,
) ActionExecutor

// Interface implementation
func (e *actionExecutor) Execute(ctx context.Context, issue model.Issue, action Action) error
func (e *actionExecutor) ExecuteBatch(ctx context.Context, issue model.Issue, actions []Action) []ActionError

// Action handlers (private)
func (e *actionExecutor) executePostComment(ctx context.Context, issue model.Issue, action Action) error
func (e *actionExecutor) executeUpdateFindings(ctx context.Context, issue model.Issue, action Action) error
func (e *actionExecutor) executeUpdateGaps(ctx context.Context, issue model.Issue, action Action) error
```

**Status**: [ ] Not started

### 1.4 ActionValidator

**File**: `relay/internal/brain/action_validator.go` (NEW)

```go
package brain

type actionValidator struct {
    gaps   store.GapStore
    issues store.IssueStore
}

func NewActionValidator(
    gaps store.GapStore,
    issues store.IssueStore,
) ActionValidator

func (v *actionValidator) Validate(ctx context.Context, issue model.Issue, input SubmitActionsInput) error

// Per-action validators (private)
func (v *actionValidator) validatePostComment(ctx context.Context, action Action) error
func (v *actionValidator) validateUpdateFindings(ctx context.Context, action Action) error
func (v *actionValidator) validateUpdateGaps(ctx context.Context, issue model.Issue, action Action) error
func (v *actionValidator) validateReadyForPlan(ctx context.Context, issue model.Issue, action Action) error
```

**Status**: [ ] Not started

### 1.5 Add ID to CodeFinding

**File**: `relay/internal/model/issue.go`

```go
type CodeFinding struct {
    ID        string       `json:"id"`        // Snowflake ID for removal reference
    Synthesis string       `json:"synthesis"`
    Sources   []CodeSource `json:"sources"`
}
```

**Status**: [ ] Not started

---

## Phase 2: Action Implementations

### 2.1 PostComment Action

**Files**:
- `relay/internal/service/issue_tracker/gitlab.go` - Implement CreateDiscussion, ReplyToThread
- `relay/internal/brain/action_executor.go` - Implement executePostComment
- `relay/internal/brain/action_validator.go` - Implement validatePostComment

**Validation rules**:
- Content: 1-65000 chars
- ReplyToID: if provided, must be valid discussion ID format

**GitLab API**:
- New thread: `POST /projects/:id/issues/:issue_iid/discussions`
- Reply: `POST /projects/:id/issues/:issue_iid/discussions/:discussion_id/notes`

**Status**: [ ] Not started

### 2.2 UpdateGaps Action

**Files**:
- `relay/internal/brain/action_executor.go` - Implement executeUpdateGaps
- `relay/internal/brain/action_validator.go` - Implement validateUpdateGaps

**Operations**:
- Add: Create new gaps via GapStore.Create()
- Resolve: Call GapStore.Resolve(id)
- Skip: Call GapStore.Skip(id)

**Validation rules**:
- Gap IDs for resolve/skip must exist
- New gaps require: question, severity, respondent

**Status**: [ ] Not started

### 2.3 UpdateFindings Action

**Files**:
- `relay/internal/brain/action_executor.go` - Implement executeUpdateFindings
- `relay/internal/brain/action_validator.go` - Implement validateUpdateFindings

**Operations**:
- Add: Append to issue.CodeFindings (generate Snowflake ID)
- Remove: Filter out by ID

**Constraints**:
- Max 20 findings per issue (truncate oldest if exceeded)

**Validation rules**:
- New findings require synthesis + at least one source

**Status**: [ ] Not started

### 2.4 ReadyForPlan Action

**Files**:
- `relay/internal/brain/action_validator.go` - Implement validateReadyForPlan

**Validation rules**:
- No open blocking gaps allowed
- Must have at least one resolved gap or finding

**Executor behavior**: Pass-through (orchestrator handles SpecGenerator handoff)

**Status**: [ ] Not started

### 2.5 ExecuteBatch with Retry

**File**: `relay/internal/brain/action_executor.go`

**Retry logic**:
- Max 3 attempts per action
- Backoff: 1s, 2s, 4s
- Only retry recoverable errors

**Error classification**:
- Recoverable: 5xx, 429 (rate limit), network timeouts
- Non-recoverable: 4xx (except 429), validation errors, not found

**Status**: [ ] Not started

### 2.6 Orchestrator Integration

**File**: `relay/internal/brain/orchestrator_impl.go`

- Inject ActionExecutor in NewOrchestrator
- After Planner.Plan(), call validator then executor
- On failures, format terse error report and re-invoke Planner (if needed)

**Error format** (context-efficient):
```
FAILED: post_comment | 503 | retryable
FAILED: update_gaps[gap_123] | not found | permanent
```

**Status**: [ ] Not started

---

## Open Decisions

1. **ExternalIssueID parsing**: GitLab needs project ID + issue IID. Options:
   - Store project ID on Issue model
   - Derive from integration config
   - Parse from ExternalIssueID if formatted as "project:issue"
   
   **Decision**: TBD during Phase 2.1

2. **Re-invoke Planner on failure**: How many times? What's the circuit breaker?
   
   **Decision**: TBD during Phase 2.6

---

## Progress Tracker

| Phase | Item | Status |
|-------|------|--------|
| 1.1 | IssueTrackerService types | [x] |
| 1.2 | GitLab stubs | [x] |
| 1.3 | ActionExecutor scaffold | [x] |
| 1.4 | ActionValidator full impl | [x] |
| 1.5 | CodeFinding ID field | [x] |
| 2.1 | PostComment impl (GitLab CreateDiscussion/ReplyToThread + executePostComment) | [x] |
| 2.2 | UpdateGaps impl (executeUpdateGaps) | [x] |
| 2.3 | UpdateFindings impl (executeUpdateFindings) | [x] |
| 2.4 | ReadyForPlan impl | [x] Validation only |
| 2.5 | ExecuteBatch retry | [ ] Basic impl, no retry yet |
| 2.6 | Orchestrator integration | [x] |
| 2.7 | ExternalProjectID population | [x] |

## Implementation Notes

### Completed
- **Phase 1**: All scaffolding complete
- **Phase 2.1**: GitLab `CreateDiscussion` and `ReplyToThread` implemented with proper error wrapping and nil checks
- **Phase 2.2**: `executeUpdateGaps` handles Add/Resolve/Skip operations with type mapping between brain and model packages
- **Phase 2.3**: `executeUpdateFindings` generates Snowflake IDs for new findings, handles removal by ID, caps at 20 findings
- **Phase 2.4**: `validateReadyForPlan` checks for blocking gaps and requires resolved context
- **Phase 2.6**: Orchestrator wired with ActionExecutor, issue tracker map pattern, executes actions after Planner
- **Phase 2.7**: `ExternalProjectID` populated in webhook handler for new issues
- **Code restructure**: Removed `orchestrator_impl.go`, moved action interfaces to `action.go`, eliminated unnecessary interface

### Remaining
- **ExecuteBatch retry logic**: Currently returns errors but doesn't retry with backoff (1s, 2s, 4s)
- **Error classification**: Need to distinguish recoverable (5xx, 429) from non-recoverable (4xx) errors
- **Failure reporting to Planner**: Spec describes XML format for re-invoking planner on failures - not implemented
