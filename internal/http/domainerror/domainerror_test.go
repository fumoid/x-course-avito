package domainerror_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	api "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/http/domainerror"
)

func TestMap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "validation", err: domain.NewValidationError("price", "must be greater than or equal to zero"), status: http.StatusBadRequest, code: "invalid_request"},
		{name: "not found", err: domain.ErrTripNotFound, status: http.StatusNotFound, code: "trip_not_found"},
		{name: "completed", err: domain.ErrTripCompleted, status: http.StatusConflict, code: "trip_completed"},
		{name: "busy", err: domain.ErrDriverBusy, status: http.StatusConflict, code: "driver_busy"},
		{name: "idempotency", err: domain.ErrIdempotencyConflict, status: http.StatusConflict, code: "idempotency_conflict"},
		{name: "wrapped not found", err: errors.Join(errors.New("get trip"), domain.ErrTripNotFound), status: http.StatusNotFound, code: "trip_not_found"},
		{name: "internal", err: errors.New("sql: connection refused"), status: http.StatusInternalServerError, code: "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, body := domainerror.Map(tc.err)
			if status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
			if body.Code != tc.code {
				t.Fatalf("code = %s, want %s", body.Code, tc.code)
			}
			if tc.code == "internal_error" && body.Detail != nil && strings.Contains(*body.Detail, "sql:") {
				t.Fatalf("internal detail leaked: %s", *body.Detail)
			}
		})
	}
}

func TestWriteInvalidRequestParamName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		path   string
		err    error
		detail string
	}{
		{
			name:   "trip id",
			path:   "/api/v1/trips/abc",
			err:    &api.InvalidParamFormatError{ParamName: "tripId", Err: errors.New("invalid UUID length: 3")},
			detail: "tripId: invalid format",
		},
		{
			name:   "idempotency key",
			path:   "/api/v1/trips",
			err:    &api.InvalidParamFormatError{ParamName: "Idempotency-Key", Err: errors.New("invalid UUID length: 9")},
			detail: "Idempotency-Key: invalid format",
		},
		{
			name:   "non uuid param",
			path:   "/api/v1/trips",
			err:    &api.InvalidParamFormatError{ParamName: "limit", Err: errors.New("strconv.Atoi: parsing \"x\": invalid syntax")},
			detail: "limit: invalid format",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			rec := httptest.NewRecorder()
			domainerror.WriteInvalidRequest(rec, req, tc.err)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", rec.Code)
			}
			var body api.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != "invalid_request" {
				t.Fatalf("code = %s", body.Code)
			}
			if body.Detail == nil || *body.Detail != tc.detail {
				t.Fatalf("detail = %v", body.Detail)
			}
			if strings.Contains(rec.Body.String(), "*uuid.UUID") {
				t.Fatal("leaked Go type")
			}
		})
	}
}

func TestWriteHidesInternalError(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/x", nil)
	rec := httptest.NewRecorder()
	log := &captureLogger{}
	domainerror.Write(rec, req, errors.New("pq: password authentication failed"), log)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %s", rec.Header().Get("Content-Type"))
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "internal_error" {
		t.Fatalf("code = %s", body.Code)
	}
	if body.Detail != nil && strings.Contains(*body.Detail, "password") {
		t.Fatalf("leaked detail: %s", *body.Detail)
	}
	if log.msg == "" {
		t.Fatal("internal error must be logged")
	}
}

type captureLogger struct {
	msg string
}

func (c *captureLogger) ErrorContext(_ context.Context, msg string, _ ...any) {
	c.msg = msg
}
