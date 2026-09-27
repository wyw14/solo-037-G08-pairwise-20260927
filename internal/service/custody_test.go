package service

import (
	"errors"
	"testing"
	"time"

	"github.com/wyw14/solo-037/internal/model"
)

func TestCustodyChainTracksLocationsAndPeople(t *testing.T) {
	ledger, err := OpenCustodyLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Hour).UTC()
	if _, err := ledger.Register("SM-001", "hull X", model.LocationPreparation, at); err != nil {
		t.Fatal(err)
	}
	first, err := ledger.Offer("SM-001", "test_area", "sealed", "alice", "run-1", at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.From != model.LocationPreparation || first.To != model.LocationTestArea || !first.Pending() {
		t.Fatalf("unexpected first transfer: %+v", first)
	}
	confirmed, err := ledger.Confirm(first.ID, "bob", at.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ReceivedBy != "bob" || confirmed.Pending() {
		t.Fatalf("confirmation not recorded: %+v", confirmed)
	}
	second, err := ledger.Offer("SM-001", "storage", "opened", "bob", "", at.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if second.From != model.LocationTestArea || second.Run != "" {
		t.Fatalf("chain did not advance location: %+v", second)
	}
	chain, ok := ledger.Chain("SM-001")
	if !ok || len(chain) != 2 || chain[0].Run != "run-1" {
		t.Fatalf("chain incomplete: %+v", chain)
	}
	models := ledger.Models()
	if len(models) != 1 || models[0].Location != model.LocationTestArea || models[0].Pending == nil || models[0].Pending.ID != second.ID {
		t.Fatalf("status did not reflect pending transfer: %+v", models)
	}
}

func TestCustodyRejectsSecondOfferWhileReceiptUnconfirmed(t *testing.T) {
	ledger, err := OpenCustodyLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if _, err := ledger.Register("SM-002", "hull Y", model.LocationStorage, at); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Offer("SM-002", "test_area", "sealed", "alice", "run-1", at); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Offer("SM-002", "preparation", "sealed", "carol", "run-2", at); !errors.Is(err, ErrShipModelOccupied) {
		t.Fatalf("unconfirmed model occupied by another test: %v", err)
	}
	if _, err := ledger.Offer("SM-003", "test_area", "sealed", "carol", "", at); !errors.Is(err, ErrShipModelNotFound) {
		t.Fatalf("unregistered model accepted: %v", err)
	}
	if _, err := ledger.Confirm("missing", "bob", at); !errors.Is(err, ErrTransferNotFound) {
		t.Fatalf("unknown transfer confirmed: %v", err)
	}
}

func TestCustodyValidatesInputAndPersistsAcrossReload(t *testing.T) {
	directory := t.TempDir()
	ledger, err := OpenCustodyLedger(directory)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if _, err := ledger.Register("SM-004", "hull Z", model.LocationStorage, at); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []func() error{
		func() error { _, err := ledger.Register("SM-004", "hull Z", model.LocationStorage, at); return err },
		func() error { _, err := ledger.Register("", "hull Z", model.LocationStorage, at); return err },
		func() error { _, err := ledger.Register("SM-005", "hull W", "moon", at); return err },
		func() error { _, err := ledger.Offer("SM-004", "storage", "sealed", "alice", "", at); return err },
		func() error { _, err := ledger.Offer("SM-004", "test_area", "crushed", "alice", "", at); return err },
		func() error { _, err := ledger.Offer("SM-004", "test_area", "sealed", "", "", at); return err },
	} {
		if err := attempt(); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	transfer, err := ledger.Offer("SM-004", "test_area", "damaged", "alice", "run-9", at)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenCustodyLedger(directory)
	if err != nil {
		t.Fatal(err)
	}
	chain, ok := reloaded.Chain("SM-004")
	if !ok || len(chain) != 1 || chain[0].ID != transfer.ID || chain[0].Seal != model.SealDamaged || !chain[0].Pending() {
		t.Fatalf("chain lost across reload: %+v", chain)
	}
	if _, err := reloaded.Offer("SM-004", "storage", "sealed", "bob", "", at); !errors.Is(err, ErrShipModelOccupied) {
		t.Fatalf("pending state lost across reload: %v", err)
	}
	if _, err := reloaded.Confirm(transfer.ID, "bob", at); err != nil {
		t.Fatal(err)
	}
	if _, err := reloaded.Confirm(transfer.ID, "carol", at); !errors.Is(err, ErrTransferConfirmed) {
		t.Fatalf("double confirm accepted: %v", err)
	}
}
