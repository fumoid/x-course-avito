package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type (
	staleSource interface {
		ListStale(ctx context.Context, staleAfter time.Duration, limit int, cursor *domain.StaleCursor) ([]domain.StaleTrip, error)
	}

	pusher interface {
		RequestPosition(ctx context.Context, tripID, driverID uuid.UUID) error
	}

	ticker interface {
		C() <-chan time.Time
		Stop()
	}

	metrics interface {
		StaleCheck(ctx context.Context, result string)
		StaleChecksRowsRead(ctx context.Context, rows int64)
		PositionRequest(ctx context.Context, result string)
		PositionRequestDuration(ctx context.Context, seconds float64, result string)
	}

	logger interface {
		InfoContext(ctx context.Context, msg string, args ...any)
		WarnContext(ctx context.Context, msg string, args ...any)
		ErrorContext(ctx context.Context, msg string, args ...any)
	}
)
