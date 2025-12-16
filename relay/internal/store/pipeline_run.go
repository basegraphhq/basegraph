package store

import (
	"context"
	"errors"
	"time"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type pipelineRunStore struct {
	queries *sqlc.Queries
}

func newPipelineRunStore(queries *sqlc.Queries) PipelineRunStore {
	return &pipelineRunStore{queries: queries}
}

func (s *pipelineRunStore) Create(ctx context.Context, run *model.PipelineRun) (*model.PipelineRun, error) {
	row, err := s.queries.CreatePipelineRun(ctx, sqlc.CreatePipelineRunParams{
		ID:         run.ID,
		EventLogID: run.EventLogID,
		Attempt:    run.Attempt,
		Status:     run.Status,
		Error:      run.Error,
	})
	if err != nil {
		return nil, err
	}
	return toPipelineRunModel(row), nil
}

func (s *pipelineRunStore) Finish(ctx context.Context, id int64, status string, errMsg *string) error {
	return s.queries.FinishPipelineRun(ctx, sqlc.FinishPipelineRunParams{
		ID:     id,
		Status: status,
		Error:  errMsg,
	})
}

func (s *pipelineRunStore) ListByEvent(ctx context.Context, eventLogID int64, limit int32) ([]model.PipelineRun, error) {
	rows, err := s.queries.ListPipelineRunsForEvent(ctx, sqlc.ListPipelineRunsForEventParams{
		EventLogID: eventLogID,
		Limit:      limit,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.PipelineRun{}, nil
		}
		return nil, err
	}
	runs := make([]model.PipelineRun, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, *toPipelineRunModel(row))
	}
	return runs, nil
}

func toPipelineRunModel(row sqlc.PipelineRun) *model.PipelineRun {
	var finishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		finishedAt = &t
	}
	return &model.PipelineRun{
		ID:         row.ID,
		EventLogID: row.EventLogID,
		Attempt:    row.Attempt,
		Status:     row.Status,
		Error:      row.Error,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: finishedAt,
	}
}
