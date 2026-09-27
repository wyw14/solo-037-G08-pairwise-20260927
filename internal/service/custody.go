package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/wyw14/solo-037/internal/model"
)

// RunActivity reports whether a run exists and has not reached a terminal
// stage. The custody ledger uses it to validate handovers into the testing
// area without depending on the measurement archive directly.
type RunActivity interface {
	RunActive(id model.RunID) bool
}

var (
	// ErrShipModelNotFound marks references to an unregistered entity number.
	ErrShipModelNotFound = errors.New("ship model not registered")
	// ErrHandoverNotFound marks references to an unknown handover record.
	ErrHandoverNotFound = errors.New("handover not found")
)

// CustodyLedger tracks ship models by entity number and records the handover
// chain between the preparation, testing and storage areas.
type CustodyLedger struct {
	mu        sync.Mutex
	path      string
	models    map[string]model.ShipModel
	handovers []model.Handover
}

type custodySnapshot struct {
	Models    map[string]model.ShipModel `json:"models"`
	Handovers []model.Handover           `json:"handovers"`
}

func OpenCustodyLedger(directory string) (*CustodyLedger, error) {
	ledger := &CustodyLedger{models: make(map[string]model.ShipModel), handovers: []model.Handover{}}
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
	if snapshot.Models == nil || snapshot.Handovers == nil {
		return nil, errors.New("custody ledger must contain models and handovers")
	}
	ledger.models = snapshot.Models
	ledger.handovers = snapshot.Handovers
	return ledger, nil
}

func (l *CustodyLedger) commit(models map[string]model.ShipModel, handovers []model.Handover) error {
	if l.path != "" {
		data, err := json.MarshalIndent(custodySnapshot{Models: models, Handovers: handovers}, "", "  ")
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
	l.models = models
	l.handovers = handovers
	return nil
}

func (l *CustodyLedger) clone() (map[string]model.ShipModel, []model.Handover) {
	models := make(map[string]model.ShipModel, len(l.models))
	for key, value := range l.models {
		models[key] = value
	}
	handovers := append([]model.Handover(nil), l.handovers...)
	return models, handovers
}

func (l *CustodyLedger) RegisterModel(entityNo, name, note string) (model.ShipModel, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.models[entityNo]; exists {
		return model.ShipModel{}, fmt.Errorf("ship model %q already registered", entityNo)
	}
	value, err := model.NewShipModel(entityNo, name, note, time.Now().UTC())
	if err != nil {
		return model.ShipModel{}, err
	}
	models, handovers := l.clone()
	models[value.EntityNo] = value
	if err := l.commit(models, handovers); err != nil {
		return model.ShipModel{}, err
	}
	return value, nil
}

func (l *CustodyLedger) Model(entityNo string) (model.ShipModel, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	value, ok := l.models[entityNo]
	return value, ok
}

// LocationOf returns the area the model currently resides in: the destination
// of its latest confirmed handover, or the preparation area for a newly
// registered model.
func (l *CustodyLedger) LocationOf(entityNo string) (model.Location, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.models[entityNo]; !ok {
		return "", false
	}
	return l.locationOfLocked(entityNo), true
}

func (l *CustodyLedger) locationOfLocked(entityNo string) model.Location {
	location := model.LocationPreparation
	latest := time.Time{}
	for _, handover := range l.handovers {
		if handover.EntityNo == entityNo && !handover.Pending() && !handover.ConfirmedAt.Before(latest) {
			latest = handover.ConfirmedAt
			location = handover.To
		}
	}
	return location
}

// PendingFor returns the unconfirmed handover of a model, if any. A model has
// at most one pending handover; Offer enforces this.
func (l *CustodyLedger) PendingFor(entityNo string) (model.Handover, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pendingForLocked(entityNo)
}

func (l *CustodyLedger) pendingForLocked(entityNo string) (model.Handover, bool) {
	for _, handover := range l.handovers {
		if handover.EntityNo == entityNo && handover.Pending() {
			return handover, true
		}
	}
	return model.Handover{}, false
}

// Offer records a new handover from the model's current location. Handovers
// into the testing area must reference an existing, still-active run.
func (l *CustodyLedger) Offer(entityNo string, to model.Location, packaging model.PackagingState, handedOverBy string, run model.RunID, activity RunActivity) (model.Handover, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.models[entityNo]; !ok {
		return model.Handover{}, fmt.Errorf("%w: %q", ErrShipModelNotFound, entityNo)
	}
	if _, pending := l.pendingForLocked(entityNo); pending {
		return model.Handover{}, fmt.Errorf("ship model %q has an unconfirmed handover", entityNo)
	}
	if to == model.LocationTesting && (activity == nil || !activity.RunActive(run)) {
		return model.Handover{}, fmt.Errorf("run %q is not an active run", run)
	}
	handover, err := model.NewHandover(entityNo, l.locationOfLocked(entityNo), to, packaging, handedOverBy, run, time.Now().UTC())
	if err != nil {
		return model.Handover{}, err
	}
	models, handovers := l.clone()
	handovers = append(handovers, handover)
	if err := l.commit(models, handovers); err != nil {
		return model.Handover{}, err
	}
	return handover, nil
}

// Confirm marks a pending handover as received by the named person.
func (l *CustodyLedger) Confirm(id model.EventID, receivedBy string) (model.Handover, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	models, handovers := l.clone()
	for index, handover := range handovers {
		if handover.ID != id {
			continue
		}
		confirmed, err := handover.Confirm(receivedBy, time.Now().UTC())
		if err != nil {
			return model.Handover{}, err
		}
		handovers[index] = confirmed
		if err := l.commit(models, handovers); err != nil {
			return model.Handover{}, err
		}
		return confirmed, nil
	}
	return model.Handover{}, fmt.Errorf("%w: %q", ErrHandoverNotFound, id)
}

// Chain returns the full handover history of a model in recording order.
func (l *CustodyLedger) Chain(entityNo string) ([]model.Handover, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.models[entityNo]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrShipModelNotFound, entityNo)
	}
	result := make([]model.Handover, 0)
	for _, handover := range l.handovers {
		if handover.EntityNo == entityNo {
			result = append(result, handover)
		}
	}
	return result, nil
}

