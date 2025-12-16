package llm

import (
	"context"
	"fmt"

	"basegraph.app/relay/internal/domain"
)

// Client exposes the LLM powered capabilities used within the pipeline.
type Client interface {
	// ExtractKeywords extracts keywords from issue text
	ExtractKeywords(ctx context.Context, req KeywordRequest) ([]domain.Keyword, error)
	// DetectGaps detects gaps in requirements
	DetectGaps(ctx context.Context, req GapRequest) (*domain.GapAnalysis, error)
	// GenerateSpec generates a technical specification
	GenerateSpec(ctx context.Context, req SpecRequest) (string, error)
}

// NewClient creates a new LLM client (returns error if no API key provided)
func NewClient(apiKey string) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	// For now, return an error indicating that OpenAI integration is not implemented
	// TODO: Implement proper OpenAI integration with the official SDK
	return nil, fmt.Errorf("OpenAI integration not yet implemented - API key provided but SDK not configured")
}
