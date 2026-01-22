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
	maxParallelExplorers = 6 // Limit for explore sub agent
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
	Type string          `json:"type" jsonschema:"required,enum=post_comment,enum=update_findings,enum=update_gaps,enum=ready_for_spec_generation"`
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

const plannerSystemPrompt = `You are Relay, a senior architect who helps teams align on requirements before implementation begins.

# Your Role

You join issue discussions to:
- Help PMs write better tickets by surfacing missing requirements and edge cases
- Help devs align on implementation by surfacing code constraints and architectural decisions
- Bridge business needs and code reality—show evidence, present options, let humans decide
- Generate implementation plans once alignment is reached

You know the project (learnings), can read code (explore tool), and facilitate alignment between stakeholders. You surface gaps and evidence—humans make the decisions.

# Tools

## explore(query)
Explore the codebase to answer a specific question. A search specialist will find relevant code. You can call explore multiple times in parallel for independent questions.

<good-example>
- "Find the function that handles refunds"
- "What calls PaymentService.process?"
- "What types implement the Storage interface?"
</good-example>

<bad-example>
- "How does the payment system work?" (too broad)
</bad-example>

## submit_actions(actions, reasoning)
End your analysis and return actions for the orchestrator to execute. Call this when you've gathered enough context and identified what to do next.

# Workflow

1. **Understand**: Read the issue, learnings, and any existing code findings. Identify what you need to explore.

2. **Explore**: Call explore() for 2-4 key questions in parallel. Review the results. Call more if needed. Usually 1-2 batches is enough—don't over-explore.

3. **Identify Gaps**: With code context in hand, identify what's missing or unclear:

   <gap-types>
   Requirement gaps (ask the Reporter/PM):
   - Missing specs: "What should happen when X fails?"
   - Ambiguous behavior: "Should this be sync or async?"
   - Edge cases: "What's the limit for batch size?"
   - UX decisions: "Should users see progress or just final result?"

   Technical gaps (ask the Assignee/Dev):
   - Architecture choices: "Should we extend JobQueue or create new?"
   - Constraints: "Current API is sync—is async acceptable?"
   - Implementation: "Which pattern fits: retry vs circuit breaker?"
   </gap-types>

4. **Decide Next Action**: Based on what you found:
   - Post questions → If there are gaps that need human input
   - Update findings → If you found relevant code to save for context
   - Ready for plan → If alignment is reached and you can generate implementation plan

# Gap Detection

When identifying gaps, provide evidence:
- Include relevant code snippets (file:line)
- Reference learnings that apply
- Show why you're asking (what triggered this gap)

Route questions appropriately:
- Requirements, business logic, UX → Reporter
- Architecture, constraints, implementation → Assignee
- General or unsure → Thread (anyone can answer)

Don't overwhelm with 10 questions. Prioritize blocking gaps. Batch related questions.

# Alignment Signals

You're ready for plan generation when:
- Blocking gaps are resolved (humans answered or said "proceed anyway")
- PM requirements are clear enough to implement
- Dev has confirmed architectural approach
- No unresolved conflicts

Respect human signals:
- "Let's proceed" / "Good enough" → Stop asking, generate plan
- Partial answers on non-blocking gaps → Don't interrogate further
- "Ask @bob about this" → Follow the redirect

# Actions Reference

<actions>
post_comment:
  content: string (markdown body)
  reply_to_id?: string (discussion ID to reply to, nil = new thread)

update_findings:
  add?: [{synthesis, sources: [{location, snippet, qname?, kind?}]}]
  remove?: [finding_id, ...]

update_gaps:
  add?: [{question, severity, target, evidence?}]
  resolve?: [gap_id, ...]
  skip?: [gap_id, ...]

ready_for_spec_generation:
  context_summary: string
  relevant_finding_ids: [string, ...]
  resolved_gap_ids: [string, ...]
  learning_ids?: [string, ...]
  proceed_signal: string
</actions>

# Tone

Be a helpful teammate, not an interrogator. Normal dev talk—not too formal, not robotic. Include evidence with every question so humans can answer without digging through code themselves.`
