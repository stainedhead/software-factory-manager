package tui

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stainedhead/software-factory-manager/internal/adapter/session"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/planning"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/summary"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var t0 = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

// --- catalog and scan fixture: a lab Mac with some tools already there ---

func onDarwin(b catalog.BackendID, id string) map[platform.Key][]catalog.InstallStep {
	return map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: b, ID: id}}}
}

func fixtureCatalog() catalog.Catalog {
	cli := func(id string, b catalog.BackendID, ref string) catalog.Entry {
		return catalog.Entry{ID: id, Kind: catalog.KindCLI, Install: onDarwin(b, ref), Detect: catalog.DetectSpec{Commands: []string{id}}}
	}
	claude := cli("claude-code", catalog.BackendBrewCask, "claude-code")
	claude.Display, claude.SignInRequired, claude.Detect.Commands = "Claude Code", true, []string{"claude"}
	colima := cli("colima", catalog.BackendBrewFormula, "colima")
	colima.Requires = []catalog.Requirement{{Package: "docker"}}
	node := cli("node", catalog.BackendBrewFormula, "node@24")
	codex := cli("codex", catalog.BackendBrewCask, "codex")
	codex.Display, codex.SignInRequired = "Codex CLI", true
	return catalog.Catalog{APIVersion: catalog.APIVersion, Revision: "embedded:test", Entries: []catalog.Entry{
		cli("git", catalog.BackendBrewFormula, "git"),
		node,
		cli("awscli", catalog.BackendBrewFormula, "awscli"),
		cli("jq", catalog.BackendBrewFormula, "jq"),
		claude,
		codex,
		{ID: "orca", Display: "Orca", Kind: catalog.KindGUI, Install: map[platform.Key][]catalog.InstallStep{
			platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: "stablyai/orca/orca", Tap: "stablyai/orca", Trust: catalog.TrustItem}}}},
		cli("paperclip", catalog.BackendNPM, "paperclip"),
		cli("gcloud-cli", catalog.BackendBrewCask, "gcloud-cli"),
		{ID: "container-runtime", Choice: []catalog.ChoiceMember{{ID: "orbstack"}, {ID: "colima"}}, SatisfiedByAnyPresent: true},
		{ID: "orbstack", Kind: catalog.KindGUI, Install: onDarwin(catalog.BackendBrewCask, "orbstack")},
		colima,
		cli("docker", catalog.BackendBrewFormula, "docker"),
	}, Profiles: map[string]catalog.Profile{
		"base-dev": {Name: "base-dev", Packages: []catalog.ProfilePackage{{ID: "git"}, {ID: "node"}, {ID: "awscli"}}},
		"agentic-factory": {Name: "agentic-factory", Include: []string{"base-dev"}, Packages: []catalog.ProfilePackage{
			{ID: "jq"}, {ID: "claude-code"}, {ID: "codex"}, {ID: "orca"}, {ID: "paperclip"},
			{ID: "gcloud-cli", Optional: true}, {ID: "container-runtime", Optional: true}}},
	}}
}

var darwin = platform.Platform{OS: "darwin", Arch: "arm64", Display: true}

