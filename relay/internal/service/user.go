package service

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/store"
)

type UserService interface {
	Create(ctx context.Context, name, email string, avatarURL *string) (*model.User, error)
	Sync(ctx context.Context, name, email string, avatarURL *string) (*model.User, []model.Organization, error)
}

type userService struct {
	userStore store.UserStore
	orgStore  store.OrganizationStore
}

func NewUserService(userStore store.UserStore, orgStore store.OrganizationStore) UserService {
	return &userService{
		userStore: userStore,
		orgStore:  orgStore,
	}
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

func (s *userService) Sync(ctx context.Context, name, email string, avatarURL *string) (*model.User, []model.Organization, error) {
	user := &model.User{
		ID:        id.New(),
		Name:      name,
		Email:     email,
		AvatarURL: avatarURL,
	}

	if err := s.userStore.Upsert(ctx, user); err != nil {
		slog.ErrorContext(ctx, "failed to upsert user",
			"error", err,
			"email", email,
		)
		return nil, nil, fmt.Errorf("upserting user: %w", err)
	}

	orgs, err := s.orgStore.ListByAdminUser(ctx, user.ID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list organizations for user",
			"error", err,
			"user_id", user.ID,
		)
		return nil, nil, fmt.Errorf("listing organizations: %w", err)
	}

	return user, orgs, nil
}
