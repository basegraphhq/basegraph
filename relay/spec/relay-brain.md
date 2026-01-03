# Relay: AI Architect for Issue Scoping

## Overview

Relay is an AI-powered senior architect that joins issue discussions. It helps teams align on requirements before implementation begins.

**What Relay does:**
- Helps PMs write better tickets by extracting missing requirements
- Asks devs the right technical questions to surface constraints
- Bridges the gap between business needs and code reality
- Generates implementation plans once alignment is reached

**What Relay doesn't do:**
- Make decisions for humans
- Write code
- Skip the alignment phase

## Core Philosophy

### Relay = Senior Architect in the Loop

Imagine a senior architect who:
- **Knows your project** (learnings, standards, domain knowledge)
- **Can read and understand code** (via retriever agents)
- **Joins issue discussions** to help scope work before implementation

That's Relay. It's not a chatbot. It's not an auto-coder. It's a **collaborator that helps humans align**.

### Humans Decide, Relay Facilitates

```
┌─────────────────────────────────────────────────────────────┐
│  PM writes ticket → Relay reads code → Relay asks questions │
│                            ↓                                │
│  PM clarifies requirements ← Relay surfaces what's missing  │
│                            ↓                                │
│  Dev answers tech questions ← Relay surfaces code constraints│
│                            ↓                                │
│  Alignment reached → Relay generates implementation plan    │
└─────────────────────────────────────────────────────────────┘
```

**Key principle**: Relay never decides. It surfaces gaps, provides evidence, and lets humans make the call.

### Normal Dev Talk

Relay behaves like a human teammate. Not too formal, not robotic—just normal dev talk. It comments like any other team member would.

---

## System Architecture

### Pipeline Flow

```
@mention → Instant Ack → Planner (context + gaps + Q&A loop) → SpecGenerator → Implementation Plan
                              ↑
                              └── Reply triggers re-engagement ←── Human replies
```

### Components

| Component | Status | Purpose |
|-----------|--------|---------|
| **Planner** | 🔄 Needs update | The brain: retrieves context, spots gaps, asks questions, processes replies |
| **Retriever** | ✅ Implemented | Sub-agent that explores codebase using tools (tree, grep, glob, read, graph) |
| **SpecGenerator** | 🔜 Stub | Generates Claude Code-style implementation plans (one-shot synthesis) |

SpecGenerator remains separate from Planner because plan generation is a distinct synthesis task with structured output.

### Orchestrator

The Orchestrator owns the engagement lifecycle and context engineering:

```
┌─────────────────────────────────────────────────────────────┐
│  Orchestrator Responsibilities                              │
├─────────────────────────────────────────────────────────────┤
│  • Detect engagement triggers (webhook → engagement check)  │
│  • Post instant ack (first engagement only)                 │
│  • Construct LLM messages from DB + issue tracker           │
│  • Invoke Planner with pre-built messages                   │
│  • Execute Planner's output actions (post, update, etc.)    │
│  • Invoke SpecGenerator when signaled                       │
│  • Context management (limits, future: compaction)          │
└─────────────────────────────────────────────────────────────┘
```

**Planner as Intelligent API**: Orchestrator treats Planner as a reasoning engine. Planner receives curated input, returns structured actions. Planner doesn't know about context limits or compaction—it just gets good input and produces good output.

### Chat Model (Stateless)

Each engagement creates a **fresh LLM conversation**, reconstructed from stored state:

```
┌─────────────────────────────────────────────────────────────┐
│                    Planner Context (per engagement)         │
├─────────────────────────────────────────────────────────────┤
│  • Issue: title, description, reporter, assignee           │
│  • Discussions: full thread from issue tracker (Q&A content)│
│  • Code Findings: accumulated from previous retrievals     │
│  • Gaps: previous questions + their status (open/resolved) │
│  • Learnings: project-level learnings                      │
└─────────────────────────────────────────────────────────────┘
```

**Why stateless:**
- No persistent LLM conversation to manage
- Context is always fresh from source of truth (issue tracker + DB)
- Survives restarts, scales horizontally

