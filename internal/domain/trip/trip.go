package trip

import (
	"math"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"

	ReasonCreated  = "created"
	ReasonFinished = "finished"
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
	Status     Status
	StartedAt  time.Time
	FinishedAt *time.Time
}

type NewParams struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	DriverID  uuid.UUID
	StartLat  float64
	StartLon  float64
	EndLat    float64
	EndLon    float64
	Price     int64
	StartedAt time.Time
}

func New(params NewParams) (Trip, error) {
	if params.ID == uuid.Nil {
		return Trip{}, NewValidationError("id", "must be a non-zero UUID")
	}
	if params.UserID == uuid.Nil {
		return Trip{}, NewValidationError("user_id", "must be a non-zero UUID")
	}
	if params.DriverID == uuid.Nil {
		return Trip{}, NewValidationError("driver_id", "must be a non-zero UUID")
	}
	if err := validateCoordinate("start_point", params.StartLat, params.StartLon); err != nil {
		return Trip{}, err
	}
	if err := validateCoordinate("end_point", params.EndLat, params.EndLon); err != nil {
		return Trip{}, err
	}
	if params.Price < 0 {
		return Trip{}, NewValidationError("price", "must be greater than or equal to zero")
	}
	if params.StartedAt.IsZero() {
		return Trip{}, NewValidationError("started_at", "must be set")
	}

	return Trip{
		ID:        params.ID,
		UserID:    params.UserID,
		DriverID:  params.DriverID,
		StartLat:  params.StartLat,
		StartLon:  params.StartLon,
		EndLat:    params.EndLat,
		EndLon:    params.EndLon,
		Price:     params.Price,
		Status:    StatusActive,
		StartedAt: params.StartedAt,
	}, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}
	finished := at
	t.Status = StatusCompleted
	t.FinishedAt = &finished
	return nil
}

func validateCoordinate(field string, lat, lon float64) error {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		return NewValidationError(field, "latitude must be between -90 and 90")
	}
	if math.IsNaN(lon) || math.IsInf(lon, 0) || lon < -180 || lon > 180 {
		return NewValidationError(field, "longitude must be between -180 and 180")
	}
	return nil
}

type IdempotencyRecord struct {
	Key         uuid.UUID
	RequestHash string
	TripID      uuid.UUID
	ExpiresAt   time.Time
}
