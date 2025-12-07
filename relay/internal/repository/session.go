package repository

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type sessionRepository struct {
	queries *sqlc.Queries
}

// NewSessionRepository creates a new SessionRepository
func NewSessionRepository(queries *sqlc.Queries) SessionRepository {
	return &sessionRepository{queries: queries}
}

func (r *sessionRepository) GetByID(ctx context.Context, id int64) (*model.Session, error) {
	row, err := r.queries.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toSessionModel(row), nil
}

func (r *sessionRepository) GetValid(ctx context.Context, id int64) (*model.Session, error) {
	row, err := r.queries.GetValidSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toSessionModel(row), nil
}

func (r *sessionRepository) Create(ctx context.Context, session *model.Session) error {
	row, err := r.queries.CreateSession(ctx, sqlc.CreateSessionParams{
		ID:        session.ID,
		UserID:    session.UserID,
		ExpiresAt: pgtype.Timestamptz{Time: session.ExpiresAt, Valid: true},
	})
	if err != nil {
		return err
	}
	*session = *toSessionModel(row)
	return nil
}

func (r *sessionRepository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteSession(ctx, id)
}

func (r *sessionRepository) DeleteByUser(ctx context.Context, userID int64) error {
	return r.queries.DeleteSessionsByUser(ctx, userID)
}

func (r *sessionRepository) DeleteExpired(ctx context.Context) error {
	return r.queries.DeleteExpiredSessions(ctx)
}

func (r *sessionRepository) ListByUser(ctx context.Context, userID int64) ([]model.Session, error) {
	rows, err := r.queries.ListSessionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toSessionModels(rows), nil
}

// toSessionModel converts sqlc.Session to model.Session
func toSessionModel(row sqlc.Session) *model.Session {
	return &model.Session{
		ID:        row.ID,
		UserID:    row.UserID,
		CreatedAt: row.CreatedAt.Time,
		ExpiresAt: row.ExpiresAt.Time,
	}
}

func toSessionModels(rows []sqlc.Session) []model.Session {
	result := make([]model.Session, len(rows))
	for i, row := range rows {
		result[i] = *toSessionModel(row)
	}
	return result
}

