package trip_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	trip "github.com/fumoid/x-course-avito/internal/domain/trip"
)

func TestNewValidation(t *testing.T) {
	t.Parallel()

	valid := trip.NewParams{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		StartLat:  59.9,
		StartLon:  30.3,
		EndLat:    59.8,
		EndLon:    30.4,
		Price:     0,
		StartedAt: time.Now().UTC(),
	}

	cases := []struct {
		name    string
		mutate  func(*trip.NewParams)
		field   string
		wantErr bool
	}{
		{name: "ok", wantErr: false},
		{name: "nil id", mutate: func(p *trip.NewParams) { p.ID = uuid.Nil }, field: "id", wantErr: true},
		{name: "nil user", mutate: func(p *trip.NewParams) { p.UserID = uuid.Nil }, field: "user_id", wantErr: true},
		{name: "nil driver", mutate: func(p *trip.NewParams) { p.DriverID = uuid.Nil }, field: "driver_id", wantErr: true},
		{name: "lat too high", mutate: func(p *trip.NewParams) { p.StartLat = 90.1 }, field: "start_point", wantErr: true},
		{name: "lat inclusive 90", mutate: func(p *trip.NewParams) { p.StartLat = 90 }, wantErr: false},
		{name: "lat inclusive -90", mutate: func(p *trip.NewParams) { p.StartLat = -90 }, wantErr: false},
		{name: "lat too low", mutate: func(p *trip.NewParams) { p.StartLat = -90.1 }, field: "start_point", wantErr: true},
		{name: "lon inclusive -180", mutate: func(p *trip.NewParams) { p.StartLon = -180 }, wantErr: false},
		{name: "lon inclusive 180", mutate: func(p *trip.NewParams) { p.StartLon = 180 }, wantErr: false},
		{name: "lon too high", mutate: func(p *trip.NewParams) { p.StartLon = 180.1 }, field: "start_point", wantErr: true},
		{name: "nan lat", mutate: func(p *trip.NewParams) { p.EndLat = math.NaN() }, field: "end_point", wantErr: true},
		{name: "negative price", mutate: func(p *trip.NewParams) { p.Price = -1 }, field: "price", wantErr: true},
		{name: "zero started_at", mutate: func(p *trip.NewParams) { p.StartedAt = time.Time{} }, field: "started_at", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params := valid
			params.ID = uuid.New()
			if tc.mutate != nil {
				tc.mutate(&params)
			}
			got, err := trip.New(params)
			if tc.wantErr {
				var verr trip.ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("got %v, want ValidationError", err)
				}
				if tc.field != "" && verr.Field != tc.field {
					t.Fatalf("field = %q, want %q", verr.Field, tc.field)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != trip.StatusActive {
				t.Fatalf("status = %s", got.Status)
			}
			if got.FinishedAt != nil {
				t.Fatal("new trip must not be finished")
			}
		})
	}
}

func TestFinish(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	active, err := trip.New(trip.NewParams{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		StartLat:  0,
		StartLon:  0,
		EndLat:    1,
		EndLon:    1,
		Price:     10,
		StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	at := now.Add(time.Minute)
	if err := active.Finish(at); err != nil {
		t.Fatal(err)
	}
	if active.Status != trip.StatusCompleted {
		t.Fatalf("status = %s", active.Status)
	}
	if active.FinishedAt == nil || !active.FinishedAt.Equal(at) {
		t.Fatalf("finished_at = %v, want %v", active.FinishedAt, at)
	}

	if err := active.Finish(at.Add(time.Second)); !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("second finish: %v", err)
	}
	if !active.FinishedAt.Equal(at) {
		t.Fatal("second finish must not change finished_at")
	}
}

func TestValidationErrorMessage(t *testing.T) {
	t.Parallel()
	err := trip.NewValidationError("price", "must be greater than or equal to zero")
	if err.Error() != "price: must be greater than or equal to zero" {
		t.Fatalf("error = %q", err.Error())
	}
}
