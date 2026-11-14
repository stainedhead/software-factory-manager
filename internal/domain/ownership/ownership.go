// Package ownership classifies each catalog package into exactly one
// ownership state (PRD Appendix D) from what the detectors found, the
// inventory record, the conflict pre-flight and platform availability.
package ownership

import (
	"fmt"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/version"
)

// State is a package's ownership state.
type State string

// Ownership states. suppressed is reserved for Increment 2.
const (
	StateAbsent          State = "absent"
	StateForeign         State = "foreign"
	StateManaged         State = "managed"
	StateAdopted         State = "adopted"
	StateDrifted         State = "drifted"
	StateDuplicate       State = "duplicate"
	StateConflict        State = "conflict"
	StateUnavailableHere State = "unavailable-here"
)

// Provenance is where a detected install came from (declared in catalog).
type Provenance = catalog.Provenance

// DetectorID names a detector.
type DetectorID string

// Detectors.
const (
	DetectorPath   DetectorID = "path"
	DetectorBrew   DetectorID = "brew"
	DetectorBundle DetectorID = "bundle"
	DetectorNPM    DetectorID = "npm"
)

// DetectionStatus says whether a detector could answer.
type DetectionStatus string

// Detection statuses.
const (
	DetectionOK      DetectionStatus = "ok"
	DetectionUnknown DetectionStatus = "unknown"
)

// Detection is one detector's answer for one package (data-dictionary §2.6).
// A detector that timed out or failed reports Status unknown with a Reason
// for every package it covers.
type Detection struct {
	// PackageID is the catalog package.
	PackageID string
	// Detector produced it.
	Detector DetectorID
	// Status is ok or unknown.
	Status DetectionStatus
	// Reason explains an unknown status.
	Reason string
	// Found is true when the package was found.
	Found bool
	// Path is as found on PATH, or the bundle path.
	Path string
	// ResolvedPath is Path after resolving symlinks.
	ResolvedPath string
	// Provenance is where the install came from.
	Provenance Provenance
	// Version is "" when unknown.
	Version string
	// BackendRef is set when a backend reports it, e.g. brew-cask:claude-code.
	BackendRef string
	// PathWinner is true for the first match on PATH.
	PathWinner bool
}

// PackageStatus is the classification result (data-dictionary §2.7).
type PackageStatus struct {
	// PackageID is the catalog package.
	PackageID string
	// State is the ownership state.
	State State
	// Detections are every detection for the package.
	Detections []Detection
	// Winner is the install that wins (first on PATH) when present.
	Winner *Detection
	// OSBundled is true when the winning install is the OS copy ("present (OS)").
	OSBundled bool
	// Satisfying is true when the package is present and meets the catalog's
	// version rules (satisfies_if, reject_if).
	Satisfying bool
	// Offered is false when the catalog does not offer the package on this
	// platform; such packages are never shown as wanted.
	Offered bool
	// UnavailableReason explains unavailable-here (and version shortfalls).
	UnavailableReason string
	// Hint is a one-line hint.
	Hint string
	// Conflict is set for the conflict state.
	Conflict *plan.Conflict
	// Incomplete is true when a detector relevant to the package was unknown.
	Incomplete bool
	// Record is the inventory row, if any.
	Record *inventory.Record
}

// Present reports whether the package was found installed in any way.
func (s PackageStatus) Present() bool { return s.Winner != nil }

// Classify returns the package's single ownership state. Precedence:
// unavailable-here, then inventory-based (managed or adopted when detected
// through the recorded backend, else drifted; kept as recorded but marked
// incomplete when that backend's detector could not answer), then duplicate
// (two or more distinct installs, not counting one file seen by two
// detectors), then foreign (found, no record), then conflict (only when
// otherwise absent), then absent.
func Classify(entry catalog.Entry, detections []Detection, record *inventory.Record, conflict *plan.Conflict, availability catalog.Availability) PackageStatus {
	st := PackageStatus{
		PackageID:  entry.ID,
		Detections: detections,
		Record:     record,
		Offered:    availability.Offered,
	}
	for _, d := range detections {
		if d.Status == DetectionUnknown {
			st.Incomplete = true
		}
	}
	found := foundDetections(detections)
	installs := groupInstalls(found)
	if len(installs) > 0 {
		w := winner(found, installs)
		st.Winner = &w
		st.OSBundled = w.Provenance == catalog.ProvOS
		st.Satisfying, st.UnavailableReason, st.Hint = satisfaction(entry, w)
	}

	switch {
	case availability.Unavailable():
		st.State = StateUnavailableHere
		st.UnavailableReason, st.Hint = availability.Reason, availability.Hint
	case record != nil:
		st.State = recordState(record, found, detections, &st)
	case len(installs) > 1:
		st.State = StateDuplicate
	case len(installs) == 1:
		st.State = StateForeign
	case conflict != nil:
		st.State = StateConflict
		st.Conflict = conflict
	default:
		st.State = StateAbsent
	}
	return st
}

func foundDetections(ds []Detection) []Detection {
	var out []Detection
	for _, d := range ds {
		if d.Status != DetectionUnknown && d.Found {
			out = append(out, d)
		}
	}
	return out
}

