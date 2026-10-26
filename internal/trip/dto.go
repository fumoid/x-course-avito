package trip

import (
	"time"

	api "github.com/fumoid/x-course-avito/internal/generated"
)

func ToDTO(t Trip) api.Trip {
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