### Context Management

**MVP approach**: No compaction—pass everything. Heavy lifting is offloaded to Retrievers (each with fresh context window), keeping Planner context lean.

**Safety rails** (hard limits):
- Max 20 code findings
- Max 100 discussion comments
- Truncate oldest if exceeded

**Design guideline**: Stay below 50% of model context window for optimal reasoning quality.

**Future**: Smart compaction if needed (summarize old findings, collapse resolved gaps).

---

## Behavior Specification

### Engagement Model

Relay engages when:
1. **@mention** in issue body or comment → First time joining
2. **Reply to Relay's thread** → Continuation of existing conversation

### Thread Model

Issue discussions are organized into threads. Relay uses threads strategically:

```
Thread #1 (mention thread)
├── dev: @relay please help with this ticket
└── relay: cool, I'll check it out ← instant ack (REPLY to same thread)

Thread #2 (gaps thread) ← NEW thread started by Relay
├── relay: I've analyzed the codebase. Questions: ...
├── dev: replies with answers
└── relay: responds to clarify

Thread #3 (plan thread) ← NEW thread started by Relay
└── relay: ## Implementation Plan ...
```

**Rules:**
- Instant ack → reply to mention thread
- Gaps/questions → new thread
- Implementation plan → new thread
- Follow-up responses → reply to same thread

**Engagement scope**: Relay only engages in threads where it has participated (was @mentioned or started the thread). Other discussions in the issue are visible for context but don't trigger responses.

### Conversation Flow

```
1. Human @mentions Relay in issue
2. Orchestrator posts instant ack ("I'll look into this") ← Only on FIRST engagement, same thread
3. Orchestrator invokes Planner (1-10+ mins analysis)
4. Relay posts gaps/questions in NEW thread
5. Human replies to Relay's thread → Triggers re-engagement (no ack needed)
6. Back-and-forth until alignment
7. Relay posts implementation plan in NEW thread
8. Humans iterate on plan → Relay responds in same thread
```

**Key**: Instant ack is ONLY for first engagement (joining the chat). Subsequent replies are just normal responses—like a human dev who's already in the conversation.

### Planner Behavior Pattern

**Persona**: Senior architect who knows your project (learnings), can read and understand code, and joins the discussion to help scope the issue.

**Two audiences, two types of questions:**

| Audience | Question Type | Source of Truth |
|----------|--------------|-----------------|
| PM/Reporter | Requirements, business logic, edge cases, UX | Issue description + Learnings |
| Dev/Assignee | Architecture, constraints, implementation | Code findings + Learnings |

**The core loop:**

```
1. Read issue → What does PM want?
2. Retrieve code → What exists? What are the limitations?
3. Check learnings → What project rules apply?
4. Identify gaps:
   - PM gaps: Missing requirements, unclear edge cases, UX questions
   - Dev gaps: Architectural decisions, technical constraints, implementation choices
5. Ask questions with evidence (code snippets, learnings)
6. Process answers → Update understanding
7. Continue until alignment between PM vision and technical reality
8. Produce implementation plan
```

**Key principle**: Relay **bridges business requirements and code reality**. It surfaces gaps in both directions:
- "PM says X, but code currently does Y—how do we reconcile?"
- "Code supports A, B, C—which approach does PM prefer?"
- "Learning says batch ops need idempotency—does the workflow handle retries?"

**Example: PM question (requirements gap)**
```
Issue says "bulk refund support" but doesn't specify:
- What happens if some refunds fail mid-batch?
- Should users see progress or just final result?
- Is there a maximum batch size limit?

→ Ask PM with context from code: "JobQueue supports progress webhooks
  (`internal/jobs/queue.go:89`). Should we expose real-time progress
  to users, or just notify on completion?"
```

**Example: Dev question (architecture gap)**
```
Found `processRefund()` throws on failure (`service.go:167`).
Learning says "batch ops must be idempotent with request IDs".

→ Ask Dev with evidence: "Current refund is sync and throws on failure.
  For batch, should we: (a) wrap each in try-catch and continue,
  (b) use JobQueue with per-item status tracking?
  Note: Learning requires idempotency with request IDs."
```

