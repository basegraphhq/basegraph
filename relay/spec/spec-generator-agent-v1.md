# Spec: Spec Generation Agent (v1, Local Spec Store)

**Status:** Implemented
**Owner:** Relay Brain / Applied AI
**Last updated:** 2026-01-11

---

## Prior Art & Inspiration

This design draws from two leading spec-driven development frameworks:

### OpenSpec (16k+ stars)
- **Two-folder model**: `specs/` (current truth) vs `changes/` (proposed deltas) — keeps state and diffs separate.
- **Delta format**: `## ADDED|MODIFIED|REMOVED Requirements` with mandatory `#### Scenario:` blocks for each requirement.
- **Three-stage workflow**: Proposal → Implementation → Archive.
- **Validation-first**: `openspec validate --strict` before any implementation begins.
- **Key insight**: "Specs are truth. Changes are proposals. Keep them in sync."

### BMAD-METHOD (29k+ stars)
- **Scale-adaptive planning**: Automatically adjusts depth (Level 0–4) based on project complexity.
- **Specialized agents**: 21 domain experts (PM, Architect, Developer, UX, Scrum Master, QA) with distinct prompts.
- **Structured workflows**: 50+ guided workflows across analysis, planning, architecture, implementation.
- **Key insight**: "AI agents act as expert collaborators who guide you through structured workflows to bring out your best thinking."

### What we take forward
1. **Scenarios as acceptance criteria** (OpenSpec): Every requirement must have concrete WHEN/THEN scenarios.
2. **Decision traceability** (both): Decisions tied to evidence (gaps, code findings, discussion).
3. **Scale-adaptive depth** (BMAD): Spec complexity should match issue complexity (bug fix vs new feature vs architectural change).
4. **Validation before implementation** (OpenSpec): Spec must pass structural validation before dev handoff.
5. **Specialized agent separation** (BMAD): Planner ≠ Spec Generator ≠ Explorer — each has optimized prompts.

---

## 0) Summary

We will add a **Spec Generator** stage to Relay's issue workflow. The **Planner** remains the conductor (gaps/learnings/follow-ups), but when a human proceed-signal is present and gaps are resolved, the Planner emits `ready_for_spec_generation`. The orchestrator executes that action by invoking a **separate Spec Generator agent** (fresh context window) which writes/updates a **single canonical spec markdown file** stored locally for v1 (later interchangeable with bucket storage via `SpecStore` interface).

Key principles:

- **Separation of concerns:** Planner = alignment and decision extraction; Spec Generator = polished spec writing.
- **Trackability:** Spec output includes explicit decision log, assumptions, implementation plan, test plan, rollout plan.
- **Context discipline:** ContextBuilder does **not** inject the full spec into Planner's discussion context; Planner can `read_spec` on-demand.
- **Edits happen via actions:** Planner may read spec via tool; spec modifications happen via a validated `update_spec` action (executed by ActionExecutor).
- **Idempotent + safe:** Spec generation should be retry-safe and not spam tracker threads.

---

## 1) Goals / Non-goals

### Goals

1. Generate a spec that is **better than typical "plan mode"** outputs: more grounded, decision-traceable, and execution-ready.
2. Maintain high Planner quality by keeping spec writing out of Planner's context window.
3. Provide a durable v1 "single-document" experience (one canonical spec per issue).
4. Allow iterative spec improvements driven by follow-up discussion, via Planner + `read_spec` + `update_spec`.
5. Persist evaluation artifacts (`llm_evals` stage `spec_generator`) for prompt/product iteration.

### Non-goals (v1)

- Bucket storage (S3/GCS) — design must support it, but implementation can be local only for v1.
- Full bidirectional sync with GitLab/GitHub comments for the entire spec (we may post a short summary/link later).
- Multi-document "spec chapters" or splitting the spec into multiple files (unless forced by infra constraints later).

---

## 2) User Experience

### 2.1 First spec creation

1. Humans discuss an issue in the tracker.
2. Planner asks high-signal questions, tracks gaps, and waits for proceed-signal.
3. Once proceed-signal exists and all gaps are closed (or can be responsibly inferred), Planner emits:
   - `ready_for_spec_generation` action containing context summary + references to evidence (closed gaps, relevant findings).
4. Orchestrator executes that action by:
   - Invoking Spec Generator agent.
   - Persisting spec to canonical local file via `SpecStore`.
   - Updating `issues.spec` to reference the canonical spec location (see "SpecRef").
   - Optionally posting a short tracker comment: "Spec updated. Summary + pointer."

