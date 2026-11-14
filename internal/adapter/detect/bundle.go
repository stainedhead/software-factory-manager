package detect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

// BundleOptions configure the app-bundle detector.
type BundleOptions struct {
	// AppDirs are scanned (one level) when mdfind finds nothing; default
	// /Applications and ~/Applications.
	AppDirs []string
	// Mdfind and Plutil name the programs; default mdfind and plutil.
	Mdfind, Plutil string
}

// BundleDetector finds macOS app bundles by bundle ID (FR-010).
type BundleDetector struct {
	runner Runner
	opts   BundleOptions
}

var _ scan.Detector = (*BundleDetector)(nil)

// NewBundleDetector returns the app-bundle detector.
func NewBundleDetector(runner Runner, opts BundleOptions) *BundleDetector {
	if opts.Mdfind == "" {
		opts.Mdfind = "mdfind"
	}
	if opts.Plutil == "" {
		opts.Plutil = "plutil"
	}
	return &BundleDetector{runner: runner, opts: opts}
}

// ID returns ownership.DetectorBundle.
func (*BundleDetector) ID() ownership.DetectorID { return ownership.DetectorBundle }

// Detect asks Spotlight for each bundle ID; when it finds nothing, it
// scans the app directories once, reading each Info.plist with plutil
// (which handles binary plists). Matching is by bundle ID only, so a
// renamed app ("T3 Code (Beta)") is still found. On anything but macOS it
// returns nothing. mdfind failing to run or timing out is an error.
func (d *BundleDetector) Detect(ctx context.Context, entries []catalog.Entry, env scan.Env) ([]ownership.Detection, error) {
	if env.Platform.OS != "darwin" {
		return nil, nil
	}
	loc := catalog.Locations{BrewPrefix: env.BrewPrefix, NPMPrefix: env.NPMPrefix, Home: env.Home}
	var index map[string]string
	var out []ownership.Detection
	for _, e := range entries {
		if len(e.Detect.BundleIDs) == 0 {
			continue
		}
		det := ownership.Detection{PackageID: e.ID, Detector: ownership.DetectorBundle, Status: ownership.DetectionOK}
		for _, id := range e.Detect.BundleIDs {
			app, err := d.mdfind(ctx, id, env.Vars)
			if err != nil {
				return nil, err
			}
			if app == "" {
				if index == nil {
					if index, err = d.scanApps(ctx, env); err != nil {
						return nil, err
					}
				}
				app = index[id]
			}
			if app == "" {
				continue
			}
			prov := catalog.ProvenanceOf(e, app, loc)
			if prov == catalog.ProvUnknown {
				prov = catalog.ProvAppBundle
			}
			det.Found, det.Path, det.ResolvedPath, det.Provenance = true, app, app, prov
			det.Version = d.plist(ctx, app, "CFBundleShortVersionString", env.Vars)
			break
		}
		out = append(out, det)
	}
	return out, nil
}

// mdfind returns the first .app path Spotlight reports for id, or "".
func (d *BundleDetector) mdfind(ctx context.Context, id string, vars map[string]string) (string, error) {
	if strings.ContainsAny(id, "'\\") {
		return "", nil
	}
	res, err := d.runner.Run(ctx, process.Cmd{Argv: []string{d.opts.Mdfind, "kMDItemCFBundleIdentifier == '" + id + "'"}, Env: vars, Timeout: scan.DetectorTimeout})
	if err != nil {
		if errors.Is(err, process.ErrNotFound) {
			return "", nil // no Spotlight: rely on the fallback scan
		}
		return "", fmt.Errorf("mdfind %s: %w", id, err)
	}
	if !res.Success() {
		return "", nil
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if line = strings.TrimSpace(line); strings.HasSuffix(line, ".app") {
			return line, nil
		}
	}
	return "", nil
}

// scanApps maps bundle IDs to app paths in the app directories.
func (d *BundleDetector) scanApps(ctx context.Context, env scan.Env) (map[string]string, error) {
	dirs := d.opts.AppDirs
	if dirs == nil {
		dirs = []string{"/Applications"}
		if env.Home != "" {
			dirs = append(dirs, filepath.Join(env.Home, "Applications"))
		}
	}
	index := map[string]string{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".app") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("scanning %s: %w", dir, err)
			}
			app := filepath.Join(dir, n)
			if id := d.plist(ctx, app, "CFBundleIdentifier", env.Vars); id != "" {
				if _, dup := index[id]; !dup {
					index[id] = app
				}
			}
		}
	}
	return index, nil
}

// plist reads one key of an app's Info.plist through plutil ("" on failure).
func (d *BundleDetector) plist(ctx context.Context, app, key string, vars map[string]string) string {
	res, err := d.runner.Run(ctx, process.Cmd{
		Argv: []string{d.opts.Plutil, "-extract", key, "raw", "-o", "-", filepath.Join(app, "Contents", "Info.plist")},
		Env:  vars, Timeout: VersionTimeout,
	})
	if err != nil || !res.Success() {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}
