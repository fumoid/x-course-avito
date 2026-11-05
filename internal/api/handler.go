package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	spec "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/trip"
)

type Handler struct {
	svc          *trip.Service
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewHandler(svc *trip.Service, pool *pgxpool.Pool, queryTimeout time.Duration) *Handler {
	return &Handler{svc: svc, pool: pool, queryTimeout: queryTimeout}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params spec.CreateTripParams) {
	body, err := decodeCreateTripBody(w, r)
	if err != nil {
		writeProblem(w, r, problemInvalidRequest, err.Error())
		return
	}
	in := trip.CreateInput{
		UserID:   uuid.UUID(body.UserId),
		DriverID: uuid.UUID(body.DriverId),
		StartLat: body.StartPoint.Latitude,
		StartLon: body.StartPoint.Longitude,
		EndLat:   body.EndPoint.Latitude,
		EndLon:   body.EndPoint.Longitude,
		Price:    body.Price,
	}

	var key *uuid.UUID
	if params.IdempotencyKey != nil {
		id := uuid.UUID(*params.IdempotencyKey)
		key = &id
	}

	created, replay, err := h.svc.Create(r.Context(), in, key)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	if status == http.StatusCreated {
		w.Header().Set("Location", "/api/v1/trips/"+created.Id.String())
	}
	writeJSON(w, status, created)
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId spec.TripId) {
	t, err := h.svc.Get(r.Context(), uuid.UUID(tripId))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId spec.TripId) {
	t, err := h.svc.Finish(r.Context(), uuid.UUID(tripId))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, spec.HealthResponse{Status: spec.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, spec.HealthResponse{Status: spec.Unavailable})
		return
	}
	writeJSON(w, http.StatusOK, spec.HealthResponse{Status: spec.Ok})
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trip.ErrInvalidRequest):
		writeProblem(w, r, problemInvalidRequest, "Request validation failed")
	case errors.Is(err, trip.ErrTripNotFound):
		writeProblem(w, r, problemTripNotFound, "Trip was not found")
	case errors.Is(err, trip.ErrTripCompleted):
		writeProblem(w, r, problemTripCompleted, "Operation is not allowed for a completed trip")
	case errors.Is(err, trip.ErrDriverBusy):
		writeProblem(w, r, problemDriverBusy, "Driver already has an active trip")
	case errors.Is(err, trip.ErrIdempotencyConflict):
		writeProblem(w, r, problemIdempotencyConflict, "Idempotency-Key was already used with a different request body")
	default:
		slog.Error("internal error", "error", err, "path", r.URL.Path)
		writeProblem(w, r, problemInternalError, "Internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "error", err)
	}
}

func InvalidParam(w http.ResponseWriter, r *http.Request, err error) {
	detail := "Request validation failed"
	if err != nil {
		detail = err.Error()
	}
	writeProblem(w, r, problemInvalidRequest, detail)
}
