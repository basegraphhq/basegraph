package handler_test

import (
	"context"

	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/service"
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

type mockInvitationService struct {
	createFn        func(ctx context.Context, email string, invitedBy *int64) (*model.Invitation, string, error)
	validateTokenFn func(ctx context.Context, token string) (*model.Invitation, error)
	getByTokenFn    func(ctx context.Context, token string) (*model.Invitation, error)
	acceptFn        func(ctx context.Context, token string, user *model.User) (*model.Invitation, error)
	revokeFn        func(ctx context.Context, id int64) (*model.Invitation, error)
	listFn          func(ctx context.Context, limit, offset int32) ([]model.Invitation, error)
	listPendingFn   func(ctx context.Context) ([]model.Invitation, error)
}

func (m *mockInvitationService) Create(ctx context.Context, email string, invitedBy *int64) (*model.Invitation, string, error) {
	if m.createFn != nil {
		return m.createFn(ctx, email, invitedBy)
	}
	return nil, "", nil
}

func (m *mockInvitationService) ValidateToken(ctx context.Context, token string) (*model.Invitation, error) {
	if m.validateTokenFn != nil {
		return m.validateTokenFn(ctx, token)
	}
	return nil, nil
}

func (m *mockInvitationService) GetByToken(ctx context.Context, token string) (*model.Invitation, error) {
	if m.getByTokenFn != nil {
		return m.getByTokenFn(ctx, token)
	}
	return nil, nil
}

func (m *mockInvitationService) Accept(ctx context.Context, token string, user *model.User) (*model.Invitation, error) {
	if m.acceptFn != nil {
		return m.acceptFn(ctx, token, user)
	}
	return nil, nil
}

func (m *mockInvitationService) Revoke(ctx context.Context, id int64) (*model.Invitation, error) {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, id)
	}
	return nil, nil
}

func (m *mockInvitationService) List(ctx context.Context, limit, offset int32) ([]model.Invitation, error) {
	if m.listFn != nil {
		return m.listFn(ctx, limit, offset)
	}
	return []model.Invitation{}, nil
}

func (m *mockInvitationService) ListPending(ctx context.Context) ([]model.Invitation, error) {
	if m.listPendingFn != nil {
		return m.listPendingFn(ctx)
	}
	return []model.Invitation{}, nil
}

type mockAuthService struct {
	validateSessionFn func(ctx context.Context, sessionID int64) (*model.User, *service.UserContext, error)
}

func (m *mockAuthService) GetAuthorizationURL(_ string, _ ...service.AuthURLOption) (string, error) {
	return "", nil
}

func (m *mockAuthService) HandleCallback(_ context.Context, _ string) (*service.CallbackResult, error) {
	return nil, nil
}

func (m *mockAuthService) HandleSignIn(_ context.Context, _ string) (*service.CallbackResult, error) {
	return nil, nil
}

func (m *mockAuthService) ValidateSession(ctx context.Context, sessionID int64) (*model.User, *service.UserContext, error) {
	if m.validateSessionFn != nil {
		return m.validateSessionFn(ctx, sessionID)
	}
	return nil, nil, nil
}

func (m *mockAuthService) GetSessionByID(_ context.Context, _ int64) (*model.Session, error) {
	return nil, nil
}

func (m *mockAuthService) Logout(_ context.Context, _ int64) error {
	return nil
}

func (m *mockAuthService) GetLogoutURL(_ string, _ string) string {
	return ""
}
