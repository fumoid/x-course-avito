package health

import (
	"context"
	"net/http"
	"time"

	api "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/http/respond"
)

type Handler struct {
	pinger  pinger
	timeout time.Duration
}

func New(pinger pinger, timeout time.Duration) *Handler {
	return &Handler{pinger: pinger, timeout: timeout}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.pinger.Ping(ctx); err != nil {
		respond.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
