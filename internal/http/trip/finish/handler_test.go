package finish_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	api "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/http/trip/finish"
)

func TestFinishSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	id := uuid.New()
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	uc.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{
		ID:         id,
		Status:     domain.StatusCompleted,
		FinishedAt: &at,
		Price:      10,
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)
	rec := httptest.NewRecorder()
	finish.New(uc, log).FinishTrip(rec, req, id)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != api.Completed {
		t.Fatalf("status = %s", body.Status)
	}
	if body.FinishedAt == nil || !body.FinishedAt.Equal(at) {
		t.Fatalf("finished_at = %v", body.FinishedAt)
	}
}

func TestFinishCompleted(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	id := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{}, domain.ErrTripCompleted)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)
	rec := httptest.NewRecorder()
	finish.New(uc, log).FinishTrip(rec, req, id)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "trip_completed" {
		t.Fatalf("code = %s", body.Code)
	}
}

func TestFinishNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	id := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{}, domain.ErrTripNotFound)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)
	rec := httptest.NewRecorder()
	finish.New(uc, log).FinishTrip(rec, req, id)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestFinishInternalHidesDetails(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	id := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{}, errors.New("sql: boom"))
	log.EXPECT().ErrorContext(gomock.Any(), "internal error", "error", gomock.Any(), "path", gomock.Any())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)
	rec := httptest.NewRecorder()
	finish.New(uc, log).FinishTrip(rec, req, id)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sql:") {
		t.Fatalf("leaked: %s", rec.Body.String())
	}
}
