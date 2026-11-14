package brew

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/conflicts"
	"github.com/stainedhead/software-factory-manager/internal/usecase/doctor"
	"github.com/stainedhead/software-factory-manager/internal/usecase/planning"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// Target is the Homebrew a backend talks to. It changes when facman
// installs Homebrew mid-run, so backends read it through a function.
type Target struct {
	// Path is the brew executable ("" when Homebrew is missing).
	Path string
	// Prefix is the Homebrew prefix ("" when Homebrew is missing).
	Prefix string
	// Env are the child environment overrides (shellenv, node@24 PATH).
	Env map[string]string
}

// BackendOptions configure a Backend.
type BackendOptions struct {
	// Target returns the current Homebrew; required.
	Target func() Target
	// AppDir is where casks install app bundles; default /Applications.
	AppDir string
}

// Backend is the brew-formula or brew-cask install backend.
type Backend struct {
	id     catalog.BackendID
	runner Runner
	opts   BackendOptions
}

var (
	_ planning.Backend  = (*Backend)(nil)
	_ conflicts.Backend = (*Backend)(nil)
	_ doctor.TapChecker = (*Backend)(nil)
)

// NewFormula returns the brew-formula backend.
func NewFormula(runner Runner, opts BackendOptions) *Backend {
	return newBackend(catalog.BackendBrewFormula, runner, opts)
}

// NewCask returns the brew-cask backend.
func NewCask(runner Runner, opts BackendOptions) *Backend {
	return newBackend(catalog.BackendBrewCask, runner, opts)
}

func newBackend(id catalog.BackendID, runner Runner, opts BackendOptions) *Backend {
	if opts.AppDir == "" {
		opts.AppDir = "/Applications"
	}
	return &Backend{id: id, runner: runner, opts: opts}
}

// ID names the backend.
func (b *Backend) ID() catalog.BackendID { return b.id }

// Available reports whether Homebrew is located.
func (b *Backend) Available(context.Context) bool { return b.opts.Target().Path != "" }

// Capabilities says casks may need sudo (pkg and installer artifacts).
func (b *Backend) Capabilities() planning.Caps {
	return planning.Caps{NeedsSudo: b.id == catalog.BackendBrewCask}
}

