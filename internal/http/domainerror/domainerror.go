package domainerror

import (
	"encoding/json"
	"errors"
	"net/http"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	api "github.com/fumoid/x-course-avito/internal/generated"
)

const detailInvalidRequest = "Request validation failed"

type problem struct {
	status int
	typ    string
	code   string
	title  string
}

var (
	problemInvalidRequest = problem{
		status: http.StatusBadRequest,
		typ:    "https://tripgo.example/problems/invalid-request",
		code:   "invalid_request",
		title:  "Invalid request",
	}
	problemTripNotFound = problem{
		status: http.StatusNotFound,
		typ:    "https://tripgo.example/problems/trip-not-found",
		code:   "trip_not_found",
		title:  "Trip not found",
	}
	problemTripCompleted = problem{
		status: http.StatusConflict,
		typ:    "https://tripgo.example/problems/trip-completed",
		code:   "trip_completed",
		title:  "Trip completed",
	}
	problemDriverBusy = problem{
		status: http.StatusConflict,
		typ:    "https://tripgo.example/problems/driver-busy",
		code:   "driver_busy",
		title:  "Driver busy",
	}
	problemIdempotencyConflict = problem{
		status: http.StatusConflict,
		typ:    "https://tripgo.example/problems/idempotency-conflict",
		code:   "idempotency_conflict",
		title:  "Idempotency conflict",
	}
	problemInternalError = problem{
		status: http.StatusInternalServerError,
		typ:    "https://tripgo.example/problems/internal-error",
		code:   "internal_error",
		title:  "Internal Server Error",
	}
)

func Map(err error) (int, api.Problem) {
	p, detail := classify(err)
	instance := ""
	body := api.Problem{
		Type:     p.typ,
		Title:    p.title,
		Status:   int32(p.status), //nolint:gosec // HTTP status codes fit in int32
		Code:     p.code,
		Detail:   &detail,
		Instance: &instance,
	}
	return p.status, body
}

func classify(err error) (problem, string) {
	var verr domain.ValidationError
	switch {
	case errors.As(err, &verr):
		detail := detailInvalidRequest
		if verr.Field != "" && verr.Reason != "" {
			detail = verr.Field + ": " + verr.Reason
		} else if verr.Reason != "" {
			detail = verr.Reason
		}
		return problemInvalidRequest, detail
	case errors.Is(err, domain.ErrTripNotFound):
		return problemTripNotFound, "Trip was not found"
	case errors.Is(err, domain.ErrTripCompleted):
		return problemTripCompleted, "Operation is not allowed for a completed trip"
	case errors.Is(err, domain.ErrDriverBusy):
		return problemDriverBusy, "Driver already has an active trip"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return problemIdempotencyConflict, "Idempotency-Key was already used with a different request body"
	default:
		return problemInternalError, "Internal server error"
	}
}

func Write(w http.ResponseWriter, r *http.Request, err error, log logger) {
	status, body := Map(err)
	instance := r.URL.Path
	body.Instance = &instance
	if status >= http.StatusInternalServerError && log != nil {
		log.ErrorContext(r.Context(), "internal error", "error", err, "path", r.URL.Path)
	}
	writeProblem(w, status, body)
}

func WriteInvalidRequest(w http.ResponseWriter, r *http.Request, err error) {
	detail := invalidRequestDetail(err)
	instance := r.URL.Path
	body := api.Problem{
		Type:     problemInvalidRequest.typ,
		Title:    problemInvalidRequest.title,
		Status:   int32(problemInvalidRequest.status), //nolint:gosec // HTTP status codes fit in int32
		Code:     problemInvalidRequest.code,
		Detail:   &detail,
		Instance: &instance,
	}
	writeProblem(w, problemInvalidRequest.status, body)
}

func WriteInternal(w http.ResponseWriter, r *http.Request) {
	detail := "Internal server error"
	instance := r.URL.Path
	body := api.Problem{
		Type:     problemInternalError.typ,
		Title:    problemInternalError.title,
		Status:   int32(problemInternalError.status), //nolint:gosec // HTTP status codes fit in int32
		Code:     problemInternalError.code,
		Detail:   &detail,
		Instance: &instance,
	}
	writeProblem(w, problemInternalError.status, body)
}

func invalidRequestDetail(err error) string {
	if err == nil {
		return detailInvalidRequest
	}
	var format *api.InvalidParamFormatError
	if errors.As(err, &format) && format.ParamName != "" {
		return format.ParamName + ": invalid format"
	}
	return detailInvalidRequest
}

func writeProblem(w http.ResponseWriter, status int, body api.Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
