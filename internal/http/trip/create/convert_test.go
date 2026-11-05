package create_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	api "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/http/trip/create"
)

func TestCreateRejectsInvalidBodies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		body        string
		contentType string
	}{
		{name: "missing price", body: `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4}}`, contentType: "application/json"},
		{name: "null price", body: `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":null}`, contentType: "application/json"},
		{name: "unknown field", body: `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1,"Price":2}`, contentType: "application/json"},
		{name: "trailing json", body: validCreateBody + `{}`, contentType: "application/json"},
		{name: "invalid user uuid", body: `{"user_id":"not-a-uuid","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`, contentType: "application/json"},
		{name: "missing content type", body: validCreateBody, contentType: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			uc := NewMockuseCase(ctrl)
			log := NewMocklogger(ctrl)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()
			create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})
			assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
		})
	}
}

func TestCreateInvalidUUIDDetail(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	log := NewMocklogger(ctrl)
	body := `{"user_id":"not-a-uuid","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	create.New(uc, log).CreateTrip(rec, req, api.CreateTripParams{})
	assertProblem(t, rec, http.StatusBadRequest, "invalid_request")
	if !strings.Contains(rec.Body.String(), "user_id: must be a UUID") {
		t.Fatalf("detail = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "malformed JSON") {
		t.Fatal("uuid error mapped as malformed JSON")
	}
	if strings.Contains(rec.Body.String(), "tripId") {
		t.Fatal("body uuid error mapped as tripId")
	}
}