func brewFound() platform.BootstrapReport {
	return platform.BootstrapReport{Platform: darwin,
		Homebrew: platform.HomebrewInfo{Found: true, Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew", OnPATH: true, Version: "4.6.0"},
		Node:     platform.NodeInfo{Found: true, Version: "24.1.0", Provenance: "brew"}}
}

func fixtureScan(cat catalog.Catalog) scan.Result {
	absent := func(id string) ownership.PackageStatus {
		return ownership.PackageStatus{PackageID: id, State: ownership.StateAbsent, Offered: true}
	}
	found := func(id string, state ownership.State, prov catalog.Provenance, ver, path string) ownership.PackageStatus {
		d := ownership.Detection{PackageID: id, Detector: ownership.DetectorBrew, Status: ownership.DetectionOK, Found: true, Provenance: prov, Version: ver, Path: path}
		return ownership.PackageStatus{PackageID: id, State: state, Offered: true, Winner: &d, Detections: []ownership.Detection{d}, Satisfying: true}
	}
	aws := absent("awscli")
	aws.Incomplete = true
	aws.Detections = []ownership.Detection{{PackageID: "awscli", Detector: ownership.DetectorBrew, Status: ownership.DetectionUnknown, Reason: "brew timed out after 10s"}}
	claude := absent("claude-code")
	claude.State = ownership.StateConflict
	claude.Conflict = &plan.Conflict{PackageID: "claude-code", Path: "/opt/homebrew/bin/claude", Owner: plan.OwnerOrphan,
		Evidence: plan.ConflictEvidence{LsLine: "lrwxr-xr-x 1 j admin 40 Nov 1 10:00 /opt/homebrew/bin/claude -> /Users/j/.local/bin/claude",
			IsSymlink: true, SymlinkTarget: "/Users/j/.local/bin/claude", Dangling: true},
		FailingCommand: []string{"brew", "install", "--cask", "claude-code"}, FixHint: "remove the dangling link yourself: rm /opt/homebrew/bin/claude"}
	paperclip := absent("paperclip")
	paperclip.State, paperclip.UnavailableReason = ownership.StateUnavailableHere, "needs the npm backend (Increment 2)"
	byID := map[string]ownership.PackageStatus{
		"git":         found("git", ownership.StateForeign, catalog.ProvBrew, "2.47.0", "/opt/homebrew/bin/git"),
		"node":        found("node", ownership.StateManaged, catalog.ProvBrew, "24.1.0", "/opt/homebrew/opt/node@24/bin/node"),
		"awscli":      aws,
		"claude-code": claude,
		"paperclip":   paperclip,
	}
	var sts []ownership.PackageStatus
	for _, e := range cat.Entries {
		st, ok := byID[e.ID]
		if !ok {
			st = absent(e.ID)
		}
		sts = append(sts, st)
	}
	return scan.Result{Statuses: sts, Findings: []platform.Finding{{Code: platform.CodeSuperseded, Severity: platform.SeverityInfo,
		Message: "gemini-cli is installed; it is superseded by antigravity-cli", Fix: "remove it yourself when you no longer need it"}}}
}

// --- fake session steps over the real planning and execute use cases ---

type brewLike struct{}

func (brewLike) ID() catalog.BackendID          { return catalog.BackendBrewFormula }
func (brewLike) Available(context.Context) bool { return true }
func (brewLike) InstallActions(_ context.Context, e catalog.Entry, s catalog.InstallStep) ([]plan.Action, error) {
	var out []plan.Action
	if s.Tap != "" {
		tt := &plan.TapTrustStep{Tap: s.Tap, ItemKind: plan.ItemCask, Item: s.ID, PackageID: e.ID}
		out = append(out,
			plan.Action{Kind: plan.ActionTap, PackageID: e.ID, TapTrust: tt, Commands: []plan.Command{{Argv: []string{"brew", "tap", s.Tap}, Mutating: true}}},
			plan.Action{Kind: plan.ActionTrust, PackageID: e.ID, TapTrust: tt, Commands: []plan.Command{{Argv: []string{"brew", "trust", "--cask", s.ID}, Mutating: true}}})
	}
	argv := []string{"brew", "install", s.ID}
	cmd := plan.Command{Mutating: true}
	if s.Backend == catalog.BackendBrewCask {
		argv = []string{"brew", "install", "--cask", s.ID}
		cmd.NeedsSudo, cmd.Interactive = e.ID == "orca", e.ID == "orca"
	}
	cmd.Argv = argv
	return append(out, plan.Action{Kind: plan.ActionInstall, PackageID: e.ID, Commands: []plan.Command{cmd}}), nil
}
func (brewLike) LinkPaths(context.Context, catalog.Entry, catalog.InstallStep) ([]string, error) {
	return nil, nil
}
func (brewLike) TapState(context.Context, catalog.InstallStep) (bool, bool, error) {
	return false, false, nil
}
func (brewLike) Capabilities() planning.Caps { return planning.Caps{} }

type memInv struct {
	mu  sync.Mutex
	ops []inventory.Operation
}

func (m *memInv) StartOperation(_ context.Context, op inventory.Operation) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, op)
	return int64(len(m.ops)), nil
}
func (m *memInv) FinishOperation(context.Context, int64, int, inventory.OperationStatus, string) error {
	return nil
}
func (m *memInv) UpsertRecord(context.Context, inventory.Record) error          { return nil }
func (m *memInv) RecordTapTrust(context.Context, inventory.TapTrustGrant) error { return nil }

