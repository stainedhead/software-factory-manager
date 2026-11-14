package doctor

import (
	"context"
	"fmt"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// Service runs doctor (FR-018). It only reads: no command it causes
// changes the machine.
type Service struct {
	// Conflicts runs the link-path pre-flight.
	Conflicts ConflictChecker
	// Taps reports tap and trust state.
	Taps TapChecker
	// Implemented are the install backends this build has.
	Implemented catalog.BackendSet
}

// Input is what doctor checks.
type Input struct {
	// Catalog is the loaded catalog; every entry is checked, not only the
	// selected ones.
	Catalog catalog.Catalog
	// Platform is this machine.
	Platform platform.Platform
	// Statuses are the scan results by package ID; a tap item installed
	// through brew needs its trust even when its tap was removed.
	Statuses map[string]ownership.PackageStatus
	// Findings are the bootstrap and scan findings, reported first.
	Findings []platform.Finding
}

// Report is the doctor result.
type Report struct {
	// Findings in order: the given ones, then per catalog entry.
	Findings []platform.Finding
	// Conflicts found, in catalog order.
	Conflicts []*plan.Conflict
	// Checked lists the packages whose brew item was checked.
	Checked []string
}

// Err is exit 2 (drift) when any finding is a warning or worse, nil when
// the report is clean or informational only.
func (r Report) Err() error {
	n := 0
	for _, f := range r.Findings {
		if f.Severity != platform.SeverityInfo {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return failure.Drift("doctor found %d problem(s) to fix", n)
}

// Run checks every catalog brew item available on this platform for link
// conflicts, and every tap item for its tap and trust state. A check that
// cannot finish is an info finding, not an error.
func (s *Service) Run(ctx context.Context, in Input) Report {
	r := Report{Findings: append([]platform.Finding(nil), in.Findings...)}
	for _, e := range in.Catalog.Entries {
		if e.IsChoice() || e.SupersededBy != "" {
			continue
		}
		avail := catalog.SelectInstall(e, in.Platform, s.Implemented)
		if avail.Step == nil || !avail.Step.Backend.IsBrew() {
			continue
		}
		step := *avail.Step
		r.Checked = append(r.Checked, e.ID)
		if step.Tap != "" {
			if f, ok := s.tapFinding(ctx, e, step, in.Statuses[e.ID]); ok {
				r.Findings = append(r.Findings, f)
			}
		}
		c, err := s.Conflicts.Check(ctx, e, step)
		switch {
		case err != nil:
			r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeCheckFailed, Severity: platform.SeverityInfo,
				Message: fmt.Sprintf("%s: the conflict check could not finish: %v", e.ID, err)})
		case c != nil:
			r.Conflicts = append(r.Conflicts, c)
			r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeConflict, Severity: platform.SeverityWarning,
				Message: fmt.Sprintf("%s: %s is held by %s (%s), so %s would fail", e.ID, c.Path, c.Owner, c.OwnerDetail, plan.Command{Argv: c.FailingCommand}.String()),
				Fix:     c.FixHint})
		}
	}
	return r
}

// tapFinding reports a tap item's state. An item that needs item trust and
// is not trusted is a warning when its tap is added or it is installed
// through brew (Homebrew would refuse it); a tap not added yet is info,
// since apply adds and trusts it.
func (s *Service) tapFinding(ctx context.Context, e catalog.Entry, step catalog.InstallStep, st ownership.PackageStatus) (platform.Finding, bool) {
	tapped, trusted, err := s.Taps.TapState(ctx, step)
	if err != nil {
		return platform.Finding{Code: platform.CodeCheckFailed, Severity: platform.SeverityInfo,
			Message: fmt.Sprintf("%s: the state of tap %s could not be read: %v", e.ID, step.Tap, err)}, true
	}
	needsTrust := step.Trust == catalog.TrustItem && !trusted
	installed := false
	for _, d := range st.Detections {
		installed = installed || (d.Found && d.BackendRef != "")
	}
	switch {
	case needsTrust && (tapped || installed):
		kind := "formula"
		if step.Backend == catalog.BackendBrewCask {
			kind = "cask"
		}
		return platform.Finding{Code: platform.CodeUntrustedTap, Severity: platform.SeverityWarning,
			Message: fmt.Sprintf("%s: %s %s from the tap %s is not trusted, so Homebrew will refuse it", e.ID, kind, step.ID, step.Tap),
			Fix:     plan.Command{Argv: []string{"brew", "trust", "--" + kind, step.ID}}.String()}, true
	case !tapped:
		msg := fmt.Sprintf("%s: the tap %s is not added; installing %s adds it", e.ID, step.Tap, e.ID)
		if needsTrust {
			msg += " and trusts " + step.ID
		}
		return platform.Finding{Code: platform.CodeTapPending, Severity: platform.SeverityInfo, Message: msg}, true
	default:
		return platform.Finding{}, false
	}
}
