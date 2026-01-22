package store

import (
	"context"
	"errors"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type gapStore struct {
	queries *sqlc.Queries
}

func newGapStore(queries *sqlc.Queries) GapStore {
	return &gapStore{queries: queries}
}

func (s *gapStore) Create(ctx context.Context, gap model.Gap) (model.Gap, error) {
	var evidence *string
	if gap.Evidence != "" {
		evidence = &gap.Evidence
	}
	row, err := s.queries.CreateGap(ctx, sqlc.CreateGapParams{
		ID:         id.New(),
		IssueID:    gap.IssueID,
		Status:     string(gap.Status),
		Evidence:   evidence,
		Question:   gap.Question,
		Severity:   string(gap.Severity),
		Respondent: string(gap.Respondent),
	})
	if err != nil {
		return model.Gap{}, err
	}
	return toGapModel(row), nil
}

func (s *gapStore) GetByID(ctx context.Context, id int64) (model.Gap, error) {
	row, err := s.queries.GetGap(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Gap{}, ErrNotFound
		}
		return model.Gap{}, err
	}

	return toGapModel(row), nil
}

func (s *gapStore) ListByIssue(ctx context.Context, issueID int64) ([]model.Gap, error) {
	rows, err := s.queries.ListGapsByIssue(ctx, issueID)
	if err != nil {
		return nil, err
	}
	return toGapModels(rows), nil
}

func (s *gapStore) ListOpenByIssue(ctx context.Context, issueID int64) ([]model.Gap, error) {
	rows, err := s.queries.ListOpenGapsByIssue(ctx, issueID)
	if err != nil {
		return nil, err
	}
	return toGapModels(rows), nil
}

func (s *gapStore) Resolve(ctx context.Context, id int64) (model.Gap, error) {
	row, err := s.queries.ResolveGap(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Gap{}, ErrNotFound
		}
		return model.Gap{}, err
	}

	return toGapModel(row), nil
}

func (s *gapStore) Skip(ctx context.Context, id int64) (model.Gap, error) {
	row, err := s.queries.SkipGap(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Gap{}, ErrNotFound
		}
		return model.Gap{}, err
	}
	return toGapModel(row), nil
}

func (s *gapStore) SetLearning(ctx context.Context, id int64, learningID int64) (model.Gap, error) {
	row, err := s.queries.SetGapLearning(ctx, sqlc.SetGapLearningParams{
		ID:         id,
		LearningID: &learningID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Gap{}, ErrNotFound
		}
		return model.Gap{}, err
	}
	return toGapModel(row), nil
}

func (s *gapStore) CountOpenBlocking(ctx context.Context, issueID int64) (int64, error) {
	count, err := s.queries.CountOpenBlockingGapsByIssue(ctx, issueID)
	if err != nil {
		return -1, err
	}
	return count, nil
}

func toGapModel(row sqlc.Gap) model.Gap {
	gap := model.Gap{
		ID:         row.ID,
		IssueID:    row.IssueID,
		Status:     model.GapStatus(row.Status),
		Respondent: model.GapRespondent(row.Respondent),
		LearningID: row.LearningID,
		Severity:   model.GapSeverity(row.Severity),
		Question:   row.Question,
		CreatedAt:  row.CreatedAt.Time,
	}

	if row.ResolvedAt.Valid {
		gap.ResolvedAt = &row.ResolvedAt.Time
	}
	if row.Evidence == nil {
		gap.Evidence = ""
	} else {
		gap.Evidence = *row.Evidence
	}

	return gap
}

func toGapModels(rows []sqlc.Gap) []model.Gap {
	result := make([]model.Gap, len(rows))
	for i, row := range rows {
		result[i] = toGapModel(row)
	}
	return result
}
