package tx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type Manager struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewManager(pool *pgxpool.Pool, queryTimeout time.Duration) *Manager {
	return &Manager{pool: pool, queryTimeout: queryTimeout}
}

func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	beginCtx, cancelBegin := m.withTimeout(ctx)
	tx, err := m.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancelBegin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)
	defer func() {
		if rec := recover(); rec != nil {
			_ = m.rollback(txCtx, tx)
			panic(rec)
		}
	}()

	if err := fn(txCtx); err != nil {
		if rollbackErr := m.rollback(txCtx, tx); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}

	commitCtx, cancelCommit := m.withTimeout(ctx)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		commitErr := fmt.Errorf("commit transaction: %w", err)
		if rollbackErr := m.rollback(txCtx, tx); rollbackErr != nil {
			return errors.Join(commitErr, rollbackErr)
		}
		return commitErr
	}
	return nil
}

func (m *Manager) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, m.queryTimeout)
}

func (m *Manager) rollback(ctx context.Context, tx pgx.Tx) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.queryTimeout)
	defer cancel()
	if err := tx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("rollback transaction: %w", err)
	}
	return nil
}

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func Executor(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}
