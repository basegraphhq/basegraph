package service

import (
	"context"

	"basegraph.app/relay/core/db"
	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/store"
)

// StoreProvider exposes only the stores needed by a transactional operation.
type StoreProvider interface {
	Organizations() store.OrganizationStore
	Workspaces() store.WorkspaceStore
	Integrations() store.IntegrationStore
	IntegrationCredentials() store.IntegrationCredentialStore
	IntegrationConfigs() store.IntegrationConfigStore
	Repos() store.RepoStore
}

// TxRunner runs functions within a transaction and provides stores bound to that transaction.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(stores StoreProvider) error) error
}

type dbTxRunner struct {
	db *db.DB
}

// NewTxRunner builds a TxRunner backed by the core DB.
func NewTxRunner(db *db.DB) TxRunner {
	return &dbTxRunner{db: db}
}

func (r *dbTxRunner) WithTx(ctx context.Context, fn func(stores StoreProvider) error) error {
	return r.db.WithTx(ctx, func(q *sqlc.Queries) error {
		stores := store.NewStores(q)
		return fn(stores)
	})
}
