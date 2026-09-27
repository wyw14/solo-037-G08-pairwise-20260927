package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wyw14/solo-037/internal/model"
)

// Sentinel errors let the HTTP layer map ledger rejections to status codes.
var (
	ErrShipModelExists   = errors.New("ship model already registered")
	ErrShipModelNotFound = errors.New("ship model not registered")
	ErrShipModelOccupied = errors.New("ship model has an unconfirmed transfer and cannot be occupied by another test")
	ErrTransferNotFound  = errors.New("transfer not found")
	ErrTransferConfirmed = errors.New("transfer already confirmed")
)

// CustodyLedger tracks physical ship models by entity number and records the
// handover chain between laboratory areas. A model with an unconfirmed
// transfer is in transit and cannot be offered again, so it cannot be
// occupied by another test.
type CustodyLedger struct {
	mu        sync.Mutex
	path      string
	models    map[string]model.ShipModel
	transfers map[string][]model.Transfer
}

type custodySnapshot struct {
	Models    map[string]model.ShipModel  `json:"models"`
	Transfers map[string][]model.Transfer `json:"transfers"`
}

func OpenCustodyLedger(directory string) (*CustodyLedger, error) {
	ledger := &CustodyLedger{models: make(map[string]model.ShipModel), transfers: make(map[string][]model.Transfer)}
	if directory == "" {
		return ledger, nil
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	ledger.path = filepath.Join(directory, "ship-model-custody.json")
	data, err := os.ReadFile(ledger.path)
	if errors.Is(err, os.ErrNotExist) {
		return ledger, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot custodySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("read custody ledger: %w", err)
	}
	if snapshot.Models == nil || snapshot.Transfers == nil {
		return nil, errors.New("custody ledger must contain models and transfers")
	}
	ledger.models = snapshot.Models
	ledger.transfers = snapshot.Transfers
	return ledger, nil
}

func (l *CustodyLedger) commit() error {
	if l.path != "" {
		data, err := json.MarshalIndent(custodySnapshot{Models: l.models, Transfers: l.transfers}, "", "  ")
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(l.path), "custody-*.tmp")
		if err != nil {
			return err
		}
		name := file.Name()
		defer os.Remove(name)
		if _, err = file.Write(data); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err := os.Rename(name, l.path); err != nil {
			return err
		}
	}
	return nil
}

// ShipModelStatus is a registered model plus its derived custody position.
type ShipModelStatus struct {
	Model    model.ShipModel `json:"model"`
	Location model.Location  `json:"location"`
	Pending  *model.Transfer `json:"pending,omitempty"`
}

