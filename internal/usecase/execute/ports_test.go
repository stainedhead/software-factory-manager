package execute

import (
	"context"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, process.Cmd) (process.Result, error) {
	return process.Result{}, nil
}

type fakeInventory struct{}

func (fakeInventory) StartOperation(context.Context, inventory.Operation) (int64, error) {
	return 1, nil
}
func (fakeInventory) FinishOperation(context.Context, int64, int, inventory.OperationStatus, string) error {
	return nil
}
func (fakeInventory) UpsertRecord(context.Context, inventory.Record) error          { return nil }
func (fakeInventory) RecordTapTrust(context.Context, inventory.TapTrustGrant) error { return nil }

type fakeInstaller struct{}

func (fakeInstaller) Download(context.Context) (string, error) { return "/tmp/install.sh", nil }
func (fakeInstaller) Command(path string) plan.Command {
	return plan.Command{Argv: []string{"/bin/bash", path}}
}

type fakeStateDir struct{}

func (fakeStateDir) LogPath(int64, string, time.Time) string { return "" }
func (fakeStateDir) Lock(context.Context) (func(), error)    { return func() {}, nil }
func (fakeStateDir) TryLock() (func(), int, error)           { return func() {}, 0, nil }
func (fakeStateDir) PruneLogs(time.Time) error               { return nil }

type fakeClock struct{ t time.Time }

func (f fakeClock) Now() time.Time { return f.t }

type fakeUser string

func (f fakeUser) Username() string { return string(f) }

type fakeProgress struct{}

func (fakeProgress) ActionStarted(plan.Action)                {}
func (fakeProgress) CommandOutput(plan.Action, string)        {}
func (fakeProgress) ActionFinished(plan.Action, ActionResult) {}

type fakeConfirmer bool

func (f fakeConfirmer) Confirm(context.Context, plan.Plan) (bool, error) { return bool(f), nil }

var (
	_ CommandRunner     = fakeRunner{}
	_ Inventory         = fakeInventory{}
	_ HomebrewInstaller = fakeInstaller{}
	_ StateDir          = fakeStateDir{}
	_ Clock             = fakeClock{}
	_ UserInfo          = fakeUser("")
	_ Progress          = fakeProgress{}
	_ Confirmer         = fakeConfirmer(false)
)
