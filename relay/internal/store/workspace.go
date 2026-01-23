package store

import (
	"context"
	"errors"
	"time"

	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type workspaceStore struct {
	queries *sqlc.Queries
}

func newWorkspaceStore(queries *sqlc.Queries) WorkspaceStore {
	return &workspaceStore{queries: queries}
}

func (s *workspaceStore) GetByID(ctx context.Context, id int64) (*model.Workspace, error) {
	row, err := s.queries.GetWorkspace(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toWorkspaceModel(row), nil
}

func (s *workspaceStore) GetByOrgAndSlug(ctx context.Context, orgID int64, slug string) (*model.Workspace, error) {
	row, err := s.queries.GetWorkspaceByOrgAndSlug(ctx, sqlc.GetWorkspaceByOrgAndSlugParams{
		OrganizationID: orgID,
		Slug:           slug,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toWorkspaceModel(row), nil
}

func (s *workspaceStore) Create(ctx context.Context, ws *model.Workspace) error {
	row, err := s.queries.CreateWorkspace(ctx, sqlc.CreateWorkspaceParams{
		ID:             ws.ID,
		AdminUserID:    ws.AdminUserID,
		OrganizationID: ws.OrganizationID,
		UserID:         ws.UserID,
		Name:           ws.Name,
		Slug:           ws.Slug,
		Description:    ws.Description,
	})
	if err != nil {
		return err
	}
	*ws = *toWorkspaceModel(row)
	return nil
}

func (s *workspaceStore) Update(ctx context.Context, ws *model.Workspace) error {
	row, err := s.queries.UpdateWorkspace(ctx, sqlc.UpdateWorkspaceParams{
		ID:          ws.ID,
		Name:        ws.Name,
		Slug:        ws.Slug,
		Description: ws.Description,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	*ws = *toWorkspaceModel(row)
	return nil
}

func (s *workspaceStore) SetRepoReadyAt(ctx context.Context, id int64, readyAt time.Time) (*model.Workspace, error) {
	row, err := s.queries.SetWorkspaceRepoReadyAt(ctx, sqlc.SetWorkspaceRepoReadyAtParams{
		ID: id,
		RepoReadyAt: pgtype.Timestamptz{
			Time:  readyAt,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toWorkspaceModel(row), nil
}

func (s *workspaceStore) Delete(ctx context.Context, id int64) error {
	return s.queries.SoftDeleteWorkspace(ctx, id)
}

func (s *workspaceStore) ListByOrganization(ctx context.Context, orgID int64) ([]model.Workspace, error) {
	rows, err := s.queries.ListWorkspacesByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return toWorkspaceModels(rows), nil
}

func (s *workspaceStore) ListByUser(ctx context.Context, userID int64) ([]model.Workspace, error) {
	rows, err := s.queries.ListWorkspacesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toWorkspaceModels(rows), nil
}

func toWorkspaceModel(row sqlc.Workspace) *model.Workspace {
	var repoReadyAt *time.Time
	if row.RepoReadyAt.Valid {
		t := row.RepoReadyAt.Time
		repoReadyAt = &t
	}

	return &model.Workspace{
		ID:             row.ID,
		AdminUserID:    row.AdminUserID,
		OrganizationID: row.OrganizationID,
		UserID:         row.UserID,
		Name:           row.Name,
		Slug:           row.Slug,
		Description:    row.Description,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		IsDeleted:      row.IsDeleted,
		RepoReadyAt:    repoReadyAt,
	}
}

func toWorkspaceModels(rows []sqlc.Workspace) []model.Workspace {
	result := make([]model.Workspace, len(rows))
	for i, row := range rows {
		result[i] = *toWorkspaceModel(row)
	}
	return result
}
