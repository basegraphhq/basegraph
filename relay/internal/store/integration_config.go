package store

import (
	"context"
	"errors"

	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type integrationConfigStore struct {
	queries *sqlc.Queries
}

func newIntegrationConfigStore(queries *sqlc.Queries) IntegrationConfigStore {
	return &integrationConfigStore{queries: queries}
}

func (s *integrationConfigStore) GetByID(ctx context.Context, id int64) (*model.IntegrationConfig, error) {
	row, err := s.queries.GetIntegrationConfig(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toIntegrationConfigModel(row), nil
}

func (s *integrationConfigStore) GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error) {
	row, err := s.queries.GetIntegrationConfigByKey(ctx, sqlc.GetIntegrationConfigByKeyParams{
		IntegrationID: integrationID,
		Key:           key,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toIntegrationConfigModel(row), nil
}

func (s *integrationConfigStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationConfig, error) {
	rows, err := s.queries.ListIntegrationConfigs(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	return toIntegrationConfigModels(rows), nil
}

func (s *integrationConfigStore) ListByIntegrationAndType(ctx context.Context, integrationID int64, configType string) ([]model.IntegrationConfig, error) {
	rows, err := s.queries.ListIntegrationConfigsByType(ctx, sqlc.ListIntegrationConfigsByTypeParams{
		IntegrationID: integrationID,
		ConfigType:    configType,
	})
	if err != nil {
		return nil, err
	}
	return toIntegrationConfigModels(rows), nil
}

func (s *integrationConfigStore) Create(ctx context.Context, config *model.IntegrationConfig) error {
	row, err := s.queries.CreateIntegrationConfig(ctx, sqlc.CreateIntegrationConfigParams{
		ID:            config.ID,
		IntegrationID: config.IntegrationID,
		Key:           config.Key,
		Value:         config.Value,
		ConfigType:    config.ConfigType,
	})
	if err != nil {
		return err
	}
	*config = *toIntegrationConfigModel(row)
	return nil
}

func (s *integrationConfigStore) Update(ctx context.Context, config *model.IntegrationConfig) error {
	row, err := s.queries.UpdateIntegrationConfig(ctx, sqlc.UpdateIntegrationConfigParams{
		ID:    config.ID,
		Value: config.Value,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	*config = *toIntegrationConfigModel(row)
	return nil
}

func (s *integrationConfigStore) Upsert(ctx context.Context, config *model.IntegrationConfig) error {
	row, err := s.queries.UpsertIntegrationConfig(ctx, sqlc.UpsertIntegrationConfigParams{
		ID:            config.ID,
		IntegrationID: config.IntegrationID,
		Key:           config.Key,
		Value:         config.Value,
		ConfigType:    config.ConfigType,
	})
	if err != nil {
		return err
	}
	*config = *toIntegrationConfigModel(row)
	return nil
}

func (s *integrationConfigStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteIntegrationConfig(ctx, id)
}

func (s *integrationConfigStore) DeleteByIntegration(ctx context.Context, integrationID int64) error {
	return s.queries.DeleteIntegrationConfigsByIntegration(ctx, integrationID)
}

func toIntegrationConfigModel(row sqlc.IntegrationConfig) *model.IntegrationConfig {
	return &model.IntegrationConfig{
		ID:            row.ID,
		IntegrationID: row.IntegrationID,
		Key:           row.Key,
		Value:         row.Value,
		ConfigType:    row.ConfigType,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func toIntegrationConfigModels(rows []sqlc.IntegrationConfig) []model.IntegrationConfig {
	result := make([]model.IntegrationConfig, len(rows))
	for i, row := range rows {
		result[i] = *toIntegrationConfigModel(row)
	}
	return result
}
