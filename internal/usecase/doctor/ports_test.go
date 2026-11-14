package doctor

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
)

type fakeSource struct{ c catalog.Catalog }

func (f fakeSource) Load(context.Context) (catalog.Catalog, error) { return f.c, nil }

var (
	_ CatalogSource   = fakeSource{}
	_ TapChecker      = fakeTaps{}
	_ ConflictChecker = fakeConflicts{}
)
