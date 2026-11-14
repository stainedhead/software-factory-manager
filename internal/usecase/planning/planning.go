package planning

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// Service builds plans (FR-025, FR-028, FR-047, FR-051).
type Service struct {
	// Backends by ID; their keys are the implemented backends.
	Backends map[catalog.BackendID]Backend
	// InstallerCommand is the Homebrew installer command shown in Review
	// (/bin/bash <temp file>); execute runs the same program on the file
	// it downloads.
	InstallerCommand plan.Command
}

// Input is what a plan is built from.
type Input struct {
	// Catalog is the loaded catalog.
	Catalog catalog.Catalog
	// Desired is the resolved profile selection.
	Desired catalog.DesiredSet
	// Statuses are the scan results by package ID (conflicts applied).
	Statuses map[string]ownership.PackageStatus
	// Bootstrap is the bootstrap report (Homebrew found, warnings).
	Bootstrap platform.BootstrapReport
}

// Build returns the plan: for every selected package in dependency order
// (requires first, ties by catalog order), install actions when it is
// absent, nothing when it is present in any way, and an exclusion when it
// is unavailable here, in conflict, incompletely detected or depends on an
// excluded package. A satisfied choice group drops its pick. When Homebrew
// is missing and anything is planned, the installer is action 1 and every
// action depends on it. Every plan passes plan.Validate; a violation is an
// internal error.
func (s *Service) Build(ctx context.Context, in Input) (plan.Plan, error) {
	p := plan.Plan{Profiles: in.Desired.Profiles, CatalogRevision: in.Catalog.Revision}
	for _, f := range in.Bootstrap.Findings {
		if f.Severity == platform.SeverityWarning {
			p.Warnings = append(p.Warnings, f)
		}
	}
	wanted, excluded := s.wanted(in)
	p.Excluded = append(p.Excluded, excluded...)
	order, err := plan.OrderPackages(in.Catalog, wanted)
	if err != nil {
		return p, failure.Validation("ordering packages: %w", err)
	}

	implemented := catalog.BackendSet{}
	for id := range s.Backends {
		implemented[id] = true
	}
	type pending struct {
		pkg     string
		actions []plan.Action
		deps    []string
	}
	var groups []pending
	reasons := map[string]plan.ExclusionReason{}
	for _, ex := range p.Excluded {
		reasons[ex.PackageID] = ex.Reason
	}
	exclude := func(id string, r plan.ExclusionReason, detail string) {
		reasons[id] = r
		p.Excluded = append(p.Excluded, plan.Exclusion{PackageID: id, Reason: r, Detail: detail})
	}
	for _, id := range order {
		e, _ := in.Catalog.Entry(id)
		st, ok := in.Statuses[id]
		if !ok || !st.Offered {
			continue
		}
		if dep, r := excludedDep(e, reasons); dep != "" {
			exclude(id, plan.ExcludedDependency, fmt.Sprintf("depends on %s (%s)", dep, r))
			continue
		}
		switch {
		case st.State == ownership.StateUnavailableHere:
			exclude(id, plan.ExcludedUnavailableHere, joinNonEmpty(st.UnavailableReason, st.Hint))
			continue
		case st.State == ownership.StateConflict:
			exclude(id, plan.ExcludedConflict, conflictDetail(st.Conflict))
			continue
		case st.State != ownership.StateAbsent:
			continue // present in some way: never touched
		case st.Incomplete:
			exclude(id, plan.ExcludedDetectionIncomplete, incompleteDetail(st))
			continue
		}
		avail := catalog.SelectInstall(e, in.Bootstrap.Platform, implemented)
		if avail.Step == nil {
			exclude(id, plan.ExcludedUnavailableHere, joinNonEmpty(avail.Reason, avail.Hint))
			continue
		}
		actions, err := s.Backends[avail.Step.Backend].InstallActions(ctx, e, *avail.Step)
		if err != nil {
			return p, fmt.Errorf("planning %s: %w", id, err)
		}
		var deps []string
		for _, r := range e.Requires {
			deps = append(deps, r.Package)
		}
		groups = append(groups, pending{pkg: id, actions: actions, deps: deps})
	}

	next := 1
	bootstrapID := 0
	if !in.Bootstrap.Homebrew.Found && len(groups) > 0 {
		cmd := s.InstallerCommand
		cmd.NeedsSudo, cmd.Interactive, cmd.Unverified, cmd.Mutating = true, true, true, true
		p.Actions = append(p.Actions, plan.Action{ID: next, Kind: plan.ActionBootstrapHomebrew, PackageID: plan.HomebrewPackageID, Commands: []plan.Command{cmd}})
		bootstrapID = next
		next++
	}
	installID := map[string]int{}
	for _, g := range groups {
		prev := 0
		for i, a := range g.actions {
			a.ID = next
			next++
			if bootstrapID != 0 {
				a.DependsOn = append(a.DependsOn, bootstrapID)
			}
			if i == 0 {
				for _, d := range g.deps {
					if id, ok := installID[d]; ok {
						a.DependsOn = append(a.DependsOn, id)
					}
				}
			} else {
				a.DependsOn = append(a.DependsOn, prev)
			}
			prev = a.ID
			p.Actions = append(p.Actions, a)
		}
		installID[g.pkg] = prev
	}

	absent := func(id string) bool {
		if id == plan.HomebrewPackageID {
			return !in.Bootstrap.Homebrew.Found
		}
		return in.Statuses[id].State == ownership.StateAbsent
	}
	if err := p.Validate(absent); err != nil {
		return p, failure.Operation("internal error: the plan breaks a safety rule: %w", err)
	}
	return p, nil
}

