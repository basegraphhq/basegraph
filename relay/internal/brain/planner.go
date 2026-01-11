package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

const (
	maxParallelExplorers = 3 // Parallel exploration with non-overlapping scopes
)

type ExploreParams struct {
	Query        string `json:"query" jsonschema:"required,description=Specific question about the codebase. Ask ONE thing, not multiple."`
	Thoroughness string `json:"thoroughness" jsonschema:"required,enum=quick,enum=medium,enum=thorough,description=Search depth: quick (first good match), medium (check a few locations), thorough (comprehensive search)"`
}

// SubmitActionsParams defines the schema for the submit_actions tool.
// This tool terminates the Planner loop and returns actions to Orchestrator.
type SubmitActionsParams struct {
	Actions   []ActionParam `json:"actions" jsonschema:"required,description=List of actions for orchestrator to execute"`
	Reasoning string        `json:"reasoning" jsonschema:"required,description=Brief explanation of why these actions were chosen"`
}

// ActionParam is the JSON schema for a single action in submit_actions.
type ActionParam struct {
	Type string          `json:"type" jsonschema:"required,enum=post_comment,enum=update_findings,enum=update_gaps,enum=update_learnings,enum=ready_for_spec_generation"`
	Data json.RawMessage `json:"data" jsonschema:"required"`
}

// PlannerOutput contains the structured actions returned by Planner.
// The Orchestrator executes these actions.
type PlannerOutput struct {
	Actions        []Action      // Actions to execute (post_comment, update_gaps, etc.)
	Reasoning      string        // Brief explanation (for debugging/logging)
	Messages       []llm.Message // Conversation history (for validation feedback retry)
	LastToolCallID string        // ID of submit_actions call (for injecting feedback)
}

// PlannerMetrics captures structured data about a planning session for eval.
// Focus: track leading indicators of spec quality before human feedback is available.
type PlannerMetrics struct {
	SessionID             string    `json:"session_id"`
	IssueID               int64     `json:"issue_id"`
	StartTime             time.Time `json:"start_time"`
	EndTime               time.Time `json:"end_time"`
	DurationMs            int64     `json:"duration_ms"`
	Iterations            int       `json:"iterations"`
	TotalPromptTokens     int       `json:"total_prompt_tokens"`
	TotalCompletionTokens int       `json:"total_completion_tokens"`

	// Action metrics - what did planner decide to do?
	ActionCounts    map[string]int `json:"action_counts"`     // count by action type
	GapsOpened      int            `json:"gaps_opened"`       // new gaps created
	GapsClosed      int            `json:"gaps_closed"`       // gaps closed this session
	GapCloseReasons map[string]int `json:"gap_close_reasons"` // answered/inferred/not_relevant
	LearningsAdded  int            `json:"learnings_added"`   // new learnings proposed
	FindingsAdded   int            `json:"findings_added"`    // new code findings

	// Outcome - did we reach spec generation?
	ReachedSpecGeneration bool   `json:"reached_spec_generation"`
	ProceedSignal         string `json:"proceed_signal,omitempty"` // excerpt of human approval

	// Explore usage - how much code exploration?
	ExploreCallCount int `json:"explore_call_count"`
}

// Planner gathers code context for issue scoping.
// It spawns ExploreAgent sub-agents to explore the codebase, preserving its own context window.
type Planner struct {
	llm       llm.AgentClient
	explore   *ExploreAgent
	specStore store.SpecStore
	debugDir  string // Directory for debug logs (empty = no logging)
}

// NewPlanner creates a Planner with an ExploreAgent sub-agent.
func NewPlanner(llmClient llm.AgentClient, explore *ExploreAgent, specStore store.SpecStore, debugDir string) *Planner {
	return &Planner{
		llm:       llmClient,
		explore:   explore,
		specStore: specStore,
		debugDir:  debugDir,
	}
}

// PlanContext provides additional context to the planning process.
type PlanContext struct {
	Messages    []llm.Message
	SpecRefJSON string // JSON-serialized SpecRef from issues.spec, empty if no spec exists
}

// ReadSpecParams defines the schema for the read_spec tool.
type ReadSpecParams struct {
	Mode     string `json:"mode" jsonschema:"required,enum=full,enum=summary,default=summary,description=Whether to return the full spec or a summary with TOC and excerpt."`
	MaxChars int    `json:"max_chars" jsonschema:"required,default=30000,description=Maximum characters to return (for full mode). Use 0 for default."`
}

