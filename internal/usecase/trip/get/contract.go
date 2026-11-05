package get

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type tripRepository interface {
	Get(ctx context.Context, id uuid.UUID) (domain.Trip, error)
}
