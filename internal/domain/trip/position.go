package trip

import (
	"time"

	"github.com/google/uuid"
)

const maxRecordedAtSkew = time.Minute

type Position struct {
	ID         int64
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

type NewPositionParams struct {
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
	Now        time.Time
}

func NewPosition(params NewPositionParams) (Position, error) {
	if params.TripID == uuid.Nil {
		return Position{}, NewValidationError("tripId", "must be a non-zero UUID")
	}
	if err := validateCoordinate("coordinates", params.Latitude, params.Longitude); err != nil {
		return Position{}, err
	}
	if params.RecordedAt.IsZero() {
		return Position{}, NewValidationError("recorded_at", "must be set")
	}
	if params.RecordedAt.After(params.Now.Add(maxRecordedAtSkew)) {
		return Position{}, NewValidationError("recorded_at", "must not be more than one minute in the future")
	}

	return Position{
		TripID:     params.TripID,
		Latitude:   params.Latitude,
		Longitude:  params.Longitude,
		RecordedAt: params.RecordedAt,
	}, nil
}

type StaleTrip struct {
	ID          uuid.UUID
	DriverID    uuid.UUID
	SilentSince time.Time
}

type StaleCursor struct {
	SilentSince time.Time
	ID          uuid.UUID
}
