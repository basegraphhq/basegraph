package service_test

import (
	"context"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	"basegraph.app/relay/internal/store"
)

type mockUserStore struct {
	createFn           func(ctx context.Context, user *model.User) error
	getByIDFn          func(ctx context.Context, id int64) (*model.User, error)
	getByEmailFn       func(ctx context.Context, email string) (*model.User, error)
	getByWorkOSIDFn    func(ctx context.Context, workosID string) (*model.User, error)
	upsertFn           func(ctx context.Context, user *model.User) error
	upsertByWorkOSIDFn func(ctx context.Context, user *model.User) error
	updateFn           func(ctx context.Context, user *model.User) error
	deleteFn           func(ctx context.Context, id int64) error
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

func (m *mockUserStore) GetByWorkOSID(ctx context.Context, workosID string) (*model.User, error) {
	if m.getByWorkOSIDFn != nil {
		return m.getByWorkOSIDFn(ctx, workosID)
	}
	return nil, nil
}

func (m *mockUserStore) Upsert(ctx context.Context, user *model.User) error {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, user)
	}
	return nil
}

func (m *mockUserStore) UpsertByWorkOSID(ctx context.Context, user *model.User) error {
	if m.upsertByWorkOSIDFn != nil {
		return m.upsertByWorkOSIDFn(ctx, user)
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
	createCalls       int
}

func (m *mockOrganizationStore) Create(ctx context.Context, org *model.Organization) error {
	m.createCalls++
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

type mockWorkspaceStore struct {
	createFn          func(ctx context.Context, ws *model.Workspace) error
	getByOrgAndSlugFn func(ctx context.Context, orgID int64, slug string) (*model.Workspace, error)
	createCalls       int
}

func (m *mockWorkspaceStore) Create(ctx context.Context, ws *model.Workspace) error {
	m.createCalls++
	if m.createFn != nil {
		return m.createFn(ctx, ws)
	}
	return nil
}

func (m *mockWorkspaceStore) GetByOrgAndSlug(ctx context.Context, orgID int64, slug string) (*model.Workspace, error) {
	if m.getByOrgAndSlugFn != nil {
		return m.getByOrgAndSlugFn(ctx, orgID, slug)
	}
	return nil, nil
}

func (m *mockWorkspaceStore) GetByID(ctx context.Context, id int64) (*model.Workspace, error) {
	return nil, nil
}

func (m *mockWorkspaceStore) Update(ctx context.Context, ws *model.Workspace) error {
	return nil
}

func (m *mockWorkspaceStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (m *mockWorkspaceStore) ListByOrganization(ctx context.Context, orgID int64) ([]model.Workspace, error) {
	return nil, nil
}

func (m *mockWorkspaceStore) ListByUser(ctx context.Context, userID int64) ([]model.Workspace, error) {
	return nil, nil
}

type mockStoreProvider struct {
	org         store.OrganizationStore
	work        store.WorkspaceStore
	integration store.IntegrationStore
	cred        store.IntegrationCredentialStore
	config      store.IntegrationConfigStore
	repo        store.RepoStore
	issue       store.IssueStore
	eventLog    store.EventLogStore
}

func (m *mockStoreProvider) Organizations() store.OrganizationStore {
	return m.org
}

func (m *mockStoreProvider) Workspaces() store.WorkspaceStore {
	return m.work
}

func (m *mockStoreProvider) Integrations() store.IntegrationStore {
	return m.integration
}

func (m *mockStoreProvider) EventLogs() store.EventLogStore {
	return m.eventLog
}

func (m *mockStoreProvider) Issues() store.IssueStore {
	return m.issue
}

func (m *mockStoreProvider) IntegrationCredentials() store.IntegrationCredentialStore {
	return m.cred
}

func (m *mockStoreProvider) IntegrationConfigs() store.IntegrationConfigStore {
	return m.config
}

func (m *mockStoreProvider) Repos() store.RepoStore {
	return m.repo
}

type mockTxRunner struct {
	withTxFn func(ctx context.Context, fn func(stores service.StoreProvider) error) error
}

func (m *mockTxRunner) WithTx(ctx context.Context, fn func(stores service.StoreProvider) error) error {
	if m.withTxFn != nil {
		return m.withTxFn(ctx, fn)
	}
	return fn(&mockStoreProvider{})
}

type mockIntegrationConfigStore struct {
	getByIntegrationAndKeyFn func(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error)
}

func (m *mockIntegrationConfigStore) GetByID(context.Context, int64) (*model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockIntegrationConfigStore) GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error) {
	if m.getByIntegrationAndKeyFn != nil {
		return m.getByIntegrationAndKeyFn(ctx, integrationID, key)
	}
	return nil, store.ErrNotFound
}

func (m *mockIntegrationConfigStore) ListByIntegration(context.Context, int64) ([]model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockIntegrationConfigStore) ListByIntegrationAndType(context.Context, int64, string) ([]model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockIntegrationConfigStore) Create(context.Context, *model.IntegrationConfig) error {
	return nil
}

func (m *mockIntegrationConfigStore) Update(context.Context, *model.IntegrationConfig) error {
	return nil
}

func (m *mockIntegrationConfigStore) Upsert(context.Context, *model.IntegrationConfig) error {
	return nil
}

func (m *mockIntegrationConfigStore) Delete(context.Context, int64) error {
	return nil
}

func (m *mockIntegrationConfigStore) DeleteByIntegration(context.Context, int64) error {
	return nil
}
