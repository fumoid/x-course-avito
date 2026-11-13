package create

import (
	"context"

	usecase "github.com/fumoid/x-course-avito/internal/usecase/trip/create"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type (
	useCase interface {
		Execute(ctx context.Context, cmd usecase.Command) (usecase.Result, error)
	}

	logger interface {
		ErrorContext(ctx context.Context, msg string, args ...any)
	}
)
