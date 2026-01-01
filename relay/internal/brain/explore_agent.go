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
	exploreTimeout    = 5 * time.Minute
	doomLoopThreshold = 3     // Stop if same tool called 3 times with identical args
	maxParallelTools  = 4     // Limit concurrent tool executions
	maxIterations     = 6     // Force synthesis after this many iterations
	maxContextTokens  = 30000 // Token limit - keep exploration focused
)

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
// The report is compressed context - the ExploreAgent may read 50k tokens of code
// but returns a curated 2-3k token summary.
func (e *ExploreAgent) Explore(ctx context.Context, query string) (string, error) {
	start := time.Now()

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
	sessionID := time.Now().Format("2006-01-02 15:04:05")
	var debugLog strings.Builder
	debugLog.WriteString(fmt.Sprintf("=== ExploreAgent session started at %s ===\n", sessionID))
	debugLog.WriteString(fmt.Sprintf("User query: %s\n\n", query))

	slog.DebugContext(ctx, "explore agent starting",
		"query", logger.Truncate(query, 100))

	// Track token usage and iterations
	totalPromptTokens := 0
	totalCompletionTokens := 0
	iterations := 0

	defer func() {
		slog.InfoContext(ctx, "explore agent completed",
			"query", logger.Truncate(query, 50),
			"total_duration_ms", time.Since(start).Milliseconds(),
			"iterations", iterations,
			"total_prompt_tokens", totalPromptTokens,
			"total_completion_tokens", totalCompletionTokens,
			"total_tokens", totalPromptTokens+totalCompletionTokens,
			"report_length", len(debugLog.String()))
	}()

	// Track recent tool calls for doom loop detection
	var recentCalls []toolCallRecord
	for {
		iterations++

		// Check iteration limit
		if iterations > maxIterations {
			slog.InfoContext(ctx, "explore agent hit iteration limit, synthesizing findings",
				"iterations", iterations)
			debugLog.WriteString(fmt.Sprintf("\n=== ITERATION LIMIT REACHED (%d iterations) - synthesizing findings ===\n", iterations))

			messages = append(messages, llm.Message{
				Role:    "user",
				Content: "Maximum exploration steps reached. Write your final report now based on what you've found.",
			})

			synthResp, err := e.llm.ChatWithTools(ctx, llm.AgentRequest{
				Messages: messages,
				Tools:    nil,
			})
			if err != nil {
				e.writeDebugLog(sessionID, "explore", debugLog.String())
				return "", fmt.Errorf("explore agent synthesis after iteration limit: %w", err)
			}

			totalPromptTokens += synthResp.PromptTokens
			totalCompletionTokens += synthResp.CompletionTokens

			debugLog.WriteString(fmt.Sprintf("[SYNTHESIS]\n%s\n", synthResp.Content))
			e.writeDebugLog(sessionID, "explore", debugLog.String())
			return synthResp.Content, nil
		}

		// Check token limit before making another LLM call
		if totalPromptTokens+totalCompletionTokens >= maxContextTokens {
			slog.InfoContext(ctx, "explore agent hit token limit, synthesizing findings",
				"iterations", iterations,
				"total_tokens", totalPromptTokens+totalCompletionTokens)
			debugLog.WriteString(fmt.Sprintf("\n=== TOKEN LIMIT REACHED (%d tokens) - synthesizing findings ===\n",
				totalPromptTokens+totalCompletionTokens))

			// Ask the model to synthesize findings without tools
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: "You've gathered substantial information. Please write your final report now based on everything you've found.",
			})

			synthResp, err := e.llm.ChatWithTools(ctx, llm.AgentRequest{
				Messages: messages,
				Tools:    nil, // No tools = force text response
			})
			if err != nil {
				e.writeDebugLog(sessionID, "explore", debugLog.String())
				return "", fmt.Errorf("explore agent synthesis after token limit: %w", err)
			}

			totalPromptTokens += synthResp.PromptTokens
			totalCompletionTokens += synthResp.CompletionTokens

			debugLog.WriteString(fmt.Sprintf("[SYNTHESIS]\n%s\n", synthResp.Content))
			e.writeDebugLog(sessionID, "explore", debugLog.String())
			return synthResp.Content, nil
		}

		resp, err := e.llm.ChatWithTools(ctx, llm.AgentRequest{
			Messages: messages,
			Tools:    e.tools.Definitions(),
		})
		if err != nil {
			e.writeDebugLog(sessionID, "explore", debugLog.String())
			return "", fmt.Errorf("explore agent chat iteration %d: %w", iterations, err)
		}

		// Track token usage
		totalPromptTokens += resp.PromptTokens
		totalCompletionTokens += resp.CompletionTokens

		// Log assistant response
		debugLog.WriteString(fmt.Sprintf("--- ITERATION %d ---\n", iterations))
		debugLog.WriteString(fmt.Sprintf("[ASSISTANT]\n%s\n\n", resp.Content))

		// No tool calls = model is done, return the prose report
		if len(resp.ToolCalls) == 0 {
			debugLog.WriteString("=== EXPLORE AGENT COMPLETED ===\n")
			e.writeDebugLog(sessionID, "explore", debugLog.String())
			return resp.Content, nil
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
				e.writeDebugLog(sessionID, "explore", debugLog.String())

				// Force synthesis by removing tools
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: "You seem to be searching for the same thing repeatedly. Please write your final report now based on what you've found so far. If you couldn't find what you were looking for, explain what you found instead.",
				})

				synthResp, err := e.llm.ChatWithTools(ctx, llm.AgentRequest{
					Messages: messages,
					Tools:    nil, // No tools = force text response
				})
				if err != nil {
					return "", fmt.Errorf("explore agent forced synthesis: %w", err)
				}

				// Track tokens from forced synthesis
				totalPromptTokens += synthResp.PromptTokens
				totalCompletionTokens += synthResp.CompletionTokens

				return synthResp.Content, nil
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
	ctx := context.Background()
	if e.debugDir == "" {
		return
	}

	if err := os.MkdirAll(e.debugDir, 0o755); err != nil {
		slog.WarnContext(ctx, "failed to create debug dir", "dir", e.debugDir, "error", err)
		return
	}

	filename := filepath.Join(e.debugDir, fmt.Sprintf("%s_%s.txt", agentType, sessionID))
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		slog.WarnContext(ctx, "failed to write debug log", "file", filename, "error", err)
	} else {
		slog.InfoContext(ctx, "debug log written", "file", filename)
	}
}

