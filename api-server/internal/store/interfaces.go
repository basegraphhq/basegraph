package store

import (
	"context"
	"errors"
	"time"

	"basegraph.app/api-server/internal/model"
)

var ErrNotFound = errors.New("not found")

type UserStore interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByWorkOSID(ctx context.Context, workosID string) (*model.User, error)
	Upsert(ctx context.Context, user *model.User) error
	UpsertByWorkOSID(ctx context.Context, user *model.User) error
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id int64) error
}

type OrganizationStore interface {
	GetByID(ctx context.Context, id int64) (*model.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*model.Organization, error)
	Create(ctx context.Context, org *model.Organization) error
	Update(ctx context.Context, org *model.Organization) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByAdminUser(ctx context.Context, userID int64) ([]model.Organization, error)
}

type WorkspaceStore interface {
	GetByID(ctx context.Context, id int64) (*model.Workspace, error)
	GetByOrgAndSlug(ctx context.Context, orgID int64, slug string) (*model.Workspace, error)
	Create(ctx context.Context, ws *model.Workspace) error
	Update(ctx context.Context, ws *model.Workspace) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Workspace, error)
	ListByUser(ctx context.Context, userID int64) ([]model.Workspace, error)
}

type IntegrationStore interface {
	GetByID(ctx context.Context, id int64) (*model.Integration, error)
	GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error)
	Create(ctx context.Context, integration *model.Integration) error
	Update(ctx context.Context, integration *model.Integration) error
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	Delete(ctx context.Context, id int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error)
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error)
	ListByCapability(ctx context.Context, workspaceID int64, capability model.Capability) ([]model.Integration, error)
}

type IntegrationCredentialStore interface {
	GetByID(ctx context.Context, id int64) (*model.IntegrationCredential, error)
	GetPrimaryByIntegration(ctx context.Context, integrationID int64) (*model.IntegrationCredential, error)
	GetByIntegrationAndUser(ctx context.Context, integrationID int64, userID int64) (*model.IntegrationCredential, error)
	Create(ctx context.Context, cred *model.IntegrationCredential) error
	UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error
	SetAsPrimary(ctx context.Context, integrationID int64, credentialID int64) error
	Revoke(ctx context.Context, id int64) error
	RevokeAllByIntegration(ctx context.Context, integrationID int64) error
	Delete(ctx context.Context, id int64) error
	ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error)
	ListActiveByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error)
}

type IntegrationConfigStore interface {
	GetByID(ctx context.Context, id int64) (*model.IntegrationConfig, error)
	GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error)
	ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationConfig, error)
	ListByIntegrationAndType(ctx context.Context, integrationID int64, configType string) ([]model.IntegrationConfig, error)
	Create(ctx context.Context, config *model.IntegrationConfig) error
	Update(ctx context.Context, config *model.IntegrationConfig) error
	Upsert(ctx context.Context, config *model.IntegrationConfig) error
	Delete(ctx context.Context, id int64) error
	DeleteByIntegration(ctx context.Context, integrationID int64) error
}

type RepoStore interface {
	GetByID(ctx context.Context, id int64) (*model.Repository, error)
	GetByExternalID(ctx context.Context, integrationID int64, externalRepoID string) (*model.Repository, error)
	Create(ctx context.Context, repo *model.Repository) error
	Update(ctx context.Context, repo *model.Repository) error
	Delete(ctx context.Context, id int64) error
	DeleteByIntegration(ctx context.Context, integrationID int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Repository, error)
	ListByIntegration(ctx context.Context, integrationID int64) ([]model.Repository, error)
}

type SessionStore interface {
	GetByID(ctx context.Context, id int64) (*model.Session, error)
	GetValid(ctx context.Context, id int64) (*model.Session, error) // checks expiry
	Create(ctx context.Context, session *model.Session) error
	Delete(ctx context.Context, id int64) error
	DeleteByUser(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	ListByUser(ctx context.Context, userID int64) ([]model.Session, error)
}
