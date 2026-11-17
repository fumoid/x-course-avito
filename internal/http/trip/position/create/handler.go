package create

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

func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	cmd, err := decodeCommand(w, r, tripID)
	if err != nil {
		domainerror.Write(w, r, err, h.log)
		return
	}

	position, err := h.uc.Execute(r.Context(), cmd)
	if err != nil {
		domainerror.Write(w, r, err, h.log)
		return
	}
	respond.JSON(w, http.StatusCreated, respond.Position(position))
}
