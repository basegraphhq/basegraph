package store

import (
	"context"
	"errors"
	"time"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
		Provider:            string(integration.Provider),
		ProviderBaseUrl:     integration.ProviderBaseURL,
		ExternalOrgID:       integration.ExternalOrgID,
		ExternalWorkspaceID: integration.ExternalWorkspaceID,
		AccessToken:         integration.AccessToken,
		RefreshToken:        integration.RefreshToken,
		ExpiresAt:           timeToPgTimestamptz(integration.ExpiresAt),
	})
	if err != nil {
		return err
	}
	*integration = *toIntegrationModel(row)
	return nil
}

func (s *integrationStore) UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error {
	_, err := s.queries.UpdateIntegrationTokens(ctx, sqlc.UpdateIntegrationTokensParams{
		ID:           id,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    timeToPgTimestamptz(expiresAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
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

// toIntegrationModel converts sqlc.Integration to model.Integration
func toIntegrationModel(row sqlc.Integration) *model.Integration {
	var expiresAt *time.Time
	if row.ExpiresAt.Valid {
		expiresAt = &row.ExpiresAt.Time
	}

	return &model.Integration{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		OrganizationID:      row.OrganizationID,
		Provider:            model.Provider(row.Provider),
		ProviderBaseURL:     row.ProviderBaseUrl,
		ExternalOrgID:       row.ExternalOrgID,
		ExternalWorkspaceID: row.ExternalWorkspaceID,
		AccessToken:         row.AccessToken,
		RefreshToken:        row.RefreshToken,
		ExpiresAt:           expiresAt,
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

// timeToPgTimestamptz converts *time.Time to pgtype.Timestamptz
func timeToPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
