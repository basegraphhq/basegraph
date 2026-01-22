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
	exploreTimeout    = 12 * time.Minute // Increased for thorough explorations
	doomLoopThreshold = 3                // Stop if same tool called 3 times with identical args
	maxParallelTools  = 8                // Limit concurrent tool executions
)

// Thoroughness levels control how deep the explore agent searches.
type Thoroughness string

const (
	ThoroughnessQuick  Thoroughness = "quick"    // Fast lookup, ~30 iterations, ~15k soft / ~25k hard
	ThoughnessMedium   Thoroughness = "medium"   // Balanced exploration, ~50 iterations, ~40k soft / ~60k hard
	ThoughnessThorough Thoroughness = "thorough" // Comprehensive search, ~120 iterations, ~80k soft / ~120k hard
)

// ThoroughnessConfig defines limits and behavior for each thoroughness level.
// Following Anthropic's guidance: give model autonomy, use soft limits to encourage
// synthesis rather than hard cutoffs that reduce quality.
type ThoroughnessConfig struct {
	MaxIterations   int // Hard ceiling on iterations
	SoftTokenTarget int // Encourage synthesis around this point (80% triggers gentle nudge)
	HardTokenLimit  int // Safety ceiling (forces synthesis)
}

func thoroughnessConfig(t Thoroughness) ThoroughnessConfig {
	switch t {
	case ThoroughnessQuick:
		return ThoroughnessConfig{
			MaxIterations:   30,
			SoftTokenTarget: 15000,
			HardTokenLimit:  25000,
		}
	case ThoughnessMedium:
		return ThoroughnessConfig{
			MaxIterations:   50,
			SoftTokenTarget: 40000,
			HardTokenLimit:  60000,
		}
	case ThoughnessThorough:
		return ThoroughnessConfig{
			MaxIterations:   120,
			SoftTokenTarget: 80000,
			HardTokenLimit:  120000,
		}
	default:
		return thoroughnessConfig(ThoughnessMedium)
	}
}

// ExploreMetrics captures structured data about an exploration session for analysis.
type ExploreMetrics struct {
	SessionID             string         `json:"session_id"`
	Query                 string         `json:"query"`
	Thoroughness          string         `json:"thoroughness"`
	StartTime             time.Time      `json:"start_time"`
	EndTime               time.Time      `json:"end_time"`
	DurationMs            int64          `json:"duration_ms"`
	Iterations            int            `json:"iterations"`
	ContextWindowTokens   int            `json:"context_window_tokens"`   // Final context window size
	TotalCompletionTokens int            `json:"total_completion_tokens"` // Sum of all completion tokens
	ToolCalls             map[string]int `json:"tool_calls"`

	// Codegraph effectiveness metrics
	CodegraphOps            map[string]int `json:"codegraph_ops,omitempty"`
	CodegraphCallsWithQName int            `json:"codegraph_calls_with_qname"`
	CodegraphCallsWithName  int            `json:"codegraph_calls_with_name"`
	CodegraphInvalidKind    int            `json:"codegraph_invalid_kind"`
	CodegraphAmbiguous      int            `json:"codegraph_ambiguous"`
	CodegraphTraceFound     int            `json:"codegraph_trace_found"`
	CodegraphTraceNotFound  int            `json:"codegraph_trace_not_found"`

	Confidence        string `json:"confidence"`
	HitSoftLimit      bool   `json:"hit_soft_limit"`
	HitHardLimit      bool   `json:"hit_hard_limit"`
	HitIterLimit      bool   `json:"hit_iteration_limit"`
	DoomLoopDetected  bool   `json:"doom_loop_detected"`
	FinalReportLen    int    `json:"final_report_length"`
	TerminationReason string `json:"termination_reason"`
}

// ExploreAgent is a sub-agent that explores the codebase.
// Each Explore() call gets a fresh context window (disposable).
// This preserves the Planner's context window for planning quality.
type ExploreAgent struct {
	llm        llm.AgentClient
	tools      *ExploreTools
	modulePath string // Go module path for constructing qnames (e.g., "basegraph.co/relay")
	debugDir   string // Directory for debug logs (empty = no logging)

	// Mock mode fields for A/B testing planner prompts
	mockMode    bool            // When true, use fixture selection instead of real exploration
	mockLLM     llm.AgentClient // Cheap LLM (e.g., gpt-4o-mini) for fixture selection
	fixtureFile string          // Path to JSON file with pre-written explore responses
}

