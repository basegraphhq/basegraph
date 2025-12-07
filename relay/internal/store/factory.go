package store

import (
	"basegraph.app/relay/core/db/sqlc"
)

// Stores provides access to all store implementations.
// It can be instantiated with either a connection pool or a transaction.
type Stores struct {
	queries *sqlc.Queries
}

// NewStores creates a new Stores instance from sqlc.Queries.
// The queries can be backed by either a connection pool or a transaction.
//
// Usage with pool (non-transactional):
//
//	stores := store.NewStores(db.Queries())
//	user, err := stores.Users().GetByID(ctx, 123)
//
// Usage with transaction:
//
//	err := db.WithTx(ctx, func(q *sqlc.Queries) error {
//	    stores := store.NewStores(q)
//	    // All operations share the same transaction
//	    if err := stores.Users().Create(ctx, user); err != nil {
//	        return err
//	    }
//	    return stores.Organizations().Create(ctx, org)
//	})
func NewStores(queries *sqlc.Queries) *Stores {
	return &Stores{queries: queries}
}

// Users returns the UserStore
func (s *Stores) Users() UserStore {
	return newUserStore(s.queries)
}

// Organizations returns the OrganizationStore
func (s *Stores) Organizations() OrganizationStore {
	return newOrganizationStore(s.queries)
}

// Workspaces returns the WorkspaceStore
func (s *Stores) Workspaces() WorkspaceStore {
	return newWorkspaceStore(s.queries)
}

// Integrations returns the IntegrationStore
func (s *Stores) Integrations() IntegrationStore {
	return newIntegrationStore(s.queries)
}

// Repos returns the RepoStore (for code repositories)
func (s *Stores) Repos() RepoStore {
	return newRepoStore(s.queries)
}

// Sessions returns the SessionStore
func (s *Stores) Sessions() SessionStore {
	return newSessionStore(s.queries)
}
