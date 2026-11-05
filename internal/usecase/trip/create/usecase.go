package create

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

type Command struct {
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartLat       float64
	StartLon       float64
	EndLat         float64
	EndLon         float64
	Price          int64
	IdempotencyKey *uuid.UUID
}

type Result struct {
	Trip   domain.Trip
	Replay bool
}

type UseCase struct {
	repo    tripRepository
	tx      txManager
	clock   clock
	ids     idGenerator
	log     logger
	metrics metrics
	ttl     time.Duration
}

func New(repo tripRepository, tx txManager, clock clock, ids idGenerator, log logger, metrics metrics, ttl time.Duration) *UseCase {
	return &UseCase{
		repo:    repo,
		tx:      tx,
		clock:   clock,
		ids:     ids,
		log:     log,
		metrics: metrics,
		ttl:     ttl,
	}
}

func (u *UseCase) Execute(ctx context.Context, cmd Command) (Result, error) {
	if cmd.IdempotencyKey != nil && *cmd.IdempotencyKey == uuid.Nil {
		return Result{}, domain.NewValidationError("Idempotency-Key", "must be a non-zero UUID")
	}
	requestHash := hashCommand(cmd)

	var created domain.Trip
	var replay bool

	err := u.tx.Do(ctx, func(ctx context.Context) error {
		now := u.clock.Now()

		if cmd.IdempotencyKey != nil {
			if err := u.repo.LockIdempotencyKey(ctx, *cmd.IdempotencyKey); err != nil {
				return err
			}
			if err := u.repo.DeleteExpiredIdempotency(ctx, *cmd.IdempotencyKey, now); err != nil {
				return err
			}
			rec, err := u.repo.FindIdempotency(ctx, *cmd.IdempotencyKey, now)
			if err != nil {
				return err
			}
			if rec != nil {
				if rec.RequestHash != requestHash {
					return domain.ErrIdempotencyConflict
				}
				existing, err := u.repo.Get(ctx, rec.TripID)
				if err != nil {
					return err
				}
				created = existing
				replay = true
				return nil
			}
		}

		entity, err := domain.New(domain.NewParams{
			ID:        u.ids.New(),
			UserID:    cmd.UserID,
			DriverID:  cmd.DriverID,
			StartLat:  cmd.StartLat,
			StartLon:  cmd.StartLon,
			EndLat:    cmd.EndLat,
			EndLon:    cmd.EndLon,
			Price:     cmd.Price,
			StartedAt: now,
		})
		if err != nil {
			return err
		}
		if err := u.repo.Save(ctx, entity); err != nil {
			return err
		}
		if err := u.repo.SaveStatusHistory(ctx, entity.ID, nil, domain.StatusActive, domain.ReasonCreated, now); err != nil {
			return err
		}
		if cmd.IdempotencyKey != nil {
			if err := u.repo.SaveIdempotency(ctx, domain.IdempotencyRecord{
				Key:         *cmd.IdempotencyKey,
				RequestHash: requestHash,
				TripID:      entity.ID,
				ExpiresAt:   now.Add(u.ttl),
			}); err != nil {
				return err
			}
		}
		created = entity
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("create trip: %w", err)
	}
	if !replay {
		u.metrics.TripCreated(ctx)
		u.log.InfoContext(ctx, "trip created", "trip_id", created.ID.String())
	}
	return Result{Trip: created, Replay: replay}, nil
}

func hashCommand(cmd Command) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%s|%x|%x|%x|%x|%d",
		cmd.UserID, cmd.DriverID,
		math.Float64bits(cmd.StartLat),
		math.Float64bits(cmd.StartLon),
		math.Float64bits(cmd.EndLat),
		math.Float64bits(cmd.EndLon),
		cmd.Price,
	)))
	return hex.EncodeToString(sum[:])
}
