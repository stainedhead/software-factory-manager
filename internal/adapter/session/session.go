// Package session composes the use cases into the steps a facman command
// runs: bootstrap check, state directory and lock, catalog, scan, conflict
// pre-flight, plan, execute and summary (architecture.md, data flow). The
// CLI runs a whole command with Run; the TUI drives the same steps one
// screen at a time through Start, Plan and Execute.
package session

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/bootstrap"
	"github.com/stainedhead/software-factory-manager/internal/usecase/conflicts"
	"github.com/stainedhead/software-factory-manager/internal/usecase/doctor"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/planning"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/status"
	"github.com/stainedhead/software-factory-manager/internal/usecase/summary"
)

// ReviewInstallerPath is the placeholder path Review shows for the
// Homebrew installer; the real temp file exists only after consent.
const ReviewInstallerPath = "<temp file>"

// npmPrefixTimeout bounds `npm prefix -g`.
const npmPrefixTimeout = 5 * time.Second

// Bootstrapper runs the startup bootstrap check. Implemented by
// usecase/bootstrap.Service.
type Bootstrapper interface {
	Check(ctx context.Context) (platform.BootstrapReport, error)
}

// Inventory is the inventory store: everything scan, execute and status
// use. Implemented by adapter/sqlite.
type Inventory interface {
	scan.Inventory
	execute.Inventory
	status.Inventory
}

// StateDir is the state directory (logs, lock). Implemented by
// adapter/statedir.
type StateDir interface {
	execute.StateDir
}

// State is an opened state directory and inventory.
type State struct {
	// Dir is the state directory.
	Dir StateDir
	// Inventory is the opened store.
	Inventory Inventory
	// Close releases the store.
	Close func() error
}

// Backend is an install backend (brew-formula, brew-cask). Implemented by
// adapter/brew.
type Backend interface {
	planning.Backend
}

// Installer is the Homebrew installer; Cleanup removes what it downloaded.
// Implemented by adapter/installer.
type Installer interface {
	execute.HomebrewInstaller
	Cleanup()
}

// Deps are the adapters a session composes, built in cmd/facman.
type Deps struct {
	// Bootstrap runs the bootstrap check.
	Bootstrap Bootstrapper
	// OpenState opens the state directory and inventory (exit 4 on failure).
	OpenState func(ctx context.Context) (*State, error)
	// LoadCatalog loads the embedded catalog (dir "") or a --catalog dir.
	LoadCatalog func(ctx context.Context, dir string) (catalog.Catalog, error)
	// Detectors run during scan.
	Detectors []scan.Detector
	// Backends by ID; their keys are the implemented backends.
	Backends map[catalog.BackendID]Backend
	// Probe inspects conflicting paths.
	Probe conflicts.PathProbe
	// Runner runs plan commands and the npm prefix probe.
	Runner execute.CommandRunner
	// Installer downloads the Homebrew installer.
	Installer Installer
	// Clock tells the time.
	Clock execute.Clock
	// User names the consenting OS user.
	User execute.UserInfo
	// Machine holds the Homebrew the adapters talk to.
	Machine *Machine
	// Home is the user's home directory.
	Home string
	// BasePath is the base PATH (before shellenv).
	BasePath string
	// DetectorTimeout is the per-detector timeout; zero is the default.
	DetectorTimeout time.Duration
}

// Session runs facman commands.
type Session struct {
	d Deps
}

// New returns a session over d.
func New(d Deps) *Session {
	if d.Machine == nil {
		d.Machine = &Machine{}
	}
	return &Session{d: d}
}

// Request is one command.
type Request struct {
	// Command is scan, plan, apply, status or doctor (tui-run for the TUI).
	Command platform.Command
	// Profiles are the selected profiles (plan, apply, TUI).
	Profiles []string
	// With are the optional packages and choice picks to include.
	With []string
	// CatalogDir is the --catalog directory; empty means embedded.
	CatalogDir string
	// DryRun prints the plan and changes nothing (apply).
	DryRun bool
	// Confirmer asks for consent (apply, TUI).
	Confirmer execute.Confirmer
	// Progress receives run events (apply, TUI).
	Progress execute.Progress
	// Runner, when set, runs the plan commands instead of Deps.Runner: the
	// TUI wraps the system runner so a sudo command gets the terminal
	// through tea.ExecProcess (AD-16). Detection never uses it.
	Runner execute.CommandRunner
}

// Mutates reports whether the request may change the machine: apply
// without --dry-run, or the TUI's Run step.
func (r Request) Mutates() bool { return r.Command.Mutates() && !r.DryRun }

