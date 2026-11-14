package session

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/planning"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

var t0 = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

type fakeBoot struct {
	reps []platform.BootstrapReport
	err  error
	n    int
}

func (b *fakeBoot) Check(context.Context) (platform.BootstrapReport, error) {
	r := b.reps[min(b.n, len(b.reps)-1)]
	b.n++
	return r, b.err
}

type memInv struct {
	records []inventory.Record
	ops     []inventory.Operation
	err     error
}

func (m *memInv) Records(context.Context) ([]inventory.Record, error) { return m.records, m.err }
func (m *memInv) UpsertRecord(_ context.Context, r inventory.Record) error {
	m.records = append(m.records, r)
	return nil
}
func (m *memInv) MarkInterrupted(context.Context) ([]inventory.Operation, error) { return nil, nil }
func (m *memInv) SaveDetections(context.Context, []inventory.DetectionSnapshot) error {
	return nil
}
func (m *memInv) StartOperation(_ context.Context, op inventory.Operation) (int64, error) {
	m.ops = append(m.ops, op)
	return int64(len(m.ops)), nil
}
func (m *memInv) FinishOperation(context.Context, int64, int, inventory.OperationStatus, string) error {
	return nil
}
func (m *memInv) RecordTapTrust(context.Context, inventory.TapTrustGrant) error { return nil }
func (m *memInv) TapTrustGrants(context.Context) ([]inventory.TapTrustGrant, error) {
	return nil, m.err
}

type memDir struct {
	holder int
	locked bool
}

func (d *memDir) LogPath(int64, string, time.Time) string { return "/logs/x.log" }
func (d *memDir) Lock(context.Context) (func(), error)    { return func() {}, nil }
func (d *memDir) TryLock() (func(), int, error) {
	if d.holder != 0 {
		return nil, d.holder, nil
	}
	d.locked = true
	return func() { d.locked = false }, 0, nil
}
func (d *memDir) PruneLogs(time.Time) error              { return nil }
func (d *memDir) OpenLog(string) (io.WriteCloser, error) { return nopCloser{}, nil }

type nopCloser struct{}

func (nopCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopCloser) Close() error                { return nil }

type fakeBackend struct {
	id      catalog.BackendID
	tapErr  error
	linkErr error
}

func (b *fakeBackend) ID() catalog.BackendID          { return b.id }
func (b *fakeBackend) Available(context.Context) bool { return true }
func (b *fakeBackend) InstallActions(_ context.Context, e catalog.Entry, s catalog.InstallStep) ([]plan.Action, error) {
	return []plan.Action{{Kind: plan.ActionInstall, PackageID: e.ID, Commands: []plan.Command{{Argv: []string{"brew", "install", s.ID}, Mutating: true}}}}, nil
}
func (b *fakeBackend) LinkPaths(_ context.Context, _ catalog.Entry, s catalog.InstallStep) ([]string, error) {
	return []string{"/p/bin/" + s.ID}, b.linkErr
}
func (b *fakeBackend) TapState(context.Context, catalog.InstallStep) (bool, bool, error) {
	return true, true, b.tapErr
}
func (b *fakeBackend) Capabilities() planning.Caps { return planning.Caps{} }

type fakeProbe struct{}

func (fakeProbe) Inspect(context.Context, string) (*plan.ConflictEvidence, error) { return nil, nil }
func (fakeProbe) Owner(context.Context, string, catalog.Entry) (plan.OwnerClass, string, error) {
	return plan.OwnerOrphan, "", nil
}

type fakeRunner struct {
	out  string
	runs [][]string
}

func (r *fakeRunner) Run(_ context.Context, c process.Cmd) (process.Result, error) {
	r.runs = append(r.runs, c.Argv)
	if c.Argv[0] == "npm" {
		return process.Result{Stdout: r.out}, nil
	}
	return process.Result{}, nil
}

type fakeInstaller struct{ cleaned bool }

func (*fakeInstaller) Download(context.Context) (execute.InstallerScript, error) {
	return execute.InstallerScript{Path: "/tmp/i.sh"}, nil
}
func (*fakeInstaller) Command(p string) plan.Command {
	return plan.Command{Argv: []string{"/bin/bash", p}, NeedsSudo: true, Interactive: true, Unverified: true, Mutating: true}
}
func (i *fakeInstaller) Cleanup() { i.cleaned = true }

type yes bool

func (y yes) Confirm(context.Context, plan.Plan) (bool, error) { return bool(y), nil }

type clk struct{}

func (clk) Now() time.Time { return t0 }

type usr struct{}

func (usr) Username() string { return "j" }

