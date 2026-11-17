package list

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

func (u *UseCase) Execute(ctx context.Context, tripID uuid.UUID) ([]domain.Position, error) {
	if tripID == uuid.Nil {
		return nil, domain.NewValidationError("tripId", "must be a non-zero UUID")
	}
	if _, err := u.repo.Get(ctx, tripID); err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	positions, err := u.repo.ListPositions(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	return positions, nil
}
