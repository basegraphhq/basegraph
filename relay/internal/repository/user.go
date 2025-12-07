package repository

import (
	"context"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type userRepository struct {
	queries *sqlc.Queries
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(queries *sqlc.Queries) UserRepository {
	return &userRepository{queries: queries}
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	row, err := r.queries.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toUserModel(row), nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toUserModel(row), nil
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	row, err := r.queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		AvatarUrl: user.AvatarURL,
	})
	if err != nil {
		return err
	}
	// Update the model with DB-generated fields
	*user = *toUserModel(row)
	return nil
}

func (r *userRepository) Update(ctx context.Context, user *model.User) error {
	row, err := r.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
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

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteUser(ctx, id)
}

// toUserModel converts sqlc.User to model.User
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