### 2.2 Follow-up edits

1. A developer comments with changes ("please add X / update Y") in the tracker.
2. Planner ingests new discussion events.
3. Planner reads the existing spec via `read_spec` tool if needed.
4. Planner decides whether to:
   - Ask clarifying questions (gaps) and wait for proceed, or
   - Directly update spec under explicit proceed, or
   - Infer minor changes and update spec.
5. Planner emits `update_spec` action (and optionally edits the tracker note if/when supported).

### 2.3 Spec visibility and context-bloat prevention

- ContextBuilder must **not** include the full spec text inside the Planner's conversation context.
- ContextBuilder should include:
  - A short "Spec exists" stub (path/ref + last-updated + 5–15 line excerpt or summary).
  - Leave full retrieval to `read_spec` tool when necessary.

---

## 3) Architecture Overview

### 3.1 Components

| Component                          | Responsibility                                                                 |
| ---------------------------------- | ------------------------------------------------------------------------------ |
| **Planner** (existing)             | Questions, gaps/learnings, proceed gating, emits actions.                      |
| **Orchestrator** (existing)        | Runs planner cycles, validates actions, executes actions.                      |
| **ActionValidator** (existing)     | Validates proposed actions before execution.                                   |
| **ActionExecutor** (existing)      | Performs actions (post comment, close gap, etc.).                              |
| **Spec Generator Agent** (new)     | Dedicated agent invoked by orchestrator; own system prompt + context window.   |
| **SpecStore** (new interface)      | Read/write operations for spec artifacts. v1 local FS; later bucket.           |

### 3.2 Data flow

```
Planner (LLM)
    │
    ▼ emits `ready_for_spec_generation`
Orchestrator
    │
    ▼ executes action
Spec Generator Agent (fresh context)
    │
    ▼ outputs markdown spec + metadata JSON
Orchestrator / ActionExecutor
    │
    ▼ writes markdown via SpecStore
    ▼ updates `issues.spec` with SpecRef
    │
ContextBuilder
    │
    ▼ includes spec pointer/summary in context dump (NOT in discussion history)
```

---

## 4) Data Model / Storage

### 4.1 Canonical spec representation

We define a **SpecRef** serialized as JSON string stored in `issues.spec` (type `text`):

```json
{
  "version": 1,
  "backend": "local",
  "path": "relay_specs/issue_12345_ext_9876_title-slug/spec.md",
  "updated_at": "2026-01-10T12:34:56Z",
  "sha256": "abc123...",
  "format": "markdown"
}
```

Notes:

- Using JSON keeps `issues.spec` forward-compatible with future backends (`s3`, `gcs`, etc.).
- For v1 local, `path` is a relative path under a configured root.

### 4.2 SpecStore interface

```go
// SpecRef identifies a spec artifact.
type SpecRef struct {
    Version   int       `json:"version"`
    Backend   string    `json:"backend"`   // "local", "s3", "gcs"
    Path      string    `json:"path"`
    UpdatedAt time.Time `json:"updated_at"`
    SHA256    string    `json:"sha256"`
    Format    string    `json:"format"`    // "markdown"
}

// SpecMeta contains metadata about the spec.
type SpecMeta struct {
    IssueID         int64
    ExternalIssueID string
    Title           string
    UpdatedAt       time.Time
    SHA256          string
}

// SpecStore provides read/write operations for spec artifacts.
type SpecStore interface {
    Read(ctx context.Context, ref SpecRef) (content string, meta SpecMeta, err error)
    Write(ctx context.Context, issueID int64, externalIssueID string, slug string, content string) (ref SpecRef, err error)
    Exists(ctx context.Context, ref SpecRef) (bool, error)
}
```

### 4.3 v1 implementation: LocalSpecStore

```go
type LocalSpecStore struct {
    RootDir string // e.g., "/var/lib/relay/specs" or "relay_specs/"
}
```

Responsibilities:

- Validate paths (no traversal outside root).
- Atomic writes (write to temp file, rename).
- Compute SHA256 on write.

### 4.4 Local path layout (v1)

One canonical file per issue:

```
{RootDir}/
  issue_{internalID}_{provider}_{externalIssueID}_{slug}/
    spec.md
```

Example:

```
relay_specs/
  issue_12345_gitlab_9876_add-dark-mode-toggle/
    spec.md
```

Optional history (v2, out of scope for v1):

```
relay_specs/
  issue_12345_gitlab_9876_add-dark-mode-toggle/
    spec.md
    history/
      2026-01-10T12-34-56_spec.md
      2026-01-11T09-00-00_spec.md
```

