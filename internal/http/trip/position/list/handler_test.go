package list_test

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
	"github.com/fumoid/x-course-avito/internal/http/trip/position/list"
)

func TestListPositionsSuccessSorted(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 1, 10, 1, 0, 0, time.UTC)
	uc.EXPECT().Execute(gomock.Any(), tripID).Return([]domain.Position{
		{ID: 1, TripID: tripID, Latitude: 59.93, Longitude: 30.33, RecordedAt: t1},
		{ID: 2, TripID: tripID, Latitude: 59.94, Longitude: 30.34, RecordedAt: t2},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)
	rec := httptest.NewRecorder()
	list.New(uc, log).ListTripPositions(rec, req, tripID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body api.TripPositionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Positions) != 2 {
		t.Fatalf("len = %d", len(body.Positions))
	}
	if body.Positions[0].Id != 1 || body.Positions[1].Id != 2 {
		t.Fatalf("order = %+v", body.Positions)
	}
	if body.Positions[0].RecordedAt.After(body.Positions[1].RecordedAt) {
		t.Fatal("positions are not sorted by recorded_at")
	}
}

func TestListPositionsEmpty(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), tripID).Return([]domain.Position{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)
	rec := httptest.NewRecorder()
	list.New(uc, log).ListTripPositions(rec, req, tripID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"positions":[]}` {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestListPositionsNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), tripID).Return(nil, domain.ErrTripNotFound)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)
	rec := httptest.NewRecorder()
	list.New(uc, log).ListTripPositions(rec, req, tripID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "trip_not_found" {
		t.Fatalf("code = %s", body.Code)
	}
}

func TestListPositionsInternalHidesDetails(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), tripID).Return(nil, errors.New("sql: boom"))
	log.EXPECT().ErrorContext(gomock.Any(), "internal error", "error", gomock.Any(), "path", gomock.Any())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)
	rec := httptest.NewRecorder()
	list.New(uc, log).ListTripPositions(rec, req, tripID)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sql:") {
		t.Fatalf("leaked: %s", rec.Body.String())
	}
}
