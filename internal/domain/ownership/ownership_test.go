package ownership

import (
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

var (
	offered     = catalog.Availability{Offered: true, Available: true}
	unavailable = catalog.Availability{Offered: true, Reason: "needs the npm backend (Increment 2)", Hint: "later"}
	notOffered  = catalog.Availability{}
	installedAt = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)
)

func rec(state inventory.RecordState, backend catalog.BackendID, ref string) *inventory.Record {
	r := &inventory.Record{ID: "x", State: state, Backend: backend, BackendRef: ref, InstalledAt: installedAt, InstalledBy: "j@t"}
	if state == inventory.StateAdopted {
		r.AdoptedAt = &installedAt
	}
	return r
}

func found(det DetectorID, prov Provenance, path string) Detection {
	return Detection{Detector: det, Status: DetectionOK, Found: true, Path: path, ResolvedPath: path, Provenance: prov}
}

func TestClassify(t *testing.T) {
	jq := catalog.Entry{ID: "jq", Kind: catalog.KindCLI}
	claude := catalog.Entry{ID: "claude-code", Kind: catalog.KindCLI}
	nodeEntry := catalog.Entry{ID: "node", Kind: catalog.KindCLI, SatisfiesIf: &catalog.Satisfaction{MinVersion: "24.11"},
		Install: nodeInstall()}
	nodeNoBrew := catalog.Entry{ID: "node", Kind: catalog.KindCLI, SatisfiesIf: &catalog.Satisfaction{MinVersion: "24.11"}}
	aws := catalog.Entry{ID: "awscli", Kind: catalog.KindCLI, RejectIf: &catalog.Rejection{VersionLT: "2.0"}}
	conflict := &plan.Conflict{PackageID: "claude-code", Path: "/opt/homebrew/bin/claude", Owner: plan.OwnerOrphan}

	brewJQ := found(DetectorBrew, catalog.ProvBrew, "")
	brewJQ.BackendRef = "brew-formula:jq"
	pathJQ := found(DetectorPath, catalog.ProvBrew, "/opt/homebrew/bin/jq")
	osJQ := found(DetectorPath, catalog.ProvOS, "/usr/bin/jq")
	osJQ.PathWinner = true
	nativeClaude := found(DetectorPath, catalog.ProvNative, "/Users/j/.local/share/claude/versions/2.1.4")
	nativeClaude.PathWinner = true
	caskClaude := found(DetectorPath, catalog.ProvBrewCask, "/opt/homebrew/Caskroom/claude-code/2.1.4/claude")
	brewCaskDet := found(DetectorBrew, catalog.ProvBrewCask, "")
	brewCaskDet.BackendRef = "brew-cask:claude-code"
	unknownBrew := Detection{Detector: DetectorBrew, Status: DetectionUnknown, Reason: "brew info timed out after 10s"}
	notFound := Detection{Detector: DetectorPath, Status: DetectionOK}
	nvmNode := func(v string) Detection {
		d := found(DetectorPath, catalog.ProvNVM, "/Users/j/.nvm/versions/node/v"+v+"/bin/node")
		d.Version, d.PathWinner = v, true
		return d
	}
	pipAws := found(DetectorPath, catalog.ProvUnknown, "/usr/local/bin/aws")
	pipAws.Version = "1.33.0"
	sameFileTwice := []Detection{pathJQ, {Detector: DetectorBrew, Status: DetectionOK, Found: true, Path: "/opt/homebrew/bin/jq", ResolvedPath: "/opt/homebrew/bin/jq", Provenance: catalog.ProvUnknown}}
	sameFileWeakFirst := []Detection{sameFileTwice[1], sameFileTwice[0]}
	appBundle := found(DetectorBundle, catalog.ProvAppBundle, "/Applications/Claude.app")

	tests := []struct {
		name           string
		entry          catalog.Entry
		dets           []Detection
		record         *inventory.Record
		conflict       *plan.Conflict
		avail          catalog.Availability
		want           State
		wantOS         bool
		wantSatisfying bool
		wantIncomplete bool
		wantWinner     Provenance
		wantReason     string
	}{
		{name: "absent", entry: jq, dets: []Detection{notFound}, avail: offered, want: StateAbsent},
		{name: "absent with no detections", entry: jq, avail: offered, want: StateAbsent},
		{name: "foreign brew install without record", entry: jq, dets: []Detection{brewJQ, pathJQ}, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvBrew},
		{name: "foreign OS bundled", entry: jq, dets: []Detection{osJQ}, avail: offered, want: StateForeign, wantOS: true, wantSatisfying: true, wantWinner: catalog.ProvOS},
		{name: "managed", entry: jq, dets: []Detection{brewJQ, pathJQ}, record: rec(inventory.StateManaged, catalog.BackendBrewFormula, "brew-formula:jq"), avail: offered, want: StateManaged, wantSatisfying: true, wantWinner: catalog.ProvBrew},
		{name: "managed by provenance when no ref", entry: jq, dets: []Detection{pathJQ}, record: rec(inventory.StateManaged, catalog.BackendBrewFormula, ""), avail: offered, want: StateManaged, wantSatisfying: true, wantWinner: catalog.ProvBrew},
		{name: "adopted", entry: jq, dets: []Detection{brewJQ}, record: rec(inventory.StateAdopted, catalog.BackendBrewFormula, "brew-formula:jq"), avail: offered, want: StateAdopted, wantSatisfying: true, wantWinner: catalog.ProvBrew},
		{name: "drifted: record, not found", entry: jq, dets: []Detection{notFound}, record: rec(inventory.StateManaged, catalog.BackendBrewFormula, "brew-formula:jq"), avail: offered, want: StateDrifted},
		{name: "drifted: record brew-cask, found native", entry: claude, dets: []Detection{nativeClaude}, record: rec(inventory.StateManaged, catalog.BackendBrewCask, "brew-cask:claude-code"), avail: offered, want: StateDrifted, wantSatisfying: true, wantWinner: catalog.ProvNative},
		{name: "record kept when its detector is unknown", entry: jq, dets: []Detection{unknownBrew, notFound}, record: rec(inventory.StateManaged, catalog.BackendBrewFormula, "brew-formula:jq"), avail: offered, want: StateManaged, wantIncomplete: true},
		{name: "duplicate brew and native, winner first on PATH", entry: claude, dets: []Detection{caskClaude, nativeClaude, brewCaskDet}, avail: offered, want: StateDuplicate, wantSatisfying: true, wantWinner: catalog.ProvNative},
		{name: "same file seen by two detectors is not a duplicate", entry: jq, dets: sameFileTwice, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvBrew},
		{name: "same file, weak provenance first", entry: jq, dets: sameFileWeakFirst, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvBrew},
		{name: "app bundle of a cask is not a duplicate", entry: claude, dets: []Detection{appBundle, brewCaskDet}, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvBrewCask},
		{name: "app bundle alone is foreign", entry: claude, dets: []Detection{appBundle}, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvAppBundle},
		{name: "conflict when otherwise absent", entry: claude, dets: []Detection{notFound}, conflict: conflict, avail: offered, want: StateConflict},
		{name: "conflict ignored when present", entry: claude, dets: []Detection{nativeClaude}, conflict: conflict, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvNative},
		{name: "unavailable here", entry: jq, dets: []Detection{notFound}, avail: unavailable, want: StateUnavailableHere, wantReason: "npm backend"},
		{name: "unavailable beats conflict", entry: claude, conflict: conflict, avail: unavailable, want: StateUnavailableHere, wantReason: "npm backend"},
		{name: "not offered still classified", entry: jq, dets: []Detection{osJQ}, avail: notOffered, want: StateForeign, wantOS: true, wantSatisfying: true, wantWinner: catalog.ProvOS},
		{name: "node 24.11 via nvm satisfies", entry: nodeEntry, dets: []Detection{nvmNode("24.11.0")}, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvNVM},
		{name: "node 22 via nvm does not satisfy", entry: nodeEntry, dets: []Detection{nvmNode("22.9.0")}, avail: offered, want: StateForeign, wantWinner: catalog.ProvNVM, wantReason: "below the required 24.11"},
		{name: "node unknown version does not satisfy", entry: nodeNoBrew, dets: []Detection{nvmNode("")}, avail: offered, want: StateForeign, wantWinner: catalog.ProvNVM, wantReason: "found version unknown"},
		{name: "awscli v1 rejected", entry: aws, dets: []Detection{pipAws}, avail: offered, want: StateForeign, wantWinner: catalog.ProvUnknown, wantReason: "not the wanted tool"},
		{name: "awscli v2 accepted", entry: aws, dets: []Detection{func() Detection { d := pipAws; d.Version = "2.17.0"; return d }()}, avail: offered, want: StateForeign, wantSatisfying: true, wantWinner: catalog.ProvUnknown},
		{name: "incomplete absent", entry: jq, dets: []Detection{unknownBrew, notFound}, avail: offered, want: StateAbsent, wantIncomplete: true},
		{name: "incomplete but found elsewhere", entry: jq, dets: []Detection{unknownBrew, osJQ}, avail: offered, want: StateForeign, wantOS: true, wantSatisfying: true, wantIncomplete: true, wantWinner: catalog.ProvOS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.entry, tt.dets, tt.record, tt.conflict, tt.avail)
			if got.State != tt.want {
				t.Fatalf("State = %s, want %s (%+v)", got.State, tt.want, got)
			}
			if got.OSBundled != tt.wantOS {
				t.Errorf("OSBundled = %v, want %v", got.OSBundled, tt.wantOS)
			}
			if got.Satisfying != tt.wantSatisfying {
				t.Errorf("Satisfying = %v, want %v", got.Satisfying, tt.wantSatisfying)
			}
			if got.Incomplete != tt.wantIncomplete {
				t.Errorf("Incomplete = %v, want %v", got.Incomplete, tt.wantIncomplete)
			}
			if tt.wantWinner == "" {
				if got.Present() {
					t.Errorf("unexpected winner %+v", got.Winner)
				}
			} else if got.Winner == nil || got.Winner.Provenance != tt.wantWinner {
				t.Errorf("Winner = %+v, want provenance %s", got.Winner, tt.wantWinner)
			}
			if !strings.Contains(got.UnavailableReason, tt.wantReason) {
				t.Errorf("UnavailableReason = %q, want %q", got.UnavailableReason, tt.wantReason)
			}
			if (tt.want == StateConflict) != (got.Conflict != nil) {
				t.Errorf("Conflict = %+v", got.Conflict)
			}
			if got.Offered != tt.avail.Offered || got.PackageID != tt.entry.ID || got.Record != tt.record {
				t.Errorf("passthrough fields wrong: %+v", got)
			}
		})
	}
}

func nodeInstall() map[platform.Key][]catalog.InstallStep {
	return map[platform.Key][]catalog.InstallStep{"darwin": {{Backend: catalog.BackendBrewFormula, ID: "node@24"}}}
}

func TestSatisfiesHint(t *testing.T) {
	e := catalog.Entry{ID: "node", SatisfiesIf: &catalog.Satisfaction{MinVersion: "24.11"}, Install: nodeInstall()}
	if h := satisfiesHint(e); !strings.Contains(h, "brew install node@24") {
		t.Errorf("hint = %q", h)
	}
	e.Install = nil
	if h := satisfiesHint(e); !strings.Contains(h, "install node 24.11 or newer yourself") {
		t.Errorf("hint = %q", h)
	}
}

func TestDetectedThrough(t *testing.T) {
	r := inventory.Record{Backend: inventory.BackendScript}
	if !DetectedThrough(r, []Detection{found(DetectorPath, catalog.ProvScript, "/x")}) {
		t.Error("script provenance should match the script backend")
	}
	if DetectedThrough(r, []Detection{found(DetectorPath, catalog.ProvBrew, "/x")}) {
		t.Error("brew provenance should not match the script backend")
	}
}
