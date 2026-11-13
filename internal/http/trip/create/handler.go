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

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	cmd, err := decodeCommand(w, r, params)
	if err != nil {
		domainerror.Write(w, r, err, h.log)
		return
	}

	result, err := h.uc.Execute(r.Context(), cmd)
	if err != nil {
		domainerror.Write(w, r, err, h.log)
		return
	}

	status := http.StatusCreated
	if result.Replay {
		status = http.StatusOK
	}
	dto := respond.Trip(result.Trip)
	if status == http.StatusCreated {
		w.Header().Set("Location", "/api/v1/trips/"+dto.Id.String())
	}
	respond.JSON(w, status, dto)
}
