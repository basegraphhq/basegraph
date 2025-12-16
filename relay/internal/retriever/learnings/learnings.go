package learnings

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"basegraph.app/relay/internal/domain"
)

// Provider returns project or domain learnings (mock implementation for MVP).
type Provider interface {
	Retrieve(ctx context.Context, event domain.Event, issue *domain.Issue, keywords []domain.Keyword) ([]domain.Learning, error)
}

type provider struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) Provider {
	if logger == nil {
		logger = slog.Default()
	}
	return &provider{logger: logger}
}

func (p *provider) Retrieve(ctx context.Context, event domain.Event, issue *domain.Issue, keywords []domain.Keyword) ([]domain.Learning, error) { //nolint: revive // ctx reserved for future use
	_ = ctx
	if issue == nil {
		return nil, fmt.Errorf("issue context required")
	}

	if len(keywords) == 0 {
		return nil, nil
	}

	learnings := make([]domain.Learning, 0, 1)
	now := time.Now()
	text := fmt.Sprintf("Consider historical decisions impacting '%s'. Ensure existing contracts remain intact.", keywords[0].Value)
	learnings = append(learnings, domain.Learning{Text: text, UpdatedBy: "relay", UpdatedAt: now})
	return learnings, nil
}