// NewExploreAgent creates an ExploreAgent sub-agent.
func NewExploreAgent(llmClient llm.AgentClient, tools *ExploreTools, modulePath, debugDir string) *ExploreAgent {
	return &ExploreAgent{
		llm:        llmClient,
		tools:      tools,
		modulePath: modulePath,
		debugDir:   debugDir,
	}
}

// WithMockMode enables mock mode for A/B testing planner prompts.
// Instead of real exploration, it uses a cheap LLM to select from pre-written fixture responses.
// selectorLLM should be a cheap model like gpt-4o-mini.
// fixtureFile is the path to a JSON file with pre-written explore responses.
func (e *ExploreAgent) WithMockMode(selectorLLM llm.AgentClient, fixtureFile string) *ExploreAgent {
	e.mockMode = true
	e.mockLLM = selectorLLM
	e.fixtureFile = fixtureFile
	return e
}

// toolCallRecord tracks a tool invocation for doom loop detection.
type toolCallRecord struct {
	name string
	args string
}

// toolResult holds the result of a single tool execution.
type toolResult struct {
	callID string
	result string
}

// Explore explores the codebase to answer a question.
// Returns a prose report with code snippets for another LLM to read.
func (e *ExploreAgent) Explore(ctx context.Context, query string) (string, error) {
	// Mock mode: use fixture selection instead of real exploration
	if e.mockMode {
		return e.exploreWithMock(ctx, query)
	}

	config := thoroughnessConfig(ThoughnessMedium)
	start := time.Now()

	// Initialize metrics for structured logging
	metrics := ExploreMetrics{
		SessionID:    time.Now().Format("20060102-150405.000"),
		Query:        query,
		Thoroughness: "medium",
		StartTime:    start,
		ToolCalls:    make(map[string]int),
		CodegraphOps: make(map[string]int),
	}

	// Enrich context with explorer component
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.brain.explorer",
	})

	ctx, cancel := context.WithTimeout(ctx, exploreTimeout)
	defer cancel()

	messages := []llm.Message{
		{Role: "system", Content: e.systemPrompt()},
		{Role: "user", Content: query},
	}

	// Start a new session for this explore query and prepare debug logging.
	var debugLog strings.Builder
	debugLog.WriteString(fmt.Sprintf("=== ExploreAgent session started at %s ===\n", metrics.SessionID))
	debugLog.WriteString(fmt.Sprintf("Limits: max_iter=%d, soft_target=%d, hard_limit=%d\n",
		config.MaxIterations, config.SoftTokenTarget, config.HardTokenLimit))
	debugLog.WriteString(fmt.Sprintf("Query: %s\n\n", query))

	slog.DebugContext(ctx, "explore agent starting",
		"query", logger.Truncate(query, 100))

	// Track token usage and iterations
	// - contextWindowTokens: current context window size (for limit checks)
	// - totalCompletionTokens: sum of all completion tokens generated
	contextWindowTokens := 0
	totalCompletionTokens := 0
	iterations := 0
	softNudgeSent := false
	selfAssessmentDone := false
	var pendingReport string // Holds the report while waiting for self-assessment

	defer func() {
		metrics.EndTime = time.Now()
		metrics.DurationMs = time.Since(start).Milliseconds()
		metrics.Iterations = iterations
		metrics.ContextWindowTokens = contextWindowTokens
		metrics.TotalCompletionTokens = totalCompletionTokens
		metrics.FinalReportLen = debugLog.Len()

		slog.InfoContext(ctx, "explore agent completed",
			"query", logger.Truncate(query, 50),
			"duration_ms", metrics.DurationMs,
			"iterations", iterations,
			"context_window_tokens", contextWindowTokens,
			"total_completion_tokens", totalCompletionTokens,
			"confidence", metrics.Confidence,
			"termination_reason", metrics.TerminationReason)

		e.writeDebugLog(metrics.SessionID, "explore", debugLog.String())
		e.writeMetricsLog(metrics)
	}()

	// Track recent tool calls for doom loop detection
	var recentCalls []toolCallRecord

	for {
		iterations++

		// Check iteration limit
		if iterations > config.MaxIterations {
			slog.InfoContext(ctx, "explore agent hit iteration limit, synthesizing findings",
				"iterations", iterations)
			debugLog.WriteString(fmt.Sprintf("\n=== ITERATION LIMIT REACHED (%d iterations) - synthesizing findings ===\n", iterations))

			metrics.HitIterLimit = true
			metrics.TerminationReason = "iteration_limit"

			report, err := e.forceSynthesis(ctx, messages, "Maximum exploration steps reached. Write your final report now based on what you've found.")
			if err != nil {
				return "", err
			}

			metrics.FinalReportLen = len(report)
			debugLog.WriteString(fmt.Sprintf("[SYNTHESIS]\n%s\n", report))
			return report, nil
		}

		// Check limits based on current context window size
		// On first iteration, contextWindowTokens is 0 so we skip limit checks

		// Soft nudge at 80% of target (not forced - just a gentle prompt)
		if !softNudgeSent && contextWindowTokens > config.SoftTokenTarget*80/100 {
			softNudgeSent = true
			metrics.HitSoftLimit = true
			debugLog.WriteString(fmt.Sprintf("\n=== SOFT LIMIT REACHED (context=%d tokens, 80%% of %d) - adding synthesis nudge ===\n",
				contextWindowTokens, config.SoftTokenTarget))

			messages = append(messages, llm.Message{
				Role: "user",
				Content: `⚠️ CONTEXT BUDGET 80% USED

You've used most of your exploration budget. Before any more tool calls:

1. Review what you've already found above
2. If you can answer the question with current evidence → WRITE YOUR REPORT NOW
3. Only continue if there's ONE specific gap that ONE more search would fill

Stop exploring. Start synthesizing.`,
			})
		}

		// Hard limit check moved to AFTER resp.PromptTokens is known (see below)

		resp, err := e.llm.ChatWithTools(ctx, llm.AgentRequest{
			Messages: messages,
			Tools:    e.tools.Definitions(),
		})
		if err != nil {
			metrics.TerminationReason = "error"
			return "", fmt.Errorf("explore agent chat iteration %d: %w", iterations, err)
		}

		// Track token usage
		// - resp.PromptTokens is the context window size for THIS call
		// - resp.CompletionTokens is tokens generated in THIS call
		contextWindowTokens = resp.PromptTokens
		totalCompletionTokens += resp.CompletionTokens

		// Check hard token limit AFTER response (ensures we catch this iteration's tokens)
		if contextWindowTokens >= config.HardTokenLimit {
			slog.InfoContext(ctx, "explore agent hit hard token limit, synthesizing findings",
				"iterations", iterations,
				"context_window_tokens", contextWindowTokens)
			debugLog.WriteString(fmt.Sprintf("\n=== HARD TOKEN LIMIT REACHED (context=%d tokens) - forcing synthesis ===\n", contextWindowTokens))

			metrics.HitHardLimit = true
			metrics.TerminationReason = "hard_limit"

			report, err := e.forceSynthesis(ctx, messages, "Token limit reached. Write your final report now based on everything you've found.")
			if err != nil {
				return "", err
			}

			debugLog.WriteString(fmt.Sprintf("[SYNTHESIS]\n%s\n", report))
			return report, nil
		}

		// Log per-call token usage and current context window
		slog.DebugContext(ctx, "explore agent iteration completed",
			"iteration", iterations,
			"call_prompt_tokens", resp.PromptTokens,
			"call_completion_tokens", resp.CompletionTokens,
			"context_window_tokens", contextWindowTokens,
			"total_completion_tokens", totalCompletionTokens,
			"tool_calls", len(resp.ToolCalls))

		// Log assistant response
		debugLog.WriteString(fmt.Sprintf("--- ITERATION %d (context=%d, completion=%d) ---\n",
			iterations, resp.PromptTokens, resp.CompletionTokens))
		debugLog.WriteString(fmt.Sprintf("[ASSISTANT]\n%s\n\n", resp.Content))

		// No tool calls = model wants to conclude
		if len(resp.ToolCalls) == 0 {
			// Self-assessment before accepting final answer
			if !selfAssessmentDone {
				selfAssessmentDone = true
				pendingReport = resp.Content // Save the report
				debugLog.WriteString("\n=== SELF-ASSESSMENT REQUESTED ===\n")

				messages = append(messages, llm.Message{
					Role:    "assistant",
					Content: resp.Content,
				})
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: "Before finalizing: Rate your confidence in this answer (high/medium/low) and note any caveats or areas of uncertainty.",
				})
				continue
			}

			// Extract confidence from the self-assessment response
			metrics.Confidence = extractConfidence(resp.Content)
			metrics.TerminationReason = "natural"

			// Combine the original report with the confidence assessment
			finalReport := pendingReport + "\n\n---\n\n**Confidence Assessment:** " + resp.Content
			metrics.FinalReportLen = len(finalReport)

			debugLog.WriteString(fmt.Sprintf("=== EXPLORE AGENT COMPLETED (confidence: %s) ===\n", metrics.Confidence))
			return finalReport, nil
		}

		// Track tool calls for metrics
		for _, tc := range resp.ToolCalls {
			metrics.ToolCalls[tc.Name]++
			if tc.Name != "codegraph" {
				continue
			}

			params, err := llm.ParseToolArguments[CodegraphParams](tc.Arguments)
			if err != nil {
				continue
			}
			op := strings.ToLower(strings.TrimSpace(params.Operation))
			if op != "" {
				metrics.CodegraphOps[op]++
			}

			switch op {
			case "callers", "callees", "implementations", "usages":
				if params.QName != "" {
					metrics.CodegraphCallsWithQName++
				} else if params.Name != "" {
					metrics.CodegraphCallsWithName++
				}
			case "trace":
				if params.FromQName != "" && params.ToQName != "" {
					metrics.CodegraphCallsWithQName++
				} else if params.FromName != "" || params.ToName != "" {
					metrics.CodegraphCallsWithName++
				}
			}
		}

		// Check for doom loop (same tool called repeatedly with same args)
		if len(resp.ToolCalls) == 1 {
			tc := resp.ToolCalls[0]
			currentCall := toolCallRecord{name: tc.Name, args: normalizeArgs(tc.Arguments)}
			recentCalls = append(recentCalls, currentCall)

			// Keep only last N calls
			if len(recentCalls) > doomLoopThreshold {
				recentCalls = recentCalls[1:]
			}

			// Check if all recent calls are identical
			if len(recentCalls) == doomLoopThreshold && allIdentical(recentCalls) {
				slog.WarnContext(ctx, "explore agent doom loop detected, forcing completion",
					"iterations", iterations,
					"repeated_tool", tc.Name,
					"repeated_args", tc.Arguments)

				debugLog.WriteString(fmt.Sprintf("\n=== DOOM LOOP DETECTED (tool '%s' called %d times with same args) ===\n",
					tc.Name, doomLoopThreshold))

				metrics.DoomLoopDetected = true
				metrics.TerminationReason = "doom_loop"

				report, err := e.forceSynthesis(ctx, messages,
					"You seem to be searching for the same thing repeatedly. Please write your final report now based on what you've found so far. If you couldn't find what you were looking for, explain what you found instead.")
				if err != nil {
					return "", err
				}

				debugLog.WriteString(fmt.Sprintf("[SYNTHESIS]\n%s\n", report))
				return report, nil
			}
		} else {
			// Multiple tool calls in one turn, reset doom loop detection
			recentCalls = nil
		}

		// Log tool calls
		for _, tc := range resp.ToolCalls {
			debugLog.WriteString(fmt.Sprintf("[TOOL CALL] %s\n", tc.Name))
			debugLog.WriteString(fmt.Sprintf("Arguments: %s\n\n", tc.Arguments))
		}

		// Execute all tool calls
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute all tool calls in parallel
		results := e.executeToolsParallel(ctx, resp.ToolCalls)

		for i, res := range results {
			// Log tool result
			debugLog.WriteString(fmt.Sprintf("[TOOL RESULT] %s\n", resp.ToolCalls[i].Name))
			debugLog.WriteString(fmt.Sprintf("%s\n\n", res.result))

			// Codegraph effectiveness signals (parse tool output)
			if resp.ToolCalls[i].Name == "codegraph" {
				if strings.Contains(res.result, "Error: invalid kind") {
					metrics.CodegraphInvalidKind++
				}
				if strings.Contains(res.result, "Error: ambiguous symbol") {
					metrics.CodegraphAmbiguous++
				}

				params, err := llm.ParseToolArguments[CodegraphParams](resp.ToolCalls[i].Arguments)
				if err == nil {
					op := strings.ToLower(strings.TrimSpace(params.Operation))
					if op == "trace" {
						if strings.HasPrefix(res.result, "Trace path") {
							metrics.CodegraphTraceFound++
						} else if strings.HasPrefix(res.result, "No call path found") {
							metrics.CodegraphTraceNotFound++
						}
					}
				}
			}

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    res.result,
				ToolCallID: res.callID,
			})
		}
	}
}

