package brain

import (
	"context"

	"basegraph.co/relay/core/db"
	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/store"
)

// StoreProvider exposes stores needed by transactional operations in the brain package.
// This is a local interface to avoid import cycles (service → brain, not brain → service).
type StoreProvider interface {
	Issues() store.IssueStore
}

// TxRunner runs functions within a database transaction.
// This matches the pattern used in the integration package.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(stores StoreProvider) error) error
}

// dbTxRunner implements TxRunner using a db.DB connection.
type dbTxRunner struct {
	db *db.DB
}

// NewTxRunner creates a TxRunner backed by the given database.
func NewTxRunner(db *db.DB) TxRunner {
	return &dbTxRunner{db: db}
}

func (r *dbTxRunner) WithTx(ctx context.Context, fn func(stores StoreProvider) error) error {
	return r.db.WithTx(ctx, func(q *sqlc.Queries) error {
		stores := store.NewStores(q)
		return fn(stores)
	})
}
