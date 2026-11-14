// Package status lists managed, adopted, foreign, drifted and duplicate
// packages with their provenance. This file declares the ports it consumes;
// the use case lands in workstream E (T-E8).
package status

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
)

// Inventory is the subset of the inventory store status reads.
// Implemented by adapter/sqlite.
type Inventory interface {
	Records(ctx context.Context) ([]inventory.Record, error)
	TapTrustGrants(ctx context.Context) ([]inventory.TapTrustGrant, error)
}
