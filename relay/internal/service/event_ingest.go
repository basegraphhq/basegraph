package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/store"
)

type EventIngestParams struct {
	IntegrationID       int64           `json:"integration_id"`
	ExternalIssueID     string          `json:"external_issue_id"`
	TriggeredByUsername string          `json:"triggered_by_username"`
	Source              string          `json:"source"`
	EventType           string          `json:"event_type"`
	Payload             json.RawMessage `json:"payload"`
	TraceID             *string         `json:"trace_id,omitempty"`
}

type EventIngestResult struct {
	EventLog   *model.EventLog
	Issue      *model.Issue
	DedupeKey  string
	Enqueued   bool
	Duplicated bool
	Queued     bool // true if issue was transitioned from 'idle' to 'queued'
}

type EventIngestService interface {
	Ingest(ctx context.Context, params EventIngestParams) (*EventIngestResult, error)
}

var ErrIntegrationNotFound = errors.New("integration not found")

type eventIngestService struct {
	integrations store.IntegrationStore
	txRunner     TxRunner
	queue        queue.Producer
}

func NewEventIngestService(integrations store.IntegrationStore, txRunner TxRunner, queue queue.Producer) EventIngestService {
	return &eventIngestService{
		integrations: integrations,
		txRunner:     txRunner,
		queue:        queue,
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
		if err == store.ErrNotFound {
			return nil, fmt.Errorf("%w", ErrIntegrationNotFound)
		}
		return nil, fmt.Errorf("fetching integration: %w", err)
	}
	if !integration.IsEnabled {
		return nil, fmt.Errorf("integration is disabled")
	}

	source := string(integration.Provider)
	dedupeKey := computeDedupeKey(source, params.EventType, params.ExternalIssueID, params.Payload)

	var (
		issue        *model.Issue
		eventLog     *model.EventLog
		createdEvent bool
		queued       bool
	)

	if err := s.txRunner.WithTx(ctx, func(sp StoreProvider) error {
		issueModel := &model.Issue{
			ID:              id.New(),
			IntegrationID:   params.IntegrationID,
			ExternalIssueID: params.ExternalIssueID,
			Provider:        integration.Provider,
		}

		var err error
		issue, err = sp.Issues().Upsert(ctx, issueModel)
		if err != nil {
			return fmt.Errorf("upserting issue: %w", err)
		}

		event := &model.EventLog{
			ID:                  id.New(),
			WorkspaceID:         integration.WorkspaceID,
			IssueID:             issue.ID,
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

		// Queue issue if idle (atomic transition inside transaction)
		if createdEvent {
			queued, err = sp.Issues().QueueIfIdle(ctx, issue.ID)
			if err != nil {
				return fmt.Errorf("queueing issue: %w", err)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	enqueued := false
	if createdEvent {
		if err := s.queue.Enqueue(ctx, queue.EventMessage{
			EventLogID: eventLog.ID,
			IssueID:    issue.ID,
			EventType:  params.EventType,
			TraceID:    params.TraceID,
			Attempt:    1,
		}); err != nil {
			return nil, fmt.Errorf("enqueueing event: %w", err)
		}
		enqueued = true
	} else {
		slog.InfoContext(ctx, "duplicate event deduped", "event_log_id", eventLog.ID, "issue_id", issue.ID, "dedupe_key", dedupeKey)
	}

	return &EventIngestResult{
		EventLog:   eventLog,
		Issue:      issue,
		DedupeKey:  dedupeKey,
		Enqueued:   enqueued,
		Duplicated: !createdEvent,
		Queued:     queued,
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