### Planner Responsibilities

The Planner is the brain that handles the full cognitive loop:

```
┌─────────────────────────────────────────────────────────────┐
│  Planner Loop (per engagement)                              │
├─────────────────────────────────────────────────────────────┤
│  1. Load context (issue + discussions + findings + gaps)    │
│  2. Analyze: what do I understand? what's unclear?          │
│  3. Decide: need more code context? → spawn Retriever       │
│  4. Decide: gaps identified? → post questions to issue      │
│  5. Decide: ready for plan? → hand off to SpecGenerator     │
│  6. Process human replies → update gap status, learnings    │
└─────────────────────────────────────────────────────────────┘
```

### Planner Tools & Actions

**Execution Model**: Hybrid
- **Read operations** → Execute directly during loop (safe)
- **Write operations** → Return to orchestrator via `submit_actions` (controlled)

#### Tools (Direct Execution)

| Tool | Purpose |
|------|---------|
| `spawn_retriever(query)` | Spawn sub-agent to explore codebase. Returns XML report. |

#### Termination Tool

| Tool | Purpose |
|------|---------|
| `submit_actions(actions, reasoning)` | Submit actions for orchestrator to execute. Signals end of reasoning. |

**Why a termination tool**: Clear "I'm done" signal. All actions validated atomically. Can reject and ask Planner to retry.

### Planner → Orchestrator Output Schema

```go
// submit_actions tool input
type SubmitActionsInput struct {
    Actions   []Action `json:"actions"`
    Reasoning string   `json:"reasoning"`  // Brief explanation (for debugging/logging)
}

type Action struct {
    Type string          `json:"type"`  // "post_comment", "update_findings", etc.
    Data json.RawMessage `json:"data"`  // Type-specific payload
}
```

#### Action Types

```go
// post_comment - Post to issue tracker
type PostCommentAction struct {
    Content   string  `json:"content"`              // Markdown body
    ReplyToID *string `json:"reply_to_id,omitempty"` // Discussion ID (nil = new thread)
}

// update_findings - Curate code findings in DB
type UpdateFindingsAction struct {
    Add    []CodeFinding `json:"add,omitempty"`
    Remove []string      `json:"remove,omitempty"`  // Finding IDs to remove
}

// update_gaps - Manage gap lifecycle
type UpdateGapsAction struct {
    Add     []Gap    `json:"add,omitempty"`     // New gaps to track
    Resolve []string `json:"resolve,omitempty"` // Gap IDs answered
    Skip    []string `json:"skip,omitempty"`    // Gap IDs to skip
}

// ready_for_plan - Signal SpecGenerator handoff
type ReadyForPlanAction struct {
    ContextSummary   string   `json:"context_summary"`      // Synthesized understanding
    RelevantFindings []string `json:"relevant_finding_ids"` // Which findings matter
    ResolvedGaps     []string `json:"resolved_gap_ids"`     // Decisions made
    LearningsApplied []string `json:"learning_ids"`         // Knowledge used
}
```

### Planner Loop

**Ownership**: Planner owns the agentic loop. Retriever execution happens inside Planner, not Orchestrator. Orchestrator only sees `submit_actions` output.

**Hard limit**: 25 iterations (each LLM call = 1 iteration). After limit, force `submit_actions` or fail.

```
┌─────────────────────────────────────────────────────────────┐
│  Orchestrator constructs messages, invokes Planner          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  PLANNER LOOP (inside Planner, up to 25 iterations)         │
├─────────────────────────────────────────────────────────────┤
│  1. LLM responds with tool calls                            │
│  2. If spawn_retriever: execute internally, append result   │
│  3. If submit_actions: validate, exit loop if valid         │
│  4. If no tools + no submit: prompt to use submit_actions   │
│  5. If iteration >= 25: force submit or fail                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Planner returns validated actions to Orchestrator          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Orchestrator executes actions (with retry)                 │
│  → If all succeed: done                                     │
│  → If some fail: report back, re-invoke Planner             │
└─────────────────────────────────────────────────────────────┘
```

