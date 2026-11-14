package scan

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// HomebrewRef is the backend reference of the Homebrew bootstrap record.
const HomebrewRef = inventory.HomebrewRef

// Service runs a scan: detect, reconcile, classify, persist (FR-010,
// FR-011, FR-032, FR-061).
type Service struct {
	// Detectors run in parallel, each with Timeout.
	Detectors []Detector
	// Inventory holds the records and the operations log.
	Inventory Inventory
	// State provides the lock that decides whether scan may write.
	State StateDir
	// Now tells the time of the snapshot.
	Now func() time.Time
	// Implemented are the install backends this build has.
	Implemented catalog.BackendSet
	// Timeout per detector; zero means DetectorTimeout.
	Timeout time.Duration
}

// Input is what one scan looks at.
type Input struct {
	// Catalog is the loaded, validated catalog.
	Catalog catalog.Catalog
	// Env describes the machine for the detectors.
	Env Env
	// HoldsLock is true when the caller (apply, TUI Run) already holds the
	// state lock; scan then writes without taking it again.
	HoldsLock bool
	// ReadOnly is set for a dry run: scan neither takes the lock nor
	// writes anything (no reconcile, no snapshot), so nothing changes.
	ReadOnly bool
}

// Result is the outcome of a scan.
type Result struct {
	// Statuses are one per catalog entry, in catalog order.
	Statuses []ownership.PackageStatus
	// Records are the inventory rows by package ID (homebrew included).
	Records map[string]inventory.Record
	// Interrupted are the in_progress operations this scan marked
	// interrupted (only when it could write).
	Interrupted []inventory.Operation
	// Reconciled lists packages recorded managed because an interrupted
	// install of theirs is now detected through its backend (AD-8).
	Reconciled []string
	// Findings are scan observations (superseded tools, lock held).
	Findings []platform.Finding
	// Wrote is true when scan held the lock and saved its results.
	Wrote bool
}

// Status returns the status of a package.
func (r Result) Status(id string) (ownership.PackageStatus, bool) {
	for _, s := range r.Statuses {
		if s.PackageID == id {
			return s, true
		}
	}
	return ownership.PackageStatus{}, false
}

// StatusMap returns the statuses keyed by package ID.
func (r Result) StatusMap() map[string]ownership.PackageStatus {
	m := make(map[string]ownership.PackageStatus, len(r.Statuses))
	for _, s := range r.Statuses {
		m[s.PackageID] = s
	}
	return m
}

// ApplyConflicts marks absent packages that the conflict pre-flight found
// a conflict for as conflict (Classify's precedence: only when otherwise
// absent).
func (r *Result) ApplyConflicts(conflicts map[string]*plan.Conflict) {
	for i, s := range r.Statuses {
		if c := conflicts[s.PackageID]; c != nil && s.State == ownership.StateAbsent {
			r.Statuses[i].State = ownership.StateConflict
			r.Statuses[i].Conflict = c
		}
	}
}

// Covers reports whether a detector is relevant to an entry: PATH for
// entries with commands, brew for entries with a brew install step on any
// platform, bundle for entries with bundle IDs on macOS, npm for entries
// with an npm package. A detector that fails makes only the entries it
// covers incomplete.
func Covers(id ownership.DetectorID, e catalog.Entry, p platform.Platform) bool {
	switch id {
	case ownership.DetectorPath:
		return len(e.Detect.Commands) > 0
	case ownership.DetectorBrew:
		for _, steps := range e.Install {
			for _, s := range steps {
				if s.Backend.IsBrew() {
					return true
				}
			}
		}
		return false
	case ownership.DetectorBundle:
		return p.OS == "darwin" && len(e.Detect.BundleIDs) > 0
	case ownership.DetectorNPM:
		return e.Detect.NPMPackage != ""
	}
	return true
}

