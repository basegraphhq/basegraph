package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/queue"
	tracker "basegraph.app/relay/internal/service/issue_tracker"
	"basegraph.app/relay/internal/store"
)

type EventIngestParams struct {
	IntegrationID       int64           `json:"integration_id"`
	ExternalIssueID     string          `json:"external_issue_id"`
	ExternalProjectID   int64           `json:"external_project_id"`
	Provider            model.Provider  `json:"provider"`
	IssueBody           string          `json:"issue_body"`
	CommentBody         string          `json:"comment_body"`
	DiscussionID        string          `json:"discussion_id,omitempty"`
	CommentID           string          `json:"comment_id,omitempty"`
	TriggeredByUsername string          `json:"triggered_by_username"`
	Source              string          `json:"source"`
	EventType           string          `json:"event_type"`
	Payload             json.RawMessage `json:"payload"`
	TraceID             *string         `json:"trace_id,omitempty"`
}

type EventIngestResult struct {
	Engaged        bool
	EventLog       *model.EventLog
	Issue          *model.Issue
	DedupeKey      string
	EventPublished bool // true if event was sent to worker queue
	EventDuplicate bool // true if we received duplicate webhook from issue tracker
	IssuePickedUp  bool // true if Relay picked up this issue (was idle, now queued)
}

type EventIngestService interface {
	Ingest(ctx context.Context, params EventIngestParams) (*EventIngestResult, error)
}

var ErrIntegrationNotFound = errors.New("integration not found")

type eventIngestService struct {
	integrations       store.IntegrationStore
	issues             store.IssueStore
	txRunner           TxRunner
	producer           queue.Producer
	issueTrackers      map[model.Provider]tracker.IssueTrackerService
	engagementDetector EngagementDetector
}

func NewEventIngestService(
	integrations store.IntegrationStore,
	issues store.IssueStore,
	txRunner TxRunner,
	producer queue.Producer,
	issueTrackers map[model.Provider]tracker.IssueTrackerService,
	engagementDetector EngagementDetector,
) EventIngestService {
	return &eventIngestService{
		integrations:       integrations,
		issues:             issues,
		txRunner:           txRunner,
		producer:           producer,
		issueTrackers:      issueTrackers,
		engagementDetector: engagementDetector,
	}
}