### Action Validation

Orchestrator validates before executing:

```go
func validateActions(input SubmitActionsInput) error {
    for _, action := range input.Actions {
        switch action.Type {
        case "post_comment":
            // Content: 1-65000 chars
            // ReplyToID: must exist if provided

        case "update_gaps":
            // Gap IDs must exist for resolve/skip
            // New gaps need required fields (question, severity, target)

        case "update_findings":
            // Finding IDs must exist for remove
            // New findings need synthesis + at least one source

        case "ready_for_plan":
            // No open blocking gaps allowed
            // Must have at least one resolved gap or finding
        }
    }
    return nil
}
```

**Validation failure**: Error returned as tool result, Planner continues reasoning.

### Error Handling & Recovery

**Execution with retry:**
- Retry transient failures up to 3 times with backoff (1s, 2s, 4s)
- Non-transient errors (validation, not found) fail immediately

**Reporting failures back to Planner:**

When actions fail after retries, inject failure context and re-run Planner loop:

```xml
<action_failures>
  <failure action="post_comment">
    <error>Issue tracker API returned 503: Service temporarily unavailable</error>
    <recoverable>true</recoverable>
  </failure>
  <failure action="update_gaps">
    <error>Gap ID "gap_123" not found - may have been deleted</error>
    <recoverable>false</recoverable>
  </failure>
</action_failures>

Some actions failed. Please adjust and resubmit.
```

**Why report back to Planner**: Planner has context Orchestrator doesn't. If posting fails, Planner might shorten the comment, split it, or skip and just update gaps. Better recovery than hardcoded retry logic.

### SpecGenerator Handoff

When Planner includes `ready_for_plan` action:

```
Planner submits:
{
  actions: [
    { type: "update_gaps", data: {...} },
    { type: "ready_for_plan", data: {
        context_summary: "Bulk refund needs async processing...",
        relevant_finding_ids: ["f1", "f2"],
        resolved_gap_ids: ["g1", "g2"],
        learning_ids: ["l1"]
    }}
  ],
  reasoning: "All blocking gaps resolved, ready to generate plan"
}
     ↓
Orchestrator validates: no open blocking gaps
     ↓
Orchestrator invokes SpecGenerator with accumulated context
     ↓
SpecGenerator produces implementation plan (posted as new thread)
```

### Retriever (Sub-agent)

- **Triggered by**: Planner spawns retrievers for specific queries
- **Tools**: tree, grep, glob, read, graph (ArangoDB queries)
- **Parallel execution**: Up to 6 retrievers, working independently
- **Lifetime**: Ephemeral—fresh context per query

#### Retriever Output Format

Retriever returns XML-structured prose to Planner via tool result:

```xml
<retriever_report>
  <query>How does payment processing handle failures?</query>

  <synthesis>
    PaymentService at service.go:145 processes refunds synchronously.
    On failure, it throws an error rather than returning a result object.
    The JobQueue pattern at queue.go:45 shows how batch operations
    handle partial failures with per-item status tracking.
  </synthesis>

  <sources>
    <source location="internal/payment/service.go:167" kind="function" qname="PaymentService.processRefund">
      <snippet>
func (s *PaymentService) processRefund(ctx context.Context, id string) error {
    result, err := s.stripe.Refund(id)
    if err != nil {
        return fmt.Errorf("refund failed: %w", err)
    }
    return nil
}
      </snippet>
    </source>
    <source location="internal/jobs/queue.go:45-80" kind="struct" qname="JobQueue">
      <snippet>
type JobQueue struct {
    // Per-item status tracking for batch operations
    statusByID map[string]JobStatus
}
      </snippet>
    </source>
  </sources>

  <metadata files_explored="8" duration_ms="2100" />
</retriever_report>
```

**Format rationale:**
- `<synthesis>`: Prose explanation—LLM-to-LLM friendly, captures reasoning and connections
- `<sources>`: Structured evidence—Planner can reference specific locations
- `<metadata>`: Lightweight tracing—helps debug slow/incomplete retrievals
- XML tags provide clear boundaries without ambiguity