func (l *CustodyLedger) Register(id, name string, home model.Location, at time.Time) (model.ShipModel, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" {
		return model.ShipModel{}, errors.New("model id and name are required")
	}
	if err := model.ValidateLocation(home); err != nil {
		return model.ShipModel{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.models[id]; exists {
		return model.ShipModel{}, fmt.Errorf("%w: %q", ErrShipModelExists, id)
	}
	registered := model.NewShipModel(id, name, home, at)
	l.models[id] = registered
	if err := l.commit(); err != nil {
		delete(l.models, id)
		return model.ShipModel{}, err
	}
	return registered, nil
}

func (l *CustodyLedger) positionOf(modelID string) (model.Location, *model.Transfer) {
	location := l.models[modelID].Home
	var pending *model.Transfer
	for index, transfer := range l.transfers[modelID] {
		if transfer.Pending() {
			pending = &l.transfers[modelID][index]
		} else {
			location = transfer.To
		}
	}
	return location, pending
}

// Offer starts a handover of a model to another area. The model must not
// have an unconfirmed transfer: a model in transit cannot be occupied by
// another test.
func (l *CustodyLedger) Offer(modelID, to, seal, handedOverBy string, run model.RunID, at time.Time) (model.Transfer, error) {
	handedOverBy = strings.TrimSpace(handedOverBy)
	if handedOverBy == "" {
		return model.Transfer{}, errors.New("handing-over person is required")
	}
	destination := model.Location(to)
	if err := model.ValidateLocation(destination); err != nil {
		return model.Transfer{}, err
	}
	sealState := model.SealState(seal)
	if err := model.ValidateSealState(sealState); err != nil {
		return model.Transfer{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.models[modelID]; !exists {
		return model.Transfer{}, fmt.Errorf("%w: %q", ErrShipModelNotFound, modelID)
	}
	from, pending := l.positionOf(modelID)
	if pending != nil {
		return model.Transfer{}, fmt.Errorf("%w: %q", ErrShipModelOccupied, modelID)
	}
	if destination == from {
		return model.Transfer{}, fmt.Errorf("ship model %q is already in %s", modelID, from)
	}
	transfer := model.NewTransfer(modelID, from, destination, sealState, handedOverBy, run, at)
	l.transfers[modelID] = append(l.transfers[modelID], transfer)
	if err := l.commit(); err != nil {
		l.transfers[modelID] = l.transfers[modelID][:len(l.transfers[modelID])-1]
		return model.Transfer{}, err
	}
	return transfer, nil
}

// Confirm marks a pending transfer as received by the named person.
func (l *CustodyLedger) Confirm(transferID model.EventID, receivedBy string, at time.Time) (model.Transfer, error) {
	receivedBy = strings.TrimSpace(receivedBy)
	if receivedBy == "" {
		return model.Transfer{}, errors.New("receiving person is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for modelID, chain := range l.transfers {
		for index, transfer := range chain {
			if transfer.ID != transferID {
				continue
			}
			if !transfer.Pending() {
				return model.Transfer{}, ErrTransferConfirmed
			}
			confirmed := transfer.Confirm(receivedBy, at)
			l.transfers[modelID][index] = confirmed
			if err := l.commit(); err != nil {
				l.transfers[modelID][index] = transfer
				return model.Transfer{}, err
			}
			return confirmed, nil
		}
	}
	return model.Transfer{}, ErrTransferNotFound
}

func (l *CustodyLedger) Chain(modelID string) ([]model.Transfer, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.models[modelID]; !exists {
		return nil, false
	}
	return append([]model.Transfer(nil), l.transfers[modelID]...), true
}

func (l *CustodyLedger) Models() []ShipModelStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]ShipModelStatus, 0, len(l.models))
	for id, registered := range l.models {
		location, pending := l.positionOf(id)
		status := ShipModelStatus{Model: registered, Location: location}
		if pending != nil {
			copy := *pending
			status.Pending = &copy
		}
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Model.RegisteredAt.Before(result[j].Model.RegisteredAt) })
	return result
}

func (r *Runtime) RegisterShipModel(id, name string, home model.Location) (model.ShipModel, error) {
	return r.Custody.Register(id, name, home, time.Now().UTC())
}

// OfferShipModel validates the associated test before the ledger records the
// handover, keeping the ledger itself free of runtime dependencies.
func (r *Runtime) OfferShipModel(modelID, to, seal, handedOverBy string, run model.RunID) (model.Transfer, error) {
	if run != "" {
		if !r.runExists(run) {
			return model.Transfer{}, fmt.Errorf("associated test run %q not found", run)
		}
	}
	return r.Custody.Offer(modelID, to, seal, handedOverBy, run, time.Now().UTC())
}

func (r *Runtime) runExists(run model.RunID) bool {
	r.mu.Lock()
	if _, ok := r.operations[run]; ok {
		r.mu.Unlock()
		return true
	}
	r.mu.Unlock()
	_, ok := r.Measurements.Get(run)
	return ok
}

func (r *Runtime) ConfirmShipModelTransfer(transferID model.EventID, receivedBy string) (model.Transfer, error) {
	return r.Custody.Confirm(transferID, receivedBy, time.Now().UTC())
}

func (r *Runtime) ShipModels() []ShipModelStatus { return r.Custody.Models() }

func (r *Runtime) ShipModelChain(modelID string) ([]model.Transfer, bool) {
	return r.Custody.Chain(modelID)
}