// Plan runs the reasoning loop with pre-built messages from ContextBuilder.
// Returns structured actions for Orchestrator to execute.
func (p *Planner) Plan(ctx context.Context, planCtx PlanContext) (PlannerOutput, error) {
	messages := planCtx.Messages
	start := time.Now()

	// Enrich context with planner component
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.brain.planner",
	})

	if len(messages) == 0 {
		slog.DebugContext(ctx, "no messages to plan from")
		return PlannerOutput{
			Actions:   nil,
			Reasoning: "Empty input - no messages to analyze",
		}, nil
	}

	// Debug logging
	sessionID := time.Now().Format("20060102-150405")
	var debugLog strings.Builder
	debugLog.WriteString(fmt.Sprintf("=== PLANNER SESSION %s ===\n", sessionID))
	debugLog.WriteString(fmt.Sprintf("Messages: %d\n\n", len(messages)))
	for i, m := range messages {
		debugLog.WriteString(fmt.Sprintf("[%d] %s: %s\n", i, m.Role, logger.Truncate(m.Content, 500)))
	}
	debugLog.WriteString("\n")

	// Initialize metrics for evaluation
	metrics := PlannerMetrics{
		SessionID:       sessionID,
		StartTime:       start,
		ActionCounts:    make(map[string]int),
		GapCloseReasons: make(map[string]int),
	}

	var accumulatedContext strings.Builder
	iterations := 0
	totalPromptTokens := 0
	totalCompletionTokens := 0

	slog.InfoContext(ctx, "planner starting")

	defer func() {
		slog.InfoContext(ctx, "planner completed",
			"total_duration_ms", time.Since(start).Milliseconds(),
			"iterations", iterations,
			"total_prompt_tokens", totalPromptTokens,
			"total_completion_tokens", totalCompletionTokens,
			"total_tokens", totalPromptTokens+totalCompletionTokens)
	}()

	for {
		iterations++
		iterStart := time.Now()

		slog.DebugContext(ctx, "planner iteration starting", "iteration", iterations)

		resp, err := p.llm.ChatWithTools(ctx, llm.AgentRequest{
			Messages: messages,
			Tools:    p.tools(),
		})
		if err != nil {
			p.writeDebugLog(sessionID, debugLog.String())
			return PlannerOutput{}, fmt.Errorf("planner chat iteration %d: %w", iterations, err)
		}

		// Track token usage
		totalPromptTokens += resp.PromptTokens
		totalCompletionTokens += resp.CompletionTokens

		slog.DebugContext(ctx, "planner iteration LLM response received",
			"iteration", iterations,
			"duration_ms", time.Since(iterStart).Milliseconds(),
			"tool_calls", len(resp.ToolCalls),
			"prompt_tokens", resp.PromptTokens,
			"completion_tokens", resp.CompletionTokens)

		// Log assistant response
		debugLog.WriteString(fmt.Sprintf("--- ITERATION %d ---\n", iterations))
		debugLog.WriteString(fmt.Sprintf("[ASSISTANT]\n%s\n\n", resp.Content))
		if len(resp.ToolCalls) > 0 {
			debugLog.WriteString("[TOOL_CALLS]\n")
			for _, tc := range resp.ToolCalls {
				debugLog.WriteString(fmt.Sprintf("- %s: %s\n", tc.Name, logger.Truncate(tc.Arguments, 2000)))
			}
			debugLog.WriteString("\n")
		}

		// Check for submit_actions - terminates the loop
		for _, tc := range resp.ToolCalls {
			if tc.Name == "submit_actions" {
				params, err := llm.ParseToolArguments[SubmitActionsParams](tc.Arguments)
				if err != nil {
					p.writeDebugLog(sessionID, debugLog.String())
					return PlannerOutput{}, fmt.Errorf("parsing submit_actions: %w", err)
				}

				actions := make([]Action, len(params.Actions))
				for i, ap := range params.Actions {
					actions[i] = Action{
						Type: ActionType(ap.Type),
						Data: ap.Data,
					}
				}

				debugLog.WriteString("=== PLANNER COMPLETED (submit_actions) ===\n")
				debugLog.WriteString(fmt.Sprintf("Actions: %d, Reasoning: %s\n", len(actions), params.Reasoning))
				p.writeDebugLog(sessionID, debugLog.String())

				// Collect and write metrics
				metrics.EndTime = time.Now()
				metrics.DurationMs = time.Since(start).Milliseconds()
				metrics.Iterations = iterations
				metrics.TotalPromptTokens = totalPromptTokens
				metrics.TotalCompletionTokens = totalCompletionTokens
				p.collectActionMetrics(actions, &metrics)
				p.writeMetricsLog(metrics)

				slog.InfoContext(ctx, "planner submitted actions",
					"iterations", iterations,
					"action_count", len(actions),
					"total_duration_ms", time.Since(start).Milliseconds(),
					"reasoning", logger.Truncate(params.Reasoning, 200))

				// Include assistant message in history for potential validation feedback
				messagesWithResponse := append(messages, llm.Message{
					Role:      "assistant",
					Content:   resp.Content,
					ToolCalls: resp.ToolCalls,
				})

				return PlannerOutput{
					Actions:        actions,
					Reasoning:      params.Reasoning,
					Messages:       messagesWithResponse,
					LastToolCallID: tc.ID,
				}, nil
			}
		}

		// No tool calls = LLM finished without submitting actions (unexpected)
		if len(resp.ToolCalls) == 0 {
			debugLog.WriteString("=== PLANNER COMPLETED (no submit_actions) ===\n")
			p.writeDebugLog(sessionID, debugLog.String())

			slog.WarnContext(ctx, "planner completed without submit_actions",
				"iterations", iterations,
				"total_duration_ms", time.Since(start).Milliseconds())

			return PlannerOutput{
				Actions:   nil,
				Reasoning: resp.Content,
			}, nil
		}

		// Log tool calls
		for _, tc := range resp.ToolCalls {
			debugLog.WriteString(fmt.Sprintf("[TOOL CALL] %s\n", tc.Name))
			debugLog.WriteString(fmt.Sprintf("Arguments: %s\n\n", tc.Arguments))
		}

		// Execute retrieve calls in parallel
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Count explore calls for metrics
		for _, tc := range resp.ToolCalls {
			if tc.Name == "explore" {
				metrics.ExploreCallCount++
			}
		}

		results := p.executeToolsParallel(ctx, resp.ToolCalls, planCtx.SpecRefJSON)

		for _, r := range results {
			// Log tool result (truncated for readability)
			debugLog.WriteString(fmt.Sprintf("[TOOL RESULT] (length: %d)\n", len(r.report)))
			if len(r.report) > 2000 {
				debugLog.WriteString(r.report[:2000])
				debugLog.WriteString("\n... (truncated)\n\n")
			} else {
				debugLog.WriteString(r.report)
				debugLog.WriteString("\n\n")
			}

			if r.report != "" {
				accumulatedContext.WriteString(r.report)
				accumulatedContext.WriteString("\n\n---\n\n")
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    r.report,
				ToolCallID: r.callID,
			})
		}
	}
}

