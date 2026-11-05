package finish_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/usecase/trip/finish"
)

func TestFinishActiveSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	id := uuid.New()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	active := domain.Trip{
		ID:        id,
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		Status:    domain.StatusActive,
		StartedAt: now.Add(-time.Minute),
		Price:     100,
	}

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
	repo.EXPECT().Get(gomock.Any(), id).Return(active, nil)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().UpdateFinished(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, entity domain.Trip) error {
			if entity.Status != domain.StatusCompleted {
				t.Fatalf("status = %s", entity.Status)
			}
			if entity.FinishedAt == nil || !entity.FinishedAt.Equal(now) {
				t.Fatalf("finished_at = %v", entity.FinishedAt)
			}
			return nil
		},
	)
	from := domain.StatusActive
	repo.EXPECT().SaveStatusHistory(gomock.Any(), id, &from, domain.StatusCompleted, domain.ReasonFinished, now).Return(nil)
	metrics.EXPECT().TripCompleted(gomock.Any())
	log.EXPECT().InfoContext(gomock.Any(), "trip completed", "trip_id", id.String())

	uc := finish.New(repo, tx, clk, log, metrics)
	got, err := uc.Execute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusCompleted {
		t.Fatalf("status = %s", got.Status)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(now) {
		t.Fatalf("finished_at = %v want %v", got.FinishedAt, now)
	}
}

func TestFinishCompleted(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	id := uuid.New()
	doneAt := time.Date(2026, 11, 5, 11, 0, 0, 0, time.UTC)
	completed := domain.Trip{ID: id, Status: domain.StatusCompleted, FinishedAt: &doneAt}

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
	repo.EXPECT().Get(gomock.Any(), id).Return(completed, nil)
	clk.EXPECT().Now().Return(doneAt.Add(time.Minute))

	uc := finish.New(repo, tx, clk, log, metrics)
	_, err := uc.Execute(context.Background(), id)
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("got %v", err)
	}
}

func TestFinishNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	id := uuid.New()
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
	repo.EXPECT().Get(gomock.Any(), id).Return(domain.Trip{}, domain.ErrTripNotFound)

	uc := finish.New(repo, tx, clk, log, metrics)
	_, err := uc.Execute(context.Background(), id)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("got %v", err)
	}
}
