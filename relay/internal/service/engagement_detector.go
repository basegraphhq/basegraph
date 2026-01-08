package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"basegraph.app/relay/internal/model"
	tracker "basegraph.app/relay/internal/service/issue_tracker"
	"basegraph.app/relay/internal/store"
)

// mentionPattern matches @username mentions but not email addresses.
// It requires @ to be at the start of text or preceded by a non-alphanumeric character.
// Group 1 captures the preceding character (or empty at start), Group 2 captures the username.
var mentionPattern = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])@([a-zA-Z0-9_-]+)`)

// extractMentions returns all @mentioned usernames from text (lowercase, deduplicated).
func extractMentions(text string) []string {
	matches := mentionPattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]struct{})
	var result []string
	for _, match := range matches {
		if len(match) > 1 {
			username := strings.ToLower(match[1])
			if _, ok := seen[username]; !ok {
				seen[username] = struct{}{}
				result = append(result, username)
			}
		}
	}
	return result
}

// isCommentDirectedAtOthers returns true if the comment has @mentions but none are the relay user.
// Used to avoid engaging when someone in the thread is asking a question to another human.
func isCommentDirectedAtOthers(commentBody, relayUsername string) bool {
	mentions := extractMentions(commentBody)
	if len(mentions) == 0 {
		return false // No mentions = not directed at others
	}
	relayLower := strings.ToLower(relayUsername)
	for _, mention := range mentions {
		if mention == relayLower {
			return false // Relay is mentioned
		}
	}
	return true // Has mentions but none are Relay
}

type EngagementResult struct {
	ShouldEngage bool
	Discussions  []model.Discussion // Populated when we engage (nil if not engaged)
}

type EngagementDetector interface {
	ShouldEngage(ctx context.Context, integrationID int64, req EngagementRequest) (EngagementResult, error)
	IsSelfTriggered(ctx context.Context, integrationID int64, username string) (bool, error)
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

	config, err := d.configStore.GetByIntegrationAndKey(ctx, integrationID, model.ConfigKeyServiceAccount)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return notEngaged, nil
		}
		return notEngaged, fmt.Errorf("fetching service account config: %w", err)
	}

	var sa model.ServiceAccountConfig
	if err := json.Unmarshal(config.Value, &sa); err != nil {
		return notEngaged, fmt.Errorf("parsing service account config: %w", err)
	}

	mention := strings.ToLower(fmt.Sprintf("@%s", sa.Username))
	engagedViaMention := strings.Contains(strings.ToLower(req.IssueBody), mention) ||
		strings.Contains(strings.ToLower(req.CommentBody), mention)

	// For reply detection, we need to fetch discussions
	var discussions []model.Discussion
	if req.DiscussionID != "" {
		isReply, fetchedDiscussions, err := d.checkReplyWithDiscussions(ctx, integrationID, sa.UserID, sa.Username, req)
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
	serviceAccountUsername string,
	req EngagementRequest,
) (bool, []model.Discussion, error) {
	discussions, err := d.fetchDiscussions(ctx, integrationID, req)
	if err != nil {
		return false, nil, err
	}

	// Check if relay user has commented in the target thread
	expectedAuthor := fmt.Sprintf("id:%d", serviceAccountUserID)
	relayInThread := false
	for _, disc := range discussions {
		if disc.ThreadID == nil || *disc.ThreadID != req.DiscussionID {
			continue
		}
		if disc.Author == expectedAuthor {
			relayInThread = true
			break
		}
	}

	if !relayInThread {
		return false, discussions, nil
	}

	// Check if comment is directed at someone else (has @mentions but not @relay)
	if isCommentDirectedAtOthers(req.CommentBody, serviceAccountUsername) {
		slog.DebugContext(ctx, "skipping engagement: comment directed at others",
			"discussion_id", req.DiscussionID,
		)
		return false, discussions, nil
	}

	return true, discussions, nil
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

// IsSelfTriggered checks if the given username is Relay's service account.
// Used to filter out events triggered by Relay's own actions.
func (d *engagementDetector) IsSelfTriggered(ctx context.Context, integrationID int64, username string) (bool, error) {
	if username == "" {
		return false, nil
	}

	config, err := d.configStore.GetByIntegrationAndKey(ctx, integrationID, model.ConfigKeyServiceAccount)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			slog.DebugContext(ctx, "no service account config found for self-trigger check",
				"integration_id", integrationID,
			)
			return false, nil
		}
		return false, fmt.Errorf("fetching service account config: %w", err)
	}

	var sa model.ServiceAccountConfig
	if err := json.Unmarshal(config.Value, &sa); err != nil {
		return false, fmt.Errorf("parsing service account config: %w", err)
	}

	isSelf := strings.EqualFold(username, sa.Username)
	slog.DebugContext(ctx, "self-trigger comparison",
		"triggered_by", username,
		"service_account", sa.Username,
		"is_self", isSelf,
	)

	return isSelf, nil
}
