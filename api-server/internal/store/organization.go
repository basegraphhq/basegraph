package store

import (
	"context"
	"errors"

	"basegraph.app/api-server/core/db/sqlc"
	"basegraph.app/api-server/internal/model"
	"github.com/jackc/pgx/v5"
)

type organizationStore struct {
	queries *sqlc.Queries
}

func newOrganizationStore(queries *sqlc.Queries) OrganizationStore {
	return &organizationStore{queries: queries}
}

func (s *organizationStore) GetByID(ctx context.Context, id int64) (*model.Organization, error) {
	row, err := s.queries.GetOrganization(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toOrganizationModel(row), nil
}

func (s *organizationStore) GetBySlug(ctx context.Context, slug string) (*model.Organization, error) {
	row, err := s.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toOrganizationModel(row), nil
}

func (s *organizationStore) Create(ctx context.Context, org *model.Organization) error {
	row, err := s.queries.CreateOrganization(ctx, sqlc.CreateOrganizationParams{
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

func (s *organizationStore) Update(ctx context.Context, org *model.Organization) error {
	row, err := s.queries.UpdateOrganization(ctx, sqlc.UpdateOrganizationParams{
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

func (s *organizationStore) Delete(ctx context.Context, id int64) error {
	return s.queries.SoftDeleteOrganization(ctx, id)
}

func (s *organizationStore) ListByAdminUser(ctx context.Context, userID int64) ([]model.Organization, error) {
	rows, err := s.queries.ListOrganizationsByAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toOrganizationModels(rows), nil
}

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
