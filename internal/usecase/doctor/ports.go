// Package doctor runs the conflict scan for every catalog brew item
// available here and checks the trust state of the catalog's own tap items,
// returning findings and exit 0 or 2 (FR-018). This file declares the ports
// it consumes; the use case lands in workstream E (T-E8).
package doctor

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
)

// CatalogSource loads the catalog. Implemented by adapter/catalogyaml.
type CatalogSource interface {
	Load(ctx context.Context) (catalog.Catalog, error)
}

// TapChecker reports a tap item's tap and trust state (read-only).
// Implemented by adapter/brew.
type TapChecker interface {
	TapState(ctx context.Context, step catalog.InstallStep) (tapped, trusted bool, err error)
}