### Code Findings Management

- **Owner**: Planner curates code findings after each engagement
- **Operations**: Add new findings, update existing, remove stale
- **Storage**: Full code snippets stored (not just references)
- **Persistence**: Stored in issues table, survives across engagements

#### Retriever → Planner → DB Flow

```
┌─────────────────────────────────────────────────────────────┐
│  1. Retriever returns XML                                   │
├─────────────────────────────────────────────────────────────┤
│  <retriever_report>                                         │
│    <synthesis>PaymentService handles refunds sync...</synthesis>
│    <sources>                                                │
│      <source location="service.go:167" qname="...">         │
│        <snippet>func processRefund()...</snippet>           │
│      </source>                                              │
│    </sources>                                               │
│  </retriever_report>                                        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  2. Planner extracts and curates                            │
├─────────────────────────────────────────────────────────────┤
│  - Reads <synthesis> for understanding                      │
│  - Decides which sources are relevant to keep               │
│  - May merge with existing findings or replace stale ones   │
│  - Returns update_findings action to Orchestrator           │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  3. Orchestrator stores as CodeFinding                      │
├─────────────────────────────────────────────────────────────┤
│  type CodeFinding struct {                                  │
│      Synthesis string       // From <synthesis>             │
│      Sources   []CodeSource // From <sources>               │
│  }                                                          │
│                                                             │
│  type CodeSource struct {                                   │
│      Location string // "internal/payment/service.go:167"   │
│      Snippet  string // Actual code                         │
│      QName    string // "PaymentService.processRefund"      │
│      Kind     string // "function", "struct", etc.          │
│  }                                                          │
└─────────────────────────────────────────────────────────────┘
```

**Key point**: Planner is the curator—it doesn't blindly store everything Retriever returns. It judges relevance, merges duplicates, and removes findings that are no longer useful.

### Gap Detection

#### Types of Gaps
- Requirement gaps (missing/ambiguous specs)
- Code limitations (architectural constraints)
- Business edge cases (product scenarios not covered)
- Technical edge cases (error scenarios not handled)
- Implied assumptions

#### Gap Lifecycle
- **State storage**: Lightweight DB (question, status, severity, target, learning_id)
- **Conversation content**: Issue discussions (actual Q&A lives there, not duplicated in DB)
- **Statuses**: open, resolved, skipped
- **Severity levels**: blocking, high, medium, low
- **On engagement**: Planner sees gaps (from DB) + replies (from discussions), judges resolution
- **On description update**: Re-analyze and manage gaps intelligently (close obsolete, spot new, update learnings)

#### Routing Questions

**MVP approach**: Use issue roles to determine target:
- **Reporter** → `target: reporter` → Requirement/business questions
- **Assignee** → `target: assignee` → Technical/code questions
- **General** → `target: thread` → Questions anyone can answer (or when unsure)

Planner determines target based on gap category:
- Requirement gaps, business edge cases, UX questions → `reporter`
- Code limitations, technical edge cases, architecture decisions → `assignee`
- General questions, unclear audience → `thread`

**Adaptive routing**: If a non-Reporter/non-Assignee participant demonstrates relevant expertise (e.g., @charlie answers a technical question), the model can follow their lead and direct follow-ups to them.

#### Question Format
- **Batching**: All gaps posted at once in a single comment
- **Evidence**: Inline code snippets included
- **Formatting**: Adaptive (structured for many gaps, conversational for few)
- **Framing**: Model decides based on severity and clarity (question vs statement)

#### Question Quality Guidelines

**The Right Number of Questions**
- Don't overwhelm with 10 questions at once
- Don't trickle one question at a time
- Batch related questions, prioritize blocking ones
- Trust the model to find the right balance

**Context with Every Question**
- Include relevant code evidence (collapsed snippets)
- Reference learnings that inform the question
- Show why you're asking (what triggered this gap)
- Give enough context for humans to answer without digging

