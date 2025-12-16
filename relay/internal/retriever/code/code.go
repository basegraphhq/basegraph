package code

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"basegraph.app/relay/internal/domain"
)

// Retriever queries the code graph for relevant findings (mock implementation for MVP).
type Retriever interface {
	Retrieve(ctx context.Context, event domain.Event, issue *domain.Issue, keywords []domain.Keyword) ([]domain.CodeFinding, error)
}

type retriever struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) Retriever {
	if logger == nil {
		logger = slog.Default()
	}
	return &retriever{logger: logger}
}

func (r *retriever) Retrieve(ctx context.Context, event domain.Event, issue *domain.Issue, keywords []domain.Keyword) ([]domain.CodeFinding, error) { //nolint: revive // ctx reserved for future use
	_ = ctx
	if issue == nil {
		return nil, fmt.Errorf("issue context required")
	}

	if len(keywords) == 0 {
		r.logger.Info("code retriever received no keywords", "issue_id", issue.ID, "event_id", event.ID)
		return nil, nil
	}

	findings := make([]domain.CodeFinding, 0, len(keywords))
	now := time.Now()
	for _, kw := range keywords {
		if kw.Value == "" {
			continue
		}
		summary := fmt.Sprintf("Review code paths related to '%s'", kw.Value)
		sources := []string{fmt.Sprintf("typesense://project/%d/%s", issue.ID, strings.ReplaceAll(kw.Value, " ", "-"))}
		findings = append(findings, domain.CodeFinding{
			Summary:          summary,
			Severity:         deriveSeverity(kw.Value),
			Sources:          sources,
			SuggestedActions: []string{"Inspect callers and implementors for regression risk."},
			DetectedAt:       now,
		})
	}
	return findings, nil
}

func deriveSeverity(keyword string) domain.GapSeverity {
	keyword = strings.ToLower(keyword)
	switch {
	case strings.Contains(keyword, "auth"), strings.Contains(keyword, "security"):
		return domain.GapSeverityHigh
	case strings.Contains(keyword, "db"), strings.Contains(keyword, "migration"):
		return domain.GapSeverityMedium
	default:
		return domain.GapSeverityLow
	}
}