---

## 5) Planner Tooling + Actions

### 5.1 Tool: `read_spec` (Planner-only tool)

**Purpose:** Allow Planner to retrieve the current spec when follow-ups require it, without injecting full spec into every planning context.

**Tool schema:**

```json
{
  "name": "read_spec",
  "description": "Read the current spec for this issue. Use mode='summary' for a quick overview, mode='full' for complete content.",
  "parameters": {
    "type": "object",
    "properties": {
      "mode": {
        "type": "string",
        "enum": ["full", "summary"],
        "default": "summary",
        "description": "Whether to return the full spec or a summary with TOC and excerpt."
      },
      "max_chars": {
        "type": "integer",
        "default": 30000,
        "description": "Maximum characters to return (for full mode). Content is truncated with indicator if exceeded."
      }
    },
    "additionalProperties": false
  }
}
```

**Behavior:**

1. Look up current issue's `issues.spec` column.
2. Parse `SpecRef` from JSON. If missing/empty → return `"No spec exists for this issue."`.
3. Call `SpecStore.Read(ctx, ref)`.
4. Return based on `mode`:
   - `summary`: Header + table of contents (if detectable) + first N chars + last-updated metadata.
   - `full`: Entire file bounded by `max_chars`, with truncation indicator if exceeded.

**Guardrails:**

- Always include `SpecRef` metadata (path, updated_at, sha256) so Planner can confirm it is working with the correct artifact.
- Default to `summary` mode to keep Planner context lean.

### 5.2 Action: `update_spec`

**Purpose:** Deterministic, auditable spec updates. All writes go through ActionExecutor (not tool execution mid-turn).

**ActionType:** `update_spec`

**Action data schema:**

```json
{
  "type": "update_spec",
  "content_markdown": "…full markdown content…",
  "reason": "Follow-up from @alice re: edge case handling",
  "mode": "overwrite"
}
```

**Validation rules (ActionValidator):**

| Rule                        | Constraint                                      |
| --------------------------- | ----------------------------------------------- |
| `content_markdown` required | Non-empty string                                |
| Length limit                | 1 char – 200,000 chars                          |
| `mode`                      | Must be `"overwrite"` (only mode for v1)        |
| `reason`                    | Optional but recommended for audit trail        |

**Execution (ActionExecutor):**

1. Generate canonical path using `issueID`, `provider`, `externalIssueID`, and slug from issue title.
2. Call `SpecStore.Write(ctx, issueID, externalIssueID, slug, content_markdown)`.
3. Receive `SpecRef` with computed `sha256` and `updated_at`.
4. Serialize `SpecRef` to JSON and update `issues.spec` column.
5. Return success.

### 5.3 Action: `ready_for_spec_generation`

**Existing action** that Planner emits when:

- Proceed-signal exists.
- No open gaps remain (or can be closed as inferred per existing rules).
- Sufficient resolved context (closed gaps, relevant findings).

**Action data schema:**

```json
{
  "type": "ready_for_spec_generation",
  "context_summary": "User wants to add a dark mode toggle to settings. Key decisions: use CSS variables, localStorage for persistence.",
  "relevant_finding_ids": [101, 102],
  "closed_gap_ids": [5, 6, 7],
  "learnings_applied": ["Always wrap errors with context"],
  "proceed_signal": "yes, go ahead with the spec"
}
```

