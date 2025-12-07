package store

import (
	"context"
	"errors"
	"time"

	"basegraph.app/relay/internal/model"
)

// ErrNotFound is returned when a requested entity does not exist
var ErrNotFound = errors.New("not found")

// UserStore defines the contract for user data access
type UserStore interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id int64) error
}

// OrganizationStore defines the contract for organization data access
type OrganizationStore interface {
	GetByID(ctx context.Context, id int64) (*model.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*model.Organization, error)
	Create(ctx context.Context, org *model.Organization) error
	Update(ctx context.Context, org *model.Organization) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByAdminUser(ctx context.Context, userID int64) ([]model.Organization, error)
}

// WorkspaceStore defines the contract for workspace data access
type WorkspaceStore interface {
	GetByID(ctx context.Context, id int64) (*model.Workspace, error)
	GetByOrgAndSlug(ctx context.Context, orgID int64, slug string) (*model.Workspace, error)
	Create(ctx context.Context, ws *model.Workspace) error
	Update(ctx context.Context, ws *model.Workspace) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Workspace, error)
	ListByUser(ctx context.Context, userID int64) ([]model.Workspace, error)
}

// IntegrationStore defines the contract for integration data access
type IntegrationStore interface {
	GetByID(ctx context.Context, id int64) (*model.Integration, error)
	GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error)
	Create(ctx context.Context, integration *model.Integration) error
	UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error
	Delete(ctx context.Context, id int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error)
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error)
}

// RepoStore defines the contract for code repository data access
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

// SessionStore defines the contract for session data access
type SessionStore interface {
	GetByID(ctx context.Context, id int64) (*model.Session, error)
	GetValid(ctx context.Context, id int64) (*model.Session, error) // checks expiry
	Create(ctx context.Context, session *model.Session) error
	Delete(ctx context.Context, id int64) error
	DeleteByUser(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	ListByUser(ctx context.Context, userID int64) ([]model.Session, error)
}

