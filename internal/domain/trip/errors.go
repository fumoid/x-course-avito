package trip

import "errors"

var (
	ErrTripNotFound        = errors.New("trip not found")
	ErrTripCompleted       = errors.New("trip already completed")
	ErrDriverBusy          = errors.New("driver already has an active trip")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with a different request body")
)

// ValidationError describes a failed input check: which field and why.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Reason
	}
	if e.Reason == "" {
		return e.Field
	}
	return e.Field + ": " + e.Reason
}

func NewValidationError(field, reason string) ValidationError {
	return ValidationError{Field: field, Reason: reason}
}
