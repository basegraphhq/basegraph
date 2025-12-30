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
)

const (
	retrieverTimeout      = 5 * time.Minute
	doomLoopThreshold     = 3  // Stop if same tool called 3 times with identical args
	maxRetrieverSteps     = 20 // Soft limit - inject synthesis prompt after this
	hardMaxRetrieverSteps = 30 // Hard limit - force stop
	maxParallelTools      = 4  // Limit concurrent tool executions
)

// Retriever is a sub-agent that explores the codebase.
// Each Query() call gets a fresh context window (disposable).
// This preserves the Planner's context window for planning quality.
type Retriever struct {
	llm        llm.AgentClient
	tools      *RetrieverTools
	modulePath string // Go module path for constructing qnames (e.g., "basegraph.app/relay")
	debugDir   string // Directory for debug logs (empty = no logging)
}

// NewRetriever creates a Retriever sub-agent.
func NewRetriever(llmClient llm.AgentClient, tools *RetrieverTools, modulePath string) *Retriever {
	return &Retriever{
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

// Query explores the codebase to answer a question.
// Returns a prose report with code snippets for another LLM to read.
// The report is compressed context - the Retriever may read 50k tokens of code
// but returns a curated 2-3k token summary.
func (r *Retriever) Query(ctx context.Context, query string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, retrieverTimeout)
	defer cancel()

	messages := []llm.Message{
		{Role: "system", Content: r.systemPrompt()},
		{Role: "user", Content: query},
	}

	// Start a new session for this Retriever query and prepare debug logging.
	sessionID := time.Now().Format("2006-01-02 15:04:05")
	var debugLog strings.Builder
	debugLog.WriteString(fmt.Sprintf("=== Retriever session started at %s ===\n", sessionID))
	debugLog.WriteString(fmt.Sprintf("User query: %s\n\n", query))

	slog.InfoContext(ctx, "retriever starting",
		"query", truncate(query, 100))

	// Track recent tool calls for doom loop detection
	var recentCalls []toolCallRecord

	iterations := 0
	for {
		iterations++

		// Hard stop after max iterations
		if iterations > hardMaxRetrieverSteps {
			slog.WarnContext(ctx, "retriever hit hard max, forcing completion",
				"iterations", iterations)
			debugLog.WriteString(fmt.Sprintf("\n=== HARD STOP (max %d iterations) ===\n", hardMaxRetrieverSteps))
			r.writeDebugLog(sessionID, "retriever", debugLog.String())
			return "Exploration stopped after maximum iterations. Based on the code examined, please refer to the search results above for relevant context.", nil
		}

		// After soft limit, remove tools to encourage synthesis
		tools := r.tools.Definitions()
		if iterations > maxRetrieverSteps {
			slog.InfoContext(ctx, "retriever past soft limit, removing tools to encourage synthesis",
				"iterations", iterations)
			debugLog.WriteString(fmt.Sprintf("\n--- SOFT LIMIT REACHED (iteration %d > %d) - removing tools ---\n", iterations, maxRetrieverSteps))
			tools = nil // Force text-only response
		}

		resp, err := r.llm.ChatWithTools(ctx, llm.AgentRequest{
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			r.writeDebugLog(sessionID, "retriever", debugLog.String())
			return "", fmt.Errorf("retriever chat iteration %d: %w", iterations, err)
		}

		// Log assistant response
		debugLog.WriteString(fmt.Sprintf("--- ITERATION %d ---\n", iterations))
		debugLog.WriteString(fmt.Sprintf("[ASSISTANT]\n%s\n\n", resp.Content))

		// No tool calls = model is done, return the prose report
		if len(resp.ToolCalls) == 0 {
			debugLog.WriteString("=== RETRIEVER COMPLETED ===\n")
			r.writeDebugLog(sessionID, "retriever", debugLog.String())

			slog.InfoContext(ctx, "retriever completed",
				"iterations", iterations,
				"report_length", len(resp.Content))
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
				slog.WarnContext(ctx, "retriever doom loop detected, forcing completion",
					"iterations", iterations,
					"repeated_tool", tc.Name,
					"repeated_args", tc.Arguments)

				debugLog.WriteString(fmt.Sprintf("\n=== DOOM LOOP DETECTED (tool '%s' called %d times with same args) ===\n",
					tc.Name, doomLoopThreshold))
				r.writeDebugLog(sessionID, "retriever", debugLog.String())

				// Force synthesis by removing tools
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: "You seem to be searching for the same thing repeatedly. Please write your final report now based on what you've found so far. If you couldn't find what you were looking for, explain what you found instead.",
				})

				synthResp, err := r.llm.ChatWithTools(ctx, llm.AgentRequest{
					Messages: messages,
					Tools:    nil, // No tools = force text response
				})
				if err != nil {
					return "", fmt.Errorf("retriever forced synthesis: %w", err)
				}
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
		results := r.executeToolsParallel(ctx, resp.ToolCalls)

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
func (r *Retriever) executeToolsParallel(ctx context.Context, toolCalls []llm.ToolCall) []toolResult {
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

			slog.DebugContext(ctx, "retriever executing tool",
				"tool", call.Name,
				"call_id", call.ID)

			result, err := r.tools.Execute(ctx, call.Name, call.Arguments)
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

func (r *Retriever) writeDebugLog(sessionID, agentType, content string) {
	if r.debugDir == "" {
		return
	}

	if err := os.MkdirAll(r.debugDir, 0o755); err != nil {
		slog.Warn("failed to create debug dir", "dir", r.debugDir, "error", err)
		return
	}

	filename := filepath.Join(r.debugDir, fmt.Sprintf("%s_%s.txt", agentType, sessionID))
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		slog.Warn("failed to write debug log", "file", filename, "error", err)
	} else {
		slog.Info("debug log written", "file", filename)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// systemPrompt returns the system prompt with the module path for qname construction.
func (r *Retriever) systemPrompt() string {
	return fmt.Sprintf(`You are a code search specialist. You excel at thoroughly navigating and exploring codebases.

Your strengths:
- Rapidly finding files using glob patterns
- Searching code and text with powerful regex patterns
- Reading and analyzing file contents
- Querying code relationships from the indexed code graph

Guidelines:
- Use Glob for broad file pattern matching
- Use Grep for searching file contents with regex
- Use Read when you know the specific file path you need to read
- Use Graph to trace code relationships like callers, callees, and implementations
- Return file paths and line numbers in your final response
- Do not create any files or modify the codebase in any way

# Tool usage policy

- You can call multiple tools in a single response. If you intend to call multiple tools and there are no dependencies between them, make all independent tool calls in parallel. Maximize use of parallel tool calls where possible to increase efficiency. However, if some tool calls depend on previous calls to inform dependent values, do NOT call these tools in parallel and instead call them sequentially. For instance, if one operation must complete before another starts, run these operations sequentially instead. Never use placeholders or guess missing parameters in tool calls.

# Tools

## grep(pattern, include?, limit?)
- Fast content search tool that works with any codebase size
- Searches file contents using regular expressions
- Supports full regex syntax (eg. "log.*Error", "function\s+\w+", etc.)
- Filter files by pattern with the include parameter (eg. "*.go", "*.{ts,tsx}")
- Returns file paths and line numbers with at least one match sorted by modification time
- Use this tool when you need to find files containing specific patterns
- You have the capability to call multiple tools in a single response. It is always better to speculatively perform multiple searches as a batch that are potentially useful.

## glob(pattern)
- Fast file pattern matching tool that works with any codebase size
- Supports glob patterns like "**/*.go" or "internal/**/*.go"
- Returns matching file paths sorted by modification time
- Use this tool when you need to find files by name patterns
- You have the capability to call multiple tools in a single response. It is always better to speculatively perform multiple searches as a batch that are potentially useful.

## read(file, start_line?, num_lines?)
- Reads a file from the local filesystem
- By default, it reads up to 200 lines starting from the beginning of the file
- You can optionally specify a line offset and limit (especially handy for long files)
- Results are returned with line numbers
- You have the capability to call multiple tools in a single response. It is always better to speculatively read multiple files as a batch that are potentially useful.

## graph(operation, target, depth?)
- Query code relationships from the indexed code graph
- This is powerful for understanding call chains, interface implementations, and code dependencies
- You have the capability to call multiple tools in a single response. It is always better to speculatively perform multiple graph queries as a batch that are potentially useful.

**Operations:**
- callers: Who calls this function/method?
- callees: What does this function/method call?
- implementations: What types implement this interface?
- methods: What methods does this type have?
- usages: Where is this type/function used?
- inheritors: What types embed this type?

**Target must be a qualified name (qname):**
Format: {module_path}/{package_path}.{Type}.{Method} or {module_path}/{package_path}.{Function}

This repository's Go module path is: %s

Examples:
- Function: %s/internal/brain.NewRetriever
- Method: %s/internal/brain.Retriever.Query
- Type: %s/internal/brain.Retriever
- Interface: %s/internal/store.Store

**How to construct a qname from grep results:**
1. grep finds: internal/brain/retriever.go:35: func (r *Retriever) Query(...)
2. The file path internal/brain/ → package path is internal/brain
3. The receiver (r *Retriever) → type is Retriever
4. The function name → method is Query
5. qname = %s/internal/brain.Retriever.Query

Complete the search request efficiently and report your findings clearly.`,
		r.modulePath,
		r.modulePath, r.modulePath, r.modulePath, r.modulePath,
		r.modulePath)
}
