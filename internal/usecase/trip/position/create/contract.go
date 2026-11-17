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
		GetForUpdate(ctx context.Context, id uuid.UUID) (domain.Trip, error)
		SavePosition(ctx context.Context, position domain.Position) (domain.Position, error)
		UpdateLastPositionAt(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	clock interface {
		Now() time.Time
	}

	canceller interface {
		Cancel(ctx context.Context, tripID uuid.UUID)
	}

	logger interface {
		InfoContext(ctx context.Context, msg string, args ...any)
	}

	metrics interface {
		PositionSaved(ctx context.Context)
	}
)
