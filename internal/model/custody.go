package model

import (
    "errors"
    "fmt"
    "strings"
    "time"
)

// Location identifies a physical area a ship model can reside in.
type Location string

const (
    LocationPreparation Location = "preparation"
    LocationTesting     Location = "testing"
    LocationStorage     Location = "storage"
)

func ValidateLocation(location Location) error {
    switch location {
    case LocationPreparation, LocationTesting, LocationStorage:
        return nil
    }
    return fmt.Errorf("unknown location %q", location)
}

// PackagingState describes the sealing condition of a ship model at handover.
type PackagingState string

const (
    PackagingSealed   PackagingState = "sealed"
    PackagingOpened   PackagingState = "opened"
    PackagingRepacked PackagingState = "repacked"
)

func ValidatePackaging(state PackagingState) error {
    switch state {
    case PackagingSealed, PackagingOpened, PackagingRepacked:
        return nil
    }
    return fmt.Errorf("unknown packaging state %q", state)
}

// ShipModel is a physical model identified by a unique entity number, unlike
// the free-form model name used by legacy operations.
type ShipModel struct {
    EntityNo     string    `json:"entity_no"`
    Name         string    `json:"name"`
    Note         string    `json:"note"`
    RegisteredAt time.Time `json:"registered_at"`
}

func NewShipModel(entityNo, name, note string, at time.Time) (ShipModel, error) {
    if strings.TrimSpace(entityNo) == "" || strings.TrimSpace(name) == "" {
        return ShipModel{}, errors.New("entity number and name are required")
    }
    return ShipModel{EntityNo: entityNo, Name: name, Note: note, RegisteredAt: at.UTC()}, nil
}

// Handover is one link in the custody chain of a ship model. It is created
// when the model is handed over and becomes final once the receiver confirms.
type Handover struct {
    ID           EventID        `json:"id"`
    EntityNo     string         `json:"entity_no"`
    From         Location       `json:"from"`
    To           Location       `json:"to"`
    Packaging    PackagingState `json:"packaging"`
    HandedOverBy string         `json:"handed_over_by"`
    ReceivedBy   string         `json:"received_by"`
    Run          RunID          `json:"run,omitempty"`
    OfferedAt    time.Time      `json:"offered_at"`
    ConfirmedAt  time.Time      `json:"confirmed_at,omitempty"`
}

func (h Handover) Pending() bool {
    return h.ConfirmedAt.IsZero()
}

func NewHandover(entityNo string, from, to Location, packaging PackagingState, handedOverBy string, run RunID, at time.Time) (Handover, error) {
    if strings.TrimSpace(entityNo) == "" {
        return Handover{}, errors.New("entity number is required")
    }
    if err := ValidateLocation(from); err != nil {
        return Handover{}, err
    }
    if err := ValidateLocation(to); err != nil {
        return Handover{}, err
    }
    if from == to {
        return Handover{}, errors.New("handover destination must differ from current location")
    }
    if err := ValidatePackaging(packaging); err != nil {
        return Handover{}, err
    }
    if strings.TrimSpace(handedOverBy) == "" {
        return Handover{}, errors.New("handing-over person is required")
    }
    if to == LocationTesting && run == "" {
        return Handover{}, errors.New("handover to the testing area must reference a run")
    }
    return Handover{
        ID:           NewEventID(),
        EntityNo:     entityNo,
        From:         from,
        To:           to,
        Packaging:    packaging,
        HandedOverBy: handedOverBy,
        Run:          run,
        OfferedAt:    at.UTC(),
    }, nil
}

func (h Handover) Confirm(receivedBy string, at time.Time) (Handover, error) {
    if !h.Pending() {
        return h, errors.New("handover already confirmed")
    }
    if strings.TrimSpace(receivedBy) == "" {
        return h, errors.New("receiving person is required")
    }
    h.ReceivedBy = receivedBy
    h.ConfirmedAt = at.UTC()
    return h, nil
}
