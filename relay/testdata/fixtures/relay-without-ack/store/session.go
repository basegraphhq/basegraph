package store

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type sessionStore struct {
	queries *sqlc.Queries
}

func newSessionStore(queries *sqlc.Queries) SessionStore {
	return &sessionStore{queries: queries}
}

func (s *sessionStore) GetByID(ctx context.Context, id int64) (*model.Session, error) {
	row, err := s.queries.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toSessionModel(row), nil
}

func (s *sessionStore) GetValid(ctx context.Context, id int64) (*model.Session, error) {
	row, err := s.queries.GetValidSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toSessionModel(row), nil
}

func (s *sessionStore) Create(ctx context.Context, session *model.Session) error {
	row, err := s.queries.CreateSession(ctx, sqlc.CreateSessionParams{
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

func (s *sessionStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteSession(ctx, id)
}

func (s *sessionStore) DeleteByUser(ctx context.Context, userID int64) error {
	return s.queries.DeleteSessionsByUser(ctx, userID)
}

func (s *sessionStore) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredSessions(ctx)
}

func (s *sessionStore) ListByUser(ctx context.Context, userID int64) ([]model.Session, error) {
	rows, err := s.queries.ListSessionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toSessionModels(rows), nil
}

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