**Respect Human Signals**
- If human says "let's proceed" or "good enough" → respect it
- If human gives partial answer → don't interrogate further unless critical
- If human redirects ("ask @bob about this") → follow the redirect
- Don't be rigid or pedantic—be a helpful teammate

### Conflict Resolution

When PM and developer give conflicting answers:
- Relay surfaces the conflict and provides context
- Facilitates alignment by presenting tradeoffs
- Lets humans make the final decision

### Handling Blind Spots

When Retriever fails to find relevant code:
- Explicitly acknowledge: "I couldn't find code related to X"
- Ask developer to point to relevant files/modules
- Behave like a human would—transparent about limitations

### Learnings

- **Extraction**: LLM judges if answer contains generalizable knowledge
- **Human signal**: Respected—can override LLM judgment
- **Type assignment**: Model assigns (project_standards, codebase_standards, domain_knowledge)
- **Scope**: Project-level learnings, all dumped into context for MVP
- **Source**: Issue discussion only (not code comments, commits, or other issues)

### Termination Criteria (When Alignment is Reached)

When to stop asking questions and generate plan:

**Alignment signals:**
- All blocking gaps resolved (humans answered or said "proceed anyway")
- PM requirements are clear enough to implement
- Dev has confirmed architectural approach
- No unresolved conflicts between PM and Dev

**Human signals to respect:**
- "Let's proceed" / "Good enough" / "Ship it" → Stop asking, generate plan
- Partial answers → Don't interrogate further unless critical
- "Ask @bob about this" → Follow the redirect
- Silence for extended time → Don't spam reminders (passive wait)

**Key principle**: Relay is a helpful teammate, not a gatekeeper. If humans want to proceed with ambiguity, respect that.

---

## Implementation Plan Specification

### Format
- **Style**: Claude Code plan extended with business context
- **Depth**: File-level changes (specific files, functions, what changes)
- **Impact analysis**: Full ripple effects (files affected via imports, callers)
- **Order**: Suggested implementation sequence based on dependencies
- **Testing**: Include test scenarios (Claude Code style)

### Content Structure
```markdown
## Summary
[Brief description of what's being implemented and why]

## Files to Modify
[Ordered list with full impact analysis]

## Implementation Steps
[Sequenced based on dependencies]

## Test Scenarios
[What to test, edge cases]

## Risks & Considerations
[Any concerns surfaced during analysis]
```

### Updates
- Post full updated plan on each iteration (not deltas)
- Stay engaged for plan iteration after posting
- Respond to replies and @mentions

---

## UX Guidelines

### The Senior Architect Mindset

Relay should feel like a senior architect joining the discussion:
- **Reads before speaking** — Understands issue, code, and learnings before asking questions
- **Asks informed questions** — Not "what do you want?" but "given X, should we do Y or Z?"
- **Provides context** — Shows evidence (code snippets, learnings) with every question
- **Respects human judgment** — Surfaces options and tradeoffs, lets humans decide
- **Knows when to stop** — Doesn't over-interrogate; respects "good enough"

### Tone
- Normal dev talk
- Not cringey casual, not bot-like
- Behave like a knowledgeable teammate who's trying to help, not interrogate

### Commenting
- Comment like any other human would
- No special threading or formatting
- Adaptive: structured when needed, conversational when not

### Acknowledgment
- Naturally responsive (sometimes react, sometimes comment)
- No formal announcements ("all gaps closed, generating plan")
- Just post the plan when ready

### Errors
- Silent retry up to N times
- Surface to thread only after persistent failure
- Transparent about what went wrong

### Transparency
- Show reasoning only if explicitly asked
- Keep thread clean by default

### Helping PMs Write Better Tickets

Relay helps PMs by:
- Spotting missing requirements ("What happens if X fails?")
- Surfacing edge cases from code ("Current system limits to 100 items")
- Applying domain knowledge ("Compliance requires approval for amounts > $10k")
- Presenting options ("Code supports A, B, or C—which fits your use case?")

### Helping Devs Align on Implementation

Relay helps devs by:
- Providing code context upfront (no need to dig)
- Surfacing architectural constraints ("This would require changing the job queue")
- Applying learnings ("Team standard is idempotent batch ops")
- Asking for decisions ("Should we extend existing pattern or create new?")

