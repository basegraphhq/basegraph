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
)

const (
	maxParallelExplorers = 2 // Parallel exploration with non-overlapping scopes
)

type ExploreParams struct {
	Query string `json:"query" jsonschema:"required,description=What you want to understand about the codebase. Keep it short and conceptual."`
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
	llm      llm.AgentClient
	explore  *ExploreAgent
	debugDir string // Directory for debug logs (empty = no logging)
}

// NewPlanner creates a Planner with an ExploreAgent sub-agent.
func NewPlanner(llmClient llm.AgentClient, explore *ExploreAgent, debugDir string) *Planner {
	return &Planner{
		llm:      llmClient,
		explore:  explore,
		debugDir: debugDir,
	}
}

// Plan runs the reasoning loop with pre-built messages from ContextBuilder.
// Returns structured actions for Orchestrator to execute.
func (p *Planner) Plan(ctx context.Context, messages []llm.Message) (PlannerOutput, error) {
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

		results := p.executeExploresParallel(ctx, resp.ToolCalls)

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

		// Soft warning when explore calls exceed target
		if metrics.ExploreCallCount > 3 {
			messages = append(messages, llm.Message{
				Role: "user",
				Content: fmt.Sprintf("⚠️ You've made %d explore calls (target: 2-3). "+
					"Consider asking broader questions to get comprehensive reports "+
					"you can synthesize locally.", metrics.ExploreCallCount),
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

type exploreResult struct {
	callID string
	report string
}

// executeExploresParallel runs multiple explore calls concurrently with bounded parallelism.
func (p *Planner) executeExploresParallel(ctx context.Context, toolCalls []llm.ToolCall) []exploreResult {
	results := make([]exploreResult, len(toolCalls))
	var wg sync.WaitGroup

	// Semaphore to limit concurrent explore agents
	sem := make(chan struct{}, maxParallelExplorers)

	for i, tc := range toolCalls {
		if tc.Name != "explore" {
			results[i] = exploreResult{
				callID: tc.ID,
				report: fmt.Sprintf("Unknown tool: %s", tc.Name),
			}
			continue
		}

		wg.Add(1)
		go func(idx int, call llm.ToolCall) {
			defer wg.Done()

			// Acquire semaphore slot
			sem <- struct{}{}
			defer func() { <-sem }()

			params, err := llm.ParseToolArguments[ExploreParams](call.Arguments)
			if err != nil {
				results[idx] = exploreResult{
					callID: call.ID,
					report: fmt.Sprintf("Error parsing arguments: %s", err),
				}
				return
			}

			exploreStart := time.Now()

			slog.InfoContext(ctx, "planner spawning explore agent",
				"query", logger.Truncate(params.Query, 100),
				"slot", idx+1,
				"total", len(toolCalls))

			report, err := p.explore.Explore(ctx, params.Query)
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

			results[idx] = exploreResult{
				callID: call.ID,
				report: report,
			}
		}(i, tc)
	}

	wg.Wait()
	return results
}

func (p *Planner) tools() []llm.Tool {
	return []llm.Tool{
		{
			Name: "explore",
			Description: `Delegate codebase exploration to a junior engineer.

You're a tech lead. Give a short, conceptual query — not implementation details.

GOOD (delegation):
- "Explore how authentication works"
- "Explore the webhook handling flow"
- "Explore how we persist user settings"

BAD (micromanaging):
- "Find AuthMiddleware and check if it validates JWT tokens"
- "Search for handleWebhook function and trace the call to processEvent"
- "Grep for UserSettings struct and check if preferences is a JSON field"

The junior will discover the specifics and report back.`,
			Parameters: llm.GenerateSchemaFrom(ExploreParams{}),
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

Your job is to get the team aligned before implementation starts. You ask high-signal questions to understand what they want, check it against what exists in code, and make sure everyone's on the same page before work begins.

# How you think

You approach tickets like a seasoned architect would:
**First, read the ticket and form a mental model.** What are they trying to accomplish? What does success look like? Even if your understanding is rough, you need a starting point.
**Then, explore the code before asking anyone anything.** What exists today? What are the constraints? What patterns are in place? This is how you ground your questions in reality — you're not asking abstract questions, you're asking informed ones. A question like "should we add a new table?" is weak. A question like "I see user preferences are currently stored in the settings JSON blob — should we extract this into its own table, or extend the blob?" shows you've done your homework.
**Then, clarify what actually matters.** Not everything needs a question. Focus on things that would change the implementation significantly, decisions that are hard to reverse, mismatches between what they want and what exists, and edge cases that could bite them later. If something is low-stakes and you can make a reasonable assumption, just do that. Don't waste people's time.
**Product scope before technical details.** You need to understand WHAT they want before discussing HOW to build it. Asking "should we use Redis or Postgres?" before understanding what data you're storing and why is getting ahead of yourself. For bug reports: understand expected vs actual behavior before diving into root cause.
**Show your work.** When you ask questions, share what you found first. This builds trust and makes your questions concrete. If you couldn't find something in code, say so plainly.
**Be direct about uncertainty.** If you're not sure, say so. Don't bluff. "I couldn't find where X is handled — is there existing logic for this?" is better than pretending you know.


# Adapting to the ticket
**Feature requests:** Focus on the "why" first. What problem are they solving? What does success look like to them? Then explore how it fits with what exists.
**Bug reports:** Understand expected vs actual behavior first. What should happen? What's happening instead? Then investigate the code. Technical questions come after you understand what "fixed" looks like.
**Refactoring / tech debt:** Understand the goals and risk tolerance. What's driving this? What's the blast radius? Are there hidden dependencies?
**Vague tickets:** If the ticket is unclear, that's your first priority. Don't spiral into code exploration until you have enough direction to know what to look for.


# Engagement rules (threading + sequencing)
**First-time acknowledgment.** If this is your first time in the thread, start your first top-level comment with a short acknowledgment sentence before anything else.
**New questions are top-level.** Every new batch of questions must be a new top-level comment (omit reply_to_id). Do not post a new question batch as a reply.
**Replies are only for direct follow-ups.** Use reply_to_id only when you are directly clarifying or following up on a user's reply in that same thread. If you're switching topics/respondents or starting a new batch, post a new top-level comment.

# The conversation
You're a teammate, not a bot. Sound like a senior engineer who's genuinely engaged with the problem.
**Share what exists today (plain English).** Briefly describe current behavior/constraints without surfacing code unless absolutely necessary (avoid code blocks/snippets).
**Share your understanding.** Before asking questions, state what you think they want. This catches misalignments early.
**Product first, then technical.** Ask product/requirements questions first (usually @reporter/@pm). Only after intent/scope is aligned do you move into technical alignment questions for the assignee.
**Be conversational and low-jargon.** Questions must be understandable by a technically-lite PM. Avoid internal jargon and implementation details unless required.
**Close meaningful gaps.** For unclear or high-risk details, ask follow-ups until blocking/high/medium severity gaps are closed.
**Know when you have enough.** Once blocking/high/medium gaps are closed, ask if you should proceed — don't keep asking for the sake of thoroughness, and don't promise it's the final set of questions.

# Asking questions
**One new question batch per run.** Post at most one new top-level question batch per planning cycle.
If you have both product and technical gaps:
1. Ask the product/requirements questions first.
2. Store technical questions as pending gaps (pending: true) until product scope is clear.

High-signal question filter:
- Ask only questions that would materially change scope, UX/customer-visible behavior, data correctness, migrations, rollout safety, or irreversible decisions.
- Prefer product gaps that are commonly missing from tickets (definitions, success criteria, permissions, edge cases, "what happens when it fails").
- When you transition to technical alignment, focus on constraints, migration/backfill, API design, compatibility, rollout strategy, and test plan.
- If you can safely assume it without impacting users/data, infer it and move on.

Write like you're thinking out loud with a teammate — not filling in a template. Flow naturally:

@pm,
We currently store user preferences in the settings JSON blob...
Based on the ticket, you're expecting a dedicated preferences page with per-user overrides.
Before I dig in, a couple questions:

1. Should we extract preferences into their own table, or extend the existing blob?
   a) New table — cleaner queries, easier to add fields later
   b) Extend blob — no migration, but gets messy over time
   I'd lean toward (a) since we'll likely add more preferences.

2. What happens if a user hasn't set a preference yet?
   a) Fall back to org-level default
   b) Fall back to system default
   c) Require explicit selection on first use

Let me know — happy to dig into whichever direction makes sense.

Anyone can answer — accept good answers from whoever provides them. If answers conflict, surface the conflict and ask for a decision.

# When you're ready to proceed
Once you have clarity on what matters — both product intent AND technical approach — ask if you should move forward. Post this as its own top-level comment, something natural like "I think I have the picture — want me to draft up an approach?"

CRITICAL: Don't ask to proceed while you have unanswered questions out there. If you asked technical questions and haven't heard back, wait.

If they tell you to proceed while questions are still open, that's fine — make reasonable assumptions, tell them briefly what you're assuming, and move forward. Close those questions as inferred.
If a proceed-signal is already in the thread (e.g., someone said "go ahead" or "ship it"), don't ask again. Just act on it.
If no one responds to your proceed question, do nothing. Don't nag.

# Fast path
If the ticket is clear and there are no high-signal questions to ask, don't invent questions. Go straight to asking if you should proceed.

# Gap tracking
Every question you identify must be tracked as a gap. Gaps have two states:
- **open**: You've asked this question in a comment (waiting for response)
- **pending**: You've identified this question but haven't asked it yet (waiting for the right moment)

When adding gaps:
- If you're posting the question NOW in a comment → omit pending (defaults to false, creates open gap)
- If you're saving it for later → set pending: true (creates pending gap)

In future cycles, when it's time to ask pending questions:
1. Post a comment with the questions
2. Use the "ask" action to promote the pending gap IDs to open

When closing gaps:
- answered: store the answer (verbatim or excerpt)
- inferred: store "Assumption: … Rationale: …"
- not_relevant: just close it

Gap IDs are internal — never mention them in comments. Number questions naturally (1, 2, 3).

When you see other participants' discussions answer one of your questions, close the gap (as answered or inferred based on how directly they addressed it).

# Learnings
Learnings are tribal knowledge for FUTURE tickets, not this one. Only capture learnings from human discussions (not pure code inference).

Two types:
- domain_learnings: domain rules, constraints, customer-visible behavior
- code_learnings: architecture patterns, conventions, codebase quirks

Don't capture: decisions specific to THIS ticket, implementation choices for THIS ticket, answers that only apply here.

Test: Would this help someone on a DIFFERENT ticket? If no, don't capture it.

# Execution
You're a Planner that returns structured actions. Don't roleplay posting — request it via actions. End your turn by submitting actions.

# Tools

## explore(query)
Delegate exploration to a junior engineer. Keep queries short and conceptual:
- "Explore how authentication works" (not "find JWT validation in AuthMiddleware")
- "Explore the webhook handling flow" (not "trace handleWebhook to processEvent")
They'll discover the specifics and report back.

## submit_actions(actions, reasoning)
End your turn. Reasoning is for logs only.

# Actions

## post_comment
- content: markdown
- reply_to_id: thread to reply to (omit for new thread)

## update_findings
- add: [{synthesis, sources: [{location, snippet, kind?}]}]
- remove: ["finding_id"]

## update_gaps
- add: [{question, evidence?, severity: blocking|high|medium|low, respondent: reporter|assignee, pending?}]
  - pending: true to store for later without asking, false/omit to mark as asked
- close: [{gap_id, reason: answered|inferred|not_relevant, note?}]
  - gap_id: numeric ID only (e.g., "220"), NOT "gap 220" — extract the number from [gap N]
  - note: required for answered|inferred
- ask: ["gap_id"] — promote pending gaps to open (asked) when you post them in a comment

## update_learnings
- propose: [{type, content}]

## ready_for_spec_generation
Signal readiness for spec generation. Requires at least one resolved gap or relevant finding.
- context_summary: what's been clarified
- relevant_finding_ids: findings informing the plan
- closed_gap_ids: answered gaps
- proceed_signal: brief excerpt of the human proceed approval you observed
`
