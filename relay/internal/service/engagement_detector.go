package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"basegraph.app/relay/internal/store"
)

type EngagementDetector interface {
	ShouldEngage(ctx context.Context, integrationID int64, req EngagementRequest) (bool, error)
}

type EngagementRequest struct {
	IssueBody   string
	CommentBody string
}

type ServiceAccountConfig struct {
	Username string `json:"username"`
	UserID   int    `json:"user_id"`
}

type engagementDetector struct {
	configStore store.IntegrationConfigStore
}

func NewEngagementDetector(configStore store.IntegrationConfigStore) EngagementDetector {
	return &engagementDetector{
		configStore: configStore,
	}
}

func (d *engagementDetector) ShouldEngage(ctx context.Context, integrationID int64, req EngagementRequest) (bool, error) {
	config, err := d.configStore.GetByIntegrationAndKey(ctx, integrationID, "service_account")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("fetching service account config: %w", err)
	}

	var sa ServiceAccountConfig
	if err := json.Unmarshal(config.Value, &sa); err != nil {
		return false, fmt.Errorf("parsing service account config: %w", err)
	}

	mention := strings.ToLower(fmt.Sprintf("@%s", sa.Username))

	if strings.Contains(strings.ToLower(req.IssueBody), mention) {
		return true, nil
	}

	if strings.Contains(strings.ToLower(req.CommentBody), mention) {
		return true, nil
	}

	return false, nil
}
