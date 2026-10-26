package trip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	api "github.com/fumoid/x-course-avito/internal/generated"
)

type Service struct {
	txm   TxManager
	repo  *Repository
	idTTL time.Duration
}

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

func NewService(txm TxManager, repo *Repository, idTTL time.Duration) *Service {
	return &Service{txm: txm, repo: repo, idTTL: idTTL}
}

func (s *Service) Create(ctx context.Context, in CreateInput, idempotencyKey *uuid.UUID) (api.Trip, bool, error) {
	if err := in.validate(); err != nil {
		return api.Trip{}, false, err
	}
	if idempotencyKey != nil && *idempotencyKey == uuid.Nil {
		return api.Trip{}, false, ErrInvalidRequest
	}
	requestHash := hashCreateInput(in)

	var created Trip
	var replay bool

	err := s.txm.Do(ctx, func(ctx context.Context) error {
		if idempotencyKey != nil {
			if err := s.repo.LockIdempotencyKey(ctx, *idempotencyKey); err != nil {
				return err
			}
			rec, err := s.repo.FindIdempotency(ctx, *idempotencyKey, time.Now().UTC())
			if err != nil {
				return err
			}
			if rec != nil {
				if rec.RequestHash != requestHash {
					return ErrIdempotencyConflict
				}
				existing, err := s.repo.Get(ctx, rec.TripID)
				if err != nil {
					return err
				}
				created = existing
				replay = true
				return nil
			}
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
		created = Trip{
			ID:        uuid.New(),
			UserID:    in.UserID,
			DriverID:  in.DriverID,
			StartLat:  in.StartLat,
			StartLon:  in.StartLon,
			EndLat:    in.EndLat,
			EndLon:    in.EndLon,
			Price:     in.Price,
			Status:    StatusActive,
			StartedAt: now,
		}

		if err := s.repo.Create(ctx, created); err != nil {
			return err
		}
		if err := s.repo.InsertStatusHistory(ctx, created.ID, nil, StatusActive, "created"); err != nil {
			return err
		}
		if idempotencyKey != nil {
			err := s.repo.SaveIdempotency(ctx, IdempotencyRecord{
				Key:         *idempotencyKey,
				RequestHash: requestHash,
				TripID:      created.ID,
				ExpiresAt:   now.Add(s.idTTL),
			})
			if errors.Is(err, errUniqueIdempotency) {
				rec, findErr := s.repo.FindIdempotency(ctx, *idempotencyKey, time.Now().UTC())
				if findErr != nil {
					return findErr
				}
				if rec == nil {
					return ErrIdempotencyConflict
				}
				if rec.RequestHash != requestHash {
					return ErrIdempotencyConflict
				}
				existing, getErr := s.repo.Get(ctx, rec.TripID)
				if getErr != nil {
					return getErr
				}
				created = existing
				replay = true
				return nil
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return api.Trip{}, false, err
	}
	return ToDTO(created), replay, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (api.Trip, error) {
	if id == uuid.Nil {
		return api.Trip{}, ErrInvalidRequest
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return api.Trip{}, err
	}
	return ToDTO(t), nil
}

func (s *Service) Finish(ctx context.Context, id uuid.UUID) (api.Trip, error) {
	if id == uuid.Nil {
		return api.Trip{}, ErrInvalidRequest
	}
	var finished Trip
	err := s.txm.Do(ctx, func(ctx context.Context) error {
		now := time.Now().UTC().Truncate(time.Microsecond)
		t, err := s.repo.FinishActive(ctx, id, now)
		if err != nil {
			return err
		}
		from := StatusActive
		if err := s.repo.InsertStatusHistory(ctx, t.ID, &from, StatusCompleted, "finished"); err != nil {
			return err
		}
		finished = t
		return nil
	})
	if err != nil {
		return api.Trip{}, err
	}
	return ToDTO(finished), nil
}

func hashCreateInput(in CreateInput) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%s|%x|%x|%x|%x|%d",
		in.UserID, in.DriverID,
		math.Float64bits(in.StartLat),
		math.Float64bits(in.StartLon),
		math.Float64bits(in.EndLat),
		math.Float64bits(in.EndLon),
		in.Price,
	)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Ping(ctx context.Context) error {
	if err := s.repo.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}
