package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/usecase/trip/get"
)

func TestGetSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	id := uuid.New()
	want := domain.Trip{ID: id, Status: domain.StatusActive}
	repo.EXPECT().Get(gomock.Any(), id).Return(want, nil)

	got, err := get.New(repo).Execute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id {
		t.Fatalf("id = %s", got.ID)
	}
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	id := uuid.New()
	repo.EXPECT().Get(gomock.Any(), id).Return(domain.Trip{}, domain.ErrTripNotFound)

	_, err := get.New(repo).Execute(context.Background(), id)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestGetNilID(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	_, err := get.New(repo).Execute(context.Background(), uuid.Nil)
	var verr domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %v", err)
	}
}
