package store

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type integrationStore struct {
	queries *sqlc.Queries
}

func newIntegrationStore(queries *sqlc.Queries) IntegrationStore {
	return &integrationStore{queries: queries}
}

func (s *integrationStore) GetByID(ctx context.Context, id int64) (*model.Integration, error) {
	row, err := s.queries.GetIntegration(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toIntegrationModel(row), nil
}

func (s *integrationStore) GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error) {
	row, err := s.queries.GetIntegrationByWorkspaceAndProvider(ctx, sqlc.GetIntegrationByWorkspaceAndProviderParams{
		WorkspaceID: workspaceID,
		Provider:    string(provider),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toIntegrationModel(row), nil
}

func (s *integrationStore) Create(ctx context.Context, integration *model.Integration) error {
	row, err := s.queries.CreateIntegration(ctx, sqlc.CreateIntegrationParams{
		ID:                  integration.ID,
		WorkspaceID:         integration.WorkspaceID,
		OrganizationID:      integration.OrganizationID,
		ConnectedByUserID:   integration.ConnectedByUserID,
		Provider:            string(integration.Provider),
		ProviderBaseUrl:     integration.ProviderBaseURL,
		ExternalOrgID:       integration.ExternalOrgID,
		ExternalWorkspaceID: integration.ExternalWorkspaceID,
	})
	if err != nil {
		return err
	}
	*integration = *toIntegrationModel(row)
	return nil
}

func (s *integrationStore) Update(ctx context.Context, integration *model.Integration) error {
	row, err := s.queries.UpdateIntegration(ctx, sqlc.UpdateIntegrationParams{
		ID:                  integration.ID,
		ProviderBaseUrl:     integration.ProviderBaseURL,
		ExternalOrgID:       integration.ExternalOrgID,
		ExternalWorkspaceID: integration.ExternalWorkspaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	*integration = *toIntegrationModel(row)
	return nil
}

func (s *integrationStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteIntegration(ctx, id)
}

func (s *integrationStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error) {
	rows, err := s.queries.ListIntegrationsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return toIntegrationModels(rows), nil
}

func (s *integrationStore) ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error) {
	rows, err := s.queries.ListIntegrationsByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return toIntegrationModels(rows), nil
}

func toIntegrationModel(row sqlc.Integration) *model.Integration {
	return &model.Integration{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		OrganizationID:      row.OrganizationID,
		ConnectedByUserID:   row.ConnectedByUserID,
		Provider:            model.Provider(row.Provider),
		ProviderBaseURL:     row.ProviderBaseUrl,
		ExternalOrgID:       row.ExternalOrgID,
		ExternalWorkspaceID: row.ExternalWorkspaceID,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}

func toIntegrationModels(rows []sqlc.Integration) []model.Integration {
	result := make([]model.Integration, len(rows))
	for i, row := range rows {
		result[i] = *toIntegrationModel(row)
	}
	return result
}