// forceSynthesis forces the model to write a final report without tools.
func (e *ExploreAgent) forceSynthesis(ctx context.Context, messages []llm.Message, prompt string) (string, error) {
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: prompt,
	})

	resp, err := e.llm.ChatWithTools(ctx, llm.AgentRequest{
		Messages: messages,
		Tools:    nil, // No tools = force text response
	})
	if err != nil {
		return "", fmt.Errorf("explore agent forced synthesis: %w", err)
	}

	return resp.Content, nil
}

// extractConfidence parses confidence level from model's self-assessment.
func extractConfidence(content string) string {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "high confidence") || strings.Contains(lower, "confidence: high") || strings.Contains(lower, "confidence is high"):
		return "high"
	case strings.Contains(lower, "low confidence") || strings.Contains(lower, "confidence: low") || strings.Contains(lower, "confidence is low"):
		return "low"
	case strings.Contains(lower, "medium confidence") || strings.Contains(lower, "confidence: medium") || strings.Contains(lower, "confidence is medium") || strings.Contains(lower, "moderate confidence"):
		return "medium"
	default:
		return "unknown"
	}
}

// executeToolsParallel runs multiple tool calls concurrently with bounded parallelism.
// Individual tool failures are captured as error messages in the result, not propagated.
func (e *ExploreAgent) executeToolsParallel(ctx context.Context, toolCalls []llm.ToolCall) []toolResult {
	results := make([]toolResult, len(toolCalls))
	var wg sync.WaitGroup

	// Semaphore to limit concurrent tool executions
	sem := make(chan struct{}, maxParallelTools)

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, call llm.ToolCall) {
			defer wg.Done()

			// Acquire semaphore slot
			sem <- struct{}{}
			defer func() { <-sem }()

			slog.DebugContext(ctx, "explore agent executing tool",
				"tool", call.Name,
				"call_id", call.ID)

			result, err := e.tools.Execute(ctx, call.Name, call.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error: %s", err)
			}

			results[idx] = toolResult{
				callID: call.ID,
				result: result,
			}
		}(i, tc)
	}

	wg.Wait()
	return results
}

