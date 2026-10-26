package trip

import "errors"

var (
	ErrInvalidRequest      = errors.New("invalid_request")
	ErrTripNotFound        = errors.New("trip_not_found")
	ErrTripCompleted       = errors.New("trip_completed")
	ErrDriverBusy          = errors.New("driver_busy")
	ErrIdempotencyConflict = errors.New("idempotency_conflict")
)
