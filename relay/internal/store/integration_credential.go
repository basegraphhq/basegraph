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

type integrationCredentialStore struct {
	queries *sqlc.Queries
}

func newIntegrationCredentialStore(queries *sqlc.Queries) IntegrationCredentialStore {
	return &integrationCredentialStore{queries: queries}
}

func (s *integrationCredentialStore) GetByID(ctx context.Context, id int64) (*model.IntegrationCredential, error) {
	row, err := s.queries.GetIntegrationCredential(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toCredentialModel(row), nil
}

func (s *integrationCredentialStore) GetPrimaryByIntegration(ctx context.Context, integrationID int64) (*model.IntegrationCredential, error) {
	row, err := s.queries.GetPrimaryCredentialByIntegration(ctx, integrationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toCredentialModel(row), nil
}

func (s *integrationCredentialStore) GetByIntegrationAndUser(ctx context.Context, integrationID int64, userID int64) (*model.IntegrationCredential, error) {
	row, err := s.queries.GetCredentialByIntegrationAndUser(ctx, sqlc.GetCredentialByIntegrationAndUserParams{
		IntegrationID: integrationID,
		UserID:        &userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toCredentialModel(row), nil
}

func (s *integrationCredentialStore) Create(ctx context.Context, cred *model.IntegrationCredential) error {
	row, err := s.queries.CreateIntegrationCredential(ctx, sqlc.CreateIntegrationCredentialParams{
		ID:             cred.ID,
		IntegrationID:  cred.IntegrationID,
		UserID:         cred.UserID,
		CredentialType: string(cred.CredentialType),
		AccessToken:    cred.AccessToken,
		RefreshToken:   cred.RefreshToken,
		TokenExpiresAt: timeToPgTimestamptz(cred.TokenExpiresAt),
		Scopes:         cred.Scopes,
		IsPrimary:      cred.IsPrimary,
	})
	if err != nil {
		return err
	}
	*cred = *toCredentialModel(row)
	return nil
}

func (s *integrationCredentialStore) UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error {
	_, err := s.queries.UpdateCredentialTokens(ctx, sqlc.UpdateCredentialTokensParams{
		ID:             id,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		TokenExpiresAt: timeToPgTimestamptz(expiresAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *integrationCredentialStore) SetAsPrimary(ctx context.Context, integrationID int64, credentialID int64) error {
	return s.queries.SetCredentialAsPrimary(ctx, sqlc.SetCredentialAsPrimaryParams{
		IntegrationID: integrationID,
		ID:            credentialID,
	})
}

func (s *integrationCredentialStore) Revoke(ctx context.Context, id int64) error {
	_, err := s.queries.RevokeCredential(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *integrationCredentialStore) RevokeAllByIntegration(ctx context.Context, integrationID int64) error {
	return s.queries.RevokeAllCredentialsByIntegration(ctx, integrationID)
}

func (s *integrationCredentialStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteIntegrationCredential(ctx, id)
}

func (s *integrationCredentialStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error) {
	rows, err := s.queries.ListCredentialsByIntegration(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	return toCredentialModels(rows), nil
}

func (s *integrationCredentialStore) ListActiveByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error) {
	rows, err := s.queries.ListActiveCredentialsByIntegration(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	return toCredentialModels(rows), nil
}

func toCredentialModel(row sqlc.IntegrationCredential) *model.IntegrationCredential {
	var tokenExpiresAt *time.Time
	if row.TokenExpiresAt.Valid {
		tokenExpiresAt = &row.TokenExpiresAt.Time
	}

	var revokedAt *time.Time
	if row.RevokedAt.Valid {
		revokedAt = &row.RevokedAt.Time
	}

	return &model.IntegrationCredential{
		ID:             row.ID,
		IntegrationID:  row.IntegrationID,
		UserID:         row.UserID,
		CredentialType: model.CredentialType(row.CredentialType),
		AccessToken:    row.AccessToken,
		RefreshToken:   row.RefreshToken,
		TokenExpiresAt: tokenExpiresAt,
		Scopes:         row.Scopes,
		IsPrimary:      row.IsPrimary,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		RevokedAt:      revokedAt,
	}
}

func toCredentialModels(rows []sqlc.IntegrationCredential) []model.IntegrationCredential {
	result := make([]model.IntegrationCredential, len(rows))
	for i, row := range rows {
		result[i] = *toCredentialModel(row)
	}
	return result
}

func timeToPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