func testCatalog() catalog.Catalog {
	step := func(id string) map[platform.Key][]catalog.InstallStep {
		return map[platform.Key][]catalog.InstallStep{platform.Linux: {{Backend: catalog.BackendBrewFormula, ID: id}}}
	}
	return catalog.Catalog{Revision: "r", Entries: []catalog.Entry{
		{ID: "jq", Kind: catalog.KindCLI, Install: step("jq"), Detect: catalog.DetectSpec{Commands: []string{"jq"}}},
		{ID: "gh", Kind: catalog.KindCLI, Install: step("gh"), Detect: catalog.DetectSpec{Commands: []string{"gh"}}},
	}, Profiles: map[string]catalog.Profile{"p": {Name: "p", Packages: []catalog.ProfilePackage{{ID: "jq"}, {ID: "gh"}}}}}
}

type harness struct {
	boot *fakeBoot
	inv  *memInv
	dir  *memDir
	be   *fakeBackend
	run  *fakeRunner
	inst *fakeInstaller
	deps Deps
}

func newHarness(reps ...platform.BootstrapReport) *harness {
	if len(reps) == 0 {
		reps = []platform.BootstrapReport{{Platform: platform.Platform{OS: "linux"}, Homebrew: platform.HomebrewInfo{Found: true, Path: "/p/bin/brew", Prefix: "/p"},
			Node: platform.NodeInfo{NPM: true}}}
	}
	h := &harness{boot: &fakeBoot{reps: reps}, inv: &memInv{}, dir: &memDir{}, be: &fakeBackend{id: catalog.BackendBrewFormula}, run: &fakeRunner{out: "/npm\n"}, inst: &fakeInstaller{}}
	h.deps = Deps{Bootstrap: h.boot,
		OpenState: func(context.Context) (*State, error) {
			return &State{Dir: h.dir, Inventory: h.inv, Close: func() error { return nil }}, nil
		},
		LoadCatalog: func(context.Context, string) (catalog.Catalog, error) { return testCatalog(), nil },
		Backends:    map[catalog.BackendID]Backend{catalog.BackendBrewFormula: h.be},
		Probe:       fakeProbe{}, Runner: h.run, Installer: h.inst, Clock: clk{}, User: usr{}, BasePath: "/usr/bin"}
	return h
}

func TestRunCommands(t *testing.T) {
	h := newHarness()
	s := New(h.deps)
	ctx := context.Background()
	res, err := s.Run(ctx, Request{Command: platform.CommandScan})
	if err != nil || res.Scan == nil || len(res.Scan.Statuses) != 2 || res.Catalog == nil {
		t.Fatalf("scan = %+v, %v", res, err)
	}
	if loc := h.deps.Machine; loc != nil {
		t.Error("New must not change the caller's Deps")
	}
	if l := s.d.Machine.Locations(); l.NPMPrefix != "/npm" || l.BrewPrefix != "/p" {
		t.Errorf("locations = %+v", l)
	}
	if res, err = s.Run(ctx, Request{Command: platform.CommandStatus}); err != nil || res.Status == nil {
		t.Errorf("status = %+v, %v", res.Status, err)
	}
	if res, err = s.Run(ctx, Request{Command: platform.CommandDoctor}); err != nil || res.Doctor == nil {
		t.Errorf("doctor = %+v, %v", res.Doctor, err)
	}
	res, err = s.Run(ctx, Request{Command: platform.CommandPlan, Profiles: []string{"p"}})
	if err != nil || res.Plan == nil || len(res.Plan.Actions) != 2 || res.Desired == nil {
		t.Fatalf("plan = %+v, %v", res.Plan, err)
	}
	res, err = s.Run(ctx, Request{Command: platform.CommandApply, Profiles: []string{"p"}, DryRun: true})
	if err != nil || res.Outcome != nil || len(h.inv.ops) != 0 || h.dir.locked {
		t.Fatalf("dry run = %+v, %v", res.Outcome, err)
	}
	res, err = s.Run(ctx, Request{Command: platform.CommandApply, Profiles: []string{"p"}, Confirmer: yes(true)})
	if err != nil || res.Summary == nil || len(h.inv.ops) != 2 || h.inv.ops[0].ConsentBy != "j@2026-11-14T19:00:00Z" {
		t.Fatalf("apply = %+v, %v, ops %+v", res.Outcome, err, h.inv.ops)
	}
	if !h.inst.cleaned || h.dir.locked {
		t.Error("apply must clean up the installer and release the lock")
	}
	// Everything is installed now, so decline on a fresh machine.
	res, err = New(newHarness().deps).Run(ctx, Request{Command: platform.CommandApply, Profiles: []string{"p"}, Confirmer: yes(false)})
	if err != nil || !res.Outcome.Declined || res.Summary != nil {
		t.Errorf("decline = %+v, %v", res.Outcome, err)
	}
}