// normalizeArgs normalizes JSON arguments for comparison.
func normalizeArgs(args string) string {
	var v any
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		return args
	}
	normalized, err := json.Marshal(v)
	if err != nil {
		return args
	}
	return string(normalized)
}

// allIdentical checks if all tool calls in the slice are identical.
func allIdentical(calls []toolCallRecord) bool {
	if len(calls) == 0 {
		return false
	}
	first := calls[0]
	for _, c := range calls[1:] {
		if c.name != first.name || c.args != first.args {
			return false
		}
	}
	return true
}

func (e *ExploreAgent) writeDebugLog(sessionID, agentType, content string) {
	if e.debugDir == "" {
		return
	}

	if err := os.MkdirAll(e.debugDir, 0o755); err != nil {
		slog.Warn("failed to create debug dir", "dir", e.debugDir, "error", err)
		return
	}

	filename := filepath.Join(e.debugDir, fmt.Sprintf("%s_%s.txt", agentType, sessionID))
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		slog.Warn("failed to write debug log", "file", filename, "error", err)
	}
}

// writeMetricsLog writes structured JSON metrics for analysis.
func (e *ExploreAgent) writeMetricsLog(metrics ExploreMetrics) {
	if e.debugDir == "" {
		return
	}

	if err := os.MkdirAll(e.debugDir, 0o755); err != nil {
		slog.Warn("failed to create debug dir", "dir", e.debugDir, "error", err)
		return
	}

	metricsFile := filepath.Join(e.debugDir, fmt.Sprintf("explore_metrics_%s.json", metrics.SessionID))
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		slog.Warn("failed to marshal metrics", "error", err)
		return
	}

	if err := os.WriteFile(metricsFile, data, 0o644); err != nil {
		slog.Warn("failed to write metrics", "file", metricsFile, "error", err)
	}
}