// read runs a read-only brew command (no network: D-15).
func (b *Backend) read(ctx context.Context, args ...string) (process.Result, error) {
	t := b.opts.Target()
	env := maps.Clone(t.Env)
	if env == nil {
		env = map[string]string{}
	}
	maps.Copy(env, ReadOnlyEnv())
	res, err := b.runner.Run(ctx, process.Cmd{Argv: append([]string{t.Path}, args...), Env: env, Timeout: readTimeout})
	if err != nil {
		return res, fmt.Errorf("brew %s: %w", strings.Join(args, " "), err)
	}
	if !res.Success() {
		return res, fmt.Errorf("brew %s exited %d: %s", strings.Join(args, " "), res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return res, nil
}

// info runs brew info --json=v2 for one item.
func (b *Backend) info(ctx context.Context, id string) (Info, error) {
	kind := "--formula"
	if b.id == catalog.BackendBrewCask {
		kind = "--cask"
	}
	res, err := b.read(ctx, "info", "--json=v2", kind, id)
	if err != nil {
		return Info{}, err
	}
	return ParseInfo([]byte(res.Stdout))
}

// InstallCommand is the exact install argv for step (never --force,
// --overwrite or link).
func InstallCommand(step catalog.InstallStep) []string {
	if step.Backend == catalog.BackendBrewCask {
		return []string{"brew", "install", "--cask", step.ID}
	}
	return []string{"brew", "install", step.ID}
}

// InstallActions returns, for a tap item, a tap action and an item-trust
// action (each omitted when TapState says it is already in place), then
// the install action. A cask install needs sudo when its artifacts include
// pkg or installer, and also when they cannot be read (Homebrew missing,
// tap not added yet): handing over the terminal is the safe default.
func (b *Backend) InstallActions(ctx context.Context, entry catalog.Entry, step catalog.InstallStep) ([]plan.Action, error) {
	if step.Backend != b.id {
		return nil, fmt.Errorf("backend %s cannot install a %s step", b.id, step.Backend)
	}
	var out []plan.Action
	if step.Tap != "" {
		tapped, trusted, err := b.TapState(ctx, step)
		if err != nil {
			tapped, trusted = false, false // unknown counts as not in place
		}
		tt := &plan.TapTrustStep{Tap: step.Tap, ItemKind: b.itemKind(), Item: step.ID, PackageID: entry.ID}
		if !tapped {
			out = append(out, plan.Action{Kind: plan.ActionTap, PackageID: entry.ID, TapTrust: tt,
				Commands: []plan.Command{{Argv: []string{"brew", "tap", step.Tap}, Mutating: true}}})
		}
		if !trusted && step.Trust == catalog.TrustItem {
			out = append(out, plan.Action{Kind: plan.ActionTrust, PackageID: entry.ID, TapTrust: tt,
				Commands: []plan.Command{{Argv: []string{"brew", "trust", "--" + b.itemKind(), step.ID}, Mutating: true}}})
		}
	}
	install := plan.Command{Argv: InstallCommand(step), Mutating: true}
	if b.id == catalog.BackendBrewCask {
		install.NeedsSudo = b.caskNeedsSudo(ctx, step)
		install.Interactive = install.NeedsSudo
	}
	out = append(out, plan.Action{Kind: plan.ActionInstall, PackageID: entry.ID, Commands: []plan.Command{install}})
	return out, nil
}

func (b *Backend) itemKind() string {
	if b.id == catalog.BackendBrewCask {
		return plan.ItemCask
	}
	return plan.ItemFormula
}

func (b *Backend) caskNeedsSudo(ctx context.Context, step catalog.InstallStep) bool {
	if b.opts.Target().Path == "" {
		return true
	}
	info, err := b.info(ctx, step.ID)
	if err != nil {
		return true
	}
	c, ok := info.Cask(step.ID)
	return !ok || c.NeedsSudo()
}

// LinkPaths returns the paths installing step would write (FR-015): for a
// cask, its binary targets under {prefix}/bin and app bundles under the
// app dir, from brew info; for a formula, each detect.commands name under
// {prefix}/bin, or nothing when the formula is keg-only. When brew info
// cannot be read (a tap item before its tap is added, D-9), the entry's
// detect.commands under {prefix}/bin are used. With Homebrew missing there
// is no prefix yet and nothing is returned; execute re-checks after the
// bootstrap.
func (b *Backend) LinkPaths(ctx context.Context, entry catalog.Entry, step catalog.InstallStep) ([]string, error) {
	t := b.opts.Target()
	if t.Prefix == "" || t.Path == "" {
		return nil, nil
	}
	commandPaths := func() []string {
		var out []string
		for _, c := range entry.Detect.Commands {
			out = append(out, filepath.Join(t.Prefix, "bin", c))
		}
		return out
	}
	info, err := b.info(ctx, step.ID)
	if err != nil {
		if step.Tap != "" {
			return commandPaths(), nil
		}
		return nil, fmt.Errorf("listing link paths of %s: %w", step.ID, err)
	}
	if b.id == catalog.BackendBrewFormula {
		if f, ok := info.Formula(step.ID); ok && f.KegOnly {
			return nil, nil
		}
		return commandPaths(), nil
	}
	c, ok := info.Cask(step.ID)
	if !ok {
		return commandPaths(), nil
	}
	var out []string
	for _, bin := range c.Binaries() {
		out = append(out, filepath.Join(t.Prefix, "bin", bin))
	}
	for _, app := range c.Apps() {
		out = append(out, filepath.Join(b.opts.AppDir, app))
	}
	return out, nil
}

// TapState reports whether step's tap is tapped and its item trusted, from
// `brew tap` and `brew trust` listings (read-only). An item counts as
// trusted only when the listing names it in full; unreadable output means
// not trusted. With Homebrew missing both are false.
func (b *Backend) TapState(ctx context.Context, step catalog.InstallStep) (tapped, trusted bool, err error) {
	if step.Tap == "" {
		return true, true, nil
	}
	if b.opts.Target().Path == "" {
		return false, false, nil
	}
	taps, err := b.read(ctx, "tap")
	if err != nil {
		return false, false, fmt.Errorf("checking tap %s: %w", step.Tap, err)
	}
	tapped = slices.Contains(ParseTaps(taps.Stdout), step.Tap)
	if !tapped {
		return false, false, nil
	}
	list, err := b.read(ctx, "trust")
	if err != nil {
		return true, false, fmt.Errorf("checking trust of %s: %w", step.ID, err)
	}
	return true, slices.Contains(ParseTrusted(list.Stdout), step.ID), nil
}
