package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fumoid/x-course-avito/internal/config"
)

func NewPool(ctx context.Context, cfg config.Config, tracer *QueryTracer) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolCfg.MaxConns = cfg.DatabaseMaxConns
	poolCfg.MinConns = cfg.DatabaseMinConns
	poolCfg.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolCfg.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout
	if tracer != nil {
		poolCfg.ConnConfig.Tracer = tracer
	}

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	return pool, nil
}

func ClosePool(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("close PostgreSQL pool: %w", ctx.Err())
	}
}
