package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type (
	tripRepository interface {
		Save(ctx context.Context, trip domain.Trip) error
		SaveStatusHistory(ctx context.Context, tripID uuid.UUID, from *domain.Status, to domain.Status, reason string, changedAt time.Time) error
		Get(ctx context.Context, id uuid.UUID) (domain.Trip, error)
		LockIdempotencyKey(ctx context.Context, key uuid.UUID) error
		DeleteExpiredIdempotency(ctx context.Context, key uuid.UUID, now time.Time) error
		FindIdempotency(ctx context.Context, key uuid.UUID, now time.Time) (*domain.IdempotencyRecord, error)
		SaveIdempotency(ctx context.Context, rec domain.IdempotencyRecord) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	clock interface {
		Now() time.Time
	}

	idGenerator interface {
		New() uuid.UUID
	}

	logger interface {
		InfoContext(ctx context.Context, msg string, args ...any)
	}

	metrics interface {
		TripCreated(ctx context.Context)
	}
)
