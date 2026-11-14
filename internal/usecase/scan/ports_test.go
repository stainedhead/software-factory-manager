package scan

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

type fakeDetector struct{ id ownership.DetectorID }

func (f fakeDetector) ID() ownership.DetectorID { return f.id }
func (fakeDetector) Detect(context.Context, []catalog.Entry, Env) ([]ownership.Detection, error) {
	return nil, nil
}

type fakeInventory struct{}

func (fakeInventory) Records(context.Context) ([]inventory.Record, error)            { return nil, nil }
func (fakeInventory) UpsertRecord(context.Context, inventory.Record) error           { return nil }
func (fakeInventory) MarkInterrupted(context.Context) ([]inventory.Operation, error) { return nil, nil }
func (fakeInventory) SaveDetections(context.Context, []inventory.DetectionSnapshot) error {
	return nil
}

type fakeSource struct{ c catalog.Catalog }

func (f fakeSource) Load(context.Context) (catalog.Catalog, error) { return f.c, nil }

type fakeStateDir struct{}

func (fakeStateDir) TryLock() (func(), int, error) { return func() {}, 0, nil }

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, process.Cmd) (process.Result, error) {
	return process.Result{}, nil
}

var (
	_ Detector      = fakeDetector{}
	_ Inventory     = fakeInventory{}
	_ CatalogSource = fakeSource{}
	_ StateDir      = fakeStateDir{}
	_ CommandRunner = fakeRunner{}
)
