package store

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type learningStore struct {
	queries *sqlc.Queries
}

func newLearningStore(queries *sqlc.Queries) LearningStore {
	return &learningStore{queries: queries}
}

func (s *learningStore) GetByID(ctx context.Context, id int64) (*model.Learning, error) {
	row, err := s.queries.GetLearning(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toLearningModel(row), nil
}

func (s *learningStore) GetByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) (*model.Learning, error) {
	// For now, return the first match. In future, this might need more sophisticated logic
	rows, err := s.queries.ListLearningsByWorkspaceAndType(ctx, sqlc.ListLearningsByWorkspaceAndTypeParams{
		WorkspaceID: workspaceID,
		Type:        learningType,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return toLearningModel(rows[0]), nil
}

func (s *learningStore) Create(ctx context.Context, learning *model.Learning) error {
	row, err := s.queries.CreateLearning(ctx, sqlc.CreateLearningParams{
		ID:                   learning.ID,
		WorkspaceID:          learning.WorkspaceID,
		RuleUpdatedByIssueID: learning.RuleUpdatedByIssueID,
		Type:                 learning.Type,
		Content:              learning.Content,
	})
	if err != nil {
		return err
	}
	*learning = *toLearningModel(row)
	return nil
}

func (s *learningStore) Update(ctx context.Context, learning *model.Learning) error {
	row, err := s.queries.UpdateLearning(ctx, sqlc.UpdateLearningParams{
		ID:      learning.ID,
		Content: learning.Content,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	*learning = *toLearningModel(row)
	return nil
}

func (s *learningStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteLearning(ctx, id)
}

func (s *learningStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Learning, error) {
	rows, err := s.queries.ListLearningsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return toLearningModels(rows), nil
}

func (s *learningStore) ListByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) ([]model.Learning, error) {
	rows, err := s.queries.ListLearningsByWorkspaceAndType(ctx, sqlc.ListLearningsByWorkspaceAndTypeParams{
		WorkspaceID: workspaceID,
		Type:        learningType,
	})
	if err != nil {
		return nil, err
	}
	return toLearningModels(rows), nil
}

func toLearningModel(row sqlc.Learning) *model.Learning {
	return &model.Learning{
		ID:                   row.ID,
		ShortID:              row.ShortID,
		WorkspaceID:          row.WorkspaceID,
		RuleUpdatedByIssueID: row.RuleUpdatedByIssueID,
		Type:                 row.Type,
		Content:              row.Content,
		CreatedAt:            row.CreatedAt.Time,
		UpdatedAt:            row.UpdatedAt.Time,
	}
}

func toLearningModels(rows []sqlc.Learning) []model.Learning {
	result := make([]model.Learning, len(rows))
	for i, row := range rows {
		result[i] = *toLearningModel(row)
	}
	return result
}
