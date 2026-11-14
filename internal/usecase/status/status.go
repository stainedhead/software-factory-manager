package status

import (
	"context"
	"fmt"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

// Service builds the status view (FR-033).
type Service struct {
	// Inventory holds the records and tap-trust grants.
	Inventory Inventory
}

// Row is one package in the view.
type Row struct {
	// PackageID is the catalog package (or the recorded ID).
	PackageID string
	// Display is the catalog name, empty for a record not in the catalog.
	Display string
	// State is managed, adopted, foreign, drifted or duplicate.
	State ownership.State
	// Provenance is where the winning install came from.
	Provenance catalog.Provenance
	// Path is the winning install's path.
	Path string
	// Version is the winning install's version.
	Version string
	// Record is the inventory row, if any.
	Record *inventory.Record
	// Detail explains drift, duplicates and records not in the catalog.
	Detail string
}

// Report is the status view.
type Report struct {
	// Rows in catalog order, then records not in the catalog.
	Rows []Row
	// Homebrew is the bootstrap record when facman installed Homebrew.
	Homebrew *inventory.Record
	// TapTrust are the item-trust grants facman made.
	TapTrust []inventory.TapTrustGrant
}

var shown = map[ownership.State]bool{
	ownership.StateManaged: true, ownership.StateAdopted: true, ownership.StateForeign: true,
	ownership.StateDrifted: true, ownership.StateDuplicate: true,
}

// Run lists the packages that are managed, adopted, foreign, drifted or
// duplicate in statuses (the scan result), in catalog order, with
// provenance; then the records whose package left the catalog, the
// Homebrew bootstrap record and the tap-trust grants.
func (s *Service) Run(ctx context.Context, cat catalog.Catalog, statuses []ownership.PackageStatus) (Report, error) {
	records, err := s.Inventory.Records(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reading the inventory: %w", err)
	}
	grants, err := s.Inventory.TapTrustGrants(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reading tap-trust grants: %w", err)
	}
	r := Report{TapTrust: grants}
	byID := map[string]ownership.PackageStatus{}
	for _, st := range statuses {
		byID[st.PackageID] = st
	}
	for _, e := range cat.Entries {
		st, ok := byID[e.ID]
		if !ok || !shown[st.State] {
			continue
		}
		row := Row{PackageID: e.ID, Display: e.Display, State: st.State, Record: st.Record, Detail: detail(st)}
		if w := st.Winner; w != nil {
			row.Provenance, row.Path, row.Version = w.Provenance, w.Path, w.Version
		}
		r.Rows = append(r.Rows, row)
	}
	for i := range records {
		rec := records[i]
		switch {
		case rec.ID == plan.HomebrewPackageID && rec.Bootstrap:
			r.Homebrew = &rec
		case cat.Index(rec.ID) < 0:
			r.Rows = append(r.Rows, Row{PackageID: rec.ID, State: ownership.State(rec.State), Record: &rec,
				Detail: fmt.Sprintf("recorded as %s, but the catalog no longer lists it", rec.BackendRef)})
		}
	}
	return r, nil
}

func detail(st ownership.PackageStatus) string {
	switch st.State {
	case ownership.StateDrifted:
		ref := ""
		if st.Record != nil {
			ref = st.Record.BackendRef
		}
		if st.Winner == nil {
			return fmt.Sprintf("recorded as %s, but not found", ref)
		}
		return fmt.Sprintf("recorded as %s, but found as %s at %s", ref, st.Winner.Provenance, st.Winner.Path)
	case ownership.StateDuplicate:
		var installs []string
		for _, d := range st.Detections {
			if d.Found {
				installs = append(installs, fmt.Sprintf("%s (%s)", d.Path, d.Provenance))
			}
		}
		return "installed more than once: " + strings.Join(installs, ", ")
	default:
		return ""
	}
}