func TestRunErrors(t *testing.T) {
	ctx := context.Background()
	exit := func(t *testing.T, h *harness, req Request) int {
		t.Helper()
		_, err := New(h.deps).Run(ctx, req)
		return failure.ExitCode(err)
	}
	t.Run("bootstrap check fails", func(t *testing.T) {
		h := newHarness()
		h.boot.err = errors.New("probe")
		if got := exit(t, h, Request{Command: platform.CommandScan}); got != 4 {
			t.Errorf("exit %d", got)
		}
	})
	t.Run("fatal findings", func(t *testing.T) {
		h := newHarness(platform.BootstrapReport{Findings: []platform.Finding{
			{Code: "a", Severity: platform.SeverityWarning, BlocksApply: true, Message: "prereqs"},
			{Code: "b", Severity: platform.SeverityWarning, BlocksApply: true, Message: "owner"}}})
		_, err := New(h.deps).Run(ctx, Request{Command: platform.CommandApply, Profiles: []string{"p"}})
		if failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), "owner") {
			t.Errorf("err = %v", err)
		}
		if got := exit(t, h, Request{Command: platform.CommandApply, Profiles: []string{"p"}, DryRun: true}); got != 0 {
			t.Errorf("a dry run is judged like plan: exit %d", got)
		}
	})
	t.Run("state", func(t *testing.T) {
		h := newHarness()
		h.deps.OpenState = func(context.Context) (*State, error) { return nil, errors.New("disk") }
		if got := exit(t, h, Request{Command: platform.CommandScan}); got != 4 {
			t.Errorf("exit %d", got)
		}
	})
	t.Run("lock held", func(t *testing.T) {
		h := newHarness()
		h.dir.holder = 77
		_, err := New(h.deps).Run(ctx, Request{Command: platform.CommandApply, Profiles: []string{"p"}, Confirmer: yes(true)})
		if failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), "77") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("catalog", func(t *testing.T) {
		h := newHarness()
		h.deps.LoadCatalog = func(context.Context, string) (catalog.Catalog, error) {
			return catalog.Catalog{}, errors.New("bad yaml")
		}
		if got := exit(t, h, Request{Command: platform.CommandScan}); got != 3 {
			t.Errorf("exit %d", got)
		}
	})
	t.Run("scan", func(t *testing.T) {
		h := newHarness()
		h.inv.err = errors.New("db")
		if got := exit(t, h, Request{Command: platform.CommandScan}); got != 4 {
			t.Errorf("exit %d", got)
		}
	})
	t.Run("profile", func(t *testing.T) {
		if got := exit(t, newHarness(), Request{Command: platform.CommandPlan, Profiles: []string{"nope"}}); got != 3 {
			t.Errorf("exit %d", got)
		}
	})
	t.Run("preflight failure is incomplete", func(t *testing.T) {
		h := newHarness()
		h.be.linkErr = errors.New("brew info")
		res, err := New(h.deps).Run(ctx, Request{Command: platform.CommandPlan, Profiles: []string{"p"}})
		if err != nil || len(res.Plan.Actions) != 0 || len(res.Plan.Excluded) != 2 || res.Plan.Excluded[0].Reason != plan.ExcludedDetectionIncomplete {
			t.Errorf("plan = %+v, %v", res.Plan, err)
		}
	})
	t.Run("tap check", func(t *testing.T) {
		if _, _, err := (tapChecker{}).TapState(ctx, catalog.InstallStep{Backend: "x"}); err == nil {
			t.Error("want an error for a missing backend")
		}
		h := newHarness()
		h.be.tapErr = errors.New("brew tap")
		if _, _, err := tapChecker(h.deps.Backends).TapState(ctx, catalog.InstallStep{Backend: catalog.BackendBrewFormula}); err == nil {
			t.Error("want the backend's error")
		}
	})
}

