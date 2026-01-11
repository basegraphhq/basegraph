# Planner ↔ Spec Generator Handoff & Discussions (2026-01-11)

**Status: Implemented**

## Incident Summary (spec generator failure)
- Planner emitted `ready_for_spec_generation` with `relevant_finding_ids: ["finding_1","finding_2"]` (placeholders).
- ActionExecutor built SpecGen input expecting real finding IDs (snowflake strings stored on `issue.CodeFindings`).
- SpecGen marked **zero core findings**, tried to recover via `explore`, burned all 3 attempts, never called `submit_spec`, and failed the action.
- Root cause: Planner cannot know persisted finding IDs (they are assigned when `update_findings` executes). The contract requiring Planner-provided IDs is invalid.

## Decisions
- **Full discussion thread must be stored**: If Relay engages, `issues.discussions` must contain the entire thread (all replies). Missing/partial threads are unacceptable for planner/spec quality.
- **Planner working set = structured state + recent tail**: Planner prompt keeps a cap at the most recent ~100 discussions; relies on gaps/findings + planner handoff, not full thread reread.
- **Spec Generator can see full thread**: SpecGen should receive the full discussion transcript (400k ctx budget) as a backstop; primary guidance is the planner handoff, gaps, findings, learnings, proceed signal, existing spec.
- **No Planner → SpecGen IDs**: Planner must not emit `relevant_finding_ids` / `closed_gap_ids` for spec. Orchestrator builds SpecGen input directly from stores/issue state (all findings/gaps/learnings/spec/discussions).
- **Strict discussion fetch**: If Relay engages or refreshes a tracked issue and discussions cannot be fetched, treat as retryable failure. Never clobber stored discussions on fetch error.

## Implementation Plan

### 1) Fetch all discussions (GitLab) ✅
- Added pagination in `issue_tracker/gitlab.go` `FetchDiscussions` using `ListIssueDiscussionsOptions{Page, PerPage=100}` until `NextPage == 0`.
- Added observability: logs thread count, note count, pages fetched.

### 2) Protect stored discussions (strict mode) ✅
- `event_ingest` (subscribed path): only replaces `existingIssue.Discussions` on successful fetch; on fetch error keeps existing discussions and returns retryable error.
- Note: `engagement_detector` path not yet updated (lower priority for v1).

### 3) Planner/SpecGen contract change ✅
- `BuildSpecGeneratorInput` now ignores `relevant_finding_ids` and `closed_gap_ids` from action.
- All findings from `issue.CodeFindings` are marked as core (`IsCore: true`).
- Gaps/learnings fetched directly from stores.
- Discussion transcript included in SpecGen input using provider-agnostic `ConversationMessage` format.
- XML conversation format with heuristic annotations (`answers_gap`, `is_proceed`).

### 4) SpecGen robustness ✅
- Explore limit enforced at **2 calls maximum**.
- After limit reached, returns: "Explore limit reached. Please call submit_spec with your best effort."

### 5) Observability ✅
- GitLab fetch logs: `project_id`, `issue_iid`, `threads`, `notes`, `pages`.
- Strict fetch logs: `issue_id`, `existing_count`, `error`.

## Invariants (why this is safe)
- Planner quality improves with "recent tail + structured state"; full thread in prompt degrades signal and increases cost.
- SpecGen needs full thread available as a safety net for late "oh btw" constraints; with 400k ctx and <50% target, this is acceptable.
- Strict fetch + no-clobber removes silent regressions where missing threads cause bad plans/specs.

## Scope / Open items
- If future event types are added, revisit strict-fetch applicability.
- If threads with huge logs/code blocks appear, add truncation policy for SpecGen input; for now, include full and rely on logging to detect issues.
- Explicit gap → message tracking (v2, requires schema change).

## Files Modified
| File | Changes |
|------|---------|
| `model/conversation.go` | NEW: Provider-agnostic `ConversationMessage` struct |
| `brain/spec_generator.go` | Added Discussions to input, XML formatting, explore limit, all findings = core |
| `brain/action_executor.go` | Added `configs` store, `getRelayUsername()`, passes discussions to SpecGen |
| `brain/orchestrator.go` | Added `configs` field for action executor |
| `service/issue_tracker/gitlab.go` | Pagination loop, observability logging |
| `service/event_ingest.go` | Strict fetch mode — returns retryable error |