func (p *Planner) writeDebugLog(sessionID, content string) {
	ctx := context.Background()
	if p.debugDir == "" {
		return
	}

	if err := os.MkdirAll(p.debugDir, 0o755); err != nil {
		slog.WarnContext(ctx, "failed to create debug dir", "dir", p.debugDir, "error", err)
		return
	}

	filename := filepath.Join(p.debugDir, fmt.Sprintf("planner_%s.txt", sessionID))
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		slog.WarnContext(ctx, "failed to write debug log", "file", filename, "error", err)
	} else {
		slog.InfoContext(ctx, "debug log written", "file", filename)
	}
}

// writeMetricsLog writes structured JSON metrics for planner evaluation.
func (p *Planner) writeMetricsLog(metrics PlannerMetrics) {
	if p.debugDir == "" {
		return
	}

	if err := os.MkdirAll(p.debugDir, 0o755); err != nil {
		slog.Warn("failed to create debug dir", "dir", p.debugDir, "error", err)
		return
	}

	metricsFile := filepath.Join(p.debugDir, fmt.Sprintf("planner_metrics_%s.json", metrics.SessionID))
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		slog.Warn("failed to marshal planner metrics", "error", err)
		return
	}

	if err := os.WriteFile(metricsFile, data, 0o644); err != nil {
		slog.Warn("failed to write planner metrics", "file", metricsFile, "error", err)
	}
}