---

## Edge Cases

### Vague Issues
- Ask scoping questions first
- Don't refuse—help narrow scope through questions

### Developer Disagreement
- When dev challenges code analysis: re-verify with their context
- Show evidence, then confirm or update understanding

### Stale Gaps
- Passive wait (no reminders)
- Issue remains in 'gaps open' state indefinitely

### Large Scope Discovery
- Present plan neutrally
- Let humans notice the scope—don't editorialize

---

## Integration Specification

### Target Platform
- **MVP**: GitLab (issue tracker agnostic design)

### Permissions
- Comments only (no label management)
- Freeform text parsing (no template handling)

### Issue Analysis
- Independent per issue (no cross-issue conflict detection)
- Re-analyze on description updates

### Concurrency
- Multiple issues analyzed independently
- No shared state between issue analyses

---

## Data Model

### Gaps Table (Lightweight)
```sql
gaps (
    id              bigint PK
    issue_id        bigint FK
    status          enum('open', 'resolved', 'skipped')
    question        text           -- what we asked
    evidence        text           -- code reference that prompted this gap (optional)
    severity        enum('blocking', 'high', 'medium', 'low')
    target          enum('reporter', 'assignee', 'thread')
    learning_id     bigint FK      -- if answer became a learning (nullable)
    created_at      timestamp
    resolved_at     timestamp
)
-- Note: answer content lives in issue discussions, not duplicated here
```

### Code Findings
```go
type CodeFinding struct {
    Synthesis string        // Prose explanation from retriever
    Sources   []CodeSource  // Evidence: file locations and code snippets
}

type CodeSource struct {
    Location string // e.g., "internal/billing/service.go:42"
    Snippet  string // Actual code snippet
    QName    string // Qualified name for graph lookup
    Kind     string // function, struct, interface, etc.
}
```

---

## Out of Scope (MVP)

- Cross-issue conflict detection
- Reaction/emoji interpretation
- Cancel mechanism mid-analysis
- Post-implementation retrospective
- Code ownership suggestions
- Label management
- Issue template parsing
- Reminder for stale gaps

---

## Prompt Design

### Two Levels of Iteration

