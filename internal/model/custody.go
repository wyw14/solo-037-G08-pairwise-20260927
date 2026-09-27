package model

import (
    "errors"
    "time"
)

// Location is a physical area of the laboratory where a ship model rests
// between handovers.
type Location string

const (
    LocationPreparation Location = "preparation"
    LocationTestArea Location = "test_area"
    LocationStorage Location = "storage"
)

func ValidateLocation(location Location) error {
    switch location {
    case LocationPreparation, LocationTestArea, LocationStorage:
        return nil
    }
    return errors.New("unknown location")
}

// SealState describes the packaging condition of a ship model at the moment
// it is handed over.
type SealState string

const (
    SealIntact SealState = "sealed"
    SealOpened SealState = "opened"
    SealDamaged SealState = "damaged"
)

func ValidateSealState(state SealState) error {
    switch state {
    case SealIntact, SealOpened, SealDamaged:
        return nil
    }
    return errors.New("unknown seal state")
}

// ShipModel is a physical hull model identified by a stable entity number,
// unlike Operation.ModelName which is only a display label.
type ShipModel struct {
    ID string `json:"id"`
    Name string `json:"name"`
    Home Location `json:"home"`
    RegisteredAt time.Time `json:"registered_at"`
}

func NewShipModel(id string, name string, home Location, at time.Time) ShipModel {
    return ShipModel{ID: id, Name: name, Home: home, RegisteredAt: at.UTC()}
}

// Transfer is one link in the custody chain of a ship model. ReceivedBy and
// ConfirmedAt stay empty until the receiver confirms the handover.
type Transfer struct {
    ID EventID `json:"id"`
    ModelID string `json:"model_id"`
    From Location `json:"from"`
    To Location `json:"to"`
    Seal SealState `json:"seal"`
    HandedOverBy string `json:"handed_over_by"`
    ReceivedBy string `json:"received_by"`
    Run RunID `json:"run,omitempty"`
    OfferedAt time.Time `json:"offered_at"`
    ConfirmedAt time.Time `json:"confirmed_at,omitempty"`
}

func NewTransfer(modelID string, from Location, to Location, seal SealState, handedOverBy string, run RunID, at time.Time) Transfer {
    return Transfer{
        ID: NewEventID(),
        ModelID: modelID,
        From: from,
        To: to,
        Seal: seal,
        HandedOverBy: handedOverBy,
        Run: run,
        OfferedAt: at.UTC(),
    }
}

func (t Transfer) Pending() bool {
    return t.ConfirmedAt.IsZero()
}

func (t Transfer) Confirm(receivedBy string, at time.Time) Transfer {
    t.ReceivedBy = receivedBy
    t.ConfirmedAt = at.UTC()
    return t
}