// bootstrapCommand is the command the bootstrap findings are judged for:
// a dry run is judged like plan, since it changes nothing.
func (r Request) bootstrapCommand() platform.Command {
	if r.Command.Mutates() && r.DryRun {
		return platform.CommandPlan
	}
	return r.Command
}

// Result is what a command produced; parts it did not reach are nil.
type Result struct {
	// Bootstrap is the bootstrap report.
	Bootstrap *platform.BootstrapReport
	// Catalog is the loaded catalog.
	Catalog *catalog.Catalog
	// Desired is the resolved profile selection.
	Desired *catalog.DesiredSet
	// Scan is the scan result (conflicts applied for plan and apply).
	Scan *scan.Result
	// Plan is the plan (as run, for apply).
	Plan *plan.Plan
	// Outcome is the apply outcome.
	Outcome *execute.Outcome
	// Status is the status view.
	Status *status.Report
	// Doctor is the doctor report.
	Doctor *doctor.Report
	// Summary is the apply summary.
	Summary *summary.Summary
}

// Run runs one whole command and returns what it produced and the error
// that decides the exit code. Results are kept on error, so --json can
// still report what was found.
func (s *Session) Run(ctx context.Context, req Request) (Result, error) {
	r, err := s.Start(ctx, req)
	if err != nil {
		return r.result(), err
	}
	defer r.Close()
	switch req.Command {
	case platform.CommandStatus:
		err = r.Status(ctx)
	case platform.CommandDoctor:
		err = r.Doctor(ctx)
	case platform.CommandPlan, platform.CommandApply:
		if err = r.Plan(ctx, req.Profiles, req.With); err == nil && req.Mutates() {
			err = r.Execute(ctx)
		}
	}
	return r.result(), err
}

// Run is one command in progress: the state stays open (and the lock
// held, for a mutating request) until Close.
type Run struct {
	s      *Session
	req    Request
	res    Result
	state  *State
	unlock func()
	closed bool
}

func (r *Run) result() Result {
	if r == nil {
		return Result{}
	}
	return r.res
}

// Result returns what the run produced so far.
func (r *Run) Result() Result { return r.result() }

// Close releases the lock and the state.
func (r *Run) Close() {
	if r == nil || r.closed {
		return
	}
	r.closed = true
	if r.unlock != nil {
		r.unlock()
	}
	if r.state != nil && r.state.Close != nil {
		_ = r.state.Close()
	}
}

// Start runs the steps every command shares: the bootstrap check (fatal
// findings for this command are exit 4), the state directory (and, for a
// mutating request, the exclusive lock: held elsewhere is exit 4 naming
// the PID), the catalog (exit 3 when invalid) and the scan. On error the
// returned Run holds what was found and is already closed.
func (s *Session) Start(ctx context.Context, req Request) (*Run, error) {
	r := &Run{s: s, req: req}
	if err := r.start(ctx); err != nil {
		r.Close()
		return r, err
	}
	return r, nil
}

func (r *Run) start(ctx context.Context) error {
	d := r.s.d
	rep, err := d.Bootstrap.Check(ctx)
	if err != nil {
		return asKind(failure.KindBootstrap, "checking the machine", err)
	}
	r.res.Bootstrap = &rep
	d.Machine.Set(rep.Homebrew, bootstrap.ChildEnv(rep, d.BasePath))
	if fatal := rep.Fatal(r.req.bootstrapCommand()); len(fatal) > 0 {
		return fatalError(fatal)
	}
	st, err := d.OpenState(ctx)
	if err != nil {
		return asKind(failure.KindBootstrap, "opening the state directory", err)
	}
	r.state = st
	if r.req.Mutates() {
		unlock, holder, err := st.Dir.TryLock()
		if err != nil {
			return failure.Bootstrap("taking the state lock: %w", err)
		}
		if unlock == nil {
			return failure.Bootstrap("another facman (%s) is changing this machine", inventory.HolderLabel(holder)).
				WithFix("wait for it to finish, then run facman again")
		}
		r.unlock = unlock
	}
	cat, err := d.LoadCatalog(ctx, r.req.CatalogDir)
	if err != nil {
		return asKind(failure.KindValidation, "loading the catalog", err)
	}
	r.res.Catalog = &cat
	sr, err := r.s.scanner(st).Run(ctx, scan.Input{Catalog: cat, Env: r.s.scanEnv(ctx, rep), HoldsLock: r.unlock != nil, ReadOnly: r.req.DryRun})
	if err != nil {
		return asKind(failure.KindBootstrap, "scanning", err)
	}
	r.res.Scan = &sr
	return nil
}

