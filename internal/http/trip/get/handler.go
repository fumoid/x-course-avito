package get

import (
	"net/http"

	api "github.com/fumoid/x-course-avito/internal/generated"
	"github.com/fumoid/x-course-avito/internal/http/domainerror"
	"github.com/fumoid/x-course-avito/internal/http/respond"
)

type Handler struct {
	uc  useCase
	log logger
}

func New(uc useCase, log logger) *Handler {
	return &Handler{uc: uc, log: log}
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	entity, err := h.uc.Execute(r.Context(), tripID)
	if err != nil {
		domainerror.Write(w, r, err, h.log)
		return
	}
	respond.JSON(w, http.StatusOK, respond.Trip(entity))
}
