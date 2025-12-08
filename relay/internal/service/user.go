package service

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type UserService interface {
	Create(ctx context.Context, name, email string, avatarURL *string) (*model.User, error)
}

type userService struct {
	userStore store.UserStore
}

func NewUserService(userStore store.UserStore) UserService {
	return &userService{userStore: userStore}
}

func (s *userService) Create(ctx context.Context, name, email string, avatarURL *string) (*model.User, error) {
	user := &model.User{
		ID:        id.New(),
		Name:      name,
		Email:     email,
		AvatarURL: avatarURL,
	}

	if err := s.userStore.Create(ctx, user); err != nil {
		slog.ErrorContext(ctx, "failed to create user",
			"error", err,
			"email", email,
		)
		return nil, fmt.Errorf("creating user: %w", err)
	}

	slog.InfoContext(ctx, "user created", "user_id", user.ID)
	return user, nil
}
