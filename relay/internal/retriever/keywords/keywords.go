package keywords

import (
	"context"
	"fmt"

	"basegraph.app/relay/internal/domain"
	"basegraph.app/relay/internal/llm"
)

// Extractor generates keywords for an issue using the LLM client.
type Extractor interface {
	Extract(ctx context.Context, event domain.Event, issue *domain.Issue, payload map[string]any) ([]domain.Keyword, error)
}

type extractor struct {
	llm llm.Client
}

func New(llmClient llm.Client) Extractor {
	return &extractor{llm: llmClient}
}

func (e *extractor) Extract(ctx context.Context, event domain.Event, issue *domain.Issue, payload map[string]any) ([]domain.Keyword, error) {
	if issue == nil {
		return nil, fmt.Errorf("issue context required")
	}

	text := ""
	if payload != nil {
		if value, ok := payload["text"].(string); ok {
			text = value
		}
	}
	if text == "" {
		text = string(event.Payload)
	}

	keywords, err := e.llm.ExtractKeywords(ctx, llm.KeywordRequest{
		Issue:    issue,
		Text:     text,
		Event:    event,
		Existing: issue.Keywords,
	})
	if err != nil {
		return nil, err
	}

	return keywords, nil
}
