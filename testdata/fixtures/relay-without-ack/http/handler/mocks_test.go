package handler_test

import (
	"context"

	"basegraph.co/relay/internal/model"
)

type mockUserService struct {
	syncFn   func(ctx context.Context, name, email string, avatarURL *string) (*model.User, []model.Organization, error)
	createFn func(ctx context.Context, name, email string, avatarURL *string) (*model.User, error)
}

func (m *mockUserService) Sync(ctx context.Context, name, email string, avatarURL *string) (*model.User, []model.Organization, error) {
	if m.syncFn != nil {
		return m.syncFn(ctx, name, email, avatarURL)
	}
	return nil, nil, nil
}

func (m *mockUserService) Create(ctx context.Context, name, email string, avatarURL *string) (*model.User, error) {
	if m.createFn != nil {
		return m.createFn(ctx, name, email, avatarURL)
	}
	return nil, nil
}

type mockOrganizationService struct {
	createFn func(ctx context.Context, name string, slug *string, adminUserID int64) (*model.Organization, error)
}

func (m *mockOrganizationService) Create(ctx context.Context, name string, slug *string, adminUserID int64) (*model.Organization, error) {
	if m.createFn != nil {
		return m.createFn(ctx, name, slug, adminUserID)
	}
	return nil, nil
}
