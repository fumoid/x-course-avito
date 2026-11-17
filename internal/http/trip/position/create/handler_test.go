package create_test

import (
	"context"
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
	"github.com/fumoid/x-course-avito/internal/http/trip/position/create"
	usecase "github.com/fumoid/x-course-avito/internal/usecase/trip/position/create"
)

const validPositionBody = `{
	"coordinates":{"latitude":59.9343,"longitude":30.3351},
	"recorded_at":"2026-09-01T10:05:00Z"
}`

func TestCreatePositionSuccess(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	recorded := time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, cmd usecase.Command) (domain.Position, error) {
			if cmd.TripID != tripID {
				t.Fatalf("trip_id = %s", cmd.TripID)
			}
			return domain.Position{
				ID: 42, TripID: tripID, Latitude: 59.9343, Longitude: 30.3351, RecordedAt: recorded,
			}, nil
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(validPositionBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTripPosition(rec, req, tripID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body api.TripPosition
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Id != 42 || body.TripId != tripID {
		t.Fatalf("body = %+v", body)
	}
}

func TestCreatePositionBrokenJSON(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTripPosition(rec, req, tripID)
	assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestCreatePositionInvalidRecordedAt(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	body := `{"coordinates":{"latitude":59.93,"longitude":30.33},"recorded_at":"not-a-date"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTripPosition(rec, req, tripID)
	assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
	if !strings.Contains(rec.Body.String(), "recorded_at") {
		t.Fatalf("detail = %s", rec.Body.String())
	}
}

func TestCreatePositionOutOfRange(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(domain.Position{}, domain.NewValidationError("coordinates", "latitude must be between -90 and 90"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(validPositionBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTripPosition(rec, req, tripID)
	assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestCreatePositionNotFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(domain.Position{}, domain.ErrTripNotFound)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(validPositionBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTripPosition(rec, req, tripID)
	assertProblem(t, rec, http.StatusNotFound, "trip_not_found")
}

func TestCreatePositionCompleted(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(domain.Position{}, domain.ErrTripCompleted)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(validPositionBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTripPosition(rec, req, tripID)
	assertProblem(t, rec, http.StatusConflict, "trip_completed")
}

func TestCreatePositionInternalHidesDetails(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.New()
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(domain.Position{}, errors.New("sql: boom"))
	log.EXPECT().ErrorContext(gomock.Any(), "internal error", "error", gomock.Any(), "path", gomock.Any())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(validPositionBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTripPosition(rec, req, tripID)
	assertProblem(t, rec, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rec.Body.String(), "sql:") {
		t.Fatalf("leaked: %s", rec.Body.String())
	}
}

func TestCreatePositionRejectsInvalidBodies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{name: "missing recorded_at", body: `{"coordinates":{"latitude":1,"longitude":2}}`},
		{name: "null coordinates", body: `{"coordinates":null,"recorded_at":"2026-09-01T10:05:00Z"}`},
		{name: "unknown field", body: `{"coordinates":{"latitude":1,"longitude":2},"recorded_at":"2026-09-01T10:05:00Z","extra":1}`},
		{name: "trailing json", body: validPositionBody + `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			uc := NewMockuseCase(ctrl)
			log := NewMocklogger(ctrl)
			tripID := uuid.New()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			create.New(uc, log).CreateTripPosition(rec, req, tripID)
			assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
		})
	}
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d body = %s", rec.Code, status, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %s", rec.Header().Get("Content-Type"))
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != code {
		t.Fatalf("code = %s, want %s", body.Code, code)
	}
}
