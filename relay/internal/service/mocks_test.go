package service_test

import (
	"context"

	"basegraph.app/relay/internal/model"
)

type mockUserStore struct {
	createFn     func(ctx context.Context, user *model.User) error
	getByIDFn    func(ctx context.Context, id int64) (*model.User, error)
	getByEmailFn func(ctx context.Context, email string) (*model.User, error)
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
