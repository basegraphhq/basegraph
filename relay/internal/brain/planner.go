package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type RetrieverType string

const (
	RetrieverTypeCodeGraph RetrieverType = "codegraph"
	RetrieverTypeLearnings RetrieverType = "learnings"
)

type RetrieverJob struct {
	Type     RetrieverType `json:"type" jsonschema:"enum=codegraph,enum=learnings" jsonschema_description:"The retriever to use"`
	Query    string        `json:"query" jsonschema_description:"The search query or question to answer"`
	Intent   string        `json:"intent" jsonschema_description:"What information this retrieval aims to find"`
	Priority int           `json:"priority" jsonschema_description:"Execution priority 1-3 (1=highest)"`

	SymbolHints   []string `json:"symbol_hints" jsonschema_description:"Expected symbol names, function names, or types to search for"`
	Kinds         []string `json:"kinds" jsonschema_description:"Node kinds to filter: function, struct, interface, method"`
	LearningTypes []string `json:"learning_types" jsonschema_description:"Learning categories: project_standards, codebase_standards, domain_knowledge"`
}

type PlannerResponse struct {
	ContextSufficient bool           `json:"context_sufficient" jsonschema_description:"True if current issue context is enough to generate a spec without retrieval"`
	Reasoning         string         `json:"reasoning" jsonschema_description:"Brief explanation of the decision"`
	Jobs              []RetrieverJob `json:"jobs" jsonschema_description:"Retrieval jobs to execute if context is insufficient"`
}

var plannerSchema = llm.GenerateSchema[PlannerResponse]()

type Planner struct {
	llm llm.Client
}

func NewPlanner(client llm.Client) *Planner {
	return &Planner{llm: client}
}

const plannerPromptVersion = "v1"

func (p *Planner) Plan(ctx context.Context, issue *model.Issue, evalStore store.LLMEvalStore) (*PlannerResponse, error) {
	prompt := p.buildPrompt(issue)
	if prompt == "" {
		slog.DebugContext(ctx, "no content to plan from", "issue_id", issue.ID)
		return &PlannerResponse{
			ContextSufficient: true,
			Reasoning:         "Empty issue - no content to analyze",
			Jobs:              nil,
		}, nil
	}

	var response PlannerResponse
	var llmResp *llm.Response
	start := time.Now()

	// Retry with exponential backoff to handle transient rate limits
	var err error
	for attempt := range 3 {
		llmResp, err = p.llm.Chat(ctx, llm.Request{
			SystemPrompt: plannerSystemPrompt,
			UserPrompt:   prompt,
			SchemaName:   "planner_response",
			Schema:       plannerSchema,
			Temperature:  llm.Temp(0.1),
		}, &response)

		if err == nil {
			break
		}
		if !llm.IsRetryable(ctx, err) {
			return nil, fmt.Errorf("planner: %w", err)
		}
		slog.WarnContext(ctx, "planner retry",
			"issue_id", issue.ID,
			"attempt", attempt+1,
			"error", err)
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("planner after 3 attempts: %w", err)
	}

	latency := time.Since(start)
	p.logEval(ctx, evalStore, issue.ID, prompt, response, latency, llmResp)

	slog.InfoContext(ctx, "planner completed",
		"issue_id", issue.ID,
		"context_sufficient", response.ContextSufficient,
		"job_count", len(response.Jobs),
		"latency_ms", latency.Milliseconds())

	return &response, nil
}

func (p *Planner) logEval(ctx context.Context, evalStore store.LLMEvalStore, issueID int64, prompt string, response PlannerResponse, latency time.Duration, llmResp *llm.Response) {
	if evalStore == nil {
		return
	}

	outputJSON, err := json.Marshal(response)
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal planner response for eval", "error", err)
		return
	}

	eval := &model.LLMEval{
		ID:            id.New(),
		IssueID:       int64Ptr(issueID),
		Stage:         "planner",
		InputText:     prompt,
		OutputJSON:    outputJSON,
		Model:         p.llm.Model(),
		Temperature:   floatPtr(0.1),
		PromptVersion: stringPtr(plannerPromptVersion),
		LatencyMs:     intPtr(int(latency.Milliseconds())),
	}

	if llmResp != nil {
		eval.PromptTokens = intPtr(llmResp.PromptTokens)
		eval.CompletionTokens = intPtr(llmResp.CompletionTokens)
	}

	if _, err := evalStore.Create(ctx, eval); err != nil {
		// Eval logging is observability, not critical path
		slog.ErrorContext(ctx, "failed to log planner eval", "error", err, "issue_id", issueID)
	}
}

func (p *Planner) buildPrompt(issue *model.Issue) string {
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

	if len(issue.Keywords) > 0 {
		sb.WriteString("## Extracted Keywords\n")
		for _, k := range issue.Keywords {
			sb.WriteString(fmt.Sprintf("- %s (category: %s, weight: %.2f)\n", k.Value, k.Category, k.Weight))
		}
		sb.WriteString("\n")
	}

	if len(issue.CodeFindings) > 0 {
		sb.WriteString("## Code Context (Already Retrieved)\n")
		for _, cf := range issue.CodeFindings {
			sb.WriteString(fmt.Sprintf("### %s\n", cf.Observation))
			if len(cf.Sources) > 0 {
				locations := make([]string, len(cf.Sources))
				for i, src := range cf.Sources {
					locations[i] = src.Location
				}
				sb.WriteString("Sources: " + strings.Join(locations, ", ") + "\n")
			}
			sb.WriteString("\n")
		}
	}

	if len(issue.Learnings) > 0 {
		sb.WriteString("## Team Learnings (Already Retrieved)\n")
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

const plannerSystemPrompt = `You are a planning agent that decides what context to gather before gap analysis.

## How This Works

This is a ONE-SHOT decision. You will NOT see the retrieval results.

Pipeline: Planner (you) → Retriever → Gap Detector → Questions/Spec

Your job: Decide what code context and team knowledge to fetch BEFORE the Gap Detector analyzes the issue.

## Your Decision

- context_sufficient=true: The issue is clear enough. Skip retrieval, proceed to gap analysis.
- context_sufficient=false: We need more context. Specify what to retrieve.

## Available Retrievers

### codegraph
Query the code graph to find:
- Function signatures and call chains
- Struct/interface definitions
- File locations and import relationships
- Implementation patterns

Use when the issue references code you haven't seen.

### learnings
Query team knowledge to find:
- Conventions and standards
- Past decisions and rationale
- Known gotchas and edge cases

Use when team preferences or domain rules might affect the implementation.

## Output Rules

1. Be SPECIFIC with queries:
   - Bad: "Find code related to users"
   - Good: "Find UserService.Create() and its validation logic"
2. Limit to 3-5 jobs (prioritize what's critical for gap analysis)
3. Priority 1 = most important

## Examples

### Simple Bug Fix (Skip Retrieval)
Issue: "Fix typo in login error message - says 'pasword' instead of 'password'"
Keywords: [login, auth_handler, error_message, password]

-> context_sufficient: true
-> reasoning: "Clear typo fix. Keywords point to auth_handler. Gap Detector can proceed."

### Feature Addition (Needs Context)
Issue: "Add Stripe payment integration"
Keywords: [stripe, payment, checkout, billing_service]

-> context_sufficient: false
-> reasoning: "Need to understand existing payment architecture before identifying gaps."
-> jobs: [
    {type: codegraph, query: "BillingService payment processing", intent: "Understand existing payment flow", priority: 1},
    {type: learnings, query: "payment processing security", intent: "Find team security requirements", priority: 2}
]`
