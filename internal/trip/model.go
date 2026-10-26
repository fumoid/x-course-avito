package trip

import (
	"math"
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
)

type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartLat   float64
	StartLon   float64
	EndLat     float64
	EndLon     float64
	Price      int64
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

type CreateInput struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	StartLat float64
	StartLon float64
	EndLat   float64
	EndLon   float64
	Price    int64
}

func (in CreateInput) validate() error {
	if in.UserID == uuid.Nil || in.DriverID == uuid.Nil {
		return ErrInvalidRequest
	}
	if invalidCoordinate(in.StartLat, in.StartLon) || invalidCoordinate(in.EndLat, in.EndLon) {
		return ErrInvalidRequest
	}
	if in.Price < 0 {
		return ErrInvalidRequest
	}
	return nil
}

func invalidCoordinate(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lon) || math.IsInf(lon, 0) {
		return true
	}
	return lat < -90 || lat > 90 || lon < -180 || lon > 180
}

type IdempotencyRecord struct {
	Key         uuid.UUID
	RequestHash string
	TripID      uuid.UUID
	ExpiresAt   time.Time
}
