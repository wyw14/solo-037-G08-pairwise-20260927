package service

import (
	"errors"
	"testing"

	"github.com/wyw14/solo-037/internal/model"
)

func openRuntime(t *testing.T, directory string) *Runtime {
	t.Helper()
	runtime, err := OpenRuntime(directory)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func registerModel(t *testing.T, runtime *Runtime, entity string) {
	t.Helper()
	if _, err := runtime.RegisterShipModel(entity, "hull "+entity, ""); err != nil {
		t.Fatal(err)
	}
}

func createBoundRun(t *testing.T, runtime *Runtime, entity string) MeasuredRun {
	t.Helper()
	run, err := runtime.CreateMeasuredRun("run for "+entity, "hull "+entity, "section-a", "carriage-1", entity)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestCustodyChainPersistsAcrossReload(t *testing.T) {
	directory := t.TempDir()
	runtime := openRuntime(t, directory)
	registerModel(t, runtime, "SM-001")
	run := createBoundRun(t, runtime, "SM-001")

	offered, err := runtime.OfferHandover("SM-001", model.LocationTesting, model.PackagingSealed, "operator-li", run.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !offered.Pending() || offered.From != model.LocationPreparation || offered.Run != run.Operation.ID {
		t.Fatalf("unexpected handover: %+v", offered)
	}
	if _, err := runtime.ConfirmHandover(offered.ID, "operator-wang"); err != nil {
		t.Fatal(err)
	}

	reloaded := openRuntime(t, directory)
	chain, err := reloaded.HandoverChain("SM-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 1 || chain[0].Pending() || chain[0].ReceivedBy != "operator-wang" {
		t.Fatalf("chain not persisted: %+v", chain)
	}
	location, ok := reloaded.Custody.LocationOf("SM-001")
	if !ok || location != model.LocationTesting {
		t.Fatalf("location after reload: %s %v", location, ok)
	}
}

func TestPendingHandoverBlocksNewRun(t *testing.T) {
	runtime := openRuntime(t, t.TempDir())
	registerModel(t, runtime, "SM-002")
	if _, err := runtime.OfferHandover("SM-002", model.LocationStorage, model.PackagingRepacked, "operator-li", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateMeasuredRun("second run", "hull SM-002", "section-a", "carriage-1", "SM-002"); !errors.Is(err, ErrModelOccupied) {
		t.Fatalf("expected occupancy conflict, got %v", err)
	}
	// A run without an entity binding is unaffected.
	if _, err := runtime.CreateMeasuredRun("unnamed run", "hull X", "section-a", "carriage-1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestActiveRunBlocksAndCloseReleases(t *testing.T) {
	runtime := openRuntime(t, t.TempDir())
	registerModel(t, runtime, "SM-003")
	run := createBoundRun(t, runtime, "SM-003")

	if _, err := runtime.CreateMeasuredRun("overlapping", "hull SM-003", "section-b", "carriage-2", "SM-003"); !errors.Is(err, ErrModelOccupied) {
		t.Fatalf("expected occupancy conflict, got %v", err)
	}
	statuses := runtime.ShipModelStatuses()
	if len(statuses) != 1 || statuses[0].OccupiedBy != run.Operation.ID {
		t.Fatalf("status does not show occupancy: %+v", statuses)
	}
	if _, err := runtime.CloseMeasuredRun(run.Operation.ID, "complete"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateMeasuredRun("after close", "hull SM-003", "section-b", "carriage-2", "SM-003"); err != nil {
		t.Fatalf("closed run still occupies model: %v", err)
	}
}

func TestHandoverValidationAndConfirmation(t *testing.T) {
	runtime := openRuntime(t, t.TempDir())
	registerModel(t, runtime, "SM-004")

	if _, err := runtime.OfferHandover("SM-004", model.LocationTesting, model.PackagingSealed, "operator-li", "missing-run"); err == nil {
		t.Fatal("handover to testing accepted an unknown run")
	}
	if _, err := runtime.OfferHandover("ghost", model.LocationStorage, model.PackagingSealed, "operator-li", ""); !errors.Is(err, ErrShipModelNotFound) {
		t.Fatalf("expected not-found, got %v", err)
	}
	if _, err := runtime.OfferHandover("SM-004", model.LocationPreparation, model.PackagingSealed, "operator-li", ""); err == nil {
		t.Fatal("handover to current location accepted")
	}
	offered, err := runtime.OfferHandover("SM-004", model.LocationStorage, model.PackagingOpened, "operator-li", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.OfferHandover("SM-004", model.LocationPreparation, model.PackagingSealed, "operator-li", ""); err == nil {
		t.Fatal("second pending handover accepted")
	}
	if _, err := runtime.ConfirmHandover(offered.ID, ""); err == nil {
		t.Fatal("confirmation without receiver accepted")
	}
	if _, err := runtime.ConfirmHandover(offered.ID, "operator-wang"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ConfirmHandover(offered.ID, "operator-wang"); err == nil {
		t.Fatal("double confirmation accepted")
	}
	if _, err := runtime.ConfirmHandover("missing", "operator-wang"); !errors.Is(err, ErrHandoverNotFound) {
		t.Fatalf("expected not-found, got %v", err)
	}
	if _, err := runtime.HandoverChain("ghost"); !errors.Is(err, ErrShipModelNotFound) {
		t.Fatalf("expected not-found, got %v", err)
	}
	if _, err := runtime.RegisterShipModel("SM-004", "duplicate", ""); err == nil {
		t.Fatal("duplicate entity number accepted")
	}
}

func TestCloseMeasuredRunValidation(t *testing.T) {
	runtime := openRuntime(t, t.TempDir())
	run, err := runtime.CreateMeasuredRun("plain", "hull X", "section-a", "carriage-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CloseMeasuredRun(run.Operation.ID, "explode"); err == nil {
		t.Fatal("invalid outcome accepted")
	}
	if _, err := runtime.CloseMeasuredRun("missing", "complete"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected not-found, got %v", err)
	}
	if _, err := runtime.CloseMeasuredRun(run.Operation.ID, "abort"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CloseMeasuredRun(run.Operation.ID, "complete"); err == nil {
		t.Fatal("second close accepted")
	}
}