type memDir struct{}

func (memDir) LogPath(id int64, pkg string, _ time.Time) string {
	return "/state/logs/20261114-190000-" + string(rune('0'+id)) + "-" + pkg + ".log"
}
func (memDir) Lock(context.Context) (func(), error)   { return func() {}, nil }
func (memDir) TryLock() (func(), int, error)          { return func() {}, 0, nil }
func (memDir) PruneLogs(time.Time) error              { return nil }
func (memDir) OpenLog(string) (io.WriteCloser, error) { return nopCloser{}, nil }

type nopCloser struct{}

func (nopCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopCloser) Close() error                { return nil }

type clk struct{}

func (clk) Now() time.Time { return t0 }

type usr struct{}

func (usr) Username() string { return "j" }

// fakeSteps is a session in progress with canned scan results; planning
// and execution go through the real use cases.
type fakeSteps struct {
	mu      sync.Mutex
	req     session.Request
	res     session.Result
	inv     *memInv
	planErr error
	closed  bool
	planned []catalog.Selection
	// block, when set, makes Execute wait for it before running.
	block chan struct{}
	// editPlan, when set, changes the built plan (to inject hostile argv).
	editPlan func(*plan.Plan)
}

func newSteps(req session.Request, boot platform.BootstrapReport) *fakeSteps {
	cat := fixtureCatalog()
	sr := fixtureScan(cat)
	return &fakeSteps{req: req, inv: &memInv{}, res: session.Result{Bootstrap: &boot, Catalog: &cat, Scan: &sr}}
}

func (f *fakeSteps) Result() session.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.res
}

func (f *fakeSteps) Resolve(sel catalog.Selection) (catalog.DesiredSet, error) {
	d, err := catalog.ResolveSelection(*f.res.Catalog, sel)
	if err != nil {
		return d, failure.Validation("resolving: %w", err)
	}
	return d, nil
}

func (f *fakeSteps) PlanSelection(ctx context.Context, sel catalog.Selection) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.planned = append(f.planned, cloneSelection(sel))
	if f.planErr != nil {
		return f.planErr
	}
	d, err := catalog.ResolveSelection(*f.res.Catalog, sel)
	if err != nil {
		return failure.Validation("resolving: %w", err)
	}
	f.res.Desired = &d
	svc := planning.Service{Backends: map[catalog.BackendID]planning.Backend{catalog.BackendBrewFormula: brewLike{}, catalog.BackendBrewCask: brewLike{}},
		InstallerCommand: plan.Command{Argv: []string{"/bin/bash", session.ReviewInstallerPath}}}
	p, err := svc.Build(ctx, planning.Input{Catalog: *f.res.Catalog, Desired: d, Statuses: f.res.Scan.StatusMap(), Bootstrap: *f.res.Bootstrap})
	if f.editPlan != nil {
		f.editPlan(&p)
	}
	f.res.Plan = &p
	return err
}

func (f *fakeSteps) Execute(ctx context.Context) error {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	p, cat := *f.res.Plan, *f.res.Catalog
	f.mu.Unlock()
	ex := &execute.Service{Runner: f.req.Runner, Inventory: f.inv, State: memDir{}, Clock: clk{}, User: usr{},
		Progress: f.req.Progress, Confirmer: f.req.Confirmer}
	out, err := ex.Apply(ctx, p, execute.Options{})
	f.mu.Lock()
	defer f.mu.Unlock()
	f.res.Outcome = &out
	if out.Declined {
		return nil
	}
	var rs []summary.Result
	for _, r := range out.Results {
		rs = append(rs, summary.Result{ActionID: r.ActionID, PackageID: r.PackageID, Status: string(r.Status), Detail: r.Detail, LogPath: r.LogPath})
	}
	s := summary.Build(summary.Input{Catalog: cat, Plan: out.Plan, Results: rs, Homebrew: f.res.Bootstrap.Homebrew})
	f.res.Summary = &s
	if err != nil {
		return err
	}
	return out.Err()
}

