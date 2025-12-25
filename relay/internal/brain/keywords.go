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

type KeywordsResponse struct {
	Keywords []KeywordItem `json:"keywords" jsonschema_description:"Extracted keywords from the issue"`
}

type KeywordItem struct {
	Value    string  `json:"value" jsonschema_description:"The keyword (lowercase, snake_case normalized)"`
	Weight   float64 `json:"weight" jsonschema_description:"Relevance weight 0.0-1.0"`
	Category string  `json:"category" jsonschema:"enum=entity,enum=concept,enum=library" jsonschema_description:"Type of keyword"`
	Source   string  `json:"source" jsonschema:"enum=title,enum=description,enum=discussion" jsonschema_description:"Where the keyword was found"`
}

var keywordsSchema = llm.GenerateSchema[KeywordsResponse]()

type KeywordsExtractor struct {
	llm llm.Client
}

func NewKeywordsExtractor(client llm.Client) *KeywordsExtractor {
	return &KeywordsExtractor{llm: client}
}

const keywordsPromptVersion = "v2"

func (e *KeywordsExtractor) Extract(ctx context.Context, issue *model.Issue, evalStore store.LLMEvalStore) (*model.Issue, error) {
	prompt := e.buildPrompt(issue)
	if prompt == "" {
		slog.DebugContext(ctx, "no content to extract keywords from", "issue_id", issue.ID)
		return issue, nil
	}

	var response KeywordsResponse
	var llmResp *llm.Response
	start := time.Now()

	// Retry with exponential backoff (1s, 2s, 4s) to handle transient rate limits.
	// Keywords extraction is non-critical preprocessing - fail after 3 attempts
	// rather than blocking the pipeline indefinitely.
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		llmResp, err = e.llm.Chat(ctx, llm.Request{
			SystemPrompt: keywordsSystemPrompt,
			UserPrompt:   prompt,
			SchemaName:   "keywords_response",
			Schema:       keywordsSchema,
			Temperature:  llm.Temp(0.1), // Low temp for consistent extraction
		}, &response)

		if err == nil {
			break
		}
		if !llm.IsRetryable(ctx, err) {
			return nil, fmt.Errorf("keywords extraction: %w", err)
		}
		slog.WarnContext(ctx, "keywords extraction retry",
			"issue_id", issue.ID,
			"attempt", attempt+1,
			"error", err)
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("keywords extraction after 3 attempts: %w", err)
	}

	latency := time.Since(start)

	// Log to evals table for quality tracking
	e.logEval(ctx, evalStore, issue.ID, prompt, response, latency, llmResp)

	now := time.Now()
	keywords := make([]model.Keyword, len(response.Keywords))
	for i, k := range response.Keywords {
		keywords[i] = model.Keyword{
			Value:       k.Value,
			Weight:      k.Weight,
			Category:    k.Category,
			Source:      k.Source,
			ExtractedAt: now,
		}
	}

	issue.Keywords = keywords

	slog.InfoContext(ctx, "keywords extracted",
		"issue_id", issue.ID,
		"keyword_count", len(keywords),
		"latency_ms", latency.Milliseconds())

	return issue, nil
}

func (e *KeywordsExtractor) logEval(ctx context.Context, evalStore store.LLMEvalStore, issueID int64, prompt string, response KeywordsResponse, latency time.Duration, llmResp *llm.Response) {
	if evalStore == nil {
		return
	}

	outputJSON, err := json.Marshal(response)
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal keywords response for eval", "error", err)
		return
	}

	eval := &model.LLMEval{
		ID:            id.New(),
		IssueID:       int64Ptr(issueID),
		Stage:         "keywords",
		InputText:     prompt,
		OutputJSON:    outputJSON,
		Model:         e.llm.Model(),
		Temperature:   floatPtr(0.1),
		PromptVersion: stringPtr(keywordsPromptVersion),
		LatencyMs:     intPtr(int(latency.Milliseconds())),
	}

	if llmResp != nil {
		eval.PromptTokens = intPtr(llmResp.PromptTokens)
		eval.CompletionTokens = intPtr(llmResp.CompletionTokens)
	}

	if _, err := evalStore.Create(ctx, eval); err != nil {
		slog.ErrorContext(ctx, "failed to log eval", "error", err, "issue_id", issueID)
		// Don't fail extraction - eval logging is observability, not critical path
	}
}

func floatPtr(f float64) *float64 { return &f }
func stringPtr(s string) *string  { return &s }
func intPtr(i int) *int           { return &i }
func int64Ptr(i int64) *int64     { return &i }

func (e *KeywordsExtractor) buildPrompt(issue *model.Issue) string {
	var sb strings.Builder

	if issue.Title != nil && *issue.Title != "" {
		sb.WriteString("## Title\n")
		sb.WriteString(*issue.Title)
		sb.WriteString("\n\n")
	}

	if issue.Description != nil && *issue.Description != "" {
		sb.WriteString("## Description\n")
		sb.WriteString(*issue.Description)
		sb.WriteString("\n\n")
	}

	if len(issue.Discussions) > 0 {
		sb.WriteString("## Discussions\n")
		for _, d := range issue.Discussions {
			sb.WriteString(fmt.Sprintf("- [%s]: %s\n", d.Author, d.Body))
		}
	}

	return sb.String()
}

const keywordsSystemPrompt = `You extract keywords from software issues to find relevant code.

Think: "What would a developer search for to find the code that needs to change?"

## Categories

- entity: Code identifiers — function names, class names, file names, error types
- concept: Technical patterns — authentication, caching, rate_limiting, webhook
- library: Dependencies — redis, postgres, twilio, stripe, jwt

## Examples

Input: "Add rate limiting to the /api/users endpoint to prevent abuse"
Output:
- rate_limiting (concept, 0.85) — core feature being added
- api_users (entity, 0.8) — specific endpoint mentioned
- rate_limiter (entity, 0.75) — likely class/function name
- middleware (concept, 0.6) — common implementation pattern
- throttle (concept, 0.5) — alternative naming

Input: "Login fails when password contains special characters like @#$"
Output:
- login (concept, 0.85) — core functionality
- password (concept, 0.8) — specific area
- authentication (concept, 0.75) — broader system
- validation (concept, 0.7) — likely cause
- special_characters (concept, 0.6) — specific condition
- auth_handler (entity, 0.5) — likely code location

Input: "Integrate Stripe for payment processing"
Output:
- stripe (library, 0.95) — explicit dependency
- payment (concept, 0.85) — core domain
- payment_processor (entity, 0.7) — likely class name
- checkout (concept, 0.6) — related flow
- billing (concept, 0.5) — related area

## Rules

- Lowercase, snake_case: "Rate Limiting" → rate_limiting
- Max 15 keywords
- Higher weight = more specific to this issue
- Think about what code files/functions would implement this

## Do NOT extract

- Action verbs: add, fix, update, implement, change
- Filler words: please, should, would, need, want
- Vague terms: feature, functionality, issue, problem`