func TestReplanner(t *testing.T) {
	ctx := context.Background()
	missing := platform.BootstrapReport{Platform: platform.Platform{OS: "linux"}}
	found := platform.BootstrapReport{Platform: platform.Platform{OS: "linux"}, Homebrew: platform.HomebrewInfo{Found: true, Path: "/p/bin/brew", Prefix: "/p"},
		ShellEnv: map[string]string{"HOMEBREW_PREFIX": "/p", "PATH": "/p/bin:/usr/bin"}}
	h := newHarness(missing, found)
	s := New(h.deps)
	res, err := s.Run(ctx, Request{Command: platform.CommandApply, Profiles: []string{"p"}, Confirmer: yes(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Outcome.BrewInstalled || res.Outcome.Env["HOMEBREW_PREFIX"] != "/p" || len(h.inv.ops) != 3 {
		t.Errorf("outcome = %+v, ops %d", res.Outcome, len(h.inv.ops))
	}
	if s.d.Machine.Brew().Prefix != "/p" {
		t.Error("the machine was not updated after the bootstrap")
	}

	rp := &Replanner{Session: New(newHarness(missing).deps), Catalog: testCatalog(), State: &State{Dir: &memDir{}, Inventory: &memInv{}}}
	if _, err := rp.AfterBootstrap(ctx, plan.Plan{}); err == nil {
		t.Error("Homebrew still missing must be an error")
	}
	hb := newHarness(found)
	hb.boot.err = errors.New("boom")
	rp.Session = New(hb.deps)
	if _, err := rp.AfterBootstrap(ctx, plan.Plan{}); err == nil {
		t.Error("want the bootstrap error")
	}
	hs := newHarness(found)
	rp = &Replanner{Session: New(hs.deps), Catalog: testCatalog(), State: &State{Dir: &memDir{}, Inventory: &memInv{err: errors.New("db")}}}
	if _, err := rp.AfterBootstrap(ctx, plan.Plan{}); err == nil {
		t.Error("want the scan error")
	}
	hf := newHarness(found)
	hf.be.linkErr = errors.New("brew info")
	rp = &Replanner{Session: New(hf.deps), Catalog: testCatalog(), State: &State{Dir: &memDir{}, Inventory: &memInv{}}}
	out, err := rp.AfterBootstrap(ctx, plan.Plan{Actions: []plan.Action{{PackageID: "homebrew"}, {PackageID: "jq"}, {PackageID: "jq"}, {PackageID: "ghost"}}})
	if err != nil || !strings.Contains(out.Incomplete["jq"], "brew info") || out.Incomplete["ghost"] == "" {
		t.Errorf("replan = %+v, %v", out, err)
	}
}

func TestHelpers(t *testing.T) {
	if (&Run{}).Result().Plan != nil || (*Run)(nil).result().Plan != nil {
		t.Error("empty result")
	}
	(*Run)(nil).Close()
	if err := asKind(failure.KindValidation, "x", failure.Usage("u")); failure.ExitCode(err) != 64 {
		t.Error("a typed error keeps its kind")
	}
	r := &fakeRunner{out: "relative\n"}
	s := New(Deps{Runner: r})
	if got := s.npmPrefix(context.Background(), nil); got != "" {
		t.Errorf("npmPrefix = %q", got)
	}
	if (Request{Command: platform.CommandApply}).Mutates() != true || (Request{Command: platform.CommandApply, DryRun: true}).Mutates() {
		t.Error("Mutates")
	}
	var _ execute.Replanner = (*Replanner)(nil)
}

// wrapRunner is the TUI's hand-off wrapper stand-in: it records the plan
// commands it is given.
type wrapRunner struct{ runs [][]string }

func (w *wrapRunner) Run(_ context.Context, c process.Cmd) (process.Result, error) {
	w.runs = append(w.runs, c.Argv)
	return process.Result{}, nil
}

func TestTUISteps(t *testing.T) {
	ctx := context.Background()
	h := newHarness()
	wrap := &wrapRunner{}
	r, err := New(h.deps).Start(ctx, Request{Command: platform.CommandTUIRun, Profiles: []string{"p"}, Confirmer: yes(true), Runner: wrap})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if !h.dir.locked {
		t.Error("the TUI holds the lock from the scan on")
	}
	d, err := r.Resolve(catalog.Selection{Profiles: []string{"p"}, Without: []string{"gh"}})
	if err != nil {
		t.Fatal(err)
	}
	if gh, _ := d.Item("gh"); gh.Selected {
		t.Errorf("gh = %+v", gh)
	}
	if _, err := r.Resolve(catalog.Selection{Profiles: []string{"p"}, With: []string{"nope"}}); failure.ExitCode(err) != 3 {
		t.Errorf("bad selection err = %v", err)
	}
	if err := r.PlanSelection(ctx, catalog.Selection{Profiles: []string{"p"}, Without: []string{"gh"}}); err != nil {
		t.Fatal(err)
	}
	res := r.Result()
	if len(res.Plan.Actions) != 1 || res.Plan.Actions[0].PackageID != "jq" || res.Plan.Excluded[0].Reason != plan.ExcludedNotSelected {
		t.Fatalf("plan = %+v", res.Plan)
	}
	if err := r.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if len(wrap.runs) != 1 || strings.Join(wrap.runs[0], " ") != "/p/bin/brew install jq" {
		t.Errorf("plan commands went to %v, want the request's runner running the located brew", wrap.runs)
	}
	for _, argv := range h.run.runs {
		if argv[0] == "brew" || argv[0] == "/p/bin/brew" && len(argv) > 1 && argv[1] == "install" {
			t.Errorf("plan command ran on the base runner: %v", argv)
		}
	}
	if r.Result().Summary == nil {
		t.Error("no summary")
	}
}
