package store

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type userStore struct {
	queries *sqlc.Queries
}

func newUserStore(queries *sqlc.Queries) UserStore {
	return &userStore{queries: queries}
}

func (s *userStore) GetByID(ctx context.Context, id int64) (*model.User, error) {
	row, err := s.queries.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toUserModel(row), nil
}

func (s *userStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toUserModel(row), nil
}

func (s *userStore) Create(ctx context.Context, user *model.User) error {
	row, err := s.queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		AvatarUrl: user.AvatarURL,
	})
	if err != nil {
		return err
	}
	*user = *toUserModel(row)
	return nil
}

func (s *userStore) Update(ctx context.Context, user *model.User) error {
	row, err := s.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		AvatarUrl: user.AvatarURL,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	*user = *toUserModel(row)
	return nil
}

func (s *userStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteUser(ctx, id)
}

func toUserModel(row sqlc.User) *model.User {
	return &model.User{
		ID:        row.ID,
		Name:      row.Name,
		Email:     row.Email,
		AvatarURL: row.AvatarUrl,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}
