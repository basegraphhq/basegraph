package repository

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type organizationRepository struct {
	queries *sqlc.Queries
}

// NewOrganizationRepository creates a new OrganizationRepository
func NewOrganizationRepository(queries *sqlc.Queries) OrganizationRepository {
	return &organizationRepository{queries: queries}
}

func (r *organizationRepository) GetByID(ctx context.Context, id int64) (*model.Organization, error) {
	row, err := r.queries.GetOrganization(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toOrganizationModel(row), nil
}

func (r *organizationRepository) GetBySlug(ctx context.Context, slug string) (*model.Organization, error) {
	row, err := r.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toOrganizationModel(row), nil
}

func (r *organizationRepository) Create(ctx context.Context, org *model.Organization) error {
	row, err := r.queries.CreateOrganization(ctx, sqlc.CreateOrganizationParams{
		ID:          org.ID,
		AdminUserID: org.AdminUserID,
		Name:        org.Name,
		Slug:        org.Slug,
	})
	if err != nil {
		return err
	}
	*org = *toOrganizationModel(row)
	return nil
}

func (r *organizationRepository) Update(ctx context.Context, org *model.Organization) error {
	row, err := r.queries.UpdateOrganization(ctx, sqlc.UpdateOrganizationParams{
		ID:   org.ID,
		Name: org.Name,
		Slug: org.Slug,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	*org = *toOrganizationModel(row)
	return nil
}

func (r *organizationRepository) Delete(ctx context.Context, id int64) error {
	return r.queries.SoftDeleteOrganization(ctx, id)
}

func (r *organizationRepository) ListByAdminUser(ctx context.Context, userID int64) ([]model.Organization, error) {
	rows, err := r.queries.ListOrganizationsByAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toOrganizationModels(rows), nil
}

// toOrganizationModel converts sqlc.Organization to model.Organization
func toOrganizationModel(row sqlc.Organization) *model.Organization {
	return &model.Organization{
		ID:          row.ID,
		AdminUserID: row.AdminUserID,
		Name:        row.Name,
		Slug:        row.Slug,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
		IsDeleted:   row.IsDeleted,
	}
}

func toOrganizationModels(rows []sqlc.Organization) []model.Organization {
	result := make([]model.Organization, len(rows))
	for i, row := range rows {
		result[i] = *toOrganizationModel(row)
	}
	return result
}

