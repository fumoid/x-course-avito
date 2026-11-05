package container

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fumoid/x-course-avito/internal/config"
	httpapi "github.com/fumoid/x-course-avito/internal/http"
	"github.com/fumoid/x-course-avito/internal/http/health"
	createhttp "github.com/fumoid/x-course-avito/internal/http/trip/create"
	finishhttp "github.com/fumoid/x-course-avito/internal/http/trip/finish"
	gethttp "github.com/fumoid/x-course-avito/internal/http/trip/get"
	"github.com/fumoid/x-course-avito/internal/infra/postgres"
	"github.com/fumoid/x-course-avito/internal/infra/postgres/transaction"
	"github.com/fumoid/x-course-avito/internal/infra/telemetry"
	triprepo "github.com/fumoid/x-course-avito/internal/repository/trip"
	createuc "github.com/fumoid/x-course-avito/internal/usecase/trip/create"
	finishuc "github.com/fumoid/x-course-avito/internal/usecase/trip/finish"
	getuc "github.com/fumoid/x-course-avito/internal/usecase/trip/get"
)

type Container struct {
	Config    config.Config
	Logger    *slog.Logger
	Pool      *pgxpool.Pool
	Server    *http.Server
	telemetry *telemetry.Provider
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	tel, err := telemetry.New(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("init telemetry: %w", err)
	}

	tracer := postgres.NewQueryTracer(tel.Tracer, tel.DBDuration)
	pool, err := postgres.NewPool(ctx, cfg, tracer)
	if err != nil {
		_ = tel.Shutdown(ctx)
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		_ = tel.Shutdown(ctx)
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	repo := triprepo.New(pool, cfg.DatabaseQueryTimeout)
	if err := tel.RegisterActiveTrips(repo.CountActive); err != nil {
		pool.Close()
		_ = tel.Shutdown(ctx)
		return nil, err
	}

	txm := transaction.NewManager(pool, cfg.DatabaseQueryTimeout)
	clock := systemClock{}
	ids := uuidGenerator{}

	createUseCase := createuc.New(repo, txm, clock, ids, tel.Logger, tel.TripMetrics, cfg.IdempotencyTTL)
	getUseCase := getuc.New(repo)
	finishUseCase := finishuc.New(repo, txm, clock, tel.Logger, tel.TripMetrics)

	server := httpapi.New(
		createhttp.New(createUseCase, tel.Logger),
		gethttp.New(getUseCase, tel.Logger),
		finishhttp.New(finishUseCase, tel.Logger),
		health.New(pool, cfg.DatabaseQueryTimeout),
	)
	handler := httpapi.NewRouter(server, tel.Logger, tel.ActiveRequests)
	httpServer := httpapi.NewHTTPServer(
		cfg.HTTPAddr,
		handler,
		cfg.HTTPReadTimeout,
		cfg.HTTPReadHeaderTimeout,
		cfg.HTTPWriteTimeout,
		cfg.HTTPIdleTimeout,
	)

	return &Container{
		Config:    cfg,
		Logger:    tel.Logger,
		Pool:      pool,
		Server:    httpServer,
		telemetry: tel,
	}, nil
}

func (c *Container) Close(ctx context.Context) error {
	var first error
	if err := postgres.ClosePool(ctx, c.Pool); err != nil && first == nil {
		first = err
	}
	if c.telemetry != nil {
		if err := c.telemetry.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

type uuidGenerator struct{}

func (uuidGenerator) New() uuid.UUID {
	return uuid.New()
}
