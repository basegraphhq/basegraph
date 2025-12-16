package gap

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/internal/domain"
	"basegraph.app/relay/internal/llm"
)

// Detector runs the gap detection stage given the enriched issue context.
type Detector interface {
	Detect(ctx context.Context, event domain.Event, issue *domain.Issue) (*domain.GapAnalysis, error)
}

type detector struct {
	llm    llm.Client
	logger *slog.Logger
}

func New(llmClient llm.Client, logger *slog.Logger) Detector {
	if logger == nil {
		logger = slog.Default()
	}
	return &detector{llm: llmClient, logger: logger}
}

func (d *detector) Detect(ctx context.Context, event domain.Event, issue *domain.Issue) (*domain.GapAnalysis, error) {
	if issue == nil {
		return nil, fmt.Errorf("issue context required")
	}

	req := llm.GapRequest{
		Issue:   issue,
		Event:   event,
		Context: domain.ContextSnapshot{Keywords: issue.Keywords, CodeFindings: issue.CodeFindings, Learnings: issue.Learnings, Discussions: issue.Discussions},
	}
	analysis, err := d.llm.DetectGaps(ctx, req)
	if err != nil {
		return nil, err
	}
	if analysis == nil {
		analysis = &domain.GapAnalysis{Gaps: []domain.Gap{}, Questions: []domain.Discussion{}, ReadyForSpec: false, Confidence: 0.0}
	}
	return analysis, nil
}
