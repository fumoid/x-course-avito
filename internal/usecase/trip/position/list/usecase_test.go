package list_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/usecase/trip/position/list"
)

func TestListPositionsSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tripID := uuid.New()
	want := []domain.Position{
		{ID: 1, TripID: tripID, Latitude: 1, Longitude: 2, RecordedAt: time.Unix(1, 0).UTC()},
		{ID: 2, TripID: tripID, Latitude: 3, Longitude: 4, RecordedAt: time.Unix(2, 0).UTC()},
	}
	repo.EXPECT().Get(gomock.Any(), tripID).Return(domain.Trip{ID: tripID, Status: domain.StatusActive}, nil)
	repo.EXPECT().ListPositions(gomock.Any(), tripID).Return(want, nil)

	got, err := list.New(repo).Execute(context.Background(), tripID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestListPositionsEmpty(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tripID := uuid.New()
	repo.EXPECT().Get(gomock.Any(), tripID).Return(domain.Trip{ID: tripID}, nil)
	repo.EXPECT().ListPositions(gomock.Any(), tripID).Return([]domain.Position{}, nil)

	got, err := list.New(repo).Execute(context.Background(), tripID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestListPositionsNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tripID := uuid.New()
	repo.EXPECT().Get(gomock.Any(), tripID).Return(domain.Trip{}, domain.ErrTripNotFound)

	_, err := list.New(repo).Execute(context.Background(), tripID)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestListPositionsNilID(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	_, err := list.New(repo).Execute(context.Background(), uuid.Nil)
	var verr domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %v", err)
	}
}
