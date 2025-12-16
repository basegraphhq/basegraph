package llm

import (
	"context"
	"strings"
)

// LLMClient exposes the LLM powered capabilities used within the pipeline.
type LLMClient interface {
	// ExtractKeywords extracts keywords from issue text
	ExtractKeywords(ctx context.Context, issueText string) ([]string, error)
	// DetectGaps detects gaps in requirements
	DetectGaps(ctx context.Context, issueText string) ([]string, error)
}

// NewClient creates a new LLM client (mock implementation for now)
func NewClient(apiKey string) LLMClient {
	return &mockClient{}
}

type mockClient struct{}

func (c *mockClient) ExtractKeywords(ctx context.Context, issueText string) ([]string, error) {
	// Simple mock implementation - extract words that look like technical terms
	words := strings.Fields(issueText)
	var keywords []string
	for _, word := range words {
		if len(word) > 4 && strings.ContainsAny(word, "-_.") {
			keywords = append(keywords, word)
		}
	}
	if len(keywords) == 0 {
		keywords = []string{"API", "integration", "backend"}
	}
	return keywords, nil
}

func (c *mockClient) DetectGaps(ctx context.Context, issueText string) ([]string, error) {
	// Simple mock implementation
	return []string{"Need more details about expected behavior", "Missing acceptance criteria"}, nil
}
