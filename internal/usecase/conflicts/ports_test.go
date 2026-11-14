package conflicts

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

type fakeBackend struct{}

func (fakeBackend) ID() catalog.BackendID { return catalog.BackendBrewCask }
func (fakeBackend) LinkPaths(context.Context, catalog.Entry, catalog.InstallStep) ([]string, error) {
	return nil, nil
}

type fakeProbe struct{}

func (fakeProbe) Inspect(context.Context, string) (*plan.ConflictEvidence, error) { return nil, nil }
func (fakeProbe) Owner(context.Context, string, catalog.Entry) (plan.OwnerClass, string, error) {
	return plan.OwnerOrphan, "", nil
}

var (
	_ Backend   = fakeBackend{}
	_ PathProbe = fakeProbe{}
)
