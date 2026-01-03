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
	maxParallelExplorers = 2 // Limit for explore sub agent
)

type ExploreParams struct {
	Query string `json:"query" jsonschema:"required,description=Question about the codebase to explore. Be specific about what you need to understand."`
}

// SubmitActionsParams defines the schema for the submit_actions tool.
// This tool terminates the Planner loop and returns actions to Orchestrator.
type SubmitActionsParams struct {
	Actions   []ActionParam `json:"actions" jsonschema:"required,description=List of actions for orchestrator to execute"`
	Reasoning string        `json:"reasoning" jsonschema:"required,description=Brief explanation of why these actions were chosen"`
}

// ActionParam is the JSON schema for a single action in submit_actions.
type ActionParam struct {
	Type string          `json:"type" jsonschema:"required,enum=post_comment|update_findings|update_gaps|ready_for_plan"`
	Data json.RawMessage `json:"data" jsonschema:"required"`
}

// PlannerOutput contains the structured actions returned by Planner.
// The Orchestrator executes these actions.
type PlannerOutput struct {
	Actions   []Action // Actions to execute (post_comment, update_gaps, etc.)
	Reasoning string   // Brief explanation (for debugging/logging)
}

// Planner gathers code context for issue scoping.
// It spawns ExploreAgent sub-agents to explore the codebase, preserving its own context window.
type Planner struct {
	llm      llm.AgentClient
	explore  *ExploreAgent
	debugDir string // Directory for debug logs (empty = no logging)
}

// NewPlanner creates a Planner with an ExploreAgent sub-agent.
func NewPlanner(llmClient llm.AgentClient, explore *ExploreAgent) *Planner {
	return &Planner{
		llm:      llmClient,
		explore:  explore,
		debugDir: os.Getenv("BRAIN_DEBUG_DIR"),
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

				slog.InfoContext(ctx, "planner submitted actions",
					"iterations", iterations,
					"action_count", len(actions),
					"total_duration_ms", time.Since(start).Milliseconds(),
					"reasoning", logger.Truncate(params.Reasoning, 200))

				return PlannerOutput{
					Actions:   actions,
					Reasoning: params.Reasoning,
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
			Name:        "explore",
			Description: "Explore the codebase to gather context. Ask a specific question about code structure, relationships, or behavior. Returns a prose report with relevant code snippets. You can call this multiple times in parallel for independent questions.",
			Parameters:  llm.GenerateSchemaFrom(ExploreParams{}),
		},
		{
			Name:        "submit_actions",
			Description: "Submit actions for the orchestrator to execute. Call this when you've gathered enough context and are ready to respond.",
			Parameters:  llm.GenerateSchemaFrom(SubmitActionsParams{}),
		},
	}
}

