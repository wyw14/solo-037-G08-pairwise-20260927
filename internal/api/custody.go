package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/solo-037/internal/model"
	"github.com/wyw14/solo-037/internal/service"
)

type CustodyService interface {
	RegisterShipModel(entityNo, name, note string) (model.ShipModel, error)
	ShipModelStatuses() []service.ShipModelStatus
	OfferHandover(entityNo string, to model.Location, packaging model.PackagingState, handedOverBy string, run model.RunID) (model.Handover, error)
	ConfirmHandover(id model.EventID, receivedBy string) (model.Handover, error)
	HandoverChain(entityNo string) ([]model.Handover, error)
}

type registerShipModelRequest struct {
	EntityNo string `json:"entity_no"`
	Name     string `json:"name"`
	Note     string `json:"note"`
}

type offerHandoverRequest struct {
	To           model.Location       `json:"to"`
	Packaging    model.PackagingState `json:"packaging"`
	HandedOverBy string               `json:"handed_over_by"`
	Run          model.RunID          `json:"run"`
}

type confirmHandoverRequest struct {
	ReceivedBy string `json:"received_by"`
}

// writeServiceError maps service-layer sentinel errors to HTTP statuses:
// occupancy conflicts are 409, unknown entities 404, everything else 400.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrModelOccupied):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, service.ErrShipModelNotFound),
		errors.Is(err, service.ErrHandoverNotFound),
		errors.Is(err, service.ErrRunNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (r *Router) shipModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.dependencies.ShipModelStatuses())
}

func (r *Router) registerShipModel(w http.ResponseWriter, request *http.Request) {
	var input registerShipModelRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.RegisterShipModel(input.EntityNo, input.Name, input.Note)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, value)
}

func (r *Router) handoverChain(w http.ResponseWriter, request *http.Request) {
	chain, err := r.dependencies.HandoverChain(chi.URLParam(request, "entity"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, chain)
}

func (r *Router) offerHandover(w http.ResponseWriter, request *http.Request) {
	var input offerHandoverRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.OfferHandover(chi.URLParam(request, "entity"), input.To, input.Packaging, input.HandedOverBy, input.Run)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, value)
}

func (r *Router) confirmHandover(w http.ResponseWriter, request *http.Request) {
	var input confirmHandoverRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.ConfirmHandover(model.EventID(chi.URLParam(request, "id")), input.ReceivedBy)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, value)
}