func fatalError(fatal []platform.Finding) error {
	f := fatal[0]
	err := failure.Bootstrap("%s", f.Message).WithFix(f.Fix)
	if len(fatal) > 1 {
		var more []string
		for _, g := range fatal[1:] {
			more = append(more, g.Message)
		}
		err = err.WithDetails(more...)
	}
	return err
}

// asKind keeps a typed failure as it is and gives an untyped one kind.
func asKind(kind failure.Kind, doing string, err error) error {
	if _, ok := failure.KindOf(err); ok {
		return err
	}
	return (&failure.Error{Kind: kind, Msg: doing}).Wrap(err)
}

func (s *Session) implemented() catalog.BackendSet {
	set := catalog.BackendSet{}
	for id := range s.d.Backends {
		set[id] = true
	}
	return set
}

func (s *Session) scanner(st *State) *scan.Service {
	return &scan.Service{Detectors: s.d.Detectors, Inventory: st.Inventory, State: st.Dir, Now: s.d.Clock.Now,
		Implemented: s.implemented(), Timeout: s.d.DetectorTimeout}
}

func (s *Session) conflictService() *conflicts.Service {
	bs := map[catalog.BackendID]conflicts.Backend{}
	for id, b := range s.d.Backends {
		bs[id] = b
	}
	return &conflicts.Service{Backends: bs, Probe: s.d.Probe}
}

// scanEnv is the detection environment: the child PATH and shellenv
// variables plus the read-only Homebrew variables (no network, D-15).
func (s *Session) scanEnv(ctx context.Context, rep platform.BootstrapReport) scan.Env {
	vars := s.d.Machine.Env()
	if vars == nil {
		vars = map[string]string{}
	}
	vars["HOMEBREW_NO_AUTO_UPDATE"], vars["HOMEBREW_NO_ENV_HINTS"] = "1", "1"
	path := vars["PATH"]
	if path == "" {
		path = s.d.BasePath
	}
	env := scan.Env{Platform: rep.Platform, Home: s.d.Home, Path: filepath.SplitList(path), BrewPrefix: rep.Homebrew.Prefix,
		BrewPath: rep.Homebrew.Path, CLT: rep.CLT, Vars: vars}
	if rep.Node.NPM {
		env.NPMPrefix = s.npmPrefix(ctx, vars)
	}
	s.d.Machine.setNPMPrefix(env.NPMPrefix)
	return env
}

func (s *Session) npmPrefix(ctx context.Context, vars map[string]string) string {
	res, err := s.d.Runner.Run(ctx, process.Cmd{Argv: []string{"npm", "prefix", "-g"}, Env: vars, Timeout: npmPrefixTimeout})
	if err != nil || !res.Success() {
		return ""
	}
	p := strings.TrimSpace(res.Stdout)
	if !filepath.IsAbs(p) {
		return ""
	}
	return p
}

// Status builds the status view.
func (r *Run) Status(ctx context.Context) error {
	rep, err := (&status.Service{Inventory: r.state.Inventory}).Run(ctx, *r.res.Catalog, r.res.Scan.Statuses)
	if err != nil {
		return asKind(failure.KindBootstrap, "building the status view", err)
	}
	r.res.Status = &rep
	return nil
}

// Doctor runs the whole-catalog conflict and tap-trust check; its error is
// exit 2 when anything needs fixing.
func (r *Run) Doctor(ctx context.Context) error {
	s := r.s
	findings := slices.Concat(r.res.Bootstrap.Findings, r.res.Scan.Findings)
	rep := (&doctor.Service{Conflicts: s.conflictService(), Taps: tapChecker(s.d.Backends), Implemented: s.implemented()}).Run(ctx, doctor.Input{
		Catalog: *r.res.Catalog, Platform: r.res.Bootstrap.Platform, Statuses: r.res.Scan.StatusMap(), Findings: findings})
	r.res.Doctor = &rep
	return rep.Err()
}

// tapChecker asks the backend of each step for its tap state.
type tapChecker map[catalog.BackendID]Backend

func (t tapChecker) TapState(ctx context.Context, step catalog.InstallStep) (bool, bool, error) {
	b, ok := t[step.Backend]
	if !ok {
		return false, false, fmt.Errorf("no backend %s", step.Backend)
	}
	tapped, trusted, err := b.TapState(ctx, step)
	if err != nil {
		return false, false, fmt.Errorf("reading the tap state of %s: %w", step.Tap, err)
	}
	return tapped, trusted, nil
}

// Plan resolves the profiles (exit 3 on unknown IDs or conflicting pins),
// runs the conflict pre-flight for the selected absent brew items and
// builds the plan.
func (r *Run) Plan(ctx context.Context, profiles, with []string) error {
	return r.PlanSelection(ctx, catalog.Selection{Profiles: profiles, With: with})
}

