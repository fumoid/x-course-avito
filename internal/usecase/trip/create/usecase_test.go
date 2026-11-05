package create_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/usecase/trip/create"
)

var errRepo = errors.New("db down")

func validCommand() create.Command {
	return create.Command{
		UserID:   uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"),
		DriverID: uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5"),
		StartLat: 59.9,
		StartLon: 30.3,
		EndLat:   59.8,
		EndLon:   30.4,
		Price:    1450,
	}
}

func expectDo(tx *MocktxManager) {
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
}

func TestCreateSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	tripID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	cmd := validCommand()

	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	ids.EXPECT().New().Return(tripID)
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, entity domain.Trip) error {
			if entity.ID != tripID || entity.Status != domain.StatusActive {
				t.Fatalf("unexpected trip: %+v", entity)
			}
			return nil
		},
	)
	repo.EXPECT().SaveStatusHistory(gomock.Any(), tripID, (*domain.Status)(nil), domain.StatusActive, domain.ReasonCreated, now).Return(nil)
	metrics.EXPECT().TripCreated(gomock.Any())
	log.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", tripID.String())

	uc := create.New(repo, tx, clk, ids, log, metrics, time.Hour)
	result, err := uc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replay {
		t.Fatal("expected new trip")
	}
	if result.Trip.ID != tripID {
		t.Fatalf("id = %s", result.Trip.ID)
	}
}

func TestCreateDriverBusyDoesNotWriteHistory(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	ids.EXPECT().New().Return(uuid.New())
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(domain.ErrDriverBusy)

	uc := create.New(repo, tx, clk, ids, log, metrics, time.Hour)
	_, err := uc.Execute(context.Background(), validCommand())
	if !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateTxBeginFails(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(errRepo)

	uc := create.New(repo, tx, clk, ids, log, metrics, time.Hour)
	_, err := uc.Execute(context.Background(), validCommand())
	if !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateRepoError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	expectDo(tx)
	clk.EXPECT().Now().Return(time.Now().UTC())
	ids.EXPECT().New().Return(uuid.New())
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(errRepo)

	uc := create.New(repo, tx, clk, ids, log, metrics, time.Hour)
	_, err := uc.Execute(context.Background(), validCommand())
	if !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	expectDo(tx)
	clk.EXPECT().Now().Return(time.Now().UTC())
	ids.EXPECT().New().Return(uuid.New())

	cmd := validCommand()
	cmd.UserID = uuid.Nil
	uc := create.New(repo, tx, clk, ids, log, metrics, time.Hour)
	_, err := uc.Execute(context.Background(), cmd)
	var verr domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateIdempotencyFirstSave(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	key := uuid.New()
	tripID := uuid.New()
	cmd := validCommand()
	cmd.IdempotencyKey = &key

	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().LockIdempotencyKey(gomock.Any(), key).Return(nil)
	repo.EXPECT().DeleteExpiredIdempotency(gomock.Any(), key, now).Return(nil)
	repo.EXPECT().FindIdempotency(gomock.Any(), key, now).Return(nil, nil)
	ids.EXPECT().New().Return(tripID)
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	repo.EXPECT().SaveStatusHistory(gomock.Any(), tripID, (*domain.Status)(nil), domain.StatusActive, domain.ReasonCreated, now).Return(nil)
	repo.EXPECT().SaveIdempotency(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, rec domain.IdempotencyRecord) error {
			if rec.Key != key || rec.TripID != tripID {
				t.Fatalf("idempotency record = %+v", rec)
			}
			return nil
		},
	)
	metrics.EXPECT().TripCreated(gomock.Any())
	log.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", tripID.String())

	uc := create.New(repo, tx, clk, ids, log, metrics, 24*time.Hour)
	result, err := uc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replay {
		t.Fatal("expected new trip")
	}
}

func TestCreateIdempotencyReplay(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	key := uuid.MustParse("7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1")
	existingID := uuid.New()
	cmd := validCommand()
	cmd.IdempotencyKey = &key
	existing := domain.Trip{ID: existingID, Status: domain.StatusActive}

	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().LockIdempotencyKey(gomock.Any(), key).Return(nil)
	repo.EXPECT().DeleteExpiredIdempotency(gomock.Any(), key, now).Return(nil)
	repo.EXPECT().FindIdempotency(gomock.Any(), key, now).Return(&domain.IdempotencyRecord{
		Key:         key,
		RequestHash: commandHash(cmd),
		TripID:      existingID,
		ExpiresAt:   now.Add(time.Hour),
	}, nil)
	repo.EXPECT().Get(gomock.Any(), existingID).Return(existing, nil)

	uc := create.New(repo, tx, clk, ids, log, metrics, time.Hour)
	result, err := uc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replay || result.Trip.ID != existingID {
		t.Fatalf("result = %+v", result)
	}
}

func TestCreateIdempotencyConflict(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	key := uuid.New()
	cmd := validCommand()
	cmd.IdempotencyKey = &key

	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().LockIdempotencyKey(gomock.Any(), key).Return(nil)
	repo.EXPECT().DeleteExpiredIdempotency(gomock.Any(), key, now).Return(nil)
	repo.EXPECT().FindIdempotency(gomock.Any(), key, now).Return(&domain.IdempotencyRecord{
		Key:         key,
		RequestHash: "other",
		TripID:      uuid.New(),
		ExpiresAt:   now.Add(time.Hour),
	}, nil)

	uc := create.New(repo, tx, clk, ids, log, metrics, time.Hour)
	_, err := uc.Execute(context.Background(), cmd)
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("got %v", err)
	}
}

func commandHash(cmd create.Command) string {
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
