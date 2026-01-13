package store

import (
	"context"
	"errors"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
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

func (s *gapStore) GetByShortID(ctx context.Context, shortID int64) (model.Gap, error) {
	row, err := s.queries.GetGapByShortID(ctx, shortID)
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

func (s *gapStore) ListPendingByIssue(ctx context.Context, issueID int64) ([]model.Gap, error) {
	rows, err := s.queries.ListPendingGapsByIssue(ctx, issueID)
	if err != nil {
		return nil, err
	}
	return toGapModels(rows), nil
}

func (s *gapStore) ListClosedByIssue(ctx context.Context, issueID int64, limit int32) ([]model.Gap, error) {
	rows, err := s.queries.ListClosedGapsByIssue(ctx, sqlc.ListClosedGapsByIssueParams{
		IssueID: issueID,
		Limit:   limit,
	})
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

func (s *gapStore) Close(ctx context.Context, id int64, status model.GapStatus, reason, note string) (model.Gap, error) {
	var closedReason *string
	var closedNote *string
	if reason != "" {
		closedReason = &reason
	}
	if note != "" {
		closedNote = &note
	}

	row, err := s.queries.CloseGap(ctx, sqlc.CloseGapParams{
		ID:           id,
		Status:       string(status),
		ClosedReason: closedReason,
		ClosedNote:   closedNote,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Gap{}, ErrNotFound
		}
		return model.Gap{}, err
	}
	return toGapModel(row), nil
}

func (s *gapStore) Open(ctx context.Context, id int64) (model.Gap, error) {
	row, err := s.queries.OpenGap(ctx, id)
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
		ID:           row.ID,
		ShortID:      row.ShortID,
		IssueID:      row.IssueID,
		Status:       model.GapStatus(row.Status),
		ClosedReason: "",
		ClosedNote:   "",
		Respondent:   model.GapRespondent(row.Respondent),
		LearningID:   row.LearningID,
		Severity:     model.GapSeverity(row.Severity),
		Question:     row.Question,
		CreatedAt:    row.CreatedAt.Time,
	}

	if row.ResolvedAt.Valid {
		gap.ResolvedAt = &row.ResolvedAt.Time
	}
	if row.Evidence == nil {
		gap.Evidence = ""
	} else {
		gap.Evidence = *row.Evidence
	}
	if row.ClosedReason != nil {
		gap.ClosedReason = *row.ClosedReason
	}
	if row.ClosedNote != nil {
		gap.ClosedNote = *row.ClosedNote
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
