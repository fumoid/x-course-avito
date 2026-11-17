package create

import (
	"context"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	usecase "github.com/fumoid/x-course-avito/internal/usecase/trip/position/create"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type (
	useCase interface {
		Execute(ctx context.Context, cmd usecase.Command) (domain.Position, error)
	}

	logger interface {
		ErrorContext(ctx context.Context, msg string, args ...any)
	}
)