func (f *fakeSteps) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// fakeEngine starts fakeSteps.
type fakeEngine struct {
	boot    platform.BootstrapReport
	err     error
	steps   *fakeSteps
	planErr error
	block   chan struct{}
	// editPlan is passed to the steps.
	editPlan func(*plan.Plan)
	// wait, when set, makes Start wait for it (first-frame test).
	wait chan struct{}
}

func (e *fakeEngine) Start(_ context.Context, req session.Request) (Steps, error) {
	if e.wait != nil {
		<-e.wait
	}
	e.steps = newSteps(req, e.boot)
	e.steps.planErr, e.steps.block, e.steps.editPlan = e.planErr, e.block, e.editPlan
	if e.err != nil {
		e.steps.res.Scan = nil
		return e.steps, e.err
	}
	return e.steps, nil
}

// fakeRunner is the system runner: every non-interactive command exits 0
// with one line of output, except those in fail.
type fakeRunner struct {
	mu   sync.Mutex
	runs []string
	fail map[string]int
}

func (r *fakeRunner) Run(_ context.Context, c process.Cmd) (process.Result, error) {
	line := strings.Join(c.Argv, " ")
	r.mu.Lock()
	r.runs = append(r.runs, line)
	r.mu.Unlock()
	if c.Interactive {
		return process.Result{}, errors.New("interactive command reached the system runner")
	}
	if code, ok := r.fail[line]; ok {
		return process.Result{ExitCode: code, Stderr: "Error: " + line + " failed"}, nil
	}
	return process.Result{Stdout: "==> " + line + " done"}, nil
}

// --- the model driver ---

type driver struct {
	t       *testing.T
	m       *Model
	eng     *fakeEngine
	runner  *fakeRunner
	handoff []process.Cmd
	// handoffExit is the exit code the handed-off command "returns".
	handoffExit int
	quit        bool
}

func newDriver(t *testing.T, eng *fakeEngine) *driver {
	t.Helper()
	d := &driver{t: t, eng: eng, runner: &fakeRunner{}}
	cfg := Config{Engine: eng, Runner: d.runner, User: "j", Profiles: []string{"agentic-factory"},
		Handoff: func(c process.Cmd) (*exec.Cmd, func(error) (process.Result, error), error) {
			d.handoff = append(d.handoff, c)
			return &exec.Cmd{Args: c.Argv}, func(err error) (process.Result, error) {
				if err != nil {
					return process.Result{ExitCode: -1}, err
				}
				return process.Result{ExitCode: d.handoffExit}, nil
			}, nil
		}}
	d.m = NewModel(context.Background(), cfg)
	d.m.tickEvery = 0
	// Stand in for tea.ExecProcess: "run" the command, then call back.
	d.m.execProcess = func(_ *exec.Cmd, cb tea.ExecCallback) tea.Cmd {
		return func() tea.Msg { return cb(nil) }
	}
	t.Cleanup(d.m.Shutdown)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 60})
	d.do(d.m.Init())
	return d
}

// send delivers msg and runs the commands it returns.
func (d *driver) send(msg tea.Msg) {
	d.t.Helper()
	_, cmd := d.m.Update(msg)
	d.do(cmd)
}

// do runs cmd and delivers what it returns, synchronously; spinner ticks
// are dropped so frames stay deterministic.
func (d *driver) do(cmd tea.Cmd) {
	d.t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil, tickMsg:
	case tea.BatchMsg:
		for _, c := range msg {
			d.do(c)
		}
	case tea.QuitMsg:
		d.quit = true
	default:
		d.send(msg)
	}
}

func (d *driver) key(keys ...string) {
	d.t.Helper()
	for _, k := range keys {
		d.send(keyMsg(k))
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// screen is the plain text of the current frame.
func (d *driver) screen() string { return ansi.Strip(d.m.Render()) }

// moveTo puts the cursor on the row of id.
func (d *driver) moveTo(id string) {
	d.t.Helper()
	for i, r := range d.m.visibleRows() {
		if r.kind != rowHeader && r.id == id {
			d.m.cursor = i
			return
		}
	}
	d.t.Fatalf("no row %s in\n%s", id, d.screen())
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s (run go test ./internal/adapter/tui -update): %v", path, err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file (run with -update to accept):\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

type execCmd = exec.Cmd

func stripped(m *Model) string { return ansi.Strip(m.Render()) }
