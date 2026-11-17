package list

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type (
	useCase interface {
		Execute(ctx context.Context, tripID uuid.UUID) ([]domain.Position, error)
	}

	logger interface {
		ErrorContext(ctx context.Context, msg string, args ...any)
	}
)
