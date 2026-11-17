package create

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

type Command struct {
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

type UseCase struct {
	repo    tripRepository
	tx      txManager
	clock   clock
	cancel  canceller
	log     logger
	metrics metrics
}

func New(repo tripRepository, tx txManager, clock clock, cancel canceller, log logger, metrics metrics) *UseCase {
	return &UseCase{
		repo:    repo,
		tx:      tx,
		clock:   clock,
		cancel:  cancel,
		log:     log,
		metrics: metrics,
	}
}

func (u *UseCase) Execute(ctx context.Context, cmd Command) (domain.Position, error) {
	position, err := domain.NewPosition(domain.NewPositionParams{
		TripID:     cmd.TripID,
		Latitude:   cmd.Latitude,
		Longitude:  cmd.Longitude,
		RecordedAt: cmd.RecordedAt,
		Now:        u.clock.Now(),
	})
	if err != nil {
		return domain.Position{}, err
	}

	var saved domain.Position
	err = u.tx.Do(ctx, func(ctx context.Context) error {
		trip, err := u.repo.GetForUpdate(ctx, cmd.TripID)
		if err != nil {
			return err
		}
		if trip.Status != domain.StatusActive {
			return domain.ErrTripCompleted
		}
		created, err := u.repo.SavePosition(ctx, position)
		if err != nil {
			return err
		}
		if err := u.repo.UpdateLastPositionAt(ctx, cmd.TripID, position.RecordedAt); err != nil {
			return err
		}
		saved = created
		return nil
	})
	if err != nil {
		return domain.Position{}, fmt.Errorf("create position: %w", err)
	}

	u.cancel.Cancel(ctx, cmd.TripID)
	u.metrics.PositionSaved(ctx)
	u.log.InfoContext(ctx, "position saved", "trip_id", cmd.TripID.String(), "position_id", saved.ID)
	return saved, nil
}
