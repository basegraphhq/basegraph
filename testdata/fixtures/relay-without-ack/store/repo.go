package store

import (
	"context"
	"errors"

	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type repoStore struct {
	queries *sqlc.Queries
}

func newRepoStore(queries *sqlc.Queries) RepoStore {
	return &repoStore{queries: queries}
}

func (s *repoStore) GetByID(ctx context.Context, id int64) (*model.Repository, error) {
	row, err := s.queries.GetRepository(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toRepoModel(row), nil
}

func (s *repoStore) GetByExternalID(ctx context.Context, integrationID int64, externalRepoID string) (*model.Repository, error) {
	row, err := s.queries.GetRepositoryByExternalID(ctx, sqlc.GetRepositoryByExternalIDParams{
		IntegrationID:  integrationID,
		ExternalRepoID: externalRepoID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toRepoModel(row), nil
}

func (s *repoStore) Create(ctx context.Context, repo *model.Repository) error {
	row, err := s.queries.CreateRepository(ctx, sqlc.CreateRepositoryParams{
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
	*repo = *toRepoModel(row)
	return nil
}

func (s *repoStore) Update(ctx context.Context, repo *model.Repository) error {
	row, err := s.queries.UpdateRepository(ctx, sqlc.UpdateRepositoryParams{
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
	*repo = *toRepoModel(row)
	return nil
}

func (s *repoStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteRepository(ctx, id)
}

func (s *repoStore) DeleteByIntegration(ctx context.Context, integrationID int64) error {
	return s.queries.DeleteRepositoriesByIntegration(ctx, integrationID)
}

func (s *repoStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Repository, error) {
	rows, err := s.queries.ListRepositoriesByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return toRepoModels(rows), nil
}

func (s *repoStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.Repository, error) {
	rows, err := s.queries.ListRepositoriesByIntegration(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	return toRepoModels(rows), nil
}

func toRepoModel(row sqlc.Repository) *model.Repository {
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

func toRepoModels(rows []sqlc.Repository) []model.Repository {
	result := make([]model.Repository, len(rows))
	for i, row := range rows {
		result[i] = *toRepoModel(row)
	}
	return result
}
