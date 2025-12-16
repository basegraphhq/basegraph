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
	IntegrationID   int64           `json:"integration_id"`
	ExternalIssueID string          `json:"external_issue_id"`
	EventType       string          `json:"event_type"`
	Source          *string         `json:"source,omitempty"`
	ExternalEventID *string         `json:"external_event_id,omitempty"`
	DedupeKey       *string         `json:"dedupe_key,omitempty"`
	Payload         json.RawMessage `json:"payload"`

	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Members     []string `json:"members,omitempty"`
	Assignees   []string `json:"assignees,omitempty"`
	Reporter    *string  `json:"reporter,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`

	TraceID *string `json:"trace_id,omitempty"`
}

type EventIngestResult struct {
	EventLog   *model.EventLog
	Issue      *model.Issue
	DedupeKey  string
	Enqueued   bool
	Duplicated bool
}

type EventIngestService interface {
	Ingest(ctx context.Context, params EventIngestParams) (*EventIngestResult, error)
}

var ErrIntegrationNotFound = errors.New("integration not found")

type eventIngestService struct {
	stores   *store.Stores
	txRunner TxRunner
	queue    queue.Producer
	logger   *slog.Logger
}

func NewEventIngestService(stores *store.Stores, txRunner TxRunner, queue queue.Producer, logger *slog.Logger) EventIngestService {
	if logger == nil {
		logger = slog.Default()
	}
	return &eventIngestService{
		stores:   stores,
		txRunner: txRunner,
		queue:    queue,
		logger:   logger,
	}
}

func (s *eventIngestService) Ingest(ctx context.Context, params EventIngestParams) (*EventIngestResult, error) {
	if params.IntegrationID == 0 || params.ExternalIssueID == "" || params.EventType == "" {
		return nil, fmt.Errorf("integration_id, external_issue_id, and event_type are required")
	}
	if len(params.Payload) == 0 {
		return nil, fmt.Errorf("payload is required")
	}

	integration, err := s.stores.Integrations().GetByID(ctx, params.IntegrationID)
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
	if params.Source != nil && *params.Source != "" {
		source = *params.Source
	}

	dedupeKey, err := computeDedupeKey(source, params.EventType, params.ExternalIssueID, params.ExternalEventID, params.Payload, params.DedupeKey)
	if err != nil {
		return nil, err
	}

	var (
		issue        *model.Issue
		eventLog     *model.EventLog
		createdEvent bool
	)

	if err := s.txRunner.WithTx(ctx, func(sp StoreProvider) error {
		issueModel := &model.Issue{
			ID:              id.New(),
			IntegrationID:   params.IntegrationID,
			ExternalIssueID: params.ExternalIssueID,
			Title:           params.Title,
			Description:     params.Description,
			Labels:          params.Labels,
			Members:         params.Members,
			Assignees:       params.Assignees,
			Reporter:        params.Reporter,
			Keywords:        params.Keywords,
		}

		var err error
		issue, err = sp.Issues().Upsert(ctx, issueModel)
		if err != nil {
			return fmt.Errorf("upserting issue: %w", err)
		}

		event := &model.EventLog{
			ID:          id.New(),
			WorkspaceID: integration.WorkspaceID,
			IssueID:     issue.ID,
			Source:      source,
			EventType:   params.EventType,
			Payload:     params.Payload,
			ExternalID:  params.ExternalEventID,
			DedupeKey:   dedupeKey,
		}

		eventLog, createdEvent, err = sp.EventLogs().CreateOrGet(ctx, event)
		if err != nil {
			return fmt.Errorf("creating event log: %w", err)
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
		s.logger.InfoContext(ctx, "duplicate event deduped", "event_log_id", eventLog.ID, "issue_id", issue.ID, "dedupe_key", dedupeKey)
	}

	return &EventIngestResult{
		EventLog:   eventLog,
		Issue:      issue,
		DedupeKey:  dedupeKey,
		Enqueued:   enqueued,
		Duplicated: !createdEvent,
	}, nil
}

func computeDedupeKey(source, eventType, externalIssueID string, externalEventID *string, payload json.RawMessage, override *string) (string, error) {
	if override != nil && *override != "" {
		return *override, nil
	}

	if externalEventID != nil && *externalEventID != "" {
		return fmt.Sprintf("%s:%s:%s", source, eventType, *externalEventID), nil
	}

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

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal dedupe payload: %w", err)
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%s:%s", source, hex.EncodeToString(hash[:])), nil
}
