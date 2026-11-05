package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

type UseCase struct {
	repo tripRepository
}

func New(repo tripRepository) *UseCase {
	return &UseCase{repo: repo}
}

func (u *UseCase) Execute(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	if id == uuid.Nil {
		return domain.Trip{}, domain.NewValidationError("tripId", "must be a non-zero UUID")
	}
	entity, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return entity, nil
}
