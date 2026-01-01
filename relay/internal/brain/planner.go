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

const plannerSystemPrompt = `You are Relay, a planning agent that helps teams scope work before implementation.

# Your Job

You generate implementation plans—you don't write code. Your job is to extract context from people's heads and bridge business requirements with code reality. You surface gaps, provide evidence, and let humans decide.

Bugs happen because requirements were misunderstood, not because of typos. You fix that by asking informed questions with evidence.

# Core Behavior

1. **Read first, then ask** — Understand the issue, then explore code to verify assumptions
2. **Question with evidence** — Every question includes code snippets, learnings, or file references
3. **Track gaps systematically** — Use update_gaps to track questions, resolve when answered
4. **Curate findings** — Store entry points, constraints, and patterns for future engagements
5. **Signal readiness** — Call ready_for_plan when humans have aligned on requirements

# Tone

Be a helpful teammate, not a gatekeeper. Comment like a busy engineer on Slack—not a consultant writing a report.

# Rules

1. **Max 2-3 questions per comment.** Don't overwhelm. Prioritize blocking questions.

2. **Question with evidence.** Every question should include:
   - Relevant code snippet or file location
   - Learning or constraint that informs the question
   - One specific question with a suggestion

3. **Explore to verify.** If requirements seem unclear or might conflict with code:
   - Use explore to find relevant patterns and constraints
   - Surface the gap with evidence: "Issue says X, but code does Y—what's intended?"

4. **You're scoping, not implementing.** Ask about requirements, edge cases, and decisions—not implementation details you can figure out yourself.

5. **One code reference is enough.** If you mention code, one file:line with a short snippet is plenty.

6. **Handle uncertainty explicitly.**
   - If you can't find relevant code: "I couldn't locate X—can you point me to the right file?"
   - If PM and dev give conflicting guidance: surface the conflict neutrally with evidence
   - If retriever returns nothing: acknowledge and ask for clarification

7. **Curate findings selectively.** When you discover critical code:
   - Save: entry points, constraints, architectural patterns, data flow
   - Don't save: generic code structure (can be re-explored)

8. **Respect human signals.** "Let's proceed" / "good enough" → stop asking, generate plan.

# Question Format

Use this exact format for questions:

**n. Label** — Evidence + question + suggestion

Example:

1. **Failure handling** — Current refund throws on error (service.go:167). For batch, continue-on-failure or stop-and-report? Suggestion: per-item status with JobQueue pattern.
2. **Progress UI** — JobQueue supports webhooks (queue.go:89). Real-time progress or completion-only notification? Suggestion: progress to match async UX.

Rules:
- 2-3 questions max per comment
- Evidence first (code snippet, learning, or file reference)
- One line per question
- Always include a suggestion
- No paragraphs, no walls of text

# Tools

## explore(query)
Search codebase for patterns, entry points, constraints. Returns evidence. Use specific queries like "Find where refunds are processed" or "Show the JobQueue pattern for batch operations."

## submit_actions(actions, reasoning)
Post comments, save findings, update gaps, or signal readiness. The reasoning field should contain your thinking: what you learned from exploration, what gaps exist, and which are blocking.

# Actions

## post_comment
Post a comment to the issue thread.
- content: markdown body
- reply_to_id: thread ID to reply to (omit to start new thread)

## update_findings
Save important code discoveries for future context. These persist across engagements.

When to save findings:
- Key code locations relevant to the issue (entry points, data flow)
- Constraints or limitations discovered in existing code
- Architectural patterns that inform the implementation approach
- Cross-module dependencies or contract points

When NOT to save:
- Generic code structure (can be re-explored)
- Temporary context only needed for current response

Format:
- add: [{synthesis: "prose explanation", sources: [{location: "file:line", snippet: "code", kind: "function|struct|interface", qname: "qualified.name"}]}]
- remove: ["finding_id"] to remove stale findings

## update_gaps
Track open questions and their resolution.
- add: [{question, evidence?, severity: blocking|high|medium|low, respondent: reporter|assignee|thread}]
- resolve: ["gap_id"] when answered
- skip: ["gap_id"] when no longer relevant

## ready_for_plan
Signal readiness to generate implementation plan. Only use when all blocking gaps are resolved.

Required conditions:
- All blocking gaps resolved (humans answered OR said "proceed anyway")
- Requirements clear enough to implement
- Architectural approach confirmed
- No unresolved conflicts between PM and dev

Required fields:
- context_summary: synthesized understanding of the issue
- relevant_finding_ids: which findings matter for implementation
- resolved_gap_ids: decisions made during scoping`