> **Implementation Note (2026-01-11):** The `relevant_finding_ids` and `closed_gap_ids` fields are **informational only**. The Planner cannot know real finding IDs at decision time (they're assigned when `update_findings` executes). The ActionExecutor fetches all data directly from stores:
> - Closed gaps: `GapStore.ListClosedByIssue()`
> - Learnings: `LearningStore.ListByWorkspace()`
> - Findings: All findings from `issue.CodeFindings` are marked as core
> - Discussions: Full conversation thread from `issue.Discussions`

**Execution (ActionExecutor):**

1. Assemble `SpecGeneratorInput` from:
   - Issue metadata (title, description, labels, assignees).
   - Proceed-signal excerpt.
   - Full discussion thread (provider-agnostic `ConversationMessage` format).
   - Closed gaps (with question + closed_note) — fetched from store.
   - All code findings (all marked as core).
   - Learnings — fetched from store.
   - Existing spec (if any) via `SpecStore.Read`.
2. Invoke **Spec Generator Agent**.
3. Receive `SpecGeneratorOutput`.
4. Call `SpecStore.Write` with `spec_markdown`.
5. Update `issues.spec` with new `SpecRef`.
6. Write `llm_evals` record (stage `spec_generator`).

---

## 6) Spec Generator Agent

### 6.1 Rationale for separate agent

| Concern                      | Why separate agent                                                        |
| ---------------------------- | ------------------------------------------------------------------------- |
| Context window preservation  | Spec writing is token-heavy; pollutes Planner's alignment loop.           |
| Prompt specialization        | Planner prompt optimized for questioning; spec prompt for polished prose. |
| Quality isolation            | Follows `ExploreAgent` pattern: disposable context preserves quality.     |

### 6.2 Inputs (`SpecGeneratorInput`)

```go
type SpecGeneratorInput struct {
    Issue            IssueSnapshot             // title, description, labels, assignees, URL
    ProceedSignal    string                    // verbatim proceed message
    ContextSummary   string                    // from Planner handoff
    ClosedGaps       []GapSnapshot             // question + closed_reason + closed_note
    RelevantFindings []FindingSnapshot         // synthesis + sources (all marked IsCore=true)
    Learnings        []LearningSnapshot        // workspace-level conventions
    Discussions      []model.ConversationMessage // Full conversation thread (see 6.2.1)
    ExistingSpec     *string                   // current spec content (nil if first creation)
    ExistingSpecRef  *SpecRef                  // metadata of existing spec
    Constraints      SpecConstraints           // max_length, format, etc.
}
```

### 6.2.1 Provider-Agnostic Conversation Model (Implemented 2026-01-11)

Discussions are passed to the Spec Generator in a provider-agnostic format to support future Slack integration:

```go
// ConversationMessage represents a provider-agnostic message in a conversation.
// This abstraction works across GitLab, GitHub, Slack, and future providers.
type ConversationMessage struct {
    Seq          int       // 1, 2, 3... (position in conversation)
    Author       string    // username (normalized across providers)
    Role         string    // reporter | assignee | self | other
    Timestamp    time.Time
    ReplyToSeq   *int      // parent message seq (nil if top-level)
    Content      string    // markdown/text body

    // Relay annotations (computed at handoff via heuristics)
    AnswersGapID *int64    // if this message likely answered a gap
    IsProceed    bool      // if this contains proceed signal
}
```

**Heuristic annotations:**
- `AnswersGapID`: Message author matches gap respondent AND timestamp in `[gap.CreatedAt, gap.ResolvedAt]`
- `IsProceed`: Message content contains the proceed signal text (case-insensitive, first 50 chars)

**XML format in user message:**

```xml
<conversation>
  <msg n="1" author="alice" role="reporter" ts="2026-01-10T10:00:00Z">
We need dark mode for accessibility.
  </msg>

  <msg n="2" author="relay" role="self" ts="2026-01-10T10:20:00Z">
Should dark mode sync across devices or be per-browser?
  </msg>

  <msg n="3" author="alice" role="reporter" ts="2026-01-10T10:30:00Z" reply_to="2" answers_gap="5" is_proceed="true">
Per-browser is fine for v1. Go ahead with the spec.
  </msg>
</conversation>
```

### 6.3 Outputs (`SpecGeneratorOutput`)

```go
type SpecGeneratorOutput struct {
    SpecMarkdown string            // full canonical document
    SpecSummary  string            // 5-10 bullet summary
    Changelog    string            // what changed (empty if first creation)
    Metadata     SpecMetadataJSON  // structured artifact for eval
}

type SpecMetadataJSON struct {
    Sections        []string  // detected section headers
    DecisionCount   int       // number of decisions in decision log
    AssumptionCount int       // number of explicit assumptions
    TaskCount       int       // number of implementation tasks
    TestCaseCount   int       // number of test cases mentioned
    CharCount       int
    SHA256          string
}
```

### 6.4 Spec template ("better than plan mode")

The generated spec must include these sections. **Inspired by OpenSpec's scenario format and BMAD's scale-adaptive depth.**

#### Scale-Adaptive Depth (BMAD-inspired)

| Issue Complexity | Spec Depth | Sections Required |
|------------------|------------|-------------------|
| **L0: Bug fix** | Minimal | TL;DR, Problem, Success Criteria, Implementation Plan |
| **L1: Small feature** | Light | + Goals/Non-goals, Decision Log (1-2 entries) |
| **L2: Medium feature** | Standard | + Assumptions, Design, Test Plan |
| **L3: Large feature** | Full | + Observability, Rollout, Gotchas |
| **L4: Architectural** | Comprehensive | + Migration plan, Risk matrix, Alternatives considered |

Spec Generator should infer complexity from issue metadata + closed gaps count + finding count.

#### Template

```markdown
# Spec: {Issue Title}

**Status:** Draft | In Review | Approved  
**Issue:** {external URL}  
**Last updated:** {timestamp}  
**Complexity:** L0-L4

---

## TL;DR
- {5 bullets: outcome, constraints, biggest risk, rollout shape, validation}

## Problem Statement
{Clear description of what we're solving}

## Success Criteria (OpenSpec-style scenarios)

### Requirement: {Primary capability}
The system SHALL {behavior description}.

#### Scenario: Happy path
- **WHEN** {precondition/action}
- **THEN** {expected outcome}

#### Scenario: Error case
- **WHEN** {error condition}
- **THEN** {graceful handling}

### Requirement: {Secondary capability}
...

## Goals / Non-goals
### Goals
- ...
### Non-goals
- ...

## Decision Log (ADR-lite)
{Each entry tied to a closed gap ID or code finding}

| # | Decision | Context (Gap/Finding) | Consequences |
|---|----------|----------------------|--------------|
| 1 | Use CSS variables for theming | Gap #5: "How should colors be managed?" → "CSS vars preferred" | Enables runtime switching; requires fallback for older browsers |
| 2 | Store preference in localStorage | Finding: existing `useLocalStorage` hook at `src/hooks/useLocalStorage.ts:12` | Consistent with existing patterns; no migration needed |

## Assumptions
{Explicit list with fallback action}

| # | Assumption | If Wrong |
|---|------------|----------|
| 1 | Users have modern browsers (CSS custom properties support) | Add PostCSS fallback in build pipeline |
| 2 | No SSR requirements for theme | Implement `prefers-color-scheme` media query check |

## Design
### API / Data Model
### Flow / Sequence
### Concurrency / Idempotency / Retry behavior

## Implementation Plan
{Ordered checklist, grouped into PRs}

| # | Task | Touch Points | Done When | Blocked By |
|---|------|--------------|-----------|------------|
| 1.1 | Add ThemeContext provider | `src/contexts/ThemeContext.tsx` | Context provides `theme` + `toggleTheme` | - |
| 1.2 | Add CSS variables to root | `src/styles/variables.css` | Light/dark tokens defined | - |
| 2.1 | Update Settings UI | `src/pages/Settings.tsx` | Toggle renders + works | 1.1 |

## Test Plan

### Unit Tests
- [ ] ThemeContext provides default theme
- [ ] toggleTheme switches between light/dark
- [ ] Theme persists across page reload

### Integration Tests
- [ ] Settings page renders toggle
- [ ] Toggle changes body class

### Failure-mode Tests
- [ ] Graceful fallback when localStorage unavailable
- [ ] No flash of unstyled content on initial load

## Observability + Rollout
- **Logging**: `theme_changed` event with `from`/`to` values
- **Metrics**: Track dark mode adoption rate
- **Safe deploy**: Feature flag `ENABLE_DARK_MODE` (default: false)
- **Backout plan**: Disable feature flag; no migration needed
- **Watch in prod**: Error rate on theme switch, CSP violations

## Gotchas / Best Practices
{From learnings, code conventions, CLAUDE.md}

- Always wrap errors with context (`fmt.Errorf("doing X: %w", err)`)
- Use `common/id.New()` for any new IDs
- Run `make format` before committing

---

## Changelog
{For revisions only; empty on first creation}
- 2026-01-10: Initial spec created from Gap #5, #6, #7; Findings #101, #102
- 2026-01-11: Added edge case for offline mode per @alice feedback (Gap #8)
```

### 6.5 System prompt (summary)

The Spec Generator system prompt should:

1. Emphasize **decision traceability**: every choice must cite evidence (gap answer or code finding).
2. Require **executable specificity**: file paths, function names, PR sequence.
3. Include repo conventions from learnings and `CLAUDE.md`.
4. Instruct: "If existing spec provided, produce a revised spec with a Changelog section."
5. Enforce output structure via tool call (`submit_spec`).

### 6.6 Tool: `submit_spec`

Spec Generator has exactly one tool to enforce structured output:

```json
{
  "name": "submit_spec",
  "description": "Submit the final spec. Call this exactly once when the spec is complete.",
  "parameters": {
    "type": "object",
    "properties": {
      "spec_markdown": { "type": "string", "description": "Full spec in markdown format." },
      "spec_summary": { "type": "string", "description": "5-10 bullet summary." },
      "changelog": { "type": "string", "description": "What changed from previous version (empty if new)." }
    },
    "required": ["spec_markdown", "spec_summary"],
    "additionalProperties": false
  }
}
```

### 6.7 Exploration budget

Spec Generator may optionally call `ExploreAgent` for **1–2 quick lookups** if it needs to verify exact symbol names, file paths, or config keys not provided in inputs. This is a safety valve, not the primary path.

> **Implementation Note (2026-01-11):** The explore limit is enforced at **2 calls maximum**. After the limit is reached, subsequent explore calls receive:
> ```
> "Explore limit reached (max 2 calls). Please call submit_spec with your best effort based on the context provided."
> ```
> This prevents the Spec Generator from burning all retry attempts on exploration instead of producing a spec.

### 6.8 Spec Validation (OpenSpec-inspired)

Before persisting, the spec must pass structural validation. This ensures quality and catches common issues early.

**Validation rules:**

| Rule | Severity | Description |
|------|----------|-------------|
| `has_tldr` | Error | TL;DR section must exist with 3-7 bullets |
| `has_problem` | Error | Problem Statement section must exist |
| `has_scenarios` | Error | At least one `#### Scenario:` block required |
| `scenario_format` | Error | Scenarios must have WHEN/THEN structure |
| `has_decision_log` | Warning | Decision Log recommended (required for L2+) |
| `decisions_have_context` | Warning | Each decision should cite a Gap ID or Finding |
| `has_implementation_plan` | Error | Implementation Plan with at least one task |
| `tasks_have_touch_points` | Warning | Tasks should specify files/packages affected |
| `no_orphan_assumptions` | Warning | Each assumption should have "If Wrong" fallback |

**Validation flow:**

```go
func (s *SpecGenerator) validateSpec(markdown string, complexity int) []ValidationError {
    errors := []ValidationError{}
    
    // Parse markdown into sections
    sections := parseMarkdownSections(markdown)
    
    // Required sections
    if !sections.Has("TL;DR") {
        errors = append(errors, ValidationError{Rule: "has_tldr", Severity: "error"})
    }
    
    // Scenario validation (OpenSpec-style)
    scenarios := extractScenarios(markdown)
    if len(scenarios) == 0 {
        errors = append(errors, ValidationError{Rule: "has_scenarios", Severity: "error"})
    }
    for _, s := range scenarios {
        if !hasWhenThen(s) {
            errors = append(errors, ValidationError{
                Rule: "scenario_format",
                Severity: "error",
                Detail: fmt.Sprintf("Scenario '%s' missing WHEN/THEN", s.Name),
            })
        }
    }
    
    // Complexity-dependent rules
    if complexity >= 2 && !sections.Has("Decision Log") {
        errors = append(errors, ValidationError{Rule: "has_decision_log", Severity: "error"})
    }
    
    return errors
}
```

**Error recovery:**

If validation fails, Spec Generator should:
1. Log the validation errors.
2. Attempt self-correction (re-prompt with specific fixes).
3. If still failing after 2 attempts, persist anyway with `validation_status: "partial"` in metadata.

---

## 7) ContextBuilder Changes

### 7.1 Do NOT inject spec into discussion history

ContextBuilder must filter out / not append spec content as assistant messages. The spec is an artifact, not a conversation turn.

### 7.2 Include spec stub in context dump

Add a "Current Spec" section to the Planner context:

```markdown
## Current Spec

- **Path:** relay_specs/issue_12345_gitlab_9876_add-dark-mode/spec.md
- **Last updated:** 2026-01-10T12:34:56Z
- **SHA256:** abc123...

### Summary (excerpt)
> TL;DR:
> - Add dark mode toggle to settings
> - Use CSS variables + localStorage
> ...

Use `read_spec` tool for full content.
```

### 7.3 Implementation location

Modify `BuildPlannerMessages` in `relay/internal/brain/context_builder.go` to:

1. Check if `issue.Spec` is non-empty.
2. Parse `SpecRef`.
3. Read summary via `SpecStore.Read` (or inline summary extraction).
4. Append spec stub section to context dump.

---

## 8) Observability + Evaluation

### 8.1 llm_evals record

Write an `llm_evals` record for each spec generation:

| Field          | Value                                                      |
| -------------- | ---------------------------------------------------------- |
| `stage`        | `"spec_generator"`                                         |
| `issue_id`     | Issue ID                                                   |
| `input_text`   | Serialized `SpecGeneratorInput` (truncate large fields)    |
| `output_json`  | `SpecGeneratorOutput` (or hash + size if too large)        |
| `model`        | Model used                                                 |
| `latency_ms`   | Generation time                                            |
| `input_tokens` | (if available)                                             |
| `output_tokens`| (if available)                                             |

### 8.2 Structured logging

Add `slog` entries for:

- Spec generation start: `issue_id`, `integration_id`, `existing_spec_exists`
- Spec generation end: `issue_id`, `duration_ms`, `spec_chars`, `sha256`
- SpecStore read/write: `ref.path`, `bytes`, `duration_ms`
- Idempotency skip: `issue_id`, `reason` (e.g., "spec unchanged")

---

## 9) Idempotency + Retry Safety

| Scenario                                      | Behavior                                         |
| --------------------------------------------- | ------------------------------------------------ |
| `ready_for_spec_generation` retried           | Regenerate spec; if content hash identical → log + no-op on SpecStore write |
| `update_spec` retried                         | Atomic overwrite; safe to re-run                 |
| Worker crash mid-write                        | Temp file not renamed; next run regenerates      |
| Spec content unchanged on regeneration        | Compare SHA256; skip write + update if identical |

---

## 10) Security / Guardrails

### 10.1 LocalSpecStore

- **Allowed root:** All paths must be under configured `RootDir`.
- **Path traversal:** Reject any path containing `..` or absolute paths outside root.
- **Atomic writes:** Write to `{path}.tmp`, then `os.Rename`.
- **Permissions:** Files created with `0644`.

### 10.2 Size limits

| Limit                    | Value       | Rationale                              |
| ------------------------ | ----------- | -------------------------------------- |
| Max spec size (write)    | 200 KB      | Prevent disk abuse                     |
| Max spec size (read)     | 200 KB      | Prevent context explosion              |
| `read_spec` default      | 30,000 char | Keep Planner context reasonable        |

### 10.3 Input sanitization

- Slugs generated from issue titles must be sanitized (alphanumeric + hyphens only).
- Issue IDs are integers; external IDs are sanitized strings.

---

## 11) Implementation Checklist

### Phase 1: Storage + Plumbing

- [ ] Define `SpecRef` struct in `relay/internal/model/spec.go`
- [ ] Define `SpecStore` interface in `relay/internal/store/spec_store.go`
- [ ] Implement `LocalSpecStore` with:
  - [ ] `Read(ctx, ref)` with path validation
  - [ ] `Write(ctx, issueID, extID, slug, content)` with atomic write + SHA256
  - [ ] `Exists(ctx, ref)`
- [ ] Add config for spec root directory
- [ ] Unit tests for `LocalSpecStore` (path validation, atomic write, SHA256)

### Phase 2: Planner Tool + Actions

- [ ] Add `read_spec` tool definition in `relay/internal/brain/planner_tools.go`
- [ ] Implement `executeReadSpec` in `relay/internal/brain/planner_tools.go`
- [ ] Add `update_spec` action type in `relay/internal/brain/action.go`
- [ ] Add validation for `update_spec` in `relay/internal/brain/action_validator.go`
- [ ] Implement `executeUpdateSpec` in `relay/internal/brain/action_executor.go`
- [ ] Unit tests for `read_spec` tool
- [ ] Unit tests for `update_spec` validation + execution

### Phase 3: Spec Generator Agent

- [ ] Create `relay/internal/brain/spec_generator.go`:
  - [ ] `SpecGeneratorInput` / `SpecGeneratorOutput` structs
  - [ ] `SpecGenerator` struct with `Generate(ctx, input)` method
  - [ ] System prompt (embed or load from file)
  - [ ] `submit_spec` tool definition + parsing
- [ ] Wire `executeReadyForSpecGeneration` to invoke `SpecGenerator`
- [ ] Persist spec via `SpecStore.Write`
- [ ] Update `issues.spec` with `SpecRef` JSON
- [ ] Write `llm_evals` record

### Phase 4: Context Discipline

- [ ] Modify `ContextBuilder.BuildPlannerMessages` to:
  - [ ] NOT include spec in discussion history
  - [ ] Include spec stub section (path, updated_at, summary excerpt)
- [ ] Add helper to extract spec summary (first N chars or TL;DR section)
- [ ] Unit tests for context builder spec handling

### Phase 5: Eval + QA

- [ ] Integration test: end-to-end `ready_for_spec_generation` → spec file created
- [ ] Integration test: `update_spec` action → spec file updated
- [ ] Integration test: `read_spec` returns correct content
- [ ] Manual QA: verify spec quality on sample issues
- [ ] Review `llm_evals` output for spec_generator stage

---

## 12) Open Questions (Resolved)

| Question                                                       | Status      | Resolution |
| -------------------------------------------------------------- | ----------- | ---------- |
| Spec root directory location (`relay_specs/` vs `/var/lib/...`)| **Resolved** | `relay_specs/` at repo root; configurable via env var for production |
| Post summary comment to tracker on spec creation?              | **Resolved** | No tracker comment for v1. Generate spec file only. Evaluate quality first, then add tracker integration later. |
| Max spec length (200KB reasonable?)                            | **Resolved** | 200KB is fine |
| Should `read_spec` support section filtering (e.g., "just Implementation Plan")? | **Deferred v2** | Keep v1 simple with `summary` vs `full` modes |
| Complexity inference (L0-L4) — auto-detect or explicit?        | **Resolved** | Both: Planner passes `complexity_hint`, Spec Generator can override based on evidence |

---

## 13) Implementation Notes (2026-01-11)

### 13.1 Provider Integration Fixes

**GitLab Pagination:**
- `FetchDiscussions` now paginates through all pages (100 per page)
- Previously only fetched first page, causing discussion truncation on active issues
- Added observability: logs thread count, note count, and pages fetched

```go
opts := &gitlab.ListIssueDiscussionsOptions{
    ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
}
for {
    discussions, resp, _ := client.Discussions.ListIssueDiscussions(...)
    allDiscussions = append(allDiscussions, discussions...)
    if resp.NextPage == 0 { break }
    opts.ListOptions.Page = int64(resp.NextPage)
}
```

**Strict Fetch Mode:**
- Discussion fetch errors now return retryable errors instead of silently clobbering existing data
- Previously: fetch error → log warning → overwrite `issue.Discussions` with nil
- Now: fetch error → return error → event retried → existing discussions preserved

### 13.2 Files Modified

| File | Changes |
|------|---------|
| `model/conversation.go` | NEW: Provider-agnostic `ConversationMessage` struct |
| `brain/spec_generator.go` | Added Discussions to input, XML formatting, explore limit (2 max), all findings = core |
| `brain/action_executor.go` | Added `configs` store, `getRelayUsername()`, passes discussions to SpecGen |
| `brain/orchestrator.go` | Added `configs` field for action executor |
| `service/issue_tracker/gitlab.go` | Pagination loop, observability logging |
| `service/event_ingest.go` | Strict fetch mode — returns retryable error |

### 13.3 Deferred to v2

- Explicit gap → message tracking (requires schema change to store `ClosedByMessageSeq` on Gap model)
- Truncation policy for very long threads (>400k tokens)
- Feature flag for strict fetch mode rollback

---

## 14) References

### Prior Art (Spec-Driven Development)

| Project | Stars | Key Contribution to This Design |
|---------|-------|--------------------------------|
| **[OpenSpec](https://github.com/Fission-AI/OpenSpec)** | 16.4k | Scenario format (`#### Scenario: ... WHEN/THEN`), delta operations, validation-first workflow |
| **[BMAD-METHOD](https://github.com/bmad-code-org/BMAD-METHOD)** | 28.9k | Scale-adaptive depth (L0-L4), specialized agent separation, 50+ structured workflows |

### Internal References

- ADR templates: https://adr.github.io/adr-templates/
- Nygard ADR format: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- ExploreAgent pattern: `relay/internal/brain/explore_agent.go`
- Planner action flow: `relay/internal/brain/orchestrator.go`
- Existing `ready_for_spec_generation` validation: `relay/internal/brain/action_validator.go`

### Key Takeaways from Research

1. **OpenSpec's AGENTS.md approach**: They use a single instruction file that AI assistants read to understand the workflow. We already have `CLAUDE.md` for repo conventions; specs extend this pattern.

2. **BMAD's "agents as collaborators"**: Their philosophy is that AI should guide structured thinking, not just generate code. Our Planner already embodies this (asks questions, waits for proceed); Spec Generator extends it.

3. **Validation before implementation**: Both frameworks emphasize that specs must be validated/approved before any code is written. Our `ready_for_spec_generation` action + validation flow aligns with this.

4. **Two-folder model (OpenSpec)**: They separate `specs/` (truth) from `changes/` (proposals). Our model is simpler for v1 (single canonical spec per issue), but we could evolve toward this if issues have multiple related specs.
