package finish

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

type UseCase struct {
	repo    tripRepository
	tx      txManager
	clock   clock
	log     logger
	metrics metrics
}

func New(repo tripRepository, tx txManager, clock clock, log logger, metrics metrics) *UseCase {
	return &UseCase{
		repo:    repo,
		tx:      tx,
		clock:   clock,
		log:     log,
		metrics: metrics,
	}
}

func (u *UseCase) Execute(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	if id == uuid.Nil {
		return domain.Trip{}, domain.NewValidationError("tripId", "must be a non-zero UUID")
	}

	var finished domain.Trip
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		existing, err := u.repo.Get(ctx, id)
		if err != nil {
			return err
		}
		at := u.clock.Now()
		if err := existing.Finish(at); err != nil {
			return err
		}
		if err := u.repo.UpdateFinished(ctx, existing); err != nil {
			return err
		}
		from := domain.StatusActive
		if err := u.repo.SaveStatusHistory(ctx, existing.ID, &from, domain.StatusCompleted, domain.ReasonFinished, at); err != nil {
			return err
		}
		finished = existing
		return nil
	})
	if err != nil {
		return domain.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	u.metrics.TripCompleted(ctx)
	u.log.InfoContext(ctx, "trip completed", "trip_id", finished.ID.String())
	return finished, nil
}