// wanted returns the selected packages to plan: choice groups are
// replaced by their pick, a satisfied group drops its pick and the
// helpers only it required, and a required group with no pick is excluded
// as not selected.
func (s *Service) wanted(in Input) ([]string, []plan.Exclusion) {
	var excluded []plan.Exclusion
	dropped := map[string]bool{}
	groups := map[string]bool{}
	for _, it := range in.Desired.Items {
		e, ok := in.Catalog.Entry(it.ID)
		if !ok || !e.IsChoice() || !it.Selected {
			continue
		}
		groups[it.ID] = true // a group has no install of its own
		if present := presentMember(e, in.Statuses); present != "" {
			dropped[it.ID] = true
			if it.ChoicePick != "" {
				dropped[it.ChoicePick] = true
			}
			continue
		}
		if it.ChoicePick == "" && it.Required {
			excluded = append(excluded, plan.Exclusion{PackageID: it.ID, Reason: plan.ExcludedNotSelected,
				Detail: "choose one member with --with <id>: " + strings.Join(e.MemberIDs(), ", ")})
		}
	}
	// helpers pulled in only by dropped items are dropped too.
	for changed := true; changed; {
		changed = false
		for _, it := range in.Desired.Items {
			if dropped[it.ID] || it.Required || len(it.FromProfiles) > 0 || len(it.RequiredBy) == 0 {
				continue
			}
			all := true
			for _, by := range it.RequiredBy {
				all = all && dropped[by]
			}
			if all {
				dropped[it.ID] = true
				changed = true
			}
		}
	}
	// required items the developer unticked (TUI Plan screen): reported
	// when they are missing, so Summary says why they were not installed.
	for _, it := range in.Desired.Items {
		st, ok := in.Statuses[it.ID]
		if it.Required && !it.Selected && !groups[it.ID] && ok && st.Offered && st.State == ownership.StateAbsent {
			excluded = append(excluded, plan.Exclusion{PackageID: it.ID, Reason: plan.ExcludedNotSelected, Detail: "unticked on the Plan screen"})
		}
	}
	var ids []string
	for _, id := range in.Desired.SelectedIDs() {
		if !dropped[id] && !groups[id] {
			ids = append(ids, id)
		}
	}
	return ids, excluded
}

func presentMember(group catalog.Entry, statuses map[string]ownership.PackageStatus) string {
	for _, m := range group.Choice {
		if st, ok := statuses[m.ID]; ok && st.Present() {
			return m.ID
		}
	}
	return ""
}

func excludedDep(e catalog.Entry, reasons map[string]plan.ExclusionReason) (string, plan.ExclusionReason) {
	for _, r := range e.Requires {
		if reason, ok := reasons[r.Package]; ok {
			return r.Package, reason
		}
	}
	return "", ""
}

func conflictDetail(c *plan.Conflict) string {
	if c == nil {
		return "a path it would write is held by something else"
	}
	return joinNonEmpty(fmt.Sprintf("%s is held by %s (%s)", c.Path, c.Owner, c.OwnerDetail), c.FixHint)
}

func incompleteDetail(st ownership.PackageStatus) string {
	var reasons []string
	for _, d := range st.Detections {
		if d.Status == ownership.DetectionUnknown && !slices.Contains(reasons, d.Reason) {
			reasons = append(reasons, d.Reason)
		}
	}
	return "detection incomplete: " + strings.Join(reasons, "; ")
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "; ")
}
