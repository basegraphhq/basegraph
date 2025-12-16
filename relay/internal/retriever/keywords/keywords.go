package keywords

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/internal/domain"
	"basegraph.app/relay/internal/llm"
)

// Extractor generates keywords for an issue using the LLM client.
type Extractor interface {
	Extract(ctx context.Context, event domain.Event, issue *domain.Issue, payload map[string]any) ([]domain.Keyword, error)
}

type extractor struct {
	llm    llm.Client
	logger *slog.Logger
}

func New(llmClient llm.Client, logger *slog.Logger) Extractor {
	if logger == nil {
		logger = slog.Default()
	}
	return &extractor{llm: llmClient, logger: logger}
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
