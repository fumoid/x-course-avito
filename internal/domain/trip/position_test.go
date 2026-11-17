package trip_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	trip "github.com/fumoid/x-course-avito/internal/domain/trip"
)

func TestNewPosition(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	valid := trip.NewPositionParams{
		TripID:     uuid.New(),
		Latitude:   59.93,
		Longitude:  30.33,
		RecordedAt: now.Add(-time.Second),
		Now:        now,
	}

	cases := []struct {
		name    string
		mutate  func(*trip.NewPositionParams)
		field   string
		wantErr bool
	}{
		{name: "ok", wantErr: false},
		{name: "nil trip", mutate: func(p *trip.NewPositionParams) { p.TripID = uuid.Nil }, field: "tripId", wantErr: true},
		{name: "lat too high", mutate: func(p *trip.NewPositionParams) { p.Latitude = 90.1 }, field: "coordinates", wantErr: true},
		{name: "lon too low", mutate: func(p *trip.NewPositionParams) { p.Longitude = -180.1 }, field: "coordinates", wantErr: true},
		{name: "nan lon", mutate: func(p *trip.NewPositionParams) { p.Longitude = math.NaN() }, field: "coordinates", wantErr: true},
		{name: "zero recorded_at", mutate: func(p *trip.NewPositionParams) { p.RecordedAt = time.Time{} }, field: "recorded_at", wantErr: true},
		{name: "future over minute", mutate: func(p *trip.NewPositionParams) { p.RecordedAt = now.Add(time.Minute + time.Second) }, field: "recorded_at", wantErr: true},
		{name: "future within minute", mutate: func(p *trip.NewPositionParams) { p.RecordedAt = now.Add(time.Minute) }, wantErr: false},
		{name: "old recorded_at", mutate: func(p *trip.NewPositionParams) { p.RecordedAt = now.Add(-time.Hour) }, wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params := valid
			params.TripID = uuid.New()
			if tc.mutate != nil {
				tc.mutate(&params)
			}
			got, err := trip.NewPosition(params)
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
			if got.TripID != params.TripID {
				t.Fatalf("trip_id = %s", got.TripID)
			}
		})
	}
}