// weak provenances describe a location, not an installer, so they never make
// a duplicate when an installer-specific provenance is present.
func weak(p Provenance) bool {
	return p == "" || p == catalog.ProvUnknown || p == catalog.ProvAppBundle
}

// groupInstalls returns one representative provenance per distinct install.
// Detections of the same resolved file are one install; detections with the
// same provenance are one install; weak provenances merge into any strong one.
func groupInstalls(found []Detection) []Provenance {
	if len(found) == 0 {
		return nil
	}
	// union by resolved path first: one file seen twice is one install, and
	// it takes the strongest provenance any detector gave it.
	byFile := map[string]Provenance{}
	var order []string
	var noPath []Provenance
	for _, d := range found {
		key := d.ResolvedPath
		if key == "" {
			key = d.Path
		}
		if key == "" {
			noPath = append(noPath, d.Provenance)
			continue
		}
		prev, seen := byFile[key]
		if !seen {
			order = append(order, key)
			byFile[key] = d.Provenance
			continue
		}
		if weak(prev) {
			byFile[key] = d.Provenance
		}
	}
	var provs []Provenance
	for _, k := range order {
		provs = append(provs, byFile[k])
	}
	provs = append(provs, noPath...)

	var strong, weakOnes []Provenance
	for _, p := range provs {
		list := &strong
		if weak(p) {
			list = &weakOnes
		}
		if !containsProv(*list, p) {
			*list = append(*list, p)
		}
	}
	if len(strong) > 0 {
		return strong
	}
	return weakOnes[:1]
}

func containsProv(ps []Provenance, p Provenance) bool {
	for _, x := range ps {
		if x == p {
			return true
		}
	}
	return false
}

// winner is the PATH winner if any, else the first strong detection, else
// the first detection.
func winner(found []Detection, installs []Provenance) Detection {
	for _, d := range found {
		if d.PathWinner {
			return d
		}
	}
	for _, d := range found {
		if containsProv(installs, d.Provenance) && !weak(d.Provenance) {
			return d
		}
	}
	return found[0]
}

func satisfaction(e catalog.Entry, w Detection) (ok bool, reason, hint string) {
	if e.RejectIf != nil && e.RejectIf.VersionLT != "" {
		if v, err := version.Parse(w.Version); err == nil {
			limit, lerr := version.Parse(e.RejectIf.VersionLT)
			if lerr == nil && version.Compare(v, limit) < 0 {
				return false, fmt.Sprintf("found version %s, which is not the wanted tool (needs %s or newer)", w.Version, e.RejectIf.VersionLT),
					"facman leaves it alone; remove it yourself before installing the wanted version"
			}
		}
	}
	if e.SatisfiesIf != nil && e.SatisfiesIf.MinVersion != "" {
		if !version.Satisfies(w.Version, e.SatisfiesIf.MinVersion) {
			return false, fmt.Sprintf("found version %s, below the required %s", orUnknown(w.Version), e.SatisfiesIf.MinVersion),
				satisfiesHint(e)
		}
	}
	return true, "", ""
}

func satisfiesHint(e catalog.Entry) string {
	for _, steps := range e.Install {
		for _, s := range steps {
			if s.Backend.IsBrew() {
				return fmt.Sprintf("install %s %s or newer yourself (for example brew install %s); facman does not replace a %s it did not install",
					e.ID, e.SatisfiesIf.MinVersion, s.ID, e.ID)
			}
		}
	}
	return fmt.Sprintf("install %s %s or newer yourself", e.ID, e.SatisfiesIf.MinVersion)
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// backendProvenance maps a recorded backend to the provenance its detections carry.
var backendProvenance = map[catalog.BackendID]Provenance{
	catalog.BackendBrewFormula: catalog.ProvBrew,
	catalog.BackendBrewCask:    catalog.ProvBrewCask,
	catalog.BackendNPM:         catalog.ProvNPM,
	inventory.BackendScript:    catalog.ProvScript,
}

// backendDetector maps a recorded backend to the detector that sees it.
var backendDetector = map[catalog.BackendID]DetectorID{
	catalog.BackendBrewFormula: DetectorBrew,
	catalog.BackendBrewCask:    DetectorBrew,
	catalog.BackendNPM:         DetectorNPM,
}

// DetectedThrough reports whether any found detection is through the
// recorded backend (matching backend ref, or the backend's provenance).
func DetectedThrough(r inventory.Record, ds []Detection) bool {
	for _, d := range foundDetections(ds) {
		if r.BackendRef != "" && d.BackendRef == r.BackendRef {
			return true
		}
		if p, ok := backendProvenance[r.Backend]; ok && d.Provenance == p {
			return true
		}
	}
	return false
}

func recordState(r *inventory.Record, found, all []Detection, st *PackageStatus) State {
	stored := StateManaged
	if r.State == inventory.StateAdopted {
		stored = StateAdopted
	}
	if DetectedThrough(*r, found) {
		return stored
	}
	if det, ok := backendDetector[r.Backend]; ok {
		for _, d := range all {
			if d.Detector == det && d.Status == DetectionUnknown {
				st.Incomplete = true
				return stored
			}
		}
	}
	return StateDrifted
}
