package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"basegraph.app/relay/internal/model"
	tracker "basegraph.app/relay/internal/service/issue_tracker"
	"basegraph.app/relay/internal/store"
)

type EngagementDetector interface {
	ShouldEngage(ctx context.Context, integrationID int64, req EngagementRequest) (bool, error)
}

type EngagementRequest struct {
	Provider          model.Provider
	IssueBody         string
	CommentBody       string
	DiscussionID      string
	CommentID         string
	ExternalProjectID int64
	ExternalIssueIID  int64
}

type ServiceAccountConfig struct {
	Username string `json:"username"`
	UserID   int64  `json:"user_id"`
}

type engagementDetector struct {
	configStore   store.IntegrationConfigStore
	issueTrackers map[model.Provider]tracker.IssueTrackerService
}

func NewEngagementDetector(
	configStore store.IntegrationConfigStore,
	issueTrackers map[model.Provider]tracker.IssueTrackerService,
) EngagementDetector {
	return &engagementDetector{
		configStore:   configStore,
		issueTrackers: issueTrackers,
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

	// Check 3: Reply to relay's comment in a threaded discussion
	if req.DiscussionID != "" {
		isReply, err := d.isReplyToRelayComment(ctx, integrationID, sa.UserID, req)
		if err != nil {
			slog.WarnContext(ctx, "failed to check reply detection, falling back to @mention only",
				"error", err,
				"integration_id", integrationID,
				"discussion_id", req.DiscussionID,
			)
			return false, nil
		}
		if isReply {
			return true, nil
		}
	}

	return false, nil
}

// isReplyToRelayComment checks if the incoming comment is part of a discussion
// where relay has previously participated. Delegates to provider-specific implementation.
func (d *engagementDetector) isReplyToRelayComment(
	ctx context.Context,
	integrationID int64,
	serviceAccountUserID int64,
	req EngagementRequest,
) (bool, error) {
	issueTracker := d.issueTrackers[req.Provider]
	if issueTracker == nil {
		return false, fmt.Errorf("unsupported provider: %s", req.Provider)
	}

	return issueTracker.IsReplyToUser(ctx, tracker.IsReplyParams{
		IntegrationID: integrationID,
		ProjectID:     req.ExternalProjectID,
		IssueIID:      req.ExternalIssueIID,
		DiscussionID:  req.DiscussionID,
		CommentID:     req.CommentID,
		UserID:        serviceAccountUserID,
	})
}
