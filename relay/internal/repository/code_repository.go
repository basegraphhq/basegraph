package repository

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type codeRepositoryRepository struct {
	queries *sqlc.Queries
}

// NewRepositoryRepository creates a new RepositoryRepository for code repositories
func NewRepositoryRepository(queries *sqlc.Queries) RepositoryRepository {
	return &codeRepositoryRepository{queries: queries}
}

func (r *codeRepositoryRepository) GetByID(ctx context.Context, id int64) (*model.Repository, error) {
	row, err := r.queries.GetRepository(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toRepositoryModel(row), nil
}

func (r *codeRepositoryRepository) GetByExternalID(ctx context.Context, integrationID int64, externalRepoID string) (*model.Repository, error) {
	row, err := r.queries.GetRepositoryByExternalID(ctx, sqlc.GetRepositoryByExternalIDParams{
		IntegrationID:  integrationID,
		ExternalRepoID: externalRepoID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toRepositoryModel(row), nil
}

func (r *codeRepositoryRepository) Create(ctx context.Context, repo *model.Repository) error {
	row, err := r.queries.CreateRepository(ctx, sqlc.CreateRepositoryParams{
		ID:             repo.ID,
		WorkspaceID:    repo.WorkspaceID,
		IntegrationID:  repo.IntegrationID,
		Name:           repo.Name,
		Slug:           repo.Slug,
		Url:            repo.URL,
		Description:    repo.Description,
		ExternalRepoID: repo.ExternalRepoID,
	})
	if err != nil {
		return err
	}
	*repo = *toRepositoryModel(row)
	return nil
}

func (r *codeRepositoryRepository) Update(ctx context.Context, repo *model.Repository) error {
	row, err := r.queries.UpdateRepository(ctx, sqlc.UpdateRepositoryParams{
		ID:          repo.ID,
		Name:        repo.Name,
		Slug:        repo.Slug,
		Url:         repo.URL,
		Description: repo.Description,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	*repo = *toRepositoryModel(row)
	return nil
}

func (r *codeRepositoryRepository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteRepository(ctx, id)
}

func (r *codeRepositoryRepository) DeleteByIntegration(ctx context.Context, integrationID int64) error {
	return r.queries.DeleteRepositoriesByIntegration(ctx, integrationID)
}

func (r *codeRepositoryRepository) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Repository, error) {
	rows, err := r.queries.ListRepositoriesByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return toRepositoryModels(rows), nil
}

func (r *codeRepositoryRepository) ListByIntegration(ctx context.Context, integrationID int64) ([]model.Repository, error) {
	rows, err := r.queries.ListRepositoriesByIntegration(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	return toRepositoryModels(rows), nil
}

// toRepositoryModel converts sqlc.Repository to model.Repository
func toRepositoryModel(row sqlc.Repository) *model.Repository {
	return &model.Repository{
		ID:             row.ID,
		WorkspaceID:    row.WorkspaceID,
		IntegrationID:  row.IntegrationID,
		Name:           row.Name,
		Slug:           row.Slug,
		URL:            row.Url,
		Description:    row.Description,
		ExternalRepoID: row.ExternalRepoID,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func toRepositoryModels(rows []sqlc.Repository) []model.Repository {
	result := make([]model.Repository, len(rows))
	for i, row := range rows {
		result[i] = *toRepositoryModel(row)
	}
	return result
}
