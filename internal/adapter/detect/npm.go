package detect

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

// NPMDetector finds global npm packages (FR-010).
type NPMDetector struct {
	runner Runner
}

var _ scan.Detector = (*NPMDetector)(nil)

// NewNPMDetector returns the npm detector.
func NewNPMDetector(runner Runner) *NPMDetector { return &NPMDetector{runner: runner} }

// ID returns ownership.DetectorNPM.
func (*NPMDetector) ID() ownership.DetectorID { return ownership.DetectorNPM }

type npmList struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

// Detect runs `npm ls -g --json --depth=0` once and maps the listed
// packages to entries by detect.npm_package. Without npm on PATH it
// returns nothing. npm exits non-zero for some tree problems while still
// printing the list, so only unreadable output is an error.
func (d *NPMDetector) Detect(ctx context.Context, entries []catalog.Entry, env scan.Env) ([]ownership.Detection, error) {
	npm := ""
	for _, dir := range env.Path {
		if p := filepath.Join(dir, "npm"); dir != "" && isExecutable(p) {
			npm = p
			break
		}
	}
	if npm == "" {
		return nil, nil
	}
	res, err := d.runner.Run(ctx, process.Cmd{Argv: []string{npm, "ls", "-g", "--json", "--depth=0"}, Env: env.Vars, Timeout: scan.DetectorTimeout})
	if err != nil {
		return nil, fmt.Errorf("npm ls: %w", err)
	}
	var list npmList
	if err := json.Unmarshal([]byte(res.Stdout), &list); err != nil {
		return nil, fmt.Errorf("npm ls returned invalid JSON (exit %d): %w", res.ExitCode, err)
	}
	var out []ownership.Detection
	for _, e := range entries {
		pkg := e.Detect.NPMPackage
		if pkg == "" {
			continue
		}
		det := ownership.Detection{PackageID: e.ID, Detector: ownership.DetectorNPM, Status: ownership.DetectionOK}
		if dep, ok := list.Dependencies[pkg]; ok {
			det.Found, det.Provenance, det.Version, det.BackendRef = true, catalog.ProvNPM, dep.Version, "npm:"+pkg
			if env.NPMPrefix != "" {
				det.Path = filepath.Join(env.NPMPrefix, "lib", "node_modules", pkg)
				det.ResolvedPath = det.Path
			}
		}
		out = append(out, det)
	}
	return out, nil
}