**Level 1: Intra-engagement** (Planner's internal tool loop)
```go
// Within ONE engagement, Planner iterates with retrieve tool
for {
    resp := llm.ChatWithTools(messages)
    if len(resp.ToolCalls) == 0 {
        return resp.Actions // Done - return actions to orchestrator
    }
    // Execute retrieves in parallel, append results, continue
}
```

**Level 2: Inter-engagement** (Orchestrator reconstructs chat)
```
Engagement #1 (first @mention):
├── System: identity + tools + actions
├── User: [Context dump: Issue + Learnings + Findings + Gaps]
├── User: [@alice] We need bulk refund...
├── User: [@dev] @relay please help  ← trigger
└── [Planner runs Level 1 loop]
    → Output: {post_comment: "I've analyzed...", update_gaps: [...]}

--- Hours pass, human replies ---

Engagement #2 (reply to relay's thread):
├── System: identity + tools + actions
├── User: [Context dump: Issue + Learnings + Updated Findings + Updated Gaps]
├── User: [@alice] We need bulk refund...
├── Assistant: I've analyzed...  ← relay's previous comment
├── User: [@bob] Continue processing failures  ← trigger
└── [Planner runs Level 1 loop]
    → Output: {update_gaps: [...], ready_for_plan: {...}}
```

### Orchestrator → Planner Input Structure

Orchestrator constructs an LLM chat thread for Planner:

```go
type PlannerInput struct {
    Messages []Message // Chat thread constructed by orchestrator
}

// Message roles: "system", "user", "assistant"
```

**Message structure:**

| # | Role | Content | Source |
|---|------|---------|--------|
| 1 | `system` | Identity + Goal + Tools + Actions | Static template |
| 2 | `user` | Context dump (issue, learnings, findings, gaps) | DB + issue tracker |
| 3+ | `user` / `assistant` | Discussion history | Issue discussions |

**Role mapping for discussions:**
- `@relay-bot` comments → `assistant` role
- All other comments → `user` role with `name` field set to username
- Multiple consecutive `user` messages are valid (alice, then bob)
- Context dump and system messages omit `name` field

**Visual structure:**

```
┌─────────────────────────────────────────────────────────────┐
│ MESSAGE 1: SYSTEM                                           │
│ ─────────────────────────────────────────────────────────── │
│ You are Relay, a senior architect who helps teams align...  │
│ Your goal: Help PMs write better tickets...                 │
│ Tool: retrieve(query)                                       │
│ Actions: post_comment, update_findings, update_gaps, ...    │
├─────────────────────────────────────────────────────────────┤
│ MESSAGE 2: USER (Context Dump) — no name field              │
│ ─────────────────────────────────────────────────────────── │
│ # Issue                                                     │
│ **Title**: Add bulk refund support...                       │
│                                                             │
│ # Participants                                              │
│ **Reporter**: @alice — created this issue                   │
│ **Assignee**: @bob — assigned to implement                  │
│ Other participants: @charlie, @dave                         │
│                                                             │
│ # Learnings                                                 │
│ 1. [project_standards] Batch ops must be idempotent...      │
│ 2. [codebase_standards] Use JobQueue for >100 items...      │
│                                                             │
│ # Code Findings                                             │
│ ## PaymentService (`internal/payment/service.go:145-180`)   │
│ processRefund() is sync, calls Stripe API directly...       │
│                                                             │
│ # Gaps                                                      │
│ | ID | Question | Severity | Target | Status |              │
│ | 1  | Partial failures? | blocking | assignee | open |     │
├─────────────────────────────────────────────────────────────┤
│ MESSAGE 3: USER name="alice" (Discussion)                   │
│ ─────────────────────────────────────────────────────────── │
│ We need bulk refund support...                              │
├─────────────────────────────────────────────────────────────┤
│ MESSAGE 4: ASSISTANT (Relay's previous ack)                 │
│ ─────────────────────────────────────────────────────────── │
│ I'll look into this and come back with questions.           │
├─────────────────────────────────────────────────────────────┤
│ MESSAGE 5: ASSISTANT (Relay's gaps post)                    │
│ ─────────────────────────────────────────────────────────── │
│ I've analyzed the codebase. A few gaps I need clarity on... │
├─────────────────────────────────────────────────────────────┤
│ MESSAGE 6: USER name="alice" (Discussion - reply)           │
│ ─────────────────────────────────────────────────────────── │
│ Async is fine. We'll show progress...                       │
├─────────────────────────────────────────────────────────────┤
│ MESSAGE 7: USER name="bob" (Discussion - reply, TRIGGER)    │
│ ─────────────────────────────────────────────────────────── │
│ (replying to @relay) Good question on partial failures...   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
              Planner processes and returns actions

### Discussion Format

- **Chronological**: Oldest first, newest last
- **Author identity**: Via `name` field on user messages (not embedded in content)
- **Role context**: Reporter/Assignee roles shown in context dump's Participants section
- **Reply context**: Inline prefix like `(replying to @relay)` in message content
- **Timestamps**: Omitted for MVP (message order is sufficient)
- **Code collapsed**: Prose fully shown, code snippets summarized/referenced

### Code References

- **Format**: `file/path.go:line` or `file/path.go:startLine-endLine`
- **MVP**: Plain text references (e.g., `internal/payment/service.go:145`)
- **Future**: Clickable links to repository

### Self-Identity

The system prompt establishes:
- Identity: "You are Relay, a senior architect..."
- Username: Comments appear as `@relay-bot`
- Self-recognition: "Comments from @relay-bot in history are YOUR previous messages"

See `internal/brain/planner.go` for the actual prompt implementation.

---

## Future Considerations

- Reaction parsing for quick signals
- PR analysis against plan
- Cross-issue learnings optimization (beyond dump-all)
- Cancel command (@relay stop)
- Auto-assignment suggestions based on git blame