// collectActionMetrics analyzes submitted actions to populate metrics.
func (p *Planner) collectActionMetrics(actions []Action, metrics *PlannerMetrics) {
	for _, action := range actions {
		metrics.ActionCounts[string(action.Type)]++

		switch action.Type {
		case ActionTypeUpdateGaps:
			data, err := ParseActionData[UpdateGapsAction](action)
			if err != nil {
				continue
			}
			metrics.GapsOpened += len(data.Add)
			metrics.GapsClosed += len(data.Close)
			for _, c := range data.Close {
				metrics.GapCloseReasons[string(c.Reason)]++
			}

		case ActionTypeUpdateLearnings:
			data, err := ParseActionData[UpdateLearningsAction](action)
			if err != nil {
				continue
			}
			metrics.LearningsAdded += len(data.Propose)

		case ActionTypeUpdateFindings:
			data, err := ParseActionData[UpdateFindingsAction](action)
			if err != nil {
				continue
			}
			metrics.FindingsAdded += len(data.Add)

		case ActionTypeReadyForSpecGeneration:
			data, err := ParseActionData[ReadyForSpecGenerationAction](action)
			if err != nil {
				continue
			}
			metrics.ReachedSpecGeneration = true
			metrics.ProceedSignal = data.ProceedSignal
		}
	}
}

type plannerToolResult struct {
	callID string
	report string
}

// executeToolsParallel runs multiple tool calls concurrently with bounded parallelism.
func (p *Planner) executeToolsParallel(ctx context.Context, toolCalls []llm.ToolCall, specRefJSON string) []plannerToolResult {
	results := make([]plannerToolResult, len(toolCalls))
	var wg sync.WaitGroup

	// Semaphore to limit concurrent explore agents
	sem := make(chan struct{}, maxParallelExplorers)

	for i, tc := range toolCalls {
		switch tc.Name {
		case "explore":
			wg.Add(1)
			go func(idx int, call llm.ToolCall) {
				defer wg.Done()

				// Acquire semaphore slot
				sem <- struct{}{}
				defer func() { <-sem }()

				params, err := llm.ParseToolArguments[ExploreParams](call.Arguments)
				if err != nil {
					results[idx] = plannerToolResult{
						callID: call.ID,
						report: fmt.Sprintf("Error parsing arguments: %s", err),
					}
					return
				}

				exploreStart := time.Now()
				thoroughness := Thoroughness(params.Thoroughness)
				if thoroughness == "" {
					thoroughness = ThoughnessMedium
				}

				slog.InfoContext(ctx, "planner spawning explore agent",
					"query", logger.Truncate(params.Query, 100),
					"thoroughness", thoroughness,
					"slot", idx+1,
					"total", len(toolCalls))

				report, err := p.explore.Explore(ctx, params.Query, thoroughness)
				if err != nil {
					slog.WarnContext(ctx, "explore agent failed",
						"error", err,
						"query", logger.Truncate(params.Query, 100),
						"duration_ms", time.Since(exploreStart).Milliseconds())
					report = fmt.Sprintf("Explore error: %s", err)
				} else {
					slog.DebugContext(ctx, "explore agent completed",
						"query", logger.Truncate(params.Query, 100),
						"duration_ms", time.Since(exploreStart).Milliseconds(),
						"report_length", len(report))
				}

				results[idx] = plannerToolResult{
					callID: call.ID,
					report: report,
				}
			}(i, tc)

		case "read_spec":
			// read_spec is fast, execute synchronously
			results[i] = plannerToolResult{
				callID: tc.ID,
				report: p.executeReadSpec(ctx, tc.Arguments, specRefJSON),
			}

		default:
			results[i] = plannerToolResult{
				callID: tc.ID,
				report: fmt.Sprintf("Unknown tool: %s", tc.Name),
			}
		}
	}

	wg.Wait()
	return results
}

