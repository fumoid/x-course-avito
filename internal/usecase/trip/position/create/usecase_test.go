package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/usecase/trip/position/create"
)

func expectDo(tx *MocktxManager) {
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
}

func TestCreatePositionSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	cancel := NewMockcanceller(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	tripID := uuid.New()
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	recorded := now.Add(-time.Second)
	cmd := create.Command{TripID: tripID, Latitude: 59.93, Longitude: 30.33, RecordedAt: recorded}

	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().GetForUpdate(gomock.Any(), tripID).Return(domain.Trip{ID: tripID, Status: domain.StatusActive}, nil)
	repo.EXPECT().SavePosition(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, p domain.Position) (domain.Position, error) {
			p.ID = 42
			return p, nil
		},
	)
	repo.EXPECT().UpdateLastPositionAt(gomock.Any(), tripID, recorded).Return(nil)
	cancel.EXPECT().Cancel(gomock.Any(), tripID)
	metrics.EXPECT().PositionSaved(gomock.Any())
	log.EXPECT().InfoContext(gomock.Any(), "position saved", "trip_id", tripID.String(), "position_id", int64(42))

	got, err := create.New(repo, tx, clk, cancel, log, metrics).Execute(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 42 || got.TripID != tripID {
		t.Fatalf("got %+v", got)
	}
}

func TestCreatePositionNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	cancel := NewMockcanceller(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	tripID := uuid.New()
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().GetForUpdate(gomock.Any(), tripID).Return(domain.Trip{}, domain.ErrTripNotFound)

	_, err := create.New(repo, tx, clk, cancel, log, metrics).Execute(context.Background(), create.Command{
		TripID: tripID, Latitude: 1, Longitude: 2, RecordedAt: now,
	})
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestCreatePositionCompleted(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	cancel := NewMockcanceller(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	tripID := uuid.New()
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().GetForUpdate(gomock.Any(), tripID).Return(domain.Trip{ID: tripID, Status: domain.StatusCompleted}, nil)

	_, err := create.New(repo, tx, clk, cancel, log, metrics).Execute(context.Background(), create.Command{
		TripID: tripID, Latitude: 1, Longitude: 2, RecordedAt: now,
	})
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("got %v", err)
	}
}

func TestCreatePositionFutureRecordedAt(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	cancel := NewMockcanceller(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	clk.EXPECT().Now().Return(now)

	_, err := create.New(repo, tx, clk, cancel, log, metrics).Execute(context.Background(), create.Command{
		TripID: uuid.New(), Latitude: 1, Longitude: 2, RecordedAt: now.Add(2 * time.Minute),
	})
	var verr domain.ValidationError
	if !errors.As(err, &verr) || verr.Field != "recorded_at" {
		t.Fatalf("got %v", err)
	}
}

func TestCreatePositionSaveErrorSkipsCancel(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	clk := NewMockclock(ctrl)
	cancel := NewMockcanceller(ctrl)
	log := NewMocklogger(ctrl)
	metrics := NewMockmetrics(ctrl)

	tripID := uuid.New()
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	expectDo(tx)
	clk.EXPECT().Now().Return(now)
	repo.EXPECT().GetForUpdate(gomock.Any(), tripID).Return(domain.Trip{ID: tripID, Status: domain.StatusActive}, nil)
	repo.EXPECT().SavePosition(gomock.Any(), gomock.Any()).Return(domain.Position{}, errors.New("db down"))

	_, err := create.New(repo, tx, clk, cancel, log, metrics).Execute(context.Background(), create.Command{
		TripID: tripID, Latitude: 1, Longitude: 2, RecordedAt: now,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
