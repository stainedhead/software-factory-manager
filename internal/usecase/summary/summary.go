// Package summary builds the end-of-run summary shown by apply and the TUI
// Summary screen (FR-112, FR-113): one line per package, sign-in next steps
// and the shell profile lines to add. It has no ports: the caller maps the
// execute outcome into Input, so this package imports the domain only.
package summary

import (
	"fmt"
	"path/filepath"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// Action statuses, as execute.ActionStatus spells them.
const (
	StatusSucceeded      = "succeeded"
	StatusFailed         = "failed"
	StatusSkipped        = "skipped"
	StatusNotStarted     = "not-started"
	StatusAlreadyPresent = "already-present"
)

// NodeKegRef is the backend reference whose install adds the node@24 PATH line.
const NodeKegRef = "brew-formula:node@24"

// Result is how one plan action ended (from execute.ActionResult).
type Result struct {
	// ActionID is the plan action.
	ActionID int
	// PackageID is the catalog package.
	PackageID string
	// Status is one of the Status constants.
	Status string
	// Detail explains a failure or skip.
	Detail string
	// LogPath is the operation log.
	LogPath string
}

// Input is what the summary is built from.
type Input struct {
	// Catalog names the packages and says which need a sign-in.
	Catalog catalog.Catalog
	// Plan is the plan as run (execute.Outcome.Plan): its actions and
	// every exclusion, the re-plan's included.
	Plan plan.Plan
	// Results are the action results in plan order.
	Results []Result
	// Homebrew is where brew was found before the run.
	Homebrew platform.HomebrewInfo
	// BrewPrefix is the Homebrew prefix after the run (set when facman
	// installed it); empty means Homebrew.Prefix.
	BrewPrefix string
	// BrewInstalled is true when facman installed Homebrew in this run.
	BrewInstalled bool
}

// Line is the summary of one package.
type Line struct {
	// PackageID is the catalog package (homebrew for the bootstrap).
	PackageID string
	// Display is the catalog name, or the ID when there is none.
	Display string
	// Installed is true when the package was installed in this run.
	Installed bool
	// Text is "installed", "installed, sign-in required", "already present"
	// or "not installed: <why>".
	Text string
	// Detail is the reason or failure, if any.
	Detail string
	// LogPath is the log of the action that decided the line.
	LogPath string
}

// String renders the line as "<display>: <text> (<detail>)".
func (l Line) String() string {
	s := l.Display + ": " + l.Text
	if l.Detail != "" {
		s += " (" + l.Detail + ")"
	}
	return s
}

// Summary is the end-of-run summary.
type Summary struct {
	// Lines per package: actions in plan order, then exclusions.
	Lines []Line
	// NextSteps are the sign-in steps for installed packages that need one.
	NextSteps []string
	// ProfileLines are the lines to add to the shell profile.
	ProfileLines []string
}

var exclusionText = map[plan.ExclusionReason]string{
	plan.ExcludedConflict:            "not installed: conflict",
	plan.ExcludedDependency:          "not installed: dependency failed",
	plan.ExcludedDetectionIncomplete: "not installed: detection incomplete",
	plan.ExcludedUnavailableHere:     "not installed: unavailable here",
	plan.ExcludedNotSelected:         "not installed: not selected",
}

// Build returns the summary for in.
func Build(in Input) Summary {
	var s Summary
	excluded := map[string]plan.Exclusion{}
	for _, ex := range in.Plan.Excluded {
		excluded[ex.PackageID] = ex
	}
	var order []string
	byPkg := map[string][]Result{}
	for _, r := range in.Results {
		if _, ok := byPkg[r.PackageID]; !ok {
			order = append(order, r.PackageID)
		}
		byPkg[r.PackageID] = append(byPkg[r.PackageID], r)
	}
	done := map[string]bool{}
	nodeInstalled := false
	for _, id := range order {
		done[id] = true
		if ex, ok := excluded[id]; ok {
			s.Lines = append(s.Lines, exclusionLine(in.Catalog, ex))
			continue
		}
		line := packageLine(in.Catalog, id, byPkg[id])
		s.Lines = append(s.Lines, line)
		if !line.Installed {
			continue
		}
		if e, ok := in.Catalog.Entry(id); ok && e.SignInRequired {
			s.NextSteps = append(s.NextSteps, signIn(e))
		}
		for _, r := range byPkg[id] {
			if a, ok := in.Plan.Action(r.ActionID); ok && a.Kind == plan.ActionInstall {
				_, ref, _ := inventory.InstallRef(a.Commands)
				nodeInstalled = nodeInstalled || ref == NodeKegRef
			}
		}
	}
	for _, ex := range in.Plan.Excluded {
		if !done[ex.PackageID] {
			done[ex.PackageID] = true
			s.Lines = append(s.Lines, exclusionLine(in.Catalog, ex))
		}
	}
	s.ProfileLines = profileLines(in, nodeInstalled)
	return s
}

func display(c catalog.Catalog, id string) string {
	if e, ok := c.Entry(id); ok && e.Display != "" {
		return e.Display
	}
	if id == plan.HomebrewPackageID {
		return "Homebrew"
	}
	return id
}

func exclusionLine(c catalog.Catalog, ex plan.Exclusion) Line {
	text, ok := exclusionText[ex.Reason]
	if !ok {
		text = "not installed: " + string(ex.Reason)
	}
	return Line{PackageID: ex.PackageID, Display: display(c, ex.PackageID), Text: text, Detail: ex.Detail}
}

// packageLine folds a package's action results: the first that did not
// succeed decides; all succeeded means installed.
func packageLine(c catalog.Catalog, id string, results []Result) Line {
	l := Line{PackageID: id, Display: display(c, id)}
	for _, r := range results {
		l.LogPath, l.Detail = r.LogPath, r.Detail
		switch r.Status {
		case StatusSucceeded:
			continue
		case StatusAlreadyPresent:
			l.Text, l.Detail = "already present", ""
		case StatusFailed:
			l.Text = "not installed: failed"
		case StatusSkipped:
			l.Text = "not installed: dependency failed"
		case StatusNotStarted:
			l.Text, l.Detail = "not installed: cancelled", ""
		default:
			l.Text = "not installed: " + r.Status
		}
		return l
	}
	l.Installed, l.Text, l.Detail = true, "installed", ""
	if e, ok := c.Entry(id); ok && e.SignInRequired {
		l.Text = "installed, sign-in required"
	}
	return l
}

func signIn(e catalog.Entry) string {
	name := e.Display
	if name == "" {
		name = e.ID
	}
	if len(e.Detect.Commands) > 0 {
		return fmt.Sprintf("Sign in to %s: run %s and follow its prompts", name, e.Detect.Commands[0])
	}
	return fmt.Sprintf("Sign in to %s: open it and follow its prompts", name)
}

// profileLines: brew shellenv when Homebrew was installed now or is not
// on PATH; the node@24 keg on PATH when it was installed now (keg-only).
func profileLines(in Input, nodeInstalled bool) []string {
	prefix := in.BrewPrefix
	if prefix == "" {
		prefix = in.Homebrew.Prefix
	}
	if prefix == "" {
		return nil
	}
	var out []string
	if in.BrewInstalled || (in.Homebrew.Found && !in.Homebrew.OnPATH) {
		out = append(out, fmt.Sprintf(`eval "$(%s shellenv)"`, filepath.Join(prefix, "bin", "brew")))
	}
	if nodeInstalled {
		out = append(out, fmt.Sprintf(`export PATH="%s:$PATH"`, filepath.Join(prefix, "opt", "node@24", "bin")))
	}
	return out
}
