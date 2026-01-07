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
	ThoroughnessQuick  Thoroughness = "quick"    // Fast lookup, ~30 iterations, ~40k tokens
	ThoughnessMedium   Thoroughness = "medium"   // Balanced exploration, ~50 iterations, ~100k tokens
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
			MaxIterations:   30,
			SoftTokenTarget: 15000,
			HardTokenLimit:  25000,
		}
	case ThoughnessMedium:
		return ThoroughnessConfig{
			MaxIterations:   50,
			SoftTokenTarget: 25000,
			HardTokenLimit:  40000,
		}
	case ThoughnessThorough:
		return ThoroughnessConfig{
			MaxIterations:   120,
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
func NewExploreAgent(llmClient llm.AgentClient, tools *ExploreTools, modulePath, debugDir string) *ExploreAgent {
	return &ExploreAgent{
		llm:        llmClient,
		tools:      tools,
		modulePath: modulePath,
		debugDir:   debugDir,
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
// Claude Code-style tools: glob, grep, read, bash.
func (e *ExploreAgent) systemPrompt() string {
	return fmt.Sprintf(`You are a code exploration agent. Answer the question with evidence from this codebase.

# Decision Framework

Before EVERY tool call, answer: "What specific question does this answer?"
If you cannot articulate the question → STOP and write your report.

## Codebase Index

.basegraph/index.md contains a directory overview and file descriptions for this codebase.

## Tool Selection (use the first that applies)

| I need to...                          | Use    | Example                                    |
|---------------------------------------|--------|--------------------------------------------|
| Find files by name/path               | glob   | glob(pattern="**/*handler*.go")            |
| Find where a string/pattern appears   | grep   | grep(pattern="func NewPlanner")            |
| Read code at a known location         | read   | read(file_path="x.go", offset=50, limit=40)|
| Understand git history                | bash   | git log --oneline -5 -- file.go            |

## Strategy: Narrow Fast, Read Small

1. **Name search first** - If the question mentions a concept, glob for files with that name
2. **Pattern search second** - grep for function/type definitions, not usages
3. **Surgical reads** - Read 30-50 lines around grep hits, never full files
4. **Stop at first sufficient evidence** - You don't need exhaustive proof

## Token Budget

Every tool call costs tokens. Typical costs:
- glob: 10 tokens/file path
- grep: 50 tokens/match line
- read: 30 tokens/line (100 lines = 3000 tokens)

Budget mindset: You have ~15-20 effective tool calls. Spend wisely.

## Anti-Patterns (immediate red flags)

❌ Reading a full file "to understand context" - read the specific function
❌ grep for common words (error, return, if) - too many matches
❌ Multiple globs for same concept - combine patterns: "**/*{plan,schedule}*.go"
❌ Reading same file twice - scroll your context, the data is already there
❌ "Let me search for X to be thorough" - if you have the answer, stop

## Tools Reference

### glob(pattern, path?)
Find files. Pattern supports ** (recursive), * (wildcard), {a,b} (alternatives).
Returns: paths sorted by modification time (newest first).

### grep(pattern, glob?, context?)
Search contents. Pattern is regex. glob filters files. context adds surrounding lines.
Returns: file:line matches with content.

### read(file_path, offset?, limit?)
Read file. offset is 0-indexed line to start. limit is line count (default 200).
Returns: numbered lines.

### bash(command)
Git commands only: log, diff, blame, show. Also ls for directory structure.
Blocked: cat, head, tail (use read tool instead).

## Module Context
Go module: %s
Use this for understanding import paths and package structure.

## Output Format

<report>
## Answer
[1-2 sentence direct answer]

## Evidence
- file.go:42 - [what this shows]
- file.go:87-95 - [what this shows]

## Key Code
`+"```"+`go
// file.go:42-50 - [description]
[relevant snippet]
`+"```"+`

## Confidence
[high/medium/low] - [why]
</report>

Stop exploring when you can write this report. More searching ≠ better answers.`, e.modulePath)
}