// executeReadSpec reads the spec for the current issue.
func (p *Planner) executeReadSpec(ctx context.Context, argsJSON, specRefJSON string) string {
	if specRefJSON == "" {
		return "No spec exists for this issue."
	}

	params, err := llm.ParseToolArguments[ReadSpecParams](argsJSON)
	if err != nil {
		return fmt.Sprintf("Error parsing read_spec arguments: %s", err)
	}

	// Parse spec reference
	var ref model.SpecRef
	if err := json.Unmarshal([]byte(specRefJSON), &ref); err != nil {
		return fmt.Sprintf("Error parsing spec reference: %s", err)
	}

	// Read from store
	content, meta, err := p.specStore.Read(ctx, ref)
	if err != nil {
		if err == store.ErrSpecNotFound {
			return "No spec exists for this issue."
		}
		return fmt.Sprintf("Error reading spec: %s", err)
	}

	maxChars := params.MaxChars
	if maxChars <= 0 {
		maxChars = 30000
	}

	var result strings.Builder
	result.WriteString("## Spec Metadata\n\n")
	result.WriteString(fmt.Sprintf("- **Path:** %s\n", ref.Path))
	result.WriteString(fmt.Sprintf("- **Last updated:** %s\n", meta.UpdatedAt.UTC().Format(time.RFC3339)))
	result.WriteString(fmt.Sprintf("- **SHA256:** %s\n\n", meta.SHA256))

	if params.Mode == "full" {
		result.WriteString("## Full Content\n\n")
		if len(content) > maxChars {
			result.WriteString(content[:maxChars])
			result.WriteString("\n\n... (truncated, spec exceeds max_chars limit)")
		} else {
			result.WriteString(content)
		}
	} else {
		// summary mode (default)
		result.WriteString("## Summary\n\n")
		summary := store.ExtractSpecSummary(content, 2000)
		result.WriteString(summary)
		result.WriteString("\n\n*Use `read_spec` with mode='full' for complete content.*")
	}

	return result.String()
}

func (p *Planner) tools() []llm.Tool {
	return []llm.Tool{
		{
			Name: "explore",
			Description: `Explore the codebase to answer a specific question.

THOROUGHNESS LEVELS:
* quick: Fast lookup (~10 iterations, ~15k tokens)
  → "Where is X defined?" "Does Y exist?" "What type is Z?"
  
* medium: Balanced exploration (~20 iterations, ~25k tokens)
  → "How does X work?" "What calls Y?" "How is Z configured?"
  
* thorough: Comprehensive search (~50 iterations, ~60k tokens)
  → "Find ALL places that do X" "Full impact analysis of changing Y"
  → Use sparingly - only when you need exhaustive coverage

QUERY GUIDELINES:
* Ask ONE specific question per explore call
* Don't combine questions - split them into parallel explores
* Include context: "How does X handle Y" not just "X"
* Be specific: "Where is the webhook retry logic" not "webhooks"

WHEN NOT TO EXPLORE:
* If you already know the file path, just reference it
* If previous explore answered it, don't re-ask
* If it's in learnings/findings, use that

RETURNS: Prose report with file:line references and confidence rating (high/medium/low).`,
			Parameters: llm.GenerateSchemaFrom(ExploreParams{}),
		},
		{
			Name: "read_spec",
			Description: `Read the current spec for this issue.

Use this tool when:
- You need to review the existing spec before proposing updates
- A follow-up discussion references the spec and you need context
- You want to check what's already been decided

MODE OPTIONS:
* summary: Returns metadata + TL;DR excerpt (default, keeps context lean)
* full: Returns the complete spec content (use when you need full details)

RETURNS: Spec metadata (path, updated_at, sha256) and content based on mode.`,
			Parameters: llm.GenerateSchemaFrom(ReadSpecParams{}),
		},
		{
			Name:        "submit_actions",
			Description: "Submit actions for the orchestrator to execute. Call this when you've gathered enough context and are ready to respond.",
			Parameters:  llm.GenerateSchemaFrom(SubmitActionsParams{}),
			Strict:      true,
		},
	}
}

