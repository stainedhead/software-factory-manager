package planning

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

type fakeBackend struct{ id catalog.BackendID }

func (f fakeBackend) ID() catalog.BackendID        { return f.id }
func (fakeBackend) Available(context.Context) bool { return true }
func (fakeBackend) InstallActions(context.Context, catalog.Entry, catalog.InstallStep) ([]plan.Action, error) {
	return nil, nil
}
func (fakeBackend) LinkPaths(context.Context, catalog.Entry, catalog.InstallStep) ([]string, error) {
	return nil, nil
}
func (fakeBackend) TapState(context.Context, catalog.InstallStep) (bool, bool, error) {
	return false, false, nil
}
func (fakeBackend) Capabilities() Caps { return Caps{} }

type fakeSource struct{ c catalog.Catalog }

func (f fakeSource) Load(context.Context) (catalog.Catalog, error) { return f.c, nil }

var (
	_ Backend       = fakeBackend{}
	_ CatalogSource = fakeSource{}
)
