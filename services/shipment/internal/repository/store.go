// Package repository gives access to the shipment Postgres database (sqlc queries) and
// the Redis read-through cache.
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository/db"
)

// Store exposes sqlc queries on the pool and runs functions inside transactions.
type Store struct {
	*db.Queries
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Queries: db.New(pool), pool: pool}
}

// InTx runs fn in a transaction: commit if fn returns nil, rollback otherwise.
func (s *Store) InTx(ctx context.Context, fn func(q *db.Queries) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(s.WithTx(tx))
	})
}