// systemPrompt returns the system prompt for the explore agent.
func (e *ExploreAgent) systemPrompt() string {
	return fmt.Sprintf(`You are a code exploration agent. Answer questions with evidence from this codebase.

# Your Tools

You have three types of tools:

**STRUCTURE tools** - Understand code relationships (Go only)
- codegraph: Call chains, interface implementations, type usages

**TEXT tools** - Find patterns in files
- glob: Find files by name/path
- grep: Find string patterns in content
- read: View code at specific locations
- bash: Read files (cat, head, tail), search (grep, rg)

**HISTORY tools** - Track changes over time
- bash: Git log, diff, blame

# Decision Tree

Ask yourself: "What am I looking for?"

→ "Who calls function X?" or "What implements interface Y?"
  USE: codegraph (if Go) — gives exact structural answers

→ "How does A flow to B?" or "Trace the path from X to Y"
  USE: codegraph(operation="trace", ...) — returns an actual call path if it exists
  Example: codegraph(operation="trace", from_name="HandleWebhook", to_name="Plan", to_kind="method", max_depth=6)

→ "Where does string/pattern appear?" or "What files contain X?"
  USE: grep — text pattern matching

→ "What files exist matching a name?"
  USE: glob — file discovery

→ "What does this code do?" (after finding location)
  USE: read — view the actual code

→ "How did this change?" or "Who wrote this?"
  USE: bash — git history

# Codegraph Workflow

## Understanding name vs qname

A qname (qualified name) is globally unique: module/path/to/package.Type.Method
  Function:  github.com/acme/app/internal/auth.ValidateToken
  Method:    github.com/acme/app/internal/store.UserRepo.Save
  Struct:    github.com/acme/app/internal/model.User

Parameters:
- name: Short symbol name (e.g., "Save", "ValidateToken") — use for resolve, search, callers, callees
- qname: Full qualified name — only use when you ALREADY HAVE it from a previous result
- from_name/to_name: Only for trace operation

The resolve operation converts name→qname. Never pass qname to resolve.

## Workflow

Supported kinds (strict): function, method, struct, interface, class.

For structural questions in Go:
1. Start with name — the tool resolves qname internally
   codegraph(operation="callers", name="Save", kind="method")
2. Use resolve only to disambiguate or get the exact qname
   codegraph(operation="resolve", name="Save", kind="method", file="store/user.go")
3. Once you have a qname from results, you can use it directly
   codegraph(operation="callees", qname="github.com/acme/app/store.UserRepo.Save")
4. Use trace for flow questions (fast + graph-accurate)
   codegraph(operation="trace", from_name="HandleWebhook", to_name="Plan", to_kind="method", max_depth=6)
5. Use read() only to confirm specific code locations

# Strategy

1. **Structure before text** — For Go, codegraph gives precise answers; grep gives noisy matches
2. **Start specific, broaden as needed** — Begin with focused queries, expand to cover all aspects
3. **Read enough to understand** — Read 50-100 lines around targets for full context
4. **Gather comprehensive evidence** — Explore all relevant aspects before synthesizing

# Anti-Patterns

❌ grep for "who calls X" in Go — codegraph gives exact answer
❌ codegraph for .js/.ts/other files — unsupported, use grep
❌ Manually constructing qnames — use codegraph(resolve) or pass name
❌ codegraph(operation="resolve", qname="X") — WRONG. resolve needs name, not qname
❌ codegraph(operation="search", qname="X") — WRONG. search needs name, not qname
❌ Reading only 10-20 lines — read enough to understand the full context
❌ Multiple searches for same thing — your context already has the data
❌ Stopping before exploring related areas — follow connections to build complete picture

# Report what's missing, not just what exists

When you explore, you're building a picture of reality. Reality includes gaps.

**Compare what you find against what you'd expect.** You have knowledge from your training about how systems typically work. When you find code that handles data flows, external calls, scheduled operations, or domain logic — ask yourself: does this match how things usually work? If you notice something that seems incomplete or unusual based on your experience, note it.

**Trace data end-to-end.** When exploring a feature, follow the data: where does it come from, how is it transformed, where is it stored, who consumes it? Notice if there are mismatches — fields that don't align, formats that differ, relationships that can't be joined.

**Always map entities + join keys.** For any feature/integration/dashboard/monitoring/scheduled work, include a short **Data Model & Persistence (Entity & Join Map)** section in your report:
- Canonical records/types involved (models/tables/structs/config)
- Persisted vs derived vs external-only
- IDs/keys that link records (or "no join path")
- Lifecycle boundary mismatches (pre vs post event)

**Report absences explicitly.** "I could not find X" is as valuable as "I found X at location Y" — especially when X is something you'd typically expect to exist. Don't just describe what's there; note what's surprisingly missing.

The goal is to give the planner a complete picture — including the holes.

# Tools Reference

glob(pattern, path?) — Find files. Supports **, *, {a,b}. Returns paths by recency.
grep(pattern, glob?, context?) — Search contents. Regex pattern. Returns file:line matches.
codegraph(operation, ...) — Query code graph. Operations: search, resolve, file_symbols, callers, callees, implementations, usages, trace.
read(file_path, offset?, limit?) — Read file. Default 200 lines. Returns numbered lines.
bash(command) — Git: log, diff, blame, show, status. File ops: cat, head, tail, grep, rg, ls, find.

# Context

Go module: %s
Codebase index: .basegraph/index.md

# Token Budget

You have approximately 60,000 tokens for this exploration (medium thoroughness).
- Around 40,000 tokens: consider starting your report synthesis
- Maximum 60,000 tokens: you must synthesize by this point

Use your budget wisely:
- Explore multiple related areas, not just the direct answer
- Read 50-100 lines for full context, not just function signatures
- Follow connections to build a complete picture

# Output

When you have gathered comprehensive evidence, write a detailed report:

<report>
## Summary
[2-3 sentence overview answering the question directly]

## 1. [First Major Topic/Component]

[Detailed explanation with context about this aspect]

**Key Files:**
| File | Purpose |
|------|---------|
| path/to/file.go | Brief description of role |

**Code:**
~~~go
// file.go:42-58 - What this code does
[relevant code snippet]
~~~

## 2. [Second Major Topic/Component]

[Continue this pattern for each major aspect discovered]

## 3. [Additional Topics as Needed]

[Add as many numbered sections as the topic requires]

## Key Findings

1. [Important architectural/design insight]
2. [Important implementation detail]
3. [Important relationship or flow]

## Files Reference

| File | Lines | Purpose |
|------|-------|---------|
| file1.go | 42-58 | Description |
| file2.go | 100-150 | Description |
| file3.go | 200-250 | Description |

## Confidence
[high/medium/low] — [reasoning about completeness of exploration]
</report>

**Report Guidelines:**
- Organize by **logical topics**, not by discovery order
- Use **tables** to summarize file lists and relationships
- Include a **Data Model & Persistence (Entity & Join Map)** section when relevant
- Include **actual code snippets** for key logic (with file:line references)
- Add as many numbered sections as needed to fully answer the question
- The report should be **self-contained** — a reader shouldn't need to explore further`, e.modulePath)
}