// systemPrompt returns the system prompt with the module path for qname construction.
func (e *ExploreAgent) systemPrompt() string {
	return fmt.Sprintf(`You are a focused code search specialist. Find specific answers quickly.

# Core Rules

1. **Be targeted** - Don't explore broadly. Find what you need and stop.
2. **Read selectively** - Don't read entire files. Use start_line/num_lines to read relevant sections only.
3. **One search strategy** - Pick the most direct approach. Don't try multiple grep patterns for the same thing.
4. **Stop when you have enough** - If you found the answer, write your report. Don't keep exploring "just in case".

# Tool Usage

- **grep**: Use specific patterns. If you get 30+ results, your pattern is too broad.
- **read**: Read 50-100 lines max. If you need more context, make a second targeted read.
- **glob**: Use specific paths like "internal/brain/*.go", not "**/*.go".
- **graph**: Great for finding callers/callees. Use it instead of grepping for function names.

# Anti-patterns (DON'T DO THESE)

- Reading the same file multiple times with overlapping ranges
- Grepping for the same concept with different patterns
- Reading an entire 500-line file when you only need one function
- Exploring "related" code that wasn't asked about

# Tools

## grep(pattern, include?, limit?)
Fast content search tool that works with any codebase size. Searches file contents using regular expressions. Supports full regex syntax (eg. "log.*Error", "function\s+\w+", etc.). Filter files by pattern with the include parameter (eg. "*.go", "*.{ts,tsx}"). Returns file paths and line numbers with at least one match sorted by modification time. Use this tool when you need to find files containing specific patterns.

## glob(pattern)
Fast file pattern matching tool that works with any codebase size. Supports glob patterns like "**/*.go" or "internal/**/*.go". Returns matching file paths sorted by modification time. Use this tool when you need to find files by name patterns.

## read(file, start_line?, num_lines?)
Reads a file from the local filesystem. By default, it reads up to 200 lines starting from the beginning of the file. You can optionally specify a line offset and limit (especially handy for long files). Results are returned with line numbers.

## graph(operation, target, depth?)
Query code relationships from the indexed code graph. This is powerful for understanding call chains, interface implementations, and code dependencies.

Operations:
- callers: Who calls this function/method?
- callees: What does this function/method call?
- implementations: What types implement this interface?
- methods: What methods does this type have?
- usages: Where is this type/function used?
- inheritors: What types embed this type?

Target must be a qualified name (qname):
Format: {module_path}/{package_path}.{Type}.{Method} or {module_path}/{package_path}.{Function}

<qname-info>
This repository's Go module path is: %s

Examples:
- Function: %s/internal/brain.NewExploreAgent
- Method: %s/internal/brain.ExploreAgent.Explore
- Type: %s/internal/brain.ExploreAgent
- Interface: %s/internal/store.Store
</qname-info>

<example>
How to construct a qname from grep results:
1. grep finds: internal/brain/explore_agent.go:35: func (e *ExploreAgent) Explore(...)
2. The file path internal/brain/ → package path is internal/brain
3. The receiver (e *ExploreAgent) → type is ExploreAgent
4. The function name → method is Explore
5. qname = %s/internal/brain.ExploreAgent.Explore
</example>

Complete the search request efficiently and report your findings clearly.`,
		e.modulePath,
		e.modulePath, e.modulePath, e.modulePath, e.modulePath,
		e.modulePath)
}