func (s *eventIngestService) Ingest(ctx context.Context, params EventIngestParams) (*EventIngestResult, error) {
	if params.IntegrationID == 0 || params.ExternalIssueID == "" || params.EventType == "" {
		return nil, fmt.Errorf("integration_id, external_issue_id, and event_type are required")
	}
	if len(params.Payload) == 0 {
		return nil, fmt.Errorf("payload is required")
	}

	integration, err := s.integrations.GetByID(ctx, params.IntegrationID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("%w", ErrIntegrationNotFound)
		}
		return nil, fmt.Errorf("fetching integration: %w", err)
	}

	if !integration.IsEnabled {
		return nil, fmt.Errorf("integration is disabled")
	}

	existingIssue, err := s.issues.GetByIntegrationAndExternalID(ctx, params.IntegrationID, params.ExternalIssueID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("checking issue existence: %w", err)
	}

	isSubscribed := existingIssue != nil

	var issueToUpsert *model.Issue

	issueIID, err := strconv.ParseInt(params.ExternalIssueID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing external issue id: %w", err)
	}

	issueTracker := s.issueTrackers[params.Provider]
	if issueTracker == nil {
		return nil, fmt.Errorf("unsupported provider: %s", params.Provider)
	}

	if isSubscribed {
		slog.InfoContext(ctx, "refreshing discussions for tracked issue",
			"integration_id", params.IntegrationID,
			"external_issue_id", params.ExternalIssueID,
		)

		discussions, err := issueTracker.FetchDiscussions(ctx, tracker.FetchDiscussionsParams{
			IntegrationID: params.IntegrationID,
			ProjectID:     params.ExternalProjectID,
			IssueIID:      issueIID,
		})
		if err != nil {
			slog.WarnContext(ctx, "failed to fetch discussions for existing issue",
				"error", err,
				"integration_id", params.IntegrationID,
			)
		}

		existingIssue.Discussions = discussions
		issueToUpsert = existingIssue
	} else {
		engagement, err := s.engagementDetector.ShouldEngage(ctx, integration.ID, EngagementRequest{
			Provider:          params.Provider,
			IssueBody:         params.IssueBody,
			CommentBody:       params.CommentBody,
			DiscussionID:      params.DiscussionID,
			CommentID:         params.CommentID,
			ExternalProjectID: params.ExternalProjectID,
			ExternalIssueIID:  issueIID,
		})
		if err != nil {
			return nil, fmt.Errorf("checking engagement: %w", err)
		}

		if !engagement.ShouldEngage {
			return &EventIngestResult{
				Engaged: false,
			}, nil
		}

		issue, err := issueTracker.FetchIssue(ctx, tracker.FetchIssueParams{
			IntegrationID: params.IntegrationID,
			ProjectID:     params.ExternalProjectID,
			IssueIID:      issueIID,
		})
		if err != nil {
			return nil, fmt.Errorf("fetching issue from provider: %w", err)
		}

		slog.InfoContext(ctx, "engagement detected, fetched issue from provider",
			"integration_id", params.IntegrationID,
			"external_issue_id", params.ExternalIssueID,
			"title", issue.Title,
			"discussions_count", len(engagement.Discussions),
		)

		// Enrich issue with our data and discussions from engagement check
		issue.ID = id.New()
		issue.IntegrationID = params.IntegrationID
		issue.ExternalIssueID = params.ExternalIssueID
		issue.Provider = integration.Provider
		issue.Discussions = engagement.Discussions

		issueToUpsert = issue
	}

	source := string(integration.Provider)
	dedupeKey := computeDedupeKey(source, params.EventType, params.ExternalIssueID, params.Payload)

	var (
		resultIssue       *model.Issue
		eventLog          *model.EventLog
		createdEvent      bool
		issueMarkedQueued bool
	)

	if err := s.txRunner.WithTx(ctx, func(sp StoreProvider) error {
		var err error
		resultIssue, err = sp.Issues().Upsert(ctx, issueToUpsert)
		if err != nil {
			return fmt.Errorf("upserting issue: %w", err)
		}

		event := &model.EventLog{
			ID:                  id.New(),
			WorkspaceID:         integration.WorkspaceID,
			IssueID:             resultIssue.ID,
			TriggeredByUsername: params.TriggeredByUsername,
			Source:              source,
			EventType:           params.EventType,
			Payload:             params.Payload,
			DedupeKey:           dedupeKey,
		}

		eventLog, createdEvent, err = sp.EventLogs().CreateOrGet(ctx, event)
		if err != nil {
			return fmt.Errorf("creating event log: %w", err)
		}

		if createdEvent {
			issueMarkedQueued, err = sp.Issues().QueueIfIdle(ctx, resultIssue.ID)
			if err != nil {
				return fmt.Errorf("queueing issue: %w", err)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	enqueued := false
	if issueMarkedQueued {
		// Only send Redis message when issue transitions idle→queued.
		// If issue is already queued/processing, the active worker will pick up
		// new events before transitioning back to idle.
		if err := s.producer.Enqueue(ctx, queue.EventMessage{
			EventLogID: eventLog.ID,
			IssueID:    resultIssue.ID,
			EventType:  params.EventType,
			TraceID:    params.TraceID,
			Attempt:    1,
		}); err != nil {
			return nil, fmt.Errorf("enqueueing event: %w", err)
		}
		enqueued = true
	} else if !createdEvent {
		slog.InfoContext(ctx, "duplicate event deduped",
			"event_log_id", eventLog.ID,
			"issue_id", resultIssue.ID,
			"dedupe_key", dedupeKey,
		)
	} else {
		slog.InfoContext(ctx, "event logged, issue already being processed",
			"event_log_id", eventLog.ID,
			"issue_id", resultIssue.ID,
		)
	}

	return &EventIngestResult{
		Engaged:        true,
		EventLog:       eventLog,
		Issue:          resultIssue,
		DedupeKey:      dedupeKey,
		EventPublished: enqueued,
		EventDuplicate: !createdEvent,
		IssuePickedUp:  issueMarkedQueued,
	}, nil
}

func computeDedupeKey(source, eventType, externalIssueID string, payload json.RawMessage) string {
	body := struct {
		Source          string          `json:"source"`
		EventType       string          `json:"event_type"`
		ExternalIssueID string          `json:"external_issue_id"`
		Payload         json.RawMessage `json:"payload,omitempty"`
	}{
		Source:          source,
		EventType:       eventType,
		ExternalIssueID: externalIssueID,
		Payload:         payload,
	}

	data, _ := json.Marshal(body)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%s:%s", source, hex.EncodeToString(hash[:]))
}
