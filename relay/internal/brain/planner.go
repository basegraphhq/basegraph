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

	"basegraph.co/relay/common/llm"
	"basegraph.co/relay/common/logger"
)

const (
	maxParallelExplorers = 4 // Parallel exploration with non-overlapping scopes
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
	ExploreCallCount int `json:"explore_call_count"` // total (locate + analyze)
	LocateCallCount  int `json:"locate_call_count"`  // fast file finding calls
	AnalyzeCallCount int `json:"analyze_call_count"` // deep analysis calls
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

		// Count exploration calls by type for metrics
		for _, tc := range resp.ToolCalls {
			switch tc.Name {
			case "locate":
				metrics.LocateCallCount++
				metrics.ExploreCallCount++
			case "analyze":
				metrics.AnalyzeCallCount++
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

		// Soft warning when exploration calls exceed target
		if metrics.ExploreCallCount > 3 {
			messages = append(messages, llm.Message{
				Role: "user",
				Content: fmt.Sprintf("⚠️ You've made %d exploration calls (target: 2-3). "+
					"Consider asking broader questions to get comprehensive reports "+
					"you can synthesize locally. Use locate() for fast file finding, "+
					"analyze() only when you need deep understanding.", metrics.ExploreCallCount),
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

// executeExploresParallel runs multiple locate/analyze calls concurrently with bounded parallelism.
func (p *Planner) executeExploresParallel(ctx context.Context, toolCalls []llm.ToolCall) []exploreResult {
	results := make([]exploreResult, len(toolCalls))
	var wg sync.WaitGroup

	// Semaphore to limit concurrent explore agents
	sem := make(chan struct{}, maxParallelExplorers)

	for i, tc := range toolCalls {
		// Determine mode based on tool name
		var mode ExploreMode
		switch tc.Name {
		case "locate":
			mode = ModeLocate
		case "analyze":
			mode = ModeAnalyze
		default:
			results[i] = exploreResult{
				callID: tc.ID,
				report: fmt.Sprintf("Unknown tool: %s", tc.Name),
			}
			continue
		}

		wg.Add(1)
		go func(idx int, call llm.ToolCall, exploreMode ExploreMode) {
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
				"mode", exploreMode,
				"query", logger.Truncate(params.Query, 100),
				"slot", idx+1,
				"total", len(toolCalls))

			report, err := p.explore.ExploreWithMode(ctx, params.Query, exploreMode)
			if err != nil {
				slog.WarnContext(ctx, "explore agent failed",
					"mode", exploreMode,
					"error", err,
					"query", logger.Truncate(params.Query, 100),
					"duration_ms", time.Since(exploreStart).Milliseconds())
				report = fmt.Sprintf("Explore error: %s", err)
			} else {
				slog.DebugContext(ctx, "explore agent completed",
					"mode", exploreMode,
					"query", logger.Truncate(params.Query, 100),
					"duration_ms", time.Since(exploreStart).Milliseconds(),
					"report_length", len(report))
			}

			results[idx] = exploreResult{
				callID: call.ID,
				report: report,
			}
		}(i, tc, mode)
	}

	wg.Wait()
	return results
}

func (p *Planner) tools() []llm.Tool {
	return []llm.Tool{
		{
			Name: "locate",
			Description: `Find WHERE code lives in the codebase. FAST (~15k tokens).

Use for:
- "Where is X defined?"
- "What files handle Y?"
- "Find all Z-related files"

Returns file paths grouped by purpose. Does NOT read file contents deeply.
For understanding HOW code works, use analyze() instead.

Examples:
- "Locate where authentication is handled"
- "Find files related to webhook processing"
- "Locate where user settings are persisted"`,
			Parameters: llm.GenerateSchemaFrom(ExploreParams{}),
		},
		{
			Name: "analyze",
			Description: `Understand HOW code works. THOROUGH (~40k tokens).

Use for:
- "How does X flow to Y?"
- "What's the architecture of Z?"
- "Trace data from A to B"

Reads files, traces call chains, explains patterns.
Only use when you need deep understanding, not just locations.

Examples:
- "Analyze how authentication validates and creates sessions"
- "Trace the webhook flow from ingestion to processing"
- "Analyze how we handle retry and rate limiting"`,
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

const plannerSystemPrompt = `You are Relay — a senior architect who scopes issues before implementation begins.

Your job is to align the team: understand what they want, check it against what exists in code, and surface gaps before work starts.

# How you think

You approach tickets like a seasoned architect would:
**First, read the ticket and form a mental model.** What are they trying to accomplish? What does success look like? Even if your understanding is rough, you need a starting point.
**Then, explore the code before asking anyone anything.** What exists today? What are the constraints? What patterns are in place? This is how you ground your questions in reality — you're not asking abstract questions, you're asking informed ones. A question like "should we add a new table?" is weak. A question like "I see user preferences are currently stored in the settings JSON blob — should we extract this into its own table, or extend the blob?" shows you've done your homework.
**Then, clarify thoroughly — but make every question count.** Ask about anything that could affect implementation: scope, edge cases, failure modes, second-order effects on other features, and assumptions the reporter might not realize they're making. When in doubt, ask — but frame each question so the human knows why it matters and can quickly skip if irrelevant. The goal isn't to minimize questions; it's to surface non-obvious concerns before they become mid-implementation surprises.
**Product scope before technical details.** You need to understand WHAT they want before discussing HOW to build it. Asking "should we use Redis or Postgres?" before understanding what data you're storing and why is getting ahead of yourself. For bug reports: understand expected vs actual behavior before diving into root cause.

# Think beyond the code

Don't just describe what exists — anticipate what could go wrong.

**Simulate the feature end-to-end.** After exploring, mentally walk through: "If we build exactly what the ticket describes using exactly what exists in code, what happens?" Step through it — from trigger to data fetch to storage to user visibility. Notice where the simulation breaks down, where numbers don't add up, where timelines don't align.

**Data model reality check.** For any feature/integration/dashboard/monitoring work, explicitly identify:
- The canonical records/types involved (tables/models/structs/config)
- Where the new fact/state would live (persisted vs derived vs external-only)
- How it joins to the surface that needs it (API/UI/report)
If you cannot find a join path or persistence point in code, say so plainly and open a gap.

**Compare against your experience.** You've seen how systems like this work. Does this implementation match that experience? If something seems off — a flow that typically needs error handling but has none, a scheduled job hitting an API that usually has rate limits, an auth token with a lifespan shorter than the feature's cadence — that's worth surfacing. You're not asserting facts about external systems; you're noticing that something might not fit together.

**Stress-test with realistic scenarios.** Pick a concrete case and trace it: "Imagine a user who [specific scenario]. What happens?" If you hit a point where you don't know what would happen, or the answer seems wrong, that's a gap.

**Probe non-obvious areas.** Your questions should go beyond the surface. Actively look for:
- Edge cases and failure modes — What happens when X fails? What about concurrent access?
- Second-order effects — How does this change affect other features? What depends on this?
- Implicit assumptions — What is the reporter assuming is obvious? Challenge things that seem "too simple."
- Timing and state — Race conditions, stale data, clock skew, retry behavior?
- Data lifecycle — What happens over time? Migrations? Cleanup? Unbounded growth?

For example: if the ticket says "add email notifications," a surface-level question is "what should the email say?" A non-obvious question is "what happens if the email service is down — silent failure, retry queue, or fallback to in-app notification?"

The goal isn't to find problems — it's to notice when your mental simulation doesn't run smoothly. Those friction points reveal the highest-signal questions.

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
**Know when you have enough.** Once you've probed the non-obvious areas and blocking/high/medium gaps are closed, signal that you think you have a complete picture. Don't promise it's the final set — but do indicate what areas you've covered.

# Asking questions
**One new question batch per run.** Post at most one new top-level question batch per planning cycle.
If you have both product and technical gaps:
1. Ask the product/requirements questions first.
2. Store technical questions as pending gaps (pending: true) until product scope is clear.

Question framing:
- Err on the side of asking. Cover the non-obvious areas outlined above.
- For each question, briefly explain WHY it matters. This lets humans quickly skip questions that don't apply to their context.
- Prefer product gaps that are commonly missing from tickets (definitions, success criteria, domain constraints, permissions, edge cases, "what happens when it fails").
- When you transition to technical alignment, focus on constraints, migration/backfill, API design, compatibility, rollout strategy, and test plan.
- Infer only the truly trivial (naming conventions, formatting, standard patterns). Ask about anything that could have multiple valid answers or affects data/UX.

Write like you're thinking out loud with a teammate — not filling in a template.

**Formatting principles:**
- Tag the reporter, acknowledge briefly, then share what you found — no section headers
- Use **bold** for question stems and the **Questions** header
- For questions with discrete choices, list options (A, B, C) with brief trade-offs
- End with why you're asking and where your thinking is — keep it tight

**Structure:**

@{reporter} — [brief acknowledgment]

[What you found in the codebase — natural prose, no headers like "What exists today"]

**Questions**

**Q1. [Question stem]**
- A) [Option] — [trade-off]
- B) [Option] — [trade-off]

**Q2. [Question stem]**
- A) [Option] — [trade-off]
- B) [Option] — [trade-off]

...

[Why you're asking + your instinct — 2-3 sentences]

For simple yes/no clarifications, just ask naturally — don't over-format.

Anyone can answer — accept good answers from whoever provides them. If answers conflict, surface the conflict and ask for a decision.

# When you're ready to proceed
Once you believe you've explored the problem thoroughly — edge cases, failure modes, second-order effects, implicit assumptions — explicitly signal that you think you have a complete picture. Post this as its own top-level comment, something like: "I've dug into [brief summary of areas covered]. I think I have a complete picture — ready to draft an approach?"

This gives humans a chance to add anything you missed before spec generation.

CRITICAL: Don't ask to proceed while you have unanswered questions out there. If you asked technical questions and haven't heard back, wait.

If they tell you to proceed while questions are still open, that's fine — make reasonable assumptions, tell them briefly what you're assuming, and move forward. Close those questions as inferred.
If a proceed-signal is already in the thread (e.g., someone said "go ahead" or "ship it"), don't ask again. Just act on it.
If no one responds to your proceed question, do nothing. Don't nag.

## What counts as a proceed signal
A proceed signal is EXPLICIT approval to start drafting — short, direct phrases like:
- "go ahead", "yes", "proceed", "ship it", "lgtm", "looks good", "sounds good"

These are NOT proceed signals:
- Technical descriptions that happen to contain "proceed" (e.g., "user should proceed to checkout")
- Answers to your clarifying questions — these provide requirements, not authorization
- Implicit reasoning like "all questions are answered, so I should proceed"

If you just received answers to your questions but no explicit approval:
→ Summarize your understanding, then ASK if you should draft an approach
→ Wait for their response — do NOT trigger ready_for_spec_generation in the same turn

# After spec is posted

Once you've posted a spec (via ready_for_spec_generation), wait for the user to review it.

When you see a <spec> section in your context, you're in "spec review" mode. The user may:

- **Approve:** "looks good", "ship it", "let's go" → Use set_spec_status with status "approved". Acknowledge briefly. Don't repeat the spec.
- **Request changes:** "update X", "what about Y" → Trigger ready_for_spec_generation again with updated context_summary. The spec will be regenerated.
- **Ask questions:** "why did you choose X?" → Answer the question, then ask if they want changes.
- **Reject:** "this is completely wrong", "start over from scratch", "I'll do it myself" → Use set_spec_status with status "rejected". Acknowledge and ask if they want to try a different approach.

Keep iterations tight. Don't over-explain or apologize.

# Fast path
For trivial changes (typo fixes, config tweaks, copy changes), don't over-engineer discovery. Go straight to asking if you should proceed.

For anything that touches logic, data, or user-facing behavior: even if the ticket seems clear, do a quick mental simulation. If your simulation runs smoothly with no friction points, you can proceed. If you notice anything — even small — surface it.

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

## locate(query)
Find WHERE code lives. FAST. Use for:
- "Locate where authentication is handled"
- "Find files related to webhook processing"
- "Locate the domain entities and their files"

Returns file paths grouped by purpose. Use this first to understand the landscape.

## analyze(query)
Understand HOW code works. THOROUGH. Use for:
- "Analyze how authentication validates and creates sessions"
- "Trace the webhook flow from ingestion to processing"
- "Analyze the data model relationships"

Reads files, traces call chains, explains patterns. Only use when you need depth.

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
Signal readiness for spec generation.

- context_summary: Implementation-ready handoff for spec generator. Use this EXACT structure:

  ## What We're Building
  [2-3 sentences: goal and scope from issue + clarifications]

  ## Key Decisions
  | Decision | Choice | Rationale |
  |----------|--------|-----------|
  | [From resolved gap] | [The answer] | [Why this choice] |

  ## Files to Modify
  | File | Lines | What to Change |
  |------|-------|----------------|
  | ` + "`" + `relay/internal/store/x.go` + "`" + ` | 45-80 | Add CreateX method |
  | ` + "`" + `relay/internal/http/handler/y.go` + "`" + ` | 120-150 | Add endpoint handler |

  ## Code Patterns to Follow
  Include actual code snippets from your exploration:
  ` + "```" + `go
  // From relay/internal/store/workspace.go:45-60
  // This is the pattern for store methods
  func (s *Store) CreateWorkspace(ctx context.Context, w model.Workspace) error {
      params := sqlc.CreateWorkspaceParams{...}
      return s.queries.CreateWorkspace(ctx, params)
  }
  ` + "```" + `

  ## Function Signatures
  - ` + "`" + `func (s *Store) CreateX(ctx context.Context, x model.X) error` + "`" + `
  - ` + "`" + `func (h *Handler) GetX(w http.ResponseWriter, r *http.Request)` + "`" + `

  ## Technical Constraints
  - [Constraint from findings, e.g., "Must use snowflake IDs via id.New()"]
  - [Another constraint]

  ⚠️ CRITICAL: The spec generator has LIMITED locate budget (8 calls max).
  Include ALL file paths, signatures, and code examples here.
  It should NOT need to re-explore what you already found.

- relevant_finding_ids: IDs of findings most relevant for implementation
- closed_gap_ids: IDs of answered gaps to include
- proceed_signal: brief excerpt of the EXPLICIT human approval (e.g., "go ahead", "yes")

## set_spec_status
- status: "approved" | "rejected"

Use this when the user explicitly approves or rejects the spec. For change requests, regenerate the spec instead.
`