// Resolve resolves a selection against the run's catalog without planning
// (the TUI Plan screen re-resolves on every toggle). Errors are exit 3.
func (r *Run) Resolve(sel catalog.Selection) (catalog.DesiredSet, error) {
	d, err := catalog.ResolveSelection(*r.res.Catalog, sel)
	if err != nil {
		return d, asKind(failure.KindValidation, "resolving the profiles", err)
	}
	return d, nil
}

// PlanSelection is Plan for a selection with deselected items (the TUI
// Plan screen). It may be called again with a changed selection: the
// pre-flight then checks the newly selected items too.
func (r *Run) PlanSelection(ctx context.Context, sel catalog.Selection) error {
	cat := *r.res.Catalog
	desired, err := catalog.ResolveSelection(cat, sel)
	if err != nil {
		return asKind(failure.KindValidation, "resolving the profiles", err)
	}
	r.res.Desired = &desired
	r.preflight(ctx, desired)
	p, err := (&planning.Service{Backends: planningBackends(r.s.d.Backends), InstallerCommand: r.s.d.Installer.Command(ReviewInstallerPath)}).Build(ctx, planning.Input{
		Catalog: cat, Desired: desired, Statuses: r.res.Scan.StatusMap(), Bootstrap: *r.res.Bootstrap})
	r.res.Plan = &p
	if err != nil {
		return asKind(failure.KindOperation, "building the plan", err)
	}
	return nil
}

func planningBackends(bs map[catalog.BackendID]Backend) map[catalog.BackendID]planning.Backend {
	out := map[catalog.BackendID]planning.Backend{}
	for id, b := range bs {
		out[id] = b
	}
	return out
}

// preflight checks the selected absent brew items for link conflicts and
// marks them conflict; an item that cannot be checked is marked
// incomplete, so it is never planned.
func (r *Run) preflight(ctx context.Context, desired catalog.DesiredSet) {
	cat, sr := *r.res.Catalog, r.res.Scan
	items := r.s.brewItems(cat, desired.SelectedIDs(), sr.StatusMap(), r.res.Bootstrap.Platform)
	cr := r.s.conflictService().CheckAll(ctx, items)
	sr.ApplyConflicts(cr.Conflicts)
	for i, st := range sr.Statuses {
		if reason, ok := cr.Failed[st.PackageID]; ok {
			sr.Statuses[i].Incomplete = true
			sr.Statuses[i].Detections = append(sr.Statuses[i].Detections, ownership.Detection{PackageID: st.PackageID,
				Status: ownership.DetectionUnknown, Reason: "conflict pre-flight: " + reason})
		}
	}
}

// brewItems are the absent, offered, completely detected packages among
// ids that a brew backend would install here.
func (s *Session) brewItems(cat catalog.Catalog, ids []string, statuses map[string]ownership.PackageStatus, p platform.Platform) []conflicts.Item {
	var items []conflicts.Item
	for _, id := range ids {
		st, ok := statuses[id]
		e, inCat := cat.Entry(id)
		if !ok || !inCat || st.State != ownership.StateAbsent || st.Incomplete || !st.Offered {
			continue
		}
		avail := catalog.SelectInstall(e, p, s.implemented())
		if avail.Step != nil && avail.Step.Backend.IsBrew() {
			items = append(items, conflicts.Item{Entry: e, Step: *avail.Step})
		}
	}
	return items
}

// Execute asks for consent and runs the plan, then builds the summary.
// A decline is exit 0 with nothing written; otherwise the error is the
// outcome's (exit 1 when anything was not installed, 4 when the Homebrew
// installer could not be fetched).
func (r *Run) Execute(ctx context.Context) error {
	d := r.s.d
	if d.Installer != nil {
		defer d.Installer.Cleanup()
	}
	runner := d.Runner
	if r.req.Runner != nil {
		runner = r.req.Runner
	}
	ex := &execute.Service{Runner: runner, Inventory: r.state.Inventory, Installer: d.Installer, State: r.state.Dir, Clock: d.Clock,
		User: d.User, Progress: r.req.Progress, Confirmer: r.req.Confirmer, Env: d.Machine.Env(), Brew: d.Machine.Brew().Path,
		Replanner: &Replanner{Session: r.s, Catalog: *r.res.Catalog, State: r.state}}
	out, err := ex.Apply(ctx, *r.res.Plan, execute.Options{DryRun: r.req.DryRun})
	r.res.Outcome = &out
	r.res.Plan = &out.Plan
	if err != nil && out.Consent.At.IsZero() {
		return asKind(failure.KindOperation, "asking for consent", err)
	}
	if out.Declined || out.DryRun {
		return nil
	}
	sum := summary.Build(summary.Input{Catalog: *r.res.Catalog, Plan: out.Plan, Results: summaryResults(out.Results),
		Homebrew: r.res.Bootstrap.Homebrew, BrewPrefix: d.Machine.Brew().Prefix, BrewInstalled: out.BrewInstalled})
	r.res.Summary = &sum
	return out.Err()
}

