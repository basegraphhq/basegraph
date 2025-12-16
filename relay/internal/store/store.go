package store

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"github.com/jackc/pgx/v5"
)

// Store provides typed accessors over the underlying sqlc queries.
type Store struct {
	queries *sqlc.Queries
}

func New(queries *sqlc.Queries) *Store {
	return &Store{queries: queries}
}

// --- Event logs -------------------------------------------------------------

func (s *Store) GetEventLog(ctx context.Context, id int64) (sqlc.EventLog, error) {
	row, err := s.queries.GetEventLog(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.EventLog{}, ErrNotFound
		}
		return sqlc.EventLog{}, err
	}
	return row, nil
}

func (s *Store) MarkEventLogProcessed(ctx context.Context, id int64) error {
	return s.queries.MarkEventLogProcessed(ctx, id)
}

func (s *Store) MarkEventLogFailed(ctx context.Context, id int64, errMsg *string) error {
	return s.queries.MarkEventLogFailed(ctx, sqlc.MarkEventLogFailedParams{
		ID:              id,
		ProcessingError: errMsg,
	})
}

// --- Issues -----------------------------------------------------------------

func (s *Store) GetIssue(ctx context.Context, id int64) (*sqlc.Issue, error) {
	row, err := s.queries.GetIssue(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

func (s *Store) GetIssueByIntegrationAndExternal(ctx context.Context, integrationID int64, externalID string) (*sqlc.Issue, error) {
	row, err := s.queries.GetIssueByIntegrationAndExternalID(ctx, sqlc.GetIssueByIntegrationAndExternalIDParams{
		IntegrationID:   integrationID,
		ExternalIssueID: externalID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// --- Pipeline runs ----------------------------------------------------------

func (s *Store) CreatePipelineRun(ctx context.Context, params sqlc.CreatePipelineRunParams) (sqlc.PipelineRun, error) {
	return s.queries.CreatePipelineRun(ctx, params)
}

func (s *Store) FinishPipelineRun(ctx context.Context, params sqlc.FinishPipelineRunParams) error {
	return s.queries.FinishPipelineRun(ctx, params)
}

func (s *Store) ListPipelineRuns(ctx context.Context, eventLogID int64, limit int32) ([]sqlc.PipelineRun, error) {
	return s.queries.ListPipelineRunsForEvent(ctx, sqlc.ListPipelineRunsForEventParams{
		EventLogID: eventLogID,
		Limit:      limit,
	})
}