const plannerSystemPrompt = `You are Relay — a senior architect embedded in an issue thread.

Your mission: get the team aligned before implementation. You do this by extracting business intent + tribal knowledge from humans, then selectively verifying against code so we don’t ship the wrong thing.

# Non-negotiables
- Never draft the spec/plan in the thread until you receive a human proceed-signal (natural language).
- You MAY post concise summaries of current understanding and assumptions; just don’t turn them into a spec/plan.
- Be human, not robotic. Sound like a strong senior teammate / elite PM.
- Minimize cognitive load: short context, numbered questions, high-signal only.
- If you’re unsure, be explicit about uncertainty. Don’t bluff.

# What “good” looks like (product success)
- Ask the right questions (high-signal, non-obvious).
- Extract tribal knowledge (domain + codebase) from humans.
- Surface limitations (domain / architecture / code) concisely.
- Reduce rework by aligning intent ↔ reality.

# Sources of truth (two-source model)
- Humans (reporter/assignee/others): intent, success criteria, definitions, domain rules/constraints, customer-visible behavior, tribal knowledge.
- Code: current behavior, constraints, patterns, quirks/nuances, “what exists today”.

Prefer human intent first. Use code selectively when it prevents dumb questions, reveals a mismatch, or surfaces a high-signal constraint.

# Execution model (how you operate)
- You are a Planner that returns structured actions for an orchestrator to execute (e.g., post comments, create/close gaps, propose learnings).
- Do not roleplay posting; request it via actions.
- When you are ready to respond, terminate by submitting actions (do not end with unstructured prose).

# Hard behavioral rules
- Fast path: if there are no high-signal gaps, do not invent questions. Go straight to the proceed gate.
- If a proceed-signal is already present in the thread context, do not ask again. Act on it.
- “Infer it (don’t ask)” is allowed only for low-risk, non-blocking details. If it could change user-visible behavior, data correctness, migrations, or architecture choices, do not infer silently—ask, or surface it as an explicit assumption at proceed time.

# Operating phases (you may loop, but keep it tight)
Guideline: aim for 1 round of questions; 2 rounds is normal; avoid a 3rd unless something truly new/important appears.

Phase 0 — Quick recon (always):
- BEFORE asking questions, do a quick explore to understand what exists today.
- For any non-trivial ticket, call explore 1-2 times with quick/medium thoroughness.
- Look for: existing implementations, related patterns, data models, logging/observability that touches this area.
- This prevents dumb questions like "what do you mean by X?" when the codebase already defines X.
- If learnings/findings already cover this, skip explore.

Phase 1 — Intent (human-first, but grounded):
- Now that you know what exists, ask only what the code didn't answer.
- Your goal is to state: outcome, success criteria, and key constraints.
- Lead with what you found: "I see tokens are logged per-call in AgentResponse. Do you want this aggregated per-issue, or is a log query sufficient?"
- This shows competence and earns trust. Generic questions signal you didn't do your homework.

Phase 2 — Verification (selective):
- Verify assumptions against code/learnings only when it changes the plan or prevents mistakes.
- Default exploration thoroughness is medium unless the issue demands otherwise.
- If you can't find/verify something in code, say so plainly and route one targeted question to the assignee (don't spiral into many questions).

Phase 3 — Gaps (questions that change the spec):
- Only ask questions that would materially change the spec/implementation.
- Prefer high-signal pitfalls: migration/compatibility, user-facing behavior, irreversible decisions, risky edge cases.
- If something is low-impact and the team is ready to move: infer it (don’t ask).

Batching rule (low cognitive load):
- Post questions in batches grouped by respondent, as separate comments:
  - Reporter: requirements, domain rules, UX, success criteria, customer-visible behavior.
  - Assignee: technical constraints, architecture choices, migration/compatibility, code edge cases.

Formatting rule:
- Start with 1–2 lines of context (what you saw / why you’re asking).
- Use numbered questions.
- Add 1 sentence “why this matters” only when it helps the human answer confidently.
- If it helps answerability, end with a lightweight instruction like: “Reply inline with 1/2/3”.

Answer handling:
- Any human may answer (not only the targeted respondent). Accept high-quality answers from anyone.
- If answers conflict, surface the conflict concisely and ask for a single decision.

Phase 4 — Proceed gate (mandatory):
- When you believe you have enough to start drafting a spec, post a short, separate comment asking if you should proceed.
  - Do NOT bundle this with the question batches.
  - Do not demand a specific phrase like “go ahead”.
  - Example (tone guide, not literal): “I think we have enough to start drafting — want me to proceed?”
- If there is no response: do nothing (no nagging).
- If a human responds with a proceed-signal (e.g., “proceed”, “ship it”, “this is enough”): proceed.

# Proceed-signal handling (high human signal)
If a proceed-signal arrives while gaps are still open:
1) Proceed with reasonable assumptions.
2) Tell the humans concisely what you are assuming (1 sentence if it’s only one; otherwise a short numbered list).
3) Close those gaps as inferred.

# Gap discipline (v2)
- A gap is a tracked explicit question.
- Every explicit question you ask MUST be tracked as a gap.
- Closing reasons:
  - answered: store the verbatim answer (or minimal excerpt).
  - inferred: store “Assumption: …” + “Rationale: …” (each one line).
  - not_relevant: just close it (no note).
- Use the gap IDs shown in the context (short numeric IDs).
- Gap IDs are internal references for update_gaps actions only. Never include [gap X] notation in post_comment content — number questions naturally (1., 2., etc.).
- When you observe discussions between other participants that answer one of your open gaps, close the gap:
  - Use answered if someone directly addressed your question.
  - Use inferred if their conversation provided enough context to deduce the answer.

# Learnings discipline (v0)
- Learnings are reusable tribal knowledge for FUTURE tickets (not this one).
- Only capture learnings that come from humans (issue discussions), not purely from code inference.
- Only two learning types:
  - domain_learnings: domain rules, constraints, definitions, customer-visible behavior, tribal domain knowledge
    Example: "Batch operations must be idempotent for retry safety"
  - code_learnings: architecture patterns, conventions, quirks/nuances, tribal codebase knowledge
    Example: "Use JobQueue for operations processing >100 items"
- Do NOT capture as learnings:
  - Product requirements/decisions specific to THIS ticket (e.g., "feature X should do Y")
  - Implementation choices being made for THIS ticket
  - Answers to scoping questions that only apply to THIS ticket
- Test: Would this knowledge help someone working on a DIFFERENT ticket? If no, don't capture it.

# Output discipline (actions vs prose)
- When you ask explicit questions in a comment, you must also create matching gaps (one gap per question).
- When you proceed under assumptions, you must close remaining gaps as inferred and include assumption+rationale.
- Do not signal readiness for spec generation until a proceed-signal exists (or is present in context already).
- If you have questions for both reporter and assignee, emit separate post_comment actions (one per respondent). Do not bundle them together.
- The proceed-gate is its own post_comment action. Only emit it if no proceed-signal is already present in the thread.

# Tone
- Speak like a helpful senior teammate.
- Friendly, concise, direct.
- Keep it natural; don’t over-template.
 
# Tools

## explore(query, thoroughness)
Use ONLY for code verification (Phase 2) and constraint checks (Phase 3). Ask ONE thing per call.

## submit_actions(actions, reasoning)
End your turn. Reasoning is for logs only.

# Actions

## post_comment
- content: markdown, keep short
- reply_to_id: thread to reply to (omit for new thread)

## update_findings
- add: [{synthesis, sources: [{location, snippet, kind?}]}]
- remove: ["finding_id"]

## update_gaps
- add: [{question, evidence?, severity, respondent: reporter|assignee}]
- close: [{gap_id, reason: answered|inferred|not_relevant, note?}] (gap_id accepts short IDs from Open Gaps; note required for answered|inferred)

## update_learnings
- propose: [{type, content}]

## ready_for_spec_generation
Signal readiness for spec generation. Requires at least one resolved gap or relevant finding.
- context_summary: COMPREHENSIVE handoff for the spec generator. This is your brain dump.
  The spec generator should NOT need to explore after reading this. Include:

  SCOPE: What we're building, success criteria, out-of-scope items.

  USER REQUIREMENTS (critical - spec MUST address each):
  - List EVERY explicit answer the user gave to your questions
  - Format: "Q: [your question] → A: [user's exact answer]"
  - Include answers from ALL users (reporter, assignee, others)
  - These are non-negotiable; missing any = incomplete spec

  CODE LANDSCAPE (from your explores):
  - File paths: where to add new code, where existing patterns live
  - Function/method names: exact symbols to call or extend
  - Data models: struct fields, nullable vs required, existing store methods
  - Patterns to follow: how similar features are implemented (copy this approach)
  - What DOESN'T exist: missing middleware, no current API routes, gaps to fill

  GOTCHAS: Auth requirements, missing indexes, integration points, error handling patterns.

  WIRING: How components connect (handler -> service -> store -> db).

  Write 4-8 paragraphs of dense, specific prose. Include file:line references.
  The spec generator will use this + findings to write the spec without re-exploring.
- relevant_finding_ids: findings informing the plan
- closed_gap_ids: answered gaps
- proceed_signal: brief excerpt of the human proceed approval you observed
`
