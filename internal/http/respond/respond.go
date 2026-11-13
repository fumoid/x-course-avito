package respond

import (
	"encoding/json"
	"net/http"
	"time"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	api "github.com/fumoid/x-course-avito/internal/generated"
)

const JSONContentType = "application/json"

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", JSONContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Trip(t domain.Trip) api.Trip {
	started := t.StartedAt.UTC()
	var finished *time.Time
	if t.FinishedAt != nil {
		utc := t.FinishedAt.UTC()
		finished = &utc
	}
	return api.Trip{
		Id:         t.ID,
		UserId:     t.UserID,
		DriverId:   t.DriverID,
		StartPoint: api.Coordinates{Latitude: t.StartLat, Longitude: t.StartLon},
		EndPoint:   api.Coordinates{Latitude: t.EndLat, Longitude: t.EndLon},
		Price:      t.Price,
		Status:     api.TripStatus(t.Status),
		StartedAt:  started,
		FinishedAt: finished,
	}
}