func summaryResults(rs []execute.ActionResult) []summary.Result {
	out := make([]summary.Result, 0, len(rs))
	for _, r := range rs {
		out = append(out, summary.Result{ActionID: r.ActionID, PackageID: r.PackageID, Status: string(r.Status), Detail: r.Detail, LogPath: r.LogPath})
	}
	return out
}

// Replanner implements execute.Replanner over the bootstrap, scan and
// conflicts use cases: after the Homebrew bootstrap it re-locates brew,
// refreshes shellenv, re-scans and re-checks conflicts for the packages
// still to install (I-29).
type Replanner struct {
	// Session supplies the adapters.
	Session *Session
	// Catalog is the run's catalog.
	Catalog catalog.Catalog
	// State is the run's state (the caller holds the lock).
	State *State
}

var _ execute.Replanner = (*Replanner)(nil)

// AfterBootstrap returns the new environment, the packages now present and
// those that must be excluded (conflict or incomplete detection).
func (rp *Replanner) AfterBootstrap(ctx context.Context, remaining plan.Plan) (execute.Replan, error) {
	s := rp.Session
	rep, err := s.d.Bootstrap.Check(ctx)
	if err != nil {
		return execute.Replan{}, fmt.Errorf("checking the machine again: %w", err)
	}
	if !rep.Homebrew.Found {
		return execute.Replan{}, errors.New("homebrew is still not found after its installer ran")
	}
	env := bootstrap.ChildEnv(rep, s.d.BasePath)
	s.d.Machine.Set(rep.Homebrew, env)
	sr, err := s.scanner(rp.State).Run(ctx, scan.Input{Catalog: rp.Catalog, Env: s.scanEnv(ctx, rep), HoldsLock: true})
	if err != nil {
		return execute.Replan{}, fmt.Errorf("scanning again: %w", err)
	}
	out := execute.Replan{Env: maps.Clone(env), Brew: rep.Homebrew.Path, Conflicts: map[string]*plan.Conflict{}, Incomplete: map[string]string{}}
	statuses := sr.StatusMap()
	var ids []string
	for _, a := range remaining.Actions {
		if a.PackageID == plan.HomebrewPackageID || slices.Contains(ids, a.PackageID) {
			continue
		}
		ids = append(ids, a.PackageID)
		st, ok := statuses[a.PackageID]
		switch {
		case !ok:
			out.Incomplete[a.PackageID] = "not in the catalog"
		case st.Present():
			out.Present = append(out.Present, a.PackageID)
		case st.Incomplete:
			out.Incomplete[a.PackageID] = "a detector could not answer"
		}
	}
	cr := s.conflictService().CheckAll(ctx, s.brewItems(rp.Catalog, ids, statuses, rep.Platform))
	maps.Copy(out.Conflicts, cr.Conflicts)
	for id, reason := range cr.Failed {
		out.Incomplete[id] = "conflict pre-flight: " + reason
	}
	return out, nil
}

// Machine is the Homebrew the adapters talk to. It changes when facman
// installs Homebrew mid-run, so the brew backends and the path probe read
// it through functions wired in cmd/facman. It is safe for concurrent use.
type Machine struct {
	mu        sync.RWMutex
	brew      platform.HomebrewInfo
	env       map[string]string
	npmPrefix string
	// Home is the user's home directory.
	Home string
}

// Set records where Homebrew is and the child environment.
func (m *Machine) Set(brew platform.HomebrewInfo, env map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.brew, m.env = brew, maps.Clone(env)
}

func (m *Machine) setNPMPrefix(p string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.npmPrefix = p
}

// Brew returns where Homebrew is.
func (m *Machine) Brew() platform.HomebrewInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.brew
}

// Env returns a copy of the child environment overrides.
func (m *Machine) Env() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return maps.Clone(m.env)
}

// Locations returns the Homebrew prefix, npm prefix and home.
func (m *Machine) Locations() catalog.Locations {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return catalog.Locations{BrewPrefix: m.brew.Prefix, NPMPrefix: m.npmPrefix, Home: m.Home}
}
