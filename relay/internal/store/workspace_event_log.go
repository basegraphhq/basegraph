package store

import (
	"context"
	"errors"
	"time"

	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type workspaceEventLogStore struct {
	queries *sqlc.Queries
}

func newWorkspaceEventLogStore(queries *sqlc.Queries) WorkspaceEventLogStore {
	return &workspaceEventLogStore{queries: queries}
}

func (s *workspaceEventLogStore) GetByID(ctx context.Context, id int64) (*model.WorkspaceEventLog, error) {
	row, err := s.queries.GetWorkspaceEventLog(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toWorkspaceEventLogModel(row), nil
}

func (s *workspaceEventLogStore) ListByWorkspace(ctx context.Context, workspaceID int64, limit int32) ([]model.WorkspaceEventLog, error) {
	rows, err := s.queries.ListWorkspaceEventLogsByWorkspace(ctx, sqlc.ListWorkspaceEventLogsByWorkspaceParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]model.WorkspaceEventLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toWorkspaceEventLogModel(row))
	}
	return result, nil
}

func (s *workspaceEventLogStore) Create(ctx context.Context, log *model.WorkspaceEventLog) (*model.WorkspaceEventLog, error) {
	row, err := s.queries.CreateWorkspaceEventLog(ctx, sqlc.CreateWorkspaceEventLogParams{
		ID:             log.ID,
		WorkspaceID:    log.WorkspaceID,
		OrganizationID: log.OrganizationID,
		RepoID:         log.RepoID,
		EventType:      log.EventType,
		Status:         log.Status,
		Error:          log.Error,
		Metadata:       []byte(log.Metadata),
	})
	if err != nil {
		return nil, err
	}
	return toWorkspaceEventLogModel(row), nil
}

func (s *workspaceEventLogStore) Update(ctx context.Context, log *model.WorkspaceEventLog, startedAt *time.Time, finishedAt *time.Time) (*model.WorkspaceEventLog, error) {
	row, err := s.queries.UpdateWorkspaceEventLog(ctx, sqlc.UpdateWorkspaceEventLogParams{
		ID:         log.ID,
		Status:     log.Status,
		Error:      log.Error,
		Metadata:   []byte(log.Metadata),
		StartedAt:  toNullableTimestamp(startedAt),
		FinishedAt: toNullableTimestamp(finishedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toWorkspaceEventLogModel(row), nil
}

func toWorkspaceEventLogModel(row sqlc.WorkspaceEventLog) *model.WorkspaceEventLog {
	return &model.WorkspaceEventLog{
		ID:             row.ID,
		WorkspaceID:    row.WorkspaceID,
		OrganizationID: row.OrganizationID,
		RepoID:         row.RepoID,
		EventType:      row.EventType,
		Status:         row.Status,
		Error:          row.Error,
		Metadata:       row.Metadata,
		StartedAt:      toTimePointer(row.StartedAt),
		FinishedAt:     toTimePointer(row.FinishedAt),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func toNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{
		Time:  *value,
		Valid: true,
	}
}

func toTimePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
