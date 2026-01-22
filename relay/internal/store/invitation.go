package store

import (
	"context"
	"errors"

	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type invitationStore struct {
	queries *sqlc.Queries
}

func newInvitationStore(queries *sqlc.Queries) InvitationStore {
	return &invitationStore{queries: queries}
}

func (s *invitationStore) Create(ctx context.Context, inv *model.Invitation) error {
	row, err := s.queries.CreateInvitation(ctx, sqlc.CreateInvitationParams{
		ID:        inv.ID,
		Email:     inv.Email,
		Token:     inv.Token,
		Status:    string(inv.Status),
		InvitedBy: inv.InvitedBy,
		ExpiresAt: pgtype.Timestamptz{Time: inv.ExpiresAt, Valid: true},
	})
	if err != nil {
		return err
	}
	*inv = *toInvitationModel(row)
	return nil
}

func (s *invitationStore) GetByID(ctx context.Context, id int64) (*model.Invitation, error) {
	row, err := s.queries.GetInvitationByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toInvitationModel(row), nil
}

func (s *invitationStore) GetByToken(ctx context.Context, token string) (*model.Invitation, error) {
	row, err := s.queries.GetInvitationByToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toInvitationModel(row), nil
}

func (s *invitationStore) GetValidByToken(ctx context.Context, token string) (*model.Invitation, error) {
	row, err := s.queries.GetValidInvitationByToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toInvitationModel(row), nil
}

func (s *invitationStore) GetByEmail(ctx context.Context, email string) (*model.Invitation, error) {
	row, err := s.queries.GetInvitationByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toInvitationModel(row), nil
}

func (s *invitationStore) Accept(ctx context.Context, id int64, userID int64) (*model.Invitation, error) {
	row, err := s.queries.AcceptInvitation(ctx, sqlc.AcceptInvitationParams{
		ID:         id,
		AcceptedBy: &userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toInvitationModel(row), nil
}

func (s *invitationStore) Revoke(ctx context.Context, id int64) (*model.Invitation, error) {
	row, err := s.queries.RevokeInvitation(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toInvitationModel(row), nil
}

func (s *invitationStore) List(ctx context.Context, limit, offset int32) ([]model.Invitation, error) {
	rows, err := s.queries.ListInvitations(ctx, sqlc.ListInvitationsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	return toInvitationModels(rows), nil
}

func (s *invitationStore) ListPending(ctx context.Context) ([]model.Invitation, error) {
	rows, err := s.queries.ListPendingInvitations(ctx)
	if err != nil {
		return nil, err
	}
	return toInvitationModels(rows), nil
}

func (s *invitationStore) ExpireOld(ctx context.Context) error {
	return s.queries.ExpireOldInvitations(ctx)
}

func toInvitationModel(row sqlc.Invitation) *model.Invitation {
	inv := &model.Invitation{
		ID:         row.ID,
		Email:      row.Email,
		Token:      row.Token,
		Status:     model.InvitationStatus(row.Status),
		InvitedBy:  row.InvitedBy,
		AcceptedBy: row.AcceptedBy,
		ExpiresAt:  row.ExpiresAt.Time,
		CreatedAt:  row.CreatedAt.Time,
	}
	if row.AcceptedAt.Valid {
		inv.AcceptedAt = &row.AcceptedAt.Time
	}
	return inv
}

func toInvitationModels(rows []sqlc.Invitation) []model.Invitation {
	result := make([]model.Invitation, len(rows))
	for i, row := range rows {
		result[i] = *toInvitationModel(row)
	}
	return result
}
