package status

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
)

type fakeInventory struct{}

func (fakeInventory) Records(context.Context) ([]inventory.Record, error) { return nil, nil }
func (fakeInventory) TapTrustGrants(context.Context) ([]inventory.TapTrustGrant, error) {
	return nil, nil
}

var _ Inventory = fakeInventory{}
