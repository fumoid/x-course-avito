package create_test

import (
	"bytes"
	"context"
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
	"github.com/fumoid/x-course-avito/internal/http/trip/create"
	usecase "github.com/fumoid/x-course-avito/internal/usecase/trip/create"
)

const validCreateBody = `{
	"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
	"driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
	"start_point":{"latitude":59.9398,"longitude":30.3146},
	"end_point":{"latitude":59.929,"longitude":30.3626},
	"price":1450
}`

func TestCreateSuccessSetsLocation(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	tripID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(usecase.Result{
		Trip: domain.Trip{ID: tripID, UserID: uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"), DriverID: uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5"), Status: domain.StatusActive},
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validCreateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/v1/trips/"+tripID.String() {
		t.Fatalf("location = %s", rec.Header().Get("Location"))
	}
	var body api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Id != tripID {
		t.Fatalf("id = %s", body.Id)
	}
}

func TestCreateBrokenJSON(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})

	assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestCreateNilUserID(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	body := `{
		"user_id":"00000000-0000-0000-0000-000000000000",
		"driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
		"start_point":{"latitude":59.9398,"longitude":30.3146},
		"end_point":{"latitude":59.929,"longitude":30.3626},
		"price":1450
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})
	assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestCreateNilDriverID(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	body := `{
		"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
		"driver_id":"00000000-0000-0000-0000-000000000000",
		"start_point":{"latitude":59.9398,"longitude":30.3146},
		"end_point":{"latitude":59.929,"longitude":30.3626},
		"price":1450
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})
	assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestCreateDriverBusy(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(usecase.Result{}, domain.ErrDriverBusy)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validCreateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})
	assertProblem(t, rec, http.StatusConflict, "driver_busy")
}

func TestCreateRepoErrorHidesDetails(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(usecase.Result{}, errors.New("pq: boom"))
	log.EXPECT().ErrorContext(gomock.Any(), "internal error", "error", gomock.Any(), "path", gomock.Any())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validCreateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})
	assertProblem(t, rec, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rec.Body.String(), "pq:") {
		t.Fatalf("leaked body: %s", rec.Body.String())
	}
}

func TestCreateZeroPriceAllowed(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, cmd usecase.Command) (usecase.Result, error) {
			if cmd.Price != 0 {
				t.Fatalf("price = %d", cmd.Price)
			}
			return usecase.Result{Trip: domain.Trip{ID: uuid.New(), Status: domain.StatusActive}}, nil
		},
	)
	body := `{
		"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
		"driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
		"start_point":{"latitude":59.9398,"longitude":30.3146},
		"end_point":{"latitude":59.929,"longitude":30.3626},
		"price":0
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
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
