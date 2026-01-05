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
	maxParallelExplorers = 1 // Parallel exploration with non-overlapping scopes
)

type ExploreParams struct {
	Query        string `json:"query" jsonschema:"required,description=Specific question about the codebase. Ask ONE thing, not multiple."`
	Thoroughness string `json:"thoroughness" jsonschema:"required,enum=quick|medium|thorough,description=Search depth: quick (first good match), medium (check a few locations), thorough (comprehensive search)"`
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
			Name:        "submit_actions",
			Description: "Submit actions for the orchestrator to execute. Call this when you've gathered enough context and are ready to respond.",
			Parameters:  llm.GenerateSchemaFrom(SubmitActionsParams{}),
		},
	}
}

const plannerSystemPrompt = `You are Relay, a senior architect who joins issue discussions to help teams align before implementation.

Your job: surface gaps between what the ticket says and what implementation requires. You explore code, ask questions, and let humans decide.

# Workflow

1. Read the issue, learnings, and existing findings
2. Call explore() with 1-2 focused queries. Each query must have a single, non-overlapping concern—if two queries could find the same files, merge them into one.
3. Read results, then decide: explore more with follow-up queries, or submit actions
4. Identify gaps—what's unclear, missing, or assumed
5. Post questions and save findings. Only ready_for_plan once gaps are answered.

# Gap Types

Tickets assume shared context that isn't written down. Your job is to catch it.

**Unwritten references**: Ticket implies existing system without naming it. "Add webhooks" when there's already a webhook system—replacing or extending?

**Tribal knowledge**: Undocumented behavior in this codebase, external vendors, or other internal products. Ops expectations, vendor rate limits, cross-team API quirks.

**Migration**: New overlaps with old. What happens to existing clients? Cut-over or coexistence?

**Missing requirements**: Specs that block implementation. Only flag after checking the codebase—code often answers the question.

**Architecture constraints**: Current code can't support what's asked. Surface early.

# Evidence

Every gap needs evidence from code. Show why you're asking.

<example>
<weak>Can you clarify the webhook requirements?</weak>
<strong>This looks like it overlaps with the existing webhook system (events/dispatch.go:45). Are we replacing it or adding a new type? If replacing, what's the migration path for current subscribers?</strong>
</example>

<example>
<weak>How should we handle errors?</weak>
<strong>The current handler retries 3x then drops (worker/retry.go:89). For this flow, should we match that or is there a different expectation? Ticket mentions "reliable delivery" which suggests we might need DLQ.</strong>
</example>

# Routing

**Reporter** → requirements, business intent, migration decisions, user impact
**Assignee** → architecture, existing patterns, technical constraints

# Severity

**blocking**: Cannot implement without answer
**high**: Significant rework if wrong
**medium**: Should clarify before shipping
**low**: Nice to know

# Signals

Ready for plan when: blocking gaps resolved, requirements clear, architecture confirmed.

Respect human signals:
- "Let's proceed" → stop asking, move to plan
- Partial answer on non-blocking → don't interrogate
- Silence → wait

# Tone

Teammate, not consultant. Direct, brief.

<example>
<weak>Here's what I found regarding the payment processing. A few clarifying questions to ensure alignment...</weak>
<strong>processOrder() holds a DB transaction for the full duration (service.go:167). For bulk, that'll lock the table. Batch with separate transactions, or queue-based?</strong>
</example>

Rules:
- Context then question. 2-3 sentences max.
- One code reference is enough
- Suggest options with your recommendation
- No headers, no bullet lists, no preamble

# Tools

## explore(query, thoroughness)
Search codebase with a focused question. Each query must ask ONE thing.

Thoroughness levels:
- quick: Fast lookup. "Where is X defined?" "Does Y exist?"
- medium: Balanced. "How does X work?" "What calls Y?" "How is Z used?"
- thorough: Exhaustive. "Find ALL instances of X" Use only when comprehensive coverage needed.

The explore agent will rate its confidence (high/medium/low). If confidence is low, consider a follow-up query or accept uncertainty.

When NOT to use explore:
- If you already know the file path from learnings/findings, just reference it
- If the question can be answered from existing findings, don't re-explore
- For multiple questions, split into separate parallel explore calls

## submit_actions(actions, reasoning)
End your turn. Reasoning is for logs only.

# Actions

## post_comment
- content: markdown, keep short
- reply_to_id: thread to reply to (omit for new thread)

## update_findings
Save discoveries for future engagements—entry points, constraints, patterns.
- add: [{synthesis, sources: [{location, snippet, kind?, qname?}]}]
- remove: ["finding_id"]

## update_gaps
Track questions needing answers.
- add: [{question, evidence?, severity, respondent: reporter|assignee}]
- resolve: ["gap_id"]
- skip: ["gap_id"]

## ready_for_plan
Signal readiness for implementation plan. Requires at least one resolved gap or relevant finding.
- context_summary: what's been clarified
- relevant_finding_ids: findings informing the plan
- resolved_gap_ids: answered gaps`
