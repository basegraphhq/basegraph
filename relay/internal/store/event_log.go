package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type eventLogStore struct {
	queries *sqlc.Queries
}

func newEventLogStore(queries *sqlc.Queries) EventLogStore {
	return &eventLogStore{queries: queries}
}

func (s *eventLogStore) Create(ctx context.Context, log *model.EventLog) (*model.EventLog, error) {
	row, err := s.queries.CreateEventLog(ctx, sqlc.CreateEventLogParams{
		ID:          log.ID,
		WorkspaceID: log.WorkspaceID,
		IssueID:     log.IssueID,
		Source:      log.Source,
		EventType:   log.EventType,
		Payload:     []byte(log.Payload),
		ExternalID:  log.ExternalID,
		DedupeKey:   log.DedupeKey,
	})
	if err != nil {
		return nil, err
	}
	return toEventLogModel(row), nil
}

func (s *eventLogStore) CreateOrGet(ctx context.Context, log *model.EventLog) (*model.EventLog, bool, error) {
	row, err := s.queries.UpsertEventLog(ctx, sqlc.UpsertEventLogParams{
		ID:          log.ID,
		WorkspaceID: log.WorkspaceID,
		IssueID:     log.IssueID,
		Source:      log.Source,
		EventType:   log.EventType,
		Payload:     []byte(log.Payload),
		ExternalID:  log.ExternalID,
		DedupeKey:   log.DedupeKey,
	})
	if err != nil {
		return nil, false, err
	}
	created := row.ID == log.ID
	return toEventLogModel(row), created, nil
}

func (s *eventLogStore) GetByID(ctx context.Context, id int64) (*model.EventLog, error) {
	row, err := s.queries.GetEventLog(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toEventLogModel(row), nil
}

func (s *eventLogStore) ListUnprocessed(ctx context.Context, limit int32) ([]model.EventLog, error) {
	rows, err := s.queries.ListUnprocessedEventLogs(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]model.EventLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toEventLogModel(row))
	}
	return result, nil
}

func (s *eventLogStore) MarkProcessed(ctx context.Context, id int64) error {
	return s.queries.MarkEventLogProcessed(ctx, id)
}

func (s *eventLogStore) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	return s.queries.MarkEventLogFailed(ctx, sqlc.MarkEventLogFailedParams{
		ID:              id,
		ProcessingError: &errMsg,
	})
}

func toEventLogModel(row sqlc.EventLog) *model.EventLog {
	var processedAt *time.Time
	if row.ProcessedAt.Valid {
		t := row.ProcessedAt.Time
		processedAt = &t
	}

	return &model.EventLog{
		ID:              row.ID,
		WorkspaceID:     row.WorkspaceID,
		IssueID:         row.IssueID,
		Source:          row.Source,
		EventType:       row.EventType,
		Payload:         json.RawMessage(row.Payload),
		ExternalID:      row.ExternalID,
		DedupeKey:       row.DedupeKey,
		ProcessedAt:     processedAt,
		ProcessingError: row.ProcessingError,
		CreatedAt:       row.CreatedAt.Time,
	}
}
