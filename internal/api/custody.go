package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/solo-037/internal/model"
	"github.com/wyw14/solo-037/internal/service"
)

type CustodyService interface {
	RegisterShipModel(id, name string, home model.Location) (model.ShipModel, error)
	OfferShipModel(modelID, to, seal, handedOverBy string, run model.RunID) (model.Transfer, error)
	ConfirmShipModelTransfer(model.EventID, string) (model.Transfer, error)
	ShipModels() []service.ShipModelStatus
	ShipModelChain(modelID string) ([]model.Transfer, bool)
}

type registerShipModelRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Home string `json:"home"`
}

type offerShipModelRequest struct {
	To           string `json:"to"`
	Seal         string `json:"seal"`
	HandedOverBy string `json:"handed_over_by"`
	Run          string `json:"run"`
}

type confirmTransferRequest struct {
	ReceivedBy string `json:"received_by"`
}

func custodyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrShipModelNotFound), errors.Is(err, service.ErrTransferNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, service.ErrShipModelExists), errors.Is(err, service.ErrShipModelOccupied), errors.Is(err, service.ErrTransferConfirmed):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (r *Router) shipModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.dependencies.ShipModels())
}

func (r *Router) registerShipModel(w http.ResponseWriter, request *http.Request) {
	var input registerShipModelRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.RegisterShipModel(input.ID, input.Name, model.Location(input.Home))
	if err != nil {
		custodyError(w, err)
		return
	}
	writeJSON(w, value)
}

func (r *Router) shipModelChain(w http.ResponseWriter, request *http.Request) {
	chain, ok := r.dependencies.ShipModelChain(chi.URLParam(request, "id"))
	if !ok {
		http.Error(w, "ship model not found", http.StatusNotFound)
		return
	}
	writeJSON(w, chain)
}

func (r *Router) offerShipModel(w http.ResponseWriter, request *http.Request) {
	var input offerShipModelRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.OfferShipModel(chi.URLParam(request, "id"), input.To, input.Seal, input.HandedOverBy, model.RunID(input.Run))
	if err != nil {
		custodyError(w, err)
		return
	}
	writeJSON(w, value)
}

func (r *Router) confirmTransfer(w http.ResponseWriter, request *http.Request) {
	var input confirmTransferRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.ConfirmShipModelTransfer(model.EventID(chi.URLParam(request, "id")), input.ReceivedBy)
	if err != nil {
		custodyError(w, err)
		return
	}
	writeJSON(w, value)
}