// Run scans. A ReadOnly scan writes nothing. When it holds the lock (taken here with TryLock, or by the
// caller) it marks interrupted operations, records interrupted installs
// that are now present (AD-8) and saves the detection snapshot; when
// another facman holds the lock it writes nothing and adds a warning.
func (s *Service) Run(ctx context.Context, in Input) (Result, error) {
	res := Result{Records: map[string]inventory.Record{}}
	write := in.HoldsLock && !in.ReadOnly
	if !write && !in.ReadOnly {
		unlock, holder, err := s.State.TryLock()
		if err != nil {
			return res, failure.Bootstrap("taking the state lock: %w", err)
		}
		if unlock == nil {
			res.Findings = append(res.Findings, platform.Finding{Code: platform.CodeStateLocked, Severity: platform.SeverityWarning,
				Message: fmt.Sprintf("another facman (%s) is running; this scan does not save its results", inventory.HolderLabel(holder))})
		} else {
			defer unlock()
			write = true
		}
	}
	res.Wrote = write
	if write {
		ops, err := s.Inventory.MarkInterrupted(ctx)
		if err != nil {
			return res, failure.Bootstrap("reconciling interrupted operations: %w", err)
		}
		res.Interrupted = ops
	}

	dets, err := s.detect(ctx, in)
	if err != nil {
		return res, err
	}
	records, err := s.Inventory.Records(ctx)
	if err != nil {
		return res, failure.Bootstrap("reading the inventory: %w", err)
	}
	for _, r := range records {
		res.Records[r.ID] = r
	}
	if write {
		if err := s.reconcile(ctx, in, dets, &res); err != nil {
			return res, err
		}
	}

	for _, e := range in.Catalog.Entries {
		avail := catalog.SelectInstall(e, in.Env.Platform, s.Implemented)
		var rec *inventory.Record
		if r, ok := res.Records[e.ID]; ok {
			rec = &r
		}
		st := ownership.Classify(e, dets[e.ID], rec, nil, avail)
		res.Statuses = append(res.Statuses, st)
		if e.SupersededBy != "" && st.Present() {
			res.Findings = append(res.Findings, superseded(in.Catalog, e, st))
		}
	}
	if f, ok := relativePathFinding(in.Env.Path); ok {
		res.Findings = append(res.Findings, f)
	}
	if write {
		if err := s.Inventory.SaveDetections(ctx, s.snapshots(res.Statuses)); err != nil {
			return res, failure.Bootstrap("saving detections: %w", err)
		}
	}
	return res, nil
}

// relativePathFinding warns when PATH has relative or empty entries. The
// runner and the detectors ignore them, so a program in the current
// directory is never run (review FR-002).
func relativePathFinding(path []string) (platform.Finding, bool) {
	var rel []string
	for _, d := range path {
		if !filepath.IsAbs(d) {
			rel = append(rel, strconv.Quote(d))
		}
	}
	if len(rel) == 0 {
		return platform.Finding{}, false
	}
	return platform.Finding{Code: platform.CodePathRelativeEntry, Severity: platform.SeverityWarning,
		Message: fmt.Sprintf("PATH has relative or empty entries (%s); facman ignores them and never runs a program from the current directory", strings.Join(rel, ", ")),
		Fix:     "remove relative entries (such as . or an empty entry from a leading, trailing or doubled :) from PATH"}, true
}

func superseded(c catalog.Catalog, e catalog.Entry, st ownership.PackageStatus) platform.Finding {
	by := e.SupersededBy
	if r, ok := c.Entry(by); ok && r.Display != "" {
		by = r.Display
	}
	name := e.Display
	if name == "" {
		name = e.ID
	}
	return platform.Finding{Code: platform.CodeSuperseded, Severity: platform.SeverityInfo,
		Message: fmt.Sprintf("%s found at %s; it has been superseded by %s", name, st.Winner.Path, by),
		Fix:     fmt.Sprintf("facman leaves it alone; remove it yourself when you no longer need it (the catalog provides %s)", e.SupersededBy)}
}

