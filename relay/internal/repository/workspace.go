package repository

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type workspaceRepository struct {
	queries *sqlc.Queries
}

// NewWorkspaceRepository creates a new WorkspaceRepository
func NewWorkspaceRepository(queries *sqlc.Queries) WorkspaceRepository {
	return &workspaceRepository{queries: queries}
}

func (r *workspaceRepository) GetByID(ctx context.Context, id int64) (*model.Workspace, error) {
	row, err := r.queries.GetWorkspace(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toWorkspaceModel(row), nil
}

func (r *workspaceRepository) GetByOrgAndSlug(ctx context.Context, orgID int64, slug string) (*model.Workspace, error) {
	row, err := r.queries.GetWorkspaceByOrgAndSlug(ctx, sqlc.GetWorkspaceByOrgAndSlugParams{
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

func (r *workspaceRepository) Create(ctx context.Context, ws *model.Workspace) error {
	row, err := r.queries.CreateWorkspace(ctx, sqlc.CreateWorkspaceParams{
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

func (r *workspaceRepository) Update(ctx context.Context, ws *model.Workspace) error {
	row, err := r.queries.UpdateWorkspace(ctx, sqlc.UpdateWorkspaceParams{
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

func (r *workspaceRepository) Delete(ctx context.Context, id int64) error {
	return r.queries.SoftDeleteWorkspace(ctx, id)
}

func (r *workspaceRepository) ListByOrganization(ctx context.Context, orgID int64) ([]model.Workspace, error) {
	rows, err := r.queries.ListWorkspacesByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return toWorkspaceModels(rows), nil
}

func (r *workspaceRepository) ListByUser(ctx context.Context, userID int64) ([]model.Workspace, error) {
	rows, err := r.queries.ListWorkspacesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toWorkspaceModels(rows), nil
}

// toWorkspaceModel converts sqlc.Workspace to model.Workspace
func toWorkspaceModel(row sqlc.Workspace) *model.Workspace {
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
	}
}

func toWorkspaceModels(rows []sqlc.Workspace) []model.Workspace {
	result := make([]model.Workspace, len(rows))
	for i, row := range rows {
		result[i] = *toWorkspaceModel(row)
	}
	return result
}

