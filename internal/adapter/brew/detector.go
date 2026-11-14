package brew

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

// Detector finds installed formulae and casks with one read-only
// `brew info --json=v2 --installed` call (FR-010).
type Detector struct {
	runner Runner
}

var _ scan.Detector = (*Detector)(nil)

// NewDetector returns the brew detector.
func NewDetector(runner Runner) *Detector { return &Detector{runner: runner} }

// ID returns ownership.DetectorBrew.
func (*Detector) ID() ownership.DetectorID { return ownership.DetectorBrew }

// Detect maps installed formulae and casks to the entries that install
// them (by install id; tap items by full name). With Homebrew missing it
// returns nothing. A failed call, a non-zero exit or invalid JSON is an
// error, which scan turns into unknown detections.
func (d *Detector) Detect(ctx context.Context, entries []catalog.Entry, env scan.Env) ([]ownership.Detection, error) {
	if env.BrewPath == "" {
		return nil, nil
	}
	vars := maps.Clone(env.Vars)
	if vars == nil {
		vars = map[string]string{}
	}
	maps.Copy(vars, ReadOnlyEnv())
	res, err := d.runner.Run(ctx, process.Cmd{Argv: []string{env.BrewPath, "info", "--json=v2", "--installed"}, Env: vars, Timeout: scan.DetectorTimeout})
	if err != nil {
		return nil, fmt.Errorf("brew info: %w", err)
	}
	if !res.Success() {
		return nil, fmt.Errorf("brew info exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	info, err := ParseInfo([]byte(res.Stdout))
	if err != nil {
		return nil, err
	}
	var out []ownership.Detection
	for _, e := range entries {
		steps := brewSteps(e)
		if len(steps) == 0 {
			continue
		}
		det := ownership.Detection{PackageID: e.ID, Detector: ownership.DetectorBrew, Status: ownership.DetectionOK}
		for _, s := range steps {
			if found, ok := lookup(info, s); ok {
				det = found
				det.PackageID = e.ID
				break
			}
		}
		out = append(out, det)
	}
	return out, nil
}

func lookup(info Info, s catalog.InstallStep) (ownership.Detection, bool) {
	d := ownership.Detection{Detector: ownership.DetectorBrew, Status: ownership.DetectionOK, Found: true, BackendRef: Ref(s)}
	if s.Backend == catalog.BackendBrewCask {
		c, ok := info.Cask(s.ID)
		if !ok || c.Installed == nil {
			return ownership.Detection{}, false
		}
		d.Provenance, d.Version, d.Path = catalog.ProvBrewCask, c.Version(), caskPath(c)
		return d, true
	}
	f, ok := info.Formula(s.ID)
	if !ok || len(f.Installed) == 0 {
		return ownership.Detection{}, false
	}
	d.Provenance, d.Version = catalog.ProvBrew, f.Version()
	return d, true
}

// caskPath is the path Homebrew reports for an installed cask's main link:
// the first binary or command_wrapper target, else the first app target,
// else "" (Homebrew 6 shapes, or nothing linked).
func caskPath(c Cask) string {
	app := ""
	for _, l := range c.Links() {
		switch {
		case l.Target == "":
		case l.Kind == "binary" || l.Kind == "command_wrapper":
			return l.Target
		case l.Kind == "app" && app == "":
			app = l.Target
		}
	}
	return app
}

// Ref is the backend reference recorded for a brew step, for example
// brew-cask:claude-code or brew-formula:jq.
func Ref(s catalog.InstallStep) string { return string(s.Backend) + ":" + s.ID }

// brewSteps returns the entry's distinct brew install steps, over every
// platform, in a stable order.
func brewSteps(e catalog.Entry) []catalog.InstallStep {
	var out []catalog.InstallStep
	seen := map[string]bool{}
	for _, key := range []platform.Key{platform.Darwin, platform.Linux, platform.WSL} {
		for _, s := range e.Install[key] {
			if !s.Backend.IsBrew() || seen[Ref(s)] {
				continue
			}
			seen[Ref(s)] = true
			out = append(out, s)
		}
	}
	return out
}