// detect runs every detector concurrently with its timeout. A detector
// error gives unknown detections for the entries it covers.
func (s *Service) detect(ctx context.Context, in Input) (map[string][]ownership.Detection, error) {
	timeout := s.Timeout
	if timeout == 0 {
		timeout = DetectorTimeout
	}
	out := make([][]ownership.Detection, len(s.Detectors))
	var wg sync.WaitGroup
	for i, d := range s.Detectors {
		var covered []catalog.Entry
		for _, e := range in.Catalog.Entries {
			if Covers(d.ID(), e, in.Env.Platform) {
				covered = append(covered, e)
			}
		}
		if len(covered) == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			dctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			got, err := d.Detect(dctx, covered, in.Env)
			if err == nil && dctx.Err() != nil {
				err = dctx.Err()
			}
			if err != nil {
				reason := fmt.Sprintf("%s detector: %v", d.ID(), err)
				if dctx.Err() == context.DeadlineExceeded {
					reason = fmt.Sprintf("%s detector timed out after %s", d.ID(), timeout)
				}
				got = nil
				for _, e := range covered {
					got = append(got, ownership.Detection{PackageID: e.ID, Detector: d.ID(), Status: ownership.DetectionUnknown, Reason: reason})
				}
			}
			out[i] = got
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("scanning: %w", err)
	}
	byPkg := map[string][]ownership.Detection{}
	for _, ds := range out {
		for _, d := range ds {
			byPkg[d.PackageID] = append(byPkg[d.PackageID], d)
		}
	}
	return byPkg, nil
}

// reconcile applies AD-8 to the operations this scan marked interrupted:
// an install now detected through the operation's backend is recorded
// managed; a Homebrew bootstrap is recorded when brew is now found.
// Anything else stays as it is, so the next apply proposes it again.
func (s *Service) reconcile(ctx context.Context, in Input, dets map[string][]ownership.Detection, res *Result) error {
	for _, op := range res.Interrupted {
		if _, ok := res.Records[op.PackageID]; ok {
			continue
		}
		var rec inventory.Record
		switch op.Op {
		case inventory.OpBootstrap:
			if in.Env.BrewPath == "" {
				continue
			}
			rec = inventory.Record{ID: op.PackageID, Backend: inventory.BackendScript, BackendRef: HomebrewRef, Bootstrap: true}
		case inventory.OpInstall:
			backend, ref, ok := inventory.InstallRef(op.Commands)
			if !ok {
				continue
			}
			rec = inventory.Record{ID: op.PackageID, Backend: backend, BackendRef: ref}
			if !ownership.DetectedThrough(rec, dets[op.PackageID]) {
				continue
			}
			for _, d := range dets[op.PackageID] {
				if d.Found && d.BackendRef == ref {
					rec.InstalledVersion = d.Version
				}
			}
		default:
			continue
		}
		rec.State, rec.InstalledAt, rec.InstalledBy, rec.Scope = inventory.StateManaged, op.StartedAt, op.ConsentBy, inventory.ScopeUser
		rec.ManifestRev = in.Catalog.Revision
		if err := s.Inventory.UpsertRecord(ctx, rec); err != nil {
			return failure.Bootstrap("recording interrupted install of %s: %w", op.PackageID, err)
		}
		res.Records[rec.ID] = rec
		res.Reconciled = append(res.Reconciled, rec.ID)
	}
	return nil
}

func (s *Service) snapshots(statuses []ownership.PackageStatus) []inventory.DetectionSnapshot {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	var out []inventory.DetectionSnapshot
	for _, st := range statuses {
		if len(st.Detections) == 0 {
			continue
		}
		snap := inventory.DetectionSnapshot{ScannedAt: now, PackageID: st.PackageID}
		if w := st.Winner; w != nil {
			snap.Found, snap.Version, snap.Provenance, snap.Path = true, w.Version, w.Provenance, w.Path
		}
		out = append(out, snap)
	}
	return out
}
