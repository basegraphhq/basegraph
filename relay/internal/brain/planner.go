package brain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/model"
)

const (
	maxParallelRetrievers = 6 // Limit for retriever sub agent
)

type RetrieveParams struct {
	Query string `json:"query" jsonschema:"required,description=Question about the codebase to explore. Be specific about what you need to understand."`
}

// PlanResult contains accumulated context for downstream agents.
type PlanResult struct {
	Context   string // Accumulated retrieval reports (prose with code snippets)
	Reasoning string // Why context is sufficient (or final thoughts)
}

// Planner gathers code context for issue scoping.
// It spawns Retriever sub-agents to explore the codebase, preserving its own context window.
type Planner struct {
	llm       llm.AgentClient
	retriever *Retriever
	debugDir  string // Directory for debug logs (empty = no logging)
}

// NewPlanner creates a Planner with a Retriever sub-agent.
func NewPlanner(llmClient llm.AgentClient, retriever *Retriever) *Planner {
	return &Planner{
		llm:       llmClient,
		retriever: retriever,
		debugDir:  os.Getenv("BRAIN_DEBUG_DIR"),
	}
}

// Plan gathers code context by spawning retrieval sub-agents.
// Returns when sufficient context is gathered for gap analysis.
func (p *Planner) Plan(ctx context.Context, issue *model.Issue) (*PlanResult, error) {
	prompt := formatIssue(issue)
	if prompt == "" {
		slog.DebugContext(ctx, "no content to plan from", "issue_id", issue.ID)
		return &PlanResult{
			Context:   "",
			Reasoning: "Empty issue - no content to analyze",
		}, nil
	}

	messages := []llm.Message{
		{Role: "system", Content: plannerSystemPrompt},
		{Role: "user", Content: prompt},
	}

	// Debug logging
	sessionID := time.Now().Format("20060102-150405")
	var debugLog strings.Builder
	debugLog.WriteString(fmt.Sprintf("=== PLANNER SESSION %s ===\n", sessionID))
	debugLog.WriteString(fmt.Sprintf("Issue ID: %d\n\n", issue.ID))
	debugLog.WriteString(fmt.Sprintf("[USER]\n%s\n\n", prompt))

	var accumulatedContext strings.Builder
	iterations := 0

	slog.InfoContext(ctx, "planner starting", "issue_id", issue.ID)

	for {
		iterations++

		resp, err := p.llm.ChatWithTools(ctx, llm.AgentRequest{
			Messages: messages,
			Tools:    p.tools(),
		})
		if err != nil {
			p.writeDebugLog(sessionID, debugLog.String())
			return nil, fmt.Errorf("planner chat iteration %d: %w", iterations, err)
		}

		// Log assistant response
		debugLog.WriteString(fmt.Sprintf("--- ITERATION %d ---\n", iterations))
		debugLog.WriteString(fmt.Sprintf("[ASSISTANT]\n%s\n\n", resp.Content))

		// No tool calls = done planning
		if len(resp.ToolCalls) == 0 {
			debugLog.WriteString("=== PLANNER COMPLETED ===\n")
			debugLog.WriteString(fmt.Sprintf("\nAccumulated Context Length: %d\n", accumulatedContext.Len()))
			p.writeDebugLog(sessionID, debugLog.String())

			slog.InfoContext(ctx, "planner completed",
				"issue_id", issue.ID,
				"iterations", iterations,
				"context_length", accumulatedContext.Len())

			return &PlanResult{
				Context:   accumulatedContext.String(),
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

		results := p.executeRetrievalsParallel(ctx, resp.ToolCalls)

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
	if p.debugDir == "" {
		return
	}

	if err := os.MkdirAll(p.debugDir, 0o755); err != nil {
		slog.Warn("failed to create debug dir", "dir", p.debugDir, "error", err)
		return
	}

	filename := filepath.Join(p.debugDir, fmt.Sprintf("planner_%s.txt", sessionID))
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		slog.Warn("failed to write debug log", "file", filename, "error", err)
	} else {
		slog.Info("debug log written", "file", filename)
	}
}

type retrievalResult struct {
	callID string
	report string
}

// executeRetrievalsParallel runs multiple retrieve calls concurrently with bounded parallelism.
func (p *Planner) executeRetrievalsParallel(ctx context.Context, toolCalls []llm.ToolCall) []retrievalResult {
	results := make([]retrievalResult, len(toolCalls))
	var wg sync.WaitGroup

	// Semaphore to limit concurrent retrievers
	sem := make(chan struct{}, maxParallelRetrievers)

	for i, tc := range toolCalls {
		if tc.Name != "retrieve" {
			results[i] = retrievalResult{
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

			params, err := llm.ParseToolArguments[RetrieveParams](call.Arguments)
			if err != nil {
				results[idx] = retrievalResult{
					callID: call.ID,
					report: fmt.Sprintf("Error parsing arguments: %s", err),
				}
				return
			}

			slog.InfoContext(ctx, "planner spawning retriever",
				"query", truncate(params.Query, 100),
				"slot", idx+1,
				"total", len(toolCalls))

			report, err := p.retriever.Query(ctx, params.Query)
			if err != nil {
				report = fmt.Sprintf("Retrieval error: %s", err)
			}

			results[idx] = retrievalResult{
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
			Name:        "retrieve",
			Description: "Explore the codebase to gather context. Ask a specific question about code structure, relationships, or behavior. Returns a prose report with relevant code snippets. You can call this multiple times in parallel for independent questions.",
			Parameters:  llm.GenerateSchemaFrom(RetrieveParams{}),
		},
	}
}

func formatIssue(issue *model.Issue) string {
	var sb strings.Builder

	if issue.Title != nil && *issue.Title != "" {
		sb.WriteString("## Issue Title\n")
		sb.WriteString(*issue.Title)
		sb.WriteString("\n\n")
	}

	if issue.Description != nil && *issue.Description != "" {
		sb.WriteString("## Issue Description\n")
		sb.WriteString(*issue.Description)
		sb.WriteString("\n\n")
	}

	if len(issue.Learnings) > 0 {
		sb.WriteString("## Team Learnings\n")
		for _, l := range issue.Learnings {
			sb.WriteString(fmt.Sprintf("- [%s]: %s\n", l.Type, l.Content))
		}
		sb.WriteString("\n")
	}

	if len(issue.Discussions) > 0 {
		sb.WriteString("## Discussion History\n")
		for _, d := range issue.Discussions {
			sb.WriteString(fmt.Sprintf("- [%s]: %s\n", d.Author, d.Body))
		}
	}

	return sb.String()
}

const plannerSystemPrompt = `You gather code context for issue scoping. You have one tool:

- retrieve(query) — Ask a specific question about the codebase. A search specialist will find the answer.

# Tool usage policy

- You can call multiple tools in a single response. If you intend to call multiple tools and there are no dependencies between them, make all independent tool calls in parallel. Maximize use of parallel tool calls where possible to increase efficiency. However, if some tool calls depend on previous calls to inform dependent values, do NOT call these tools in parallel and instead call them sequentially. For instance, if one operation must complete before another starts, run these operations sequentially instead. Never use placeholders or guess missing parameters in tool calls.
- Launch multiple retrieve calls concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses.
- It is always better to speculatively perform multiple retrievals as a batch that are potentially useful.

# How to Ask Good Questions

Ask bounded, specific questions:
- "Find the function that sends notifications" (not "How does notification work?")
- "What calls PaymentService.process?" (not "Explain the payment flow")
- "What types implement the Storage interface?"
- "Find where user permissions are checked"

Each retrieve spawns a search agent. Be specific so it knows when it's done.

# Workflow

1. Read the issue and identify 2-4 key areas to explore
2. Call retrieve() for multiple independent questions in parallel
3. Review the answers, then decide: need more context or done?
4. If needed, call more retrievals in parallel for follow-up questions
5. Usually 1-2 batches of parallel retrievals is enough

# When Done

Stop when you can identify:
- What code is affected
- Key functions/types involved

Write a brief summary of what you gathered. Don't over-explore.`
