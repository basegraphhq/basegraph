package spec

import (
	"context"
	"fmt"

	"basegraph.app/relay/internal/domain"
	"basegraph.app/relay/internal/llm"
)

// Generator produces the implementation spec when gaps are sufficiently resolved.
type Generator interface {
	Generate(ctx context.Context, issue *domain.Issue, context domain.ContextSnapshot, gaps []domain.Gap) (string, error)
}

type generator struct {
	llm llm.Client
}

func New(llmClient llm.Client) Generator {
	return &generator{llm: llmClient}
}

func (g *generator) Generate(ctx context.Context, issue *domain.Issue, context domain.ContextSnapshot, gaps []domain.Gap) (string, error) {
	if issue == nil {
		return "", fmt.Errorf("issue context required")
	}

	req := llm.SpecRequest{Issue: issue, Context: context, Gaps: gaps}
	return g.llm.GenerateSpec(ctx, req)
}
