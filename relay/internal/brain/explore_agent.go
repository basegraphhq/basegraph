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
	exploreTimeout    = 12 * time.Minute // Increased for thorough explorations
	doomLoopThreshold = 3                // Stop if same tool called 3 times with identical args
	maxParallelTools  = 4                // Limit concurrent tool executions
)

// Thoroughness levels control how deep the explore agent searches.
type Thoroughness string

const (
	ThoroughnessQuick  Thoroughness = "quick"    // Fast lookup, ~15 iterations, ~40k tokens
	ThoughnessMedium   Thoroughness = "medium"   // Balanced exploration, ~25 iterations, ~100k tokens
	ThoughnessThorough Thoroughness = "thorough" // Comprehensive search, ~100 iterations, ~200k tokens
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
			MaxIterations:   10,
			SoftTokenTarget: 15000,
			HardTokenLimit:  25000,
		}
	case ThoughnessMedium:
		return ThoroughnessConfig{
			MaxIterations:   20,
			SoftTokenTarget: 25000,
			HardTokenLimit:  40000,
		}
	case ThoughnessThorough:
		return ThoroughnessConfig{
			MaxIterations:   50,
			SoftTokenTarget: 60000,
			HardTokenLimit:  100000,
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
	Confidence            string         `json:"confidence"`
	HitSoftLimit          bool           `json:"hit_soft_limit"`
	HitHardLimit          bool           `json:"hit_hard_limit"`
	HitIterLimit          bool           `json:"hit_iteration_limit"`
	DoomLoopDetected      bool           `json:"doom_loop_detected"`
	FinalReportLen        int            `json:"final_report_length"`
	TerminationReason     string         `json:"termination_reason"`
}

// ExploreAgent is a sub-agent that explores the codebase.
// Each Explore() call gets a fresh context window (disposable).
// This preserves the Planner's context window for planning quality.
type ExploreAgent struct {
	llm        llm.AgentClient
	tools      *ExploreTools
	modulePath string // Go module path for constructing qnames (e.g., "basegraph.app/relay")
	debugDir   string // Directory for debug logs (empty = no logging)
}

// NewExploreAgent creates an ExploreAgent sub-agent.
func NewExploreAgent(llmClient llm.AgentClient, tools *ExploreTools, modulePath string) *ExploreAgent {
	return &ExploreAgent{
		llm:        llmClient,
		tools:      tools,
		modulePath: modulePath,
		debugDir:   os.Getenv("BRAIN_DEBUG_DIR"),
	}
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
// Thoroughness controls search depth: quick (first match), medium (few locations), thorough (comprehensive).
func (e *ExploreAgent) Explore(ctx context.Context, query string, thoroughness Thoroughness) (string, error) {
	config := thoroughnessConfig(thoroughness)
	start := time.Now()

	// Initialize metrics for structured logging
	metrics := ExploreMetrics{
		SessionID:    time.Now().Format("20060102-150405.000"),
		Query:        query,
		Thoroughness: string(thoroughness),
		StartTime:    start,
		ToolCalls:    make(map[string]int),
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
	debugLog.WriteString(fmt.Sprintf("Thoroughness: %s (max_iter=%d, soft_target=%d, hard_limit=%d)\n",
		thoroughness, config.MaxIterations, config.SoftTokenTarget, config.HardTokenLimit))
	debugLog.WriteString(fmt.Sprintf("Query: %s\n\n", query))

	slog.DebugContext(ctx, "explore agent starting",
		"query", logger.Truncate(query, 100),
		"thoroughness", string(thoroughness))

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
			"thoroughness", string(thoroughness),
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
// Optimized for deterministic code graph queries with bash as fallback.
func (e *ExploreAgent) systemPrompt() string {
	return fmt.Sprintf(`Explore this codebase to answer a question. Return a concise report with evidence.

## Two Tools: codegraph (PRIMARY) and bash (FALLBACK)

### codegraph - Use THIS FIRST for code exploration
Compiler-level accuracy. Deterministic. Returns structured XML with qnames and signatures.

ALWAYS use codegraph for:
  • File overview:      codegraph(operation="symbols", file="path.go")
  • Just structs:       codegraph(operation="symbols", file="path.go", kind="struct")
  • Finding symbols:    codegraph(operation="find", symbol="*Pattern*")
  • Call graph:         codegraph(operation="callers", symbol="FuncName")
  • Type hierarchy:     codegraph(operation="implementations", symbol="Interface")
  • Type methods:       codegraph(operation="methods", symbol="TypeName")
  • Type usage:         codegraph(operation="usages", symbol="TypeName")
  • What func calls:    codegraph(operation="callees", symbol="FuncName")

Example workflow:
  1. codegraph(operation="symbols", file="internal/brain/planner.go", kind="function")
     → See all functions with signatures (filter reduces noise)
  2. codegraph(operation="callers", symbol="Plan")
     → See who calls Plan() - bash cannot do this!
  3. bash(sed -n '45,90p' internal/brain/planner.go)
     → Read the specific Plan() implementation

Supported: Go, Python. Returns XML with qnames, signatures, line numbers.

### bash - Use ONLY when codegraph cannot help
For git history, reading specific line ranges, quick text searches.

  sed -n '100,150p' file.go      # Read lines 100-150
  git log --oneline -10          # Recent commits
  git blame file.go              # Line history
  rg -n "TODO" | head -20        # Text search

DO NOT use bash for:
  ✗ head -200 file.go            → Use codegraph(symbols) instead!
  ✗ rg -n "SymbolName"           → Use codegraph(find) instead!
  ✗ Understanding call graphs    → ONLY codegraph can do this!

## Decision Tree

Question: "What's in this file?"
  → codegraph(symbols, file="...") - gives structured overview
  → codegraph(symbols, file="...", kind="struct") - only structs/types

Question: "Who calls function X?"
  → codegraph(callers, symbol="X") - only tool that can answer this

Question: "Where is type Y defined?"
  → codegraph(find, symbol="Y") - AST-precise, not text match

Question: "Read lines 50-100 of file.go"
  → bash(sed -n '50,100p' file.go) - specific code reading

Question: "Find TODO comments"
  → bash(rg -n "TODO" | head -20) - text search

Question: "Recent changes to file.go"
  → bash(git log --oneline file.go) - git history

## Module: %s

## Resource Constraints (IMPORTANT)

Each tool call adds tokens to context. Be efficient:
- File reads (sed): ~30 tokens per line. Reading 100 lines = ~3000 tokens.
- Search results (rg): ~50 tokens per match.
- You have LIMITED iterations. Make each one count.

## Efficiency Rules

- NEVER search for the same pattern twice. You already have those results above.
- NEVER read the same file region twice. Reference your earlier findings.
- If codegraph found a symbol at file:line, read ONLY that region (±20 lines).
- Use sed ranges of 50-80 lines max, not 200. Chain reads if needed.

## When to Stop

Write your report immediately when:
- You found the answer with evidence (file:line references)
- You've checked 2-3 approaches with no new info
- You're about to search for something you already searched

Don't keep searching "to be thorough." Quality over quantity.

## Output Format

Concise report with:
- Direct answer to the question
- Key locations with file:line references
- Code snippets where helpful (use bash to read after codegraph gives locations)`, e.modulePath)
}
