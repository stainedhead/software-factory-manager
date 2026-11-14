package doctor

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
)

type fakeSource struct{ c catalog.Catalog }

func (f fakeSource) Load(context.Context) (catalog.Catalog, error) { return f.c, nil }

type fakeTaps struct{ tapped, trusted bool }

func (f fakeTaps) TapState(context.Context, catalog.InstallStep) (bool, bool, error) {
	return f.tapped, f.trusted, nil
}

var (
	_ CatalogSource = fakeSource{}
	_ TapChecker    = fakeTaps{}
)
