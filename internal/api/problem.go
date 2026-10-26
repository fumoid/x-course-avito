package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	spec "github.com/fumoid/x-course-avito/internal/generated"
)

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

func writeProblem(w http.ResponseWriter, r *http.Request, p problem, detail string) {
	instance := r.URL.Path
	body := spec.Problem{
		Type:     p.typ,
		Title:    p.title,
		Status:   int32(p.status),
		Code:     p.code,
		Detail:   &detail,
		Instance: &instance,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write problem", "error", err)
	}
}
