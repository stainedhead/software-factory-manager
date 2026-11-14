// Package detect holds the PATH, app-bundle and npm detectors. Each finds
// catalog packages one way, without changing anything: PATH lookups with
// symlink resolution for provenance and a version probe (argv, 5 s), app
// bundles by bundle ID through mdfind with a fallback scan of the app
// directories, and global npm packages through `npm ls -g --json`.
package detect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/version"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

// VersionTimeout bounds each version probe.
const VersionTimeout = 5 * time.Second

// parallelism bounds concurrent probes inside one detector.
const parallelism = 8

// Runner runs read-only probes. Implemented by adapter/execrunner.
type Runner interface {
	Run(ctx context.Context, cmd process.Cmd) (process.Result, error)
}

// cltShims are /usr/bin programs that on macOS are Xcode CLT shims: running
// one without the CLT opens an install dialog (FR-020).
var cltShims = []string{"git"}

// PathOptions configure the PATH detector.
type PathOptions struct {
	// ShimDir is where macOS keeps the CLT shims; default /usr/bin.
	ShimDir string
	// VersionTimeout bounds each version probe; default VersionTimeout.
	VersionTimeout time.Duration
}

// PathDetector finds catalog commands on the child PATH.
type PathDetector struct {
	runner Runner
	opts   PathOptions
}

var _ scan.Detector = (*PathDetector)(nil)

// NewPathDetector returns the PATH detector.
func NewPathDetector(runner Runner, opts PathOptions) *PathDetector {
	if opts.ShimDir == "" {
		opts.ShimDir = "/usr/bin"
	}
	if opts.VersionTimeout == 0 {
		opts.VersionTimeout = VersionTimeout
	}
	return &PathDetector{runner: runner, opts: opts}
}

// ID returns ownership.DetectorPath.
func (*PathDetector) ID() ownership.DetectorID { return ownership.DetectorPath }

// Detect looks up every detect.commands name in every PATH directory, in
// order. Each match is one detection (the first is the PATH winner), with
// its resolved path, provenance and probed version. An entry with commands
// and no match gets one not-found detection. On macOS without the CLT,
// /usr/bin/git is never run and not reported.
func (d *PathDetector) Detect(ctx context.Context, entries []catalog.Entry, env scan.Env) ([]ownership.Detection, error) {
	loc := catalog.Locations{BrewPrefix: env.BrewPrefix, NPMPrefix: env.NPMPrefix, Home: env.Home}
	resolvedLoc := loc.Resolve(filepath.EvalSymlinks)
	results := make([][]ownership.Detection, len(entries))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for i, e := range entries {
		if len(e.Detect.Commands) == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = d.entry(ctx, e, env, loc, resolvedLoc)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []ownership.Detection
	for _, r := range results {
		out = append(out, r...)
	}
	return out, nil
}

// entry finds one entry. A resolved path is classified against the
// resolved locations, the PATH path against the locations as given.
func (d *PathDetector) entry(ctx context.Context, e catalog.Entry, env scan.Env, loc, resolvedLoc catalog.Locations) []ownership.Detection {
	var out []ownership.Detection
	seen := map[string]bool{}
	for _, name := range e.Detect.Commands {
		for _, dir := range env.Path {
			// Never probe, or run a version probe from, a relative or empty
			// PATH entry (review FR-002).
			if !filepath.IsAbs(dir) {
				continue
			}
			p := filepath.Join(dir, name)
			if seen[p] || !isExecutable(p) {
				continue
			}
			seen[p] = true
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				resolved = p
			}
			if d.cltShim(env, name, resolved) {
				continue
			}
			prov := catalog.ProvenanceOf(e, resolved, resolvedLoc)
			if prov == catalog.ProvUnknown {
				prov = catalog.ProvenanceOf(e, resolved, loc)
			}
			if prov == catalog.ProvUnknown {
				prov = catalog.ProvenanceOf(e, p, loc)
			}
			out = append(out, ownership.Detection{
				PackageID: e.ID, Detector: ownership.DetectorPath, Status: ownership.DetectionOK, Found: true,
				Path: p, ResolvedPath: resolved, Provenance: prov,
				Version:    probeVersion(ctx, d.runner, e, name, p, env.Vars, d.opts.VersionTimeout),
				PathWinner: len(out) == 0,
			})
		}
	}
	if len(out) == 0 {
		out = append(out, ownership.Detection{PackageID: e.ID, Detector: ownership.DetectorPath, Status: ownership.DetectionOK})
	}
	return out
}

// cltShim reports a macOS CLT shim that must not be run because the CLT
// state is not known to be installed.
func (d *PathDetector) cltShim(env scan.Env, name, resolved string) bool {
	if env.Platform.OS != "darwin" || (env.CLT != nil && *env.CLT) {
		return false
	}
	if !slices.Contains(cltShims, name) {
		return false
	}
	dir := filepath.Dir(resolved)
	if dir == filepath.Clean(d.opts.ShimDir) {
		return true
	}
	shim, err := filepath.EvalSymlinks(d.opts.ShimDir)
	return err == nil && dir == shim
}

// probeVersion runs the entry's version probe against the install at p
// (the probe's program is replaced by p when it names the command) and
// extracts the first regex group from stdout, then stderr, when it parses
// as a version. Any failure,
// timeout or garbage gives "".
func probeVersion(ctx context.Context, r Runner, e catalog.Entry, name, p string, vars map[string]string, timeout time.Duration) string {
	v := e.Detect.Version
	if v == nil || len(v.Cmd) == 0 || v.Regex == "" {
		return ""
	}
	re, err := regexp.Compile(v.Regex)
	if err != nil {
		return ""
	}
	argv := slices.Clone(v.Cmd)
	if argv[0] == name {
		argv[0] = p
	}
	res, err := r.Run(ctx, process.Cmd{Argv: argv, Env: vars, Timeout: timeout})
	if err != nil {
		return ""
	}
	for _, out := range []string{res.Stdout, res.Stderr} {
		m := re.FindStringSubmatch(strings.TrimSpace(out))
		if len(m) < 2 {
			continue
		}
		if _, err := version.Parse(m[1]); err == nil {
			return m[1]
		}
	}
	return ""
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0
}
