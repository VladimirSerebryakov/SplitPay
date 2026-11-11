package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxKey struct{}

type Manager struct {
	pool *pgxpool.Pool
}

func (m *Manager) WithTx(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := ctx.Value(TxKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		txCtx := context.WithValue(ctx, TxKey{}, tx)
		return fn(txCtx)
	})
}

func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{pool: pool}
}

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

func querierFrom(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx, ok := ctx.Value(TxKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}
