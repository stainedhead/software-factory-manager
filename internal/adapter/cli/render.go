package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/stainedhead/software-factory-manager/internal/adapter/session"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// errWriter remembers the first write error.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, args ...any) {
	if e.err == nil {
		_, e.err = fmt.Fprintf(e.w, format, args...)
	}
}

// render writes the text output of cmd.
func render(w io.Writer, cmd platform.Command, req session.Request, res session.Result) error {
	ew := &errWriter{w: w}
	if res.Bootstrap != nil {
		machine(ew, *res.Bootstrap, cmd)
	}
	if res.Scan != nil {
		findings(ew, res.Scan.Findings)
	}
	switch cmd {
	case platform.CommandScan:
		if res.Scan != nil {
			packagesTable(ew, res)
		}
	case platform.CommandStatus:
		statusTable(ew, res)
	case platform.CommandDoctor:
		doctorText(ew, res)
	case platform.CommandPlan:
		if res.Plan != nil {
			ew.err = firstErr(ew.err, writePlan(w, *res.Plan))
		}
	case platform.CommandApply:
		applyText(ew, req, res)
	}
	return ew.err
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

func machine(ew *errWriter, r platform.BootstrapReport, cmd platform.Command) {
	p := r.Platform
	where := p.OS + "/" + p.Arch
	switch {
	case p.WSL:
		where += " (WSL)"
	case p.OS == "linux" && !p.Display:
		where += " (no display)"
	}
	if p.Distro.ID != "" {
		where += ", " + p.Distro.ID
	}
	ew.printf("Machine:  %s\n", where)
	h := r.Homebrew
	switch {
	case !h.Found:
		ew.printf("Homebrew: not installed\n")
	case h.OnPATH:
		ew.printf("Homebrew: %s\n", h.Path)
	default:
		ew.printf("Homebrew: %s (not on PATH; used through brew shellenv)\n", h.Path)
	}
	// Plan and apply show the warnings with the plan, doctor with its
	// findings; a fatal finding is the error facman ends with.
	if cmd == platform.CommandScan || cmd == platform.CommandStatus {
		for _, f := range r.Findings {
			if f.Severity != platform.SeverityFatal {
				finding(ew, f)
			}
		}
	}
	ew.printf("\n")
}

func finding(ew *errWriter, f platform.Finding) {
	ew.printf("%s: %s\n", f.Severity, f.Message)
	if f.Fix != "" {
		ew.printf("  fix: %s\n", f.Fix)
	}
}

func findings(ew *errWriter, fs []platform.Finding) {
	for _, f := range fs {
		finding(ew, f)
	}
	if len(fs) > 0 {
		ew.printf("\n")
	}
}

// stateLabel shows OS-bundled foreign packages as "present (OS)" (FR-020).
func stateLabel(st ownership.PackageStatus) string {
	label := string(st.State)
	if st.State == ownership.StateForeign && st.OSBundled {
		label = "present (OS)"
	}
	if st.Incomplete {
		label += " (detection incomplete)"
	}
	return label
}

func packagesTable(ew *errWriter, res session.Result) {
	tw := tabwriter.NewWriter(ew.w, 0, 0, 2, ' ', 0)
	tew := &errWriter{w: tw, err: ew.err}
	tew.printf("PACKAGE\tSTATE\tPROVENANCE\tVERSION\tPATH\n")
	for _, st := range res.Scan.Statuses {
		if !st.Offered && !st.Present() {
			continue // not offered here and not installed: nothing to say
		}
		prov, ver, path := "-", "-", "-"
		if w := st.Winner; w != nil {
			prov, ver, path = orDash(string(w.Provenance)), orDash(w.Version), orDash(w.Path)
		}
		tew.printf("%s\t%s\t%s\t%s\t%s\n", st.PackageID, stateLabel(st), prov, ver, path)
	}
	if tew.err == nil {
		tew.err = tw.Flush()
	}
	ew.err = tew.err
	for _, st := range res.Scan.Statuses {
		switch {
		case st.State == ownership.StateUnavailableHere:
			ew.printf("  %s: %s\n", st.PackageID, joinNonEmpty(st.UnavailableReason, st.Hint))
		case st.State == ownership.StateDuplicate && st.Winner != nil:
			ew.printf("  %s: installed more than once; %s wins on PATH\n", st.PackageID, st.Winner.Path)
		}
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
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

// writePlan prints the exact commands in order with their markers, the
// exclusions and the warnings (FR-025). plan, apply --dry-run and the
// consent prompt print the same text.
func writePlan(w io.Writer, p plan.Plan) error {
	ew := &errWriter{w: w}
	for _, f := range p.Warnings {
		finding(ew, f)
	}
	if len(p.Warnings) > 0 {
		ew.printf("\n")
	}
	if p.Empty() {
		ew.printf("Nothing to install.\n")
	} else {
		ew.printf("Plan (profiles: %s), %d command(s) in order:\n", strings.Join(p.Profiles, ", "), len(p.Commands()))
		for _, a := range p.Actions {
			for _, c := range a.Commands {
				var marks []string
				if c.NeedsSudo {
					marks = append(marks, "may ask for sudo")
				}
				if c.Unverified {
					marks = append(marks, "unverified script: Homebrew publishes no checksum")
				}
				if len(a.DependsOn) > 0 {
					marks = append(marks, "after "+ids(a.DependsOn))
				}
				suffix := ""
				if len(marks) > 0 {
					suffix = "   [" + strings.Join(marks, "; ") + "]"
				}
				ew.printf("  %2d. %-18s %s%s\n", a.ID, a.PackageID, c.String(), suffix)
			}
		}
	}
	if len(p.Excluded) > 0 {
		ew.printf("\nNot installed:\n")
		for _, ex := range p.Excluded {
			ew.printf("  %s: %s\n", ex.PackageID, joinNonEmpty(string(ex.Reason), ex.Detail))
		}
	}
	return ew.err
}

func ids(deps []int) string {
	parts := make([]string, len(deps))
	for i, d := range deps {
		parts[i] = fmt.Sprint(d)
	}
	return strings.Join(parts, ", ")
}

func applyText(ew *errWriter, req session.Request, res session.Result) {
	switch {
	case res.Plan == nil:
		return
	case res.Outcome == nil || res.Outcome.DryRun || req.DryRun:
		ew.err = firstErr(ew.err, writePlan(ew.w, *res.Plan))
		if !res.Plan.Empty() {
			ew.printf("\nDry run: nothing was changed.\n")
		}
		return
	case res.Outcome.Declined:
		ew.printf("Nothing was changed.\n")
		return
	case res.Plan.Empty() && res.Outcome.Consent.At.IsZero():
		ew.err = firstErr(ew.err, writePlan(ew.w, *res.Plan))
	}
	if s := res.Summary; s != nil {
		ew.printf("\nSummary:\n")
		for _, l := range s.Lines {
			ew.printf("  %s\n", l.String())
			if l.LogPath != "" && !l.Installed {
				ew.printf("    log: %s\n", l.LogPath)
			}
		}
		if len(s.NextSteps) > 0 {
			ew.printf("\nNext steps:\n")
			for _, n := range s.NextSteps {
				ew.printf("  %s\n", n)
			}
		}
		if len(s.ProfileLines) > 0 {
			ew.printf("\nAdd to your shell profile:\n")
			for _, l := range s.ProfileLines {
				ew.printf("  %s\n", l)
			}
		}
	}
	for _, w := range res.Outcome.Warnings {
		ew.printf("warning: %s\n", w)
	}
}

func statusTable(ew *errWriter, res session.Result) {
	r := res.Status
	if r == nil {
		return
	}
	if len(r.Rows) == 0 {
		ew.printf("No catalog packages are installed.\n")
	} else {
		tw := tabwriter.NewWriter(ew.w, 0, 0, 2, ' ', 0)
		tew := &errWriter{w: tw, err: ew.err}
		tew.printf("PACKAGE\tSTATE\tPROVENANCE\tVERSION\tPATH\n")
		for _, row := range r.Rows {
			tew.printf("%s\t%s\t%s\t%s\t%s\n", row.PackageID, row.State, orDash(string(row.Provenance)), orDash(row.Version), orDash(row.Path))
		}
		if tew.err == nil {
			tew.err = tw.Flush()
		}
		ew.err = tew.err
		for _, row := range r.Rows {
			if row.Detail != "" {
				ew.printf("  %s: %s\n", row.PackageID, row.Detail)
			}
		}
	}
	if r.Homebrew != nil {
		ew.printf("\nHomebrew was installed by facman (%s).\n", r.Homebrew.InstalledBy)
	}
	for _, g := range r.TapTrust {
		ew.printf("Trusted %s from %s for %s.\n", g.Item, g.Tap, g.PackageID)
	}
}

func doctorText(ew *errWriter, res session.Result) {
	r := res.Doctor
	if r == nil {
		return
	}
	problems := 0
	for _, f := range r.Findings {
		finding(ew, f)
		if f.Severity != platform.SeverityInfo {
			problems++
		}
	}
	if problems == 0 {
		ew.printf("No problems found (%d catalog items checked).\n", len(r.Checked))
	} else {
		ew.printf("\n%d problem(s) to fix (%d catalog items checked).\n", problems, len(r.Checked))
	}
}
