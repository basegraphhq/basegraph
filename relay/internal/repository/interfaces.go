package repository

import (
	"context"
	"errors"
	"time"

	"basegraph.app/relay/internal/model"
)

// ErrNotFound is returned when a requested entity does not exist
var ErrNotFound = errors.New("not found")

// UserRepository defines the contract for user data access
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id int64) error
}

// OrganizationRepository defines the contract for organization data access
type OrganizationRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*model.Organization, error)
	Create(ctx context.Context, org *model.Organization) error
	Update(ctx context.Context, org *model.Organization) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByAdminUser(ctx context.Context, userID int64) ([]model.Organization, error)
}

// WorkspaceRepository defines the contract for workspace data access
type WorkspaceRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Workspace, error)
	GetByOrgAndSlug(ctx context.Context, orgID int64, slug string) (*model.Workspace, error)
	Create(ctx context.Context, ws *model.Workspace) error
	Update(ctx context.Context, ws *model.Workspace) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Workspace, error)
	ListByUser(ctx context.Context, userID int64) ([]model.Workspace, error)
}

// IntegrationRepository defines the contract for integration data access
type IntegrationRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Integration, error)
	GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error)
	Create(ctx context.Context, integration *model.Integration) error
	UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error
	Delete(ctx context.Context, id int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error)
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error)
}

// RepositoryRepository defines the contract for code repository data access
type RepositoryRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Repository, error)
	GetByExternalID(ctx context.Context, integrationID int64, externalRepoID string) (*model.Repository, error)
	Create(ctx context.Context, repo *model.Repository) error
	Update(ctx context.Context, repo *model.Repository) error
	Delete(ctx context.Context, id int64) error
	DeleteByIntegration(ctx context.Context, integrationID int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Repository, error)
	ListByIntegration(ctx context.Context, integrationID int64) ([]model.Repository, error)
}

// SessionRepository defines the contract for session data access
type SessionRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Session, error)
	GetValid(ctx context.Context, id int64) (*model.Session, error) // checks expiry
	Create(ctx context.Context, session *model.Session) error
	Delete(ctx context.Context, id int64) error
	DeleteByUser(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	ListByUser(ctx context.Context, userID int64) ([]model.Session, error)
}
