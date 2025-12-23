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

type EngagementResult struct {
	ShouldEngage bool
	Discussions  []model.Discussion // Populated when we engage (nil if not engaged)
}

type EngagementDetector interface {
	ShouldEngage(ctx context.Context, integrationID int64, req EngagementRequest) (EngagementResult, error)
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

func (d *engagementDetector) ShouldEngage(ctx context.Context, integrationID int64, req EngagementRequest) (EngagementResult, error) {
	notEngaged := EngagementResult{ShouldEngage: false}

	config, err := d.configStore.GetByIntegrationAndKey(ctx, integrationID, "service_account")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return notEngaged, nil
		}
		return notEngaged, fmt.Errorf("fetching service account config: %w", err)
	}

	var sa ServiceAccountConfig
	if err := json.Unmarshal(config.Value, &sa); err != nil {
		return notEngaged, fmt.Errorf("parsing service account config: %w", err)
	}

	mention := strings.ToLower(fmt.Sprintf("@%s", sa.Username))
	engagedViaMention := strings.Contains(strings.ToLower(req.IssueBody), mention) ||
		strings.Contains(strings.ToLower(req.CommentBody), mention)

	// For reply detection, we need to fetch discussions
	var discussions []model.Discussion
	if req.DiscussionID != "" {
		isReply, fetchedDiscussions, err := d.checkReplyWithDiscussions(ctx, integrationID, sa.UserID, req)
		if err != nil {
			slog.WarnContext(ctx, "failed to check reply detection, falling back to @mention only",
				"error", err,
				"integration_id", integrationID,
				"discussion_id", req.DiscussionID,
			)
		} else {
			discussions = fetchedDiscussions
			if isReply {
				return EngagementResult{ShouldEngage: true, Discussions: discussions}, nil
			}
		}
	}

	if engagedViaMention {
		// Engaged via @mention - fetch discussions if not already fetched
		if discussions == nil {
			discussions, err = d.fetchDiscussions(ctx, integrationID, req)
			if err != nil {
				slog.WarnContext(ctx, "failed to fetch discussions for engaged issue",
					"error", err,
					"integration_id", integrationID,
				)
				// Continue without discussions - we're still engaged
			}
		}
		return EngagementResult{ShouldEngage: true, Discussions: discussions}, nil
	}

	return notEngaged, nil
}

// checkReplyWithDiscussions fetches discussions and checks if the incoming comment
// is part of a thread where relay has previously participated.
func (d *engagementDetector) checkReplyWithDiscussions(
	ctx context.Context,
	integrationID int64,
	serviceAccountUserID int64,
	req EngagementRequest,
) (bool, []model.Discussion, error) {
	discussions, err := d.fetchDiscussions(ctx, integrationID, req)
	if err != nil {
		return false, nil, err
	}

	// Check if relay user has commented in the target thread
	expectedAuthor := fmt.Sprintf("id:%d", serviceAccountUserID)
	for _, disc := range discussions {
		if disc.ThreadID == nil || *disc.ThreadID != req.DiscussionID {
			continue
		}
		if disc.Author == expectedAuthor {
			return true, discussions, nil
		}
	}

	return false, discussions, nil
}

func (d *engagementDetector) fetchDiscussions(
	ctx context.Context,
	integrationID int64,
	req EngagementRequest,
) ([]model.Discussion, error) {
	issueTracker := d.issueTrackers[req.Provider]
	if issueTracker == nil {
		return nil, fmt.Errorf("unsupported provider: %s", req.Provider)
	}

	return issueTracker.FetchDiscussions(ctx, tracker.FetchDiscussionsParams{
		IntegrationID: integrationID,
		ProjectID:     req.ExternalProjectID,
		IssueIID:      req.ExternalIssueIID,
	})
}
