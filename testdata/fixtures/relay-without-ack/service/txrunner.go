package service

import (
	"context"
	"fmt"

	"basegraph.app/relay/core/db"
	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/service/integration"
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
	Issues() store.IssueStore
	EventLogs() store.EventLogStore
}

// TxRunner runs functions within a transaction and provides stores bound to that transaction.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(stores StoreProvider) error) error
}

type dbTxRunner struct {
	db *db.DB
}

// NewTxRunner builds a TxRunner backed by the core DB.
// Returns interface (not concrete type) to avoid exposing the internal db field.
func NewTxRunner(db *db.DB) TxRunner {
	return &dbTxRunner{db: db}
}

func (r *dbTxRunner) WithTx(ctx context.Context, fn func(stores StoreProvider) error) error {
	return r.db.WithTx(ctx, func(q *sqlc.Queries) error {
		stores := store.NewStores(q)
		return fn(stores)
	})
}

// ================================
// Adapters for subpackages
// ================================

// gitLabTxRunnerAdapter bridges the service.TxRunner and integration.TxRunner interfaces.
//
// The integration package defines its own TxRunner interface (with integration.StoreProvider)
// rather than importing service.TxRunner (which uses service.StoreProvider). This avoids an
// import cycle: service → integration → service.
//
// Each subpackage that needs transactions defines a narrow TxRunner interface scoped to its
// own StoreProvider type. The service factory then adapts its TxRunner to satisfy the
// subpackage's interface, keeping dependencies flowing in one direction.
type gitLabTxRunnerAdapter struct {
	tx TxRunner
}

// Note that we are sending integration.StoreProvider to the callback, not service.StoreProvider.
func (a *gitLabTxRunnerAdapter) WithTx(ctx context.Context, fn func(stores integration.StoreProvider) error) error {
	return a.tx.WithTx(ctx, func(sp StoreProvider) error {
		s, ok := sp.(*store.Stores)
		if !ok {
			return fmt.Errorf("unexpected store provider type %T", sp)
		}
		return fn(s)
	})
}
