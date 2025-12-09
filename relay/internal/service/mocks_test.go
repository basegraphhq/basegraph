package service_test

import (
	"context"

	"basegraph.app/relay/internal/model"
)

type mockUserStore struct {
	createFn     func(ctx context.Context, user *model.User) error
	getByIDFn    func(ctx context.Context, id int64) (*model.User, error)
	getByEmailFn func(ctx context.Context, email string) (*model.User, error)
	upsertFn     func(ctx context.Context, user *model.User) error
	updateFn     func(ctx context.Context, user *model.User) error
	deleteFn     func(ctx context.Context, id int64) error
}

func (m *mockUserStore) Create(ctx context.Context, user *model.User) error {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return nil
}

func (m *mockUserStore) GetByID(ctx context.Context, id int64) (*model.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockUserStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, email)
	}
	return nil, nil
}

func (m *mockUserStore) Upsert(ctx context.Context, user *model.User) error {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, user)
	}
	return nil
}

func (m *mockUserStore) Update(ctx context.Context, user *model.User) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return nil
}

func (m *mockUserStore) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

type mockOrganizationStore struct {
	createFn          func(ctx context.Context, org *model.Organization) error
	getBySlugFn       func(ctx context.Context, slug string) (*model.Organization, error)
	listByAdminUserFn func(ctx context.Context, userID int64) ([]model.Organization, error)
}

func (m *mockOrganizationStore) Create(ctx context.Context, org *model.Organization) error {
	if m.createFn != nil {
		return m.createFn(ctx, org)
	}
	return nil
}

func (m *mockOrganizationStore) GetByID(ctx context.Context, _ int64) (*model.Organization, error) {
	return nil, nil
}

func (m *mockOrganizationStore) GetBySlug(ctx context.Context, slug string) (*model.Organization, error) {
	if m.getBySlugFn != nil {
		return m.getBySlugFn(ctx, slug)
	}
	return nil, nil
}

func (m *mockOrganizationStore) Update(ctx context.Context, _ *model.Organization) error {
	return nil
}

func (m *mockOrganizationStore) Delete(ctx context.Context, _ int64) error {
	return nil
}

func (m *mockOrganizationStore) ListByAdminUser(ctx context.Context, userID int64) ([]model.Organization, error) {
	if m.listByAdminUserFn != nil {
		return m.listByAdminUserFn(ctx, userID)
	}
	return []model.Organization{}, nil
}
