package repository

import (
	"basegraph.app/relay/core/db/sqlc"
)

// Repositories provides access to all repository implementations.
// It can be instantiated with either a connection pool or a transaction.
type Repositories struct {
	queries *sqlc.Queries
}

// NewRepositories creates a new Repositories instance from sqlc.Queries.
// The queries can be backed by either a connection pool or a transaction.
//
// Usage with pool (non-transactional):
//
//	repos := repository.NewRepositories(db.Queries())
//	user, err := repos.Users().GetByID(ctx, 123)
//
// Usage with transaction:
//
//	err := db.WithTx(ctx, func(q *sqlc.Queries) error {
//	    repos := repository.NewRepositories(q)
//	    // All operations share the same transaction
//	    if err := repos.Users().Create(ctx, user); err != nil {
//	        return err
//	    }
//	    return repos.Organizations().Create(ctx, org)
//	})
func NewRepositories(queries *sqlc.Queries) *Repositories {
	return &Repositories{queries: queries}
}

// Users returns the UserRepository
func (r *Repositories) Users() UserRepository {
	return NewUserRepository(r.queries)
}

// Organizations returns the OrganizationRepository
func (r *Repositories) Organizations() OrganizationRepository {
	return NewOrganizationRepository(r.queries)
}

// Workspaces returns the WorkspaceRepository
func (r *Repositories) Workspaces() WorkspaceRepository {
	return NewWorkspaceRepository(r.queries)
}

// Integrations returns the IntegrationRepository
func (r *Repositories) Integrations() IntegrationRepository {
	return NewIntegrationRepository(r.queries)
}

// Repositories returns the RepositoryRepository (for code repositories)
func (r *Repositories) Repositories() RepositoryRepository {
	return NewRepositoryRepository(r.queries)
}

// Sessions returns the SessionRepository
func (r *Repositories) Sessions() SessionRepository {
	return NewSessionRepository(r.queries)
}
