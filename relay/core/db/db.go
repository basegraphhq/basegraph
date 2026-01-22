package db

import (
	"context"
	"fmt"

	"basegraph.co/relay/core/db/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and provides transaction support.
// It serves as the main entry point for database operations.
type DB struct {
	pool *pgxpool.Pool
}

type Config struct {
	// ! TODO: @nithinsj -- Use sslmode in production
	DSN string

	// With PgBouncer, this can be relatively low per replica.
	MaxConns int32

	MinConns int32
}

// New creates a new DB instance with the given configuration.
func New(ctx context.Context, cfg Config) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	} else {
		poolCfg.MaxConns = 10 // sensible default for PgBouncer setup
	}

	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	} else {
		poolCfg.MinConns = 2
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &DB{pool: pool}, nil
}

func (db *DB) Close() {
	db.pool.Close()
}

// Queries returns a new Queries instance for non-transactional operations.
func (db *DB) Queries() *sqlc.Queries {
	return sqlc.New(db.pool)
}

// WithTx executes the given function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// If the function succeeds, the transaction is committed.
//
// Usage:
//
//	err := db.WithTx(ctx, func(q *sqlc.Queries) error {
//	    user, err := q.CreateUser(ctx, ...)
//	    if err != nil { return err }
//
//	    _, err = q.CreateOrganization(ctx, ...)
//	    return err
//	})
func (db *DB) WithTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	// Always attempt rollback on defer - it's a no-op if already committed
	defer tx.Rollback(ctx) //nolint:errcheck

	q := sqlc.New(tx)
	if err := fn(q); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
