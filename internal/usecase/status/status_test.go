package status

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
)

type fakeInventory struct {
	records           []inventory.Record
	grants            []inventory.TapTrustGrant
	recErr, grantsErr error
}

func (f *fakeInventory) Records(context.Context) ([]inventory.Record, error) {
	return f.records, f.recErr
}
func (f *fakeInventory) TapTrustGrants(context.Context) ([]inventory.TapTrustGrant, error) {
	return f.grants, f.grantsErr
}

var at = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

func det(path string, prov catalog.Provenance) ownership.Detection {
	return ownership.Detection{Found: true, Path: path, Provenance: prov, Version: "1.0.0"}
}

func TestRun(t *testing.T) {
	cat := catalog.Catalog{Entries: []catalog.Entry{
		{ID: "claude-code", Display: "Claude Code"}, {ID: "gh"}, {ID: "jq"}, {ID: "node"}, {ID: "orca"}, {ID: "awscli"}, {ID: "codex"},
	}}
	ghRec := &inventory.Record{ID: "gh", State: inventory.StateManaged, BackendRef: "brew-formula:gh"}
	nodeRec := &inventory.Record{ID: "node", State: inventory.StateManaged, BackendRef: "brew-formula:node@24"}
	orcaRec := &inventory.Record{ID: "orca", State: inventory.StateAdopted, BackendRef: "brew-cask:stablyai/orca/orca"}
	w := func(d ownership.Detection) *ownership.Detection { return &d }
	statuses := []ownership.PackageStatus{
		{PackageID: "codex", State: ownership.StateAbsent},
		{PackageID: "claude-code", State: ownership.StateForeign, Winner: w(det("/usr/local/bin/claude", "npm")), Detections: []ownership.Detection{det("/usr/local/bin/claude", "npm")}},
		{PackageID: "gh", State: ownership.StateManaged, Record: ghRec, Winner: w(det("/opt/homebrew/bin/gh", "brew-formula"))},
		{PackageID: "jq", State: ownership.StateDuplicate, Winner: w(det("/opt/homebrew/bin/jq", "brew-formula")),
			Detections: []ownership.Detection{det("/opt/homebrew/bin/jq", "brew-formula"), det("/usr/bin/jq", "os"), {Found: false, Path: "x"}}},
		{PackageID: "node", State: ownership.StateDrifted, Record: nodeRec, Winner: w(det("/Users/j/.nvm/versions/node/v24/bin/node", "nvm"))},
		{PackageID: "orca", State: ownership.StateAdopted, Record: orcaRec, Winner: w(det("/Applications/Orca.app", "brew-cask"))},
		{PackageID: "awscli", State: ownership.StateDrifted, Record: &inventory.Record{ID: "awscli", BackendRef: "brew-formula:awscli"}},
		{PackageID: "unknown", State: ownership.StateForeign},
	}
	inv := &fakeInventory{
		records: []inventory.Record{*ghRec, {ID: "homebrew", State: inventory.StateManaged, Bootstrap: true, BackendRef: inventory.HomebrewRef},
			{ID: "old-tool", State: inventory.StateManaged, BackendRef: "brew-formula:old-tool"}},
		grants: []inventory.TapTrustGrant{{PackageID: "orca", Tap: "stablyai/orca", Item: "stablyai/orca/orca", GrantedAt: at}},
	}
	r, err := (&Service{Inventory: inv}).Run(context.Background(), cat, statuses)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range r.Rows {
		got = append(got, row.PackageID+":"+string(row.State)+":"+string(row.Provenance))
	}
	want := []string{"claude-code:foreign:npm", "gh:managed:brew-formula", "jq:duplicate:brew-formula", "node:drifted:nvm", "orca:adopted:brew-cask", "awscli:drifted:", "old-tool:managed:"}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %v\nwant %v", got, want)
	}
	details := map[string]string{}
	for _, row := range r.Rows {
		details[row.PackageID] = row.Detail
	}
	for id, part := range map[string]string{
		"jq":       "/opt/homebrew/bin/jq (brew-formula), /usr/bin/jq (os)",
		"node":     "recorded as brew-formula:node@24, but found as nvm at /Users/j/.nvm",
		"awscli":   "recorded as brew-formula:awscli, but not found",
		"old-tool": "no longer lists it",
	} {
		if !strings.Contains(details[id], part) {
			t.Errorf("%s detail = %q, want %q", id, details[id], part)
		}
	}
	if r.Rows[0].Display != "Claude Code" || r.Rows[0].Path != "/usr/local/bin/claude" || r.Rows[1].Record != ghRec || r.Rows[1].Version != "1.0.0" {
		t.Errorf("row fields = %+v / %+v", r.Rows[0], r.Rows[1])
	}
	if r.Homebrew == nil || !r.Homebrew.Bootstrap || len(r.TapTrust) != 1 {
		t.Errorf("homebrew = %+v, grants = %+v", r.Homebrew, r.TapTrust)
	}
	if detail(ownership.PackageStatus{State: ownership.StateDrifted}) != "recorded as , but not found" {
		t.Error("drift without record")
	}
}

func TestRunErrors(t *testing.T) {
	for _, inv := range []*fakeInventory{{recErr: errors.New("locked")}, {grantsErr: errors.New("locked")}} {
		if _, err := (&Service{Inventory: inv}).Run(context.Background(), catalog.Catalog{}, nil); err == nil || !strings.Contains(err.Error(), "locked") {
			t.Errorf("err = %v", err)
		}
	}
}