// Models lists every registered model ordered by entity number.
func (l *CustodyLedger) Models() []model.ShipModel {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]model.ShipModel, 0, len(l.models))
	for _, value := range l.models {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EntityNo < result[j].EntityNo })
	return result
}

// ShipModelStatus is the operator-facing view of a ship model: where it is,
// whether a handover awaits confirmation, and which active run occupies it.
type ShipModelStatus struct {
	Model           model.ShipModel `json:"model"`
	Location        model.Location  `json:"location"`
	PendingHandover *model.Handover `json:"pending_handover,omitempty"`
	OccupiedBy      model.RunID     `json:"occupied_by,omitempty"`
}

func (r *Runtime) RegisterShipModel(entityNo, name, note string) (model.ShipModel, error) {
	return r.Custody.RegisterModel(entityNo, name, note)
}

func (r *Runtime) ShipModelStatuses() []ShipModelStatus {
	result := make([]ShipModelStatus, 0)
	for _, shipModel := range r.Custody.Models() {
		location, _ := r.Custody.LocationOf(shipModel.EntityNo)
		status := ShipModelStatus{Model: shipModel, Location: location}
		if pending, ok := r.Custody.PendingFor(shipModel.EntityNo); ok {
			pending := pending
			status.PendingHandover = &pending
		}
		for _, run := range r.Measurements.List() {
			if run.ModelEntity == shipModel.EntityNo && run.Operation.Active() {
				status.OccupiedBy = run.Operation.ID
				break
			}
		}
		result = append(result, status)
	}
	return result
}

func (r *Runtime) OfferHandover(entityNo string, to model.Location, packaging model.PackagingState, handedOverBy string, run model.RunID) (model.Handover, error) {
	return r.Custody.Offer(entityNo, to, packaging, handedOverBy, run, r.Measurements)
}

func (r *Runtime) ConfirmHandover(id model.EventID, receivedBy string) (model.Handover, error) {
	return r.Custody.Confirm(id, receivedBy)
}

func (r *Runtime) HandoverChain(entityNo string) ([]model.Handover, error) {
	return r.Custody.Chain(entityNo)
}
