package spec

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/internal/domain"
	"basegraph.app/relay/internal/llm"
)

// Generator produces the implementation spec when gaps are sufficiently resolved.
type Generator interface {
	Generate(ctx context.Context, issue *domain.Issue, context domain.ContextSnapshot, gaps []domain.Gap) (string, error)
}

type generator struct {
	llm    llm.Client
	logger *slog.Logger
}

func New(llmClient llm.Client, logger *slog.Logger) Generator {
	if logger == nil {
		logger = slog.Default()
	}
	return &generator{llm: llmClient, logger: logger}
}

func (g *generator) Generate(ctx context.Context, issue *domain.Issue, context domain.ContextSnapshot, gaps []domain.Gap) (string, error) {
	if issue == nil {
		return "", fmt.Errorf("issue context required")
	}

	req := llm.SpecRequest{Issue: issue, Context: context, Gaps: gaps}
	return g.llm.GenerateSpec(ctx, req)
}
