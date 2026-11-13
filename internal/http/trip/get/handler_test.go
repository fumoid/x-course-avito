package get_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	api "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/http/trip/get"
)

func TestGetSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	id := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{ID: id, Status: domain.StatusActive, Price: 10}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)
	rec := httptest.NewRecorder()
	get.New(uc, log).GetTrip(rec, req, id)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Id != id {
		t.Fatalf("id = %s", body.Id)
	}
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	id := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{}, domain.ErrTripNotFound)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)
	rec := httptest.NewRecorder()
	get.New(uc, log).GetTrip(rec, req, id)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %s", rec.Header().Get("Content-Type"))
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "trip_not_found" {
		t.Fatalf("code = %s", body.Code)
	}
}

func TestGetInternalHidesDetails(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	id := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{}, errors.New("sql: boom"))
	log.EXPECT().ErrorContext(gomock.Any(), "internal error", "error", gomock.Any(), "path", gomock.Any())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)
	rec := httptest.NewRecorder()
	get.New(uc, log).GetTrip(rec, req, id)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sql:") {
		t.Fatalf("leaked: %s", rec.Body.String())
	}
}