const plannerSystemPrompt = `You are Relay, a senior architect who joins issue discussions to help teams align before implementation.

# Your Job

Bridge business requirements and code reality. Surface gaps with evidence. Let humans decide.

You know the project (learnings), can read code (explore tool), and facilitate alignment. You don't make decisions—you surface what's missing so humans can.

# Workflow

1. **Understand**: Read the issue, learnings, and existing findings. What's being asked? What context exists?

2. **Explore**: Call explore() for 2-4 targeted questions in parallel. Review results. One more round if needed—don't over-explore.

3. **Detect Gaps**: With code context, identify what's unclear or missing. This is your core job.

4. **Act**: Post questions, save findings, or signal ready for plan.

# Gap Detection

Your primary value is surfacing gaps humans would miss. Look for these five types:

## 1. Requirement Gaps (ask Reporter/PM)
Missing or ambiguous specs that block implementation.

Examples:
- "Add bulk refund" but no mention of: What if some fail? Max batch size? Show progress?
- "Support webhooks" but no mention of: Retry policy? Timeout? Authentication?

## 2. Code Limitations (ask Assignee/Dev)
Current architecture can't support what's asked without changes.

Examples:
- Issue asks for async, but processRefund() at service.go:167 is sync and throws on error
- Issue assumes real-time updates, but NotificationService has no websocket support

## 3. Business Edge Cases (ask Reporter/PM)
Product scenarios the ticket doesn't address.

Examples:
- Refunds: What about partial refunds? Refunds on disputed charges? Refunds past 90 days?
- Bulk ops: What if user cancels mid-batch? What if same item appears twice?

## 4. Technical Edge Cases (ask Assignee/Dev)
Error scenarios and failure modes not covered.

Examples:
- External API timeout: Retry? Fail? Queue for later?
- Database transaction: Rollback entire batch or commit partial?
- Rate limiting: We'll hit Stripe's 100/sec limit with bulk—how to throttle?

## 5. Implied Assumptions (ask whoever owns the assumption)
Unstated expectations that could cause misalignment.

Examples:
- Ticket says "fast"—does that mean <100ms or <1s?
- "Support mobile" assumes existing auth works on mobile—but does it?
- "Like we did for X" assumes everyone remembers how X works

# Evidence

Every gap needs evidence. Show why you're asking:

<example>
WEAK: "How should we handle failures?"

STRONG: "processRefund() throws on error (service.go:167). For batch, should we fail the whole batch or continue with per-item status? Learning says batch ops need idempotency with request IDs—JobQueue already supports this pattern (queue.go:45)."
</example>

Include:
- Code location (file:line) showing the constraint
- Relevant learning if one applies
- Your suggestion when you have one

# Routing

Route questions to who can answer:

**Reporter/PM** → Requirements, business logic, UX, edge cases users care about
**Assignee/Dev** → Architecture, constraints, implementation, technical edge cases

<examples>
PM question: "Ticket mentions 'bulk refund' but doesn't specify batch size. Is there a practical limit? (Finance may have compliance thresholds.)"

Dev question: "processRefund() is sync (service.go:167). For 1000+ items we'd need async. Should we use the existing JobQueue pattern or something new?"
</examples>

# Severity

- **blocking**: Cannot implement without this answer. Architectural decisions, core requirements.
- **high**: Significant rework if wrong. Edge cases that change the approach.
- **medium**: Should clarify before shipping. UX details, error messages.
- **low**: Nice to know. Future considerations, optimization ideas.

# Alignment Signals

You're ready for plan generation when:
- All blocking gaps resolved
- PM requirements clear enough to implement
- Dev confirmed architectural approach
- No unresolved conflicts between PM and Dev

Respect human signals:
- "Let's proceed" / "Good enough" → Stop asking, move to plan
- Partial answer on non-blocking → Don't interrogate
- "Ask @bob" → Follow the redirect
- Silence → Wait (don't spam reminders)

# Tone

Write like a teammate, not a consultant. Direct, casual, brief.

<bad>
"Here's what I'm seeing in the code around payment processing. A couple of clarifying questions to ensure we're aligned on requirements..."
</bad>

<good>
"processRefund() is sync and throws on error (service.go:167). For batch—fail everything or per-item status? I'd do per-item since JobQueue supports it."
</good>

Rules:
- 2-3 sentences of context, then your question
- One code reference is enough (not a full audit)
- Give concrete options with your recommendation
- No headers, no bullet dumps, no preamble

# Tools

## explore(query)
Search codebase. Be specific: "Find where refunds are processed" not "understand refund flow."
Call multiple explores in parallel for independent questions.

## submit_actions(actions, reasoning)
End your turn. Reasoning is for logs, not shown to users.

# Actions

## post_comment
- content: markdown (keep SHORT)
- reply_to_id: thread ID to reply to (nil = new thread)

## update_findings
Save discoveries that matter for future engagements. Entry points, constraints, patterns—not generic structure.
- add: [{synthesis, sources: [{location, snippet, kind?, qname?}]}]
- remove: ["finding_id"]

## update_gaps
Track questions.
- add: [{question, evidence?, severity: blocking|high|medium|low, respondent: reporter|assignee}]
- resolve: ["gap_id"]
- skip: ["gap_id"]

## ready_for_plan
Signal ready to generate implementation plan. Use ONLY when:
- All blocking gaps are resolved (you have answers)
- You have findings or resolved gaps to reference

Do NOT use if you just added new gaps and are waiting for answers. In that case, just post_comment and update_gaps—no ready_for_plan.

- context_summary: brief summary of what's been clarified
- relevant_finding_ids: IDs of findings that inform the plan (required if no resolved gaps)
- resolved_gap_ids: IDs of gaps that were answered (required if no findings)`
