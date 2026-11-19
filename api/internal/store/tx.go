package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InTx runs fn inside a database transaction. The transaction commits when fn
// returns nil and rolls back otherwise. Every write in the system should go
// through InTx so a request either fully succeeds or changes nothing.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *Queries, tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(New(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
