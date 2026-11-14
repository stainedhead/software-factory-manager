package execute

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

var t0 = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

// journal is the shared, ordered log of what the fakes saw.
type journal struct{ events []string }

func (j *journal) add(format string, args ...any) {
	j.events = append(j.events, fmt.Sprintf(format, args...))
}

func (j *journal) index(t *testing.T, event string) int {
	t.Helper()
	i := slices.Index(j.events, event)
	if i < 0 {
		t.Fatalf("event %q missing from %q", event, j.events)
	}
	return i
}

type fakeRunner struct {
	j     *journal
	exit  map[string]int   // argv string -> exit code
	errs  map[string]error // argv string -> error
	hook  func(process.Cmd)
	cmds  []process.Cmd
	ctxOK []bool // the run context was not cancelled
}

func (r *fakeRunner) Run(ctx context.Context, cmd process.Cmd) (process.Result, error) {
	key := strings.Join(cmd.Argv, " ")
	r.j.add("run %s", key)
	r.cmds = append(r.cmds, cmd)
	if r.hook != nil {
		r.hook(cmd)
	}
	r.ctxOK = append(r.ctxOK, ctx.Err() == nil)
	if cmd.Log != nil {
		_, _ = io.WriteString(cmd.Log, "==> "+key+"\r\npartial")
	}
	return process.Result{ExitCode: r.exit[key]}, r.errs[key]
}

func (r *fakeRunner) argvs() []string {
	var out []string
	for _, c := range r.cmds {
		out = append(out, strings.Join(c.Argv, " "))
	}
	return out
}

type finished struct {
	exit   int
	status inventory.OperationStatus
	log    string
}

type fakeInventory struct {
	j                                     *journal
	next                                  int64
	ops                                   map[int64]inventory.Operation
	done                                  map[int64]finished
	records                               []inventory.Record
	grants                                []inventory.TapTrustGrant
	startErr, finishErr, upsertErr, trErr error
}

func newInventory(j *journal) *fakeInventory {
	return &fakeInventory{j: j, ops: map[int64]inventory.Operation{}, done: map[int64]finished{}}
}

func (f *fakeInventory) StartOperation(_ context.Context, op inventory.Operation) (int64, error) {
	if f.startErr != nil {
		return 0, f.startErr
	}
	f.next++
	op.ID = f.next
	f.ops[op.ID] = op
	f.j.add("start %s %s", op.PackageID, op.Op)
	return op.ID, nil
}

func (f *fakeInventory) FinishOperation(_ context.Context, id int64, exit int, st inventory.OperationStatus, log string) error {
	f.j.add("finish %s %s", f.ops[id].PackageID, st)
	f.done[id] = finished{exit, st, log}
	return f.finishErr
}

func (f *fakeInventory) UpsertRecord(_ context.Context, r inventory.Record) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	if err := r.Validate(); err != nil {
		return err
	}
	f.records = append(f.records, r)
	return nil
}

func (f *fakeInventory) RecordTapTrust(_ context.Context, g inventory.TapTrustGrant) error {
	if f.trErr != nil {
		return f.trErr
	}
	f.grants = append(f.grants, g)
	return nil
}

func (f *fakeInventory) record(id string) (inventory.Record, bool) {
	for _, r := range f.records {
		if r.ID == id {
			return r, true
		}
	}
	return inventory.Record{}, false
}

func (f *fakeInventory) writes() int { return len(f.ops) + len(f.records) + len(f.grants) }

type fakeInstaller struct {
	path  string
	err   error
	argv0 string
	hook  func()
}

func (f *fakeInstaller) Download(context.Context) (string, error) {
	if f.hook != nil {
		f.hook()
	}
	return f.path, f.err
}

func (f *fakeInstaller) Command(path string) plan.Command {
	argv0 := "/bin/bash"
	if f.argv0 != "" {
		argv0 = f.argv0
	}
	return plan.Command{Argv: []string{argv0, path}}
}

type logFile struct {
	strings.Builder
	closeErr error
}

func (l *logFile) Close() error { return l.closeErr }

type fakeState struct {
	logs     map[string]*logFile
	openErr  error
	closeErr error
	pruneErr error
	pruned   int
}

func (f *fakeState) LogPath(opID int64, pkg string, t time.Time) string {
	return fmt.Sprintf("/state/logs/%s-%d-%s.log", t.Format("20060102-150405"), opID, pkg)
}
func (f *fakeState) Lock(context.Context) (func(), error) { return func() {}, nil }
func (f *fakeState) TryLock() (func(), int, error)        { return func() {}, 0, nil }
func (f *fakeState) PruneLogs(time.Time) error            { f.pruned++; return f.pruneErr }
func (f *fakeState) OpenLog(path string) (io.WriteCloser, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	l := &logFile{closeErr: f.closeErr}
	f.logs[path] = l
	return l, nil
}

type fakeClock struct{ t time.Time }

func (f fakeClock) Now() time.Time { return f.t }

type fakeUser string

func (f fakeUser) Username() string { return string(f) }

type fakeProgress struct {
	started  []int
	lines    []string
	finished []ActionResult
}

func (p *fakeProgress) ActionStarted(a plan.Action) { p.started = append(p.started, a.ID) }
func (p *fakeProgress) CommandOutput(_ plan.Action, line string) {
	p.lines = append(p.lines, line)
}
func (p *fakeProgress) ActionFinished(_ plan.Action, r ActionResult) {
	p.finished = append(p.finished, r)
}

type fakeConfirmer struct {
	ok    bool
	err   error
	asked *int
}

func (f fakeConfirmer) Confirm(context.Context, plan.Plan) (bool, error) {
	if f.asked != nil {
		*f.asked++
	}
	return f.ok, f.err
}

type replanFunc func(context.Context, plan.Plan) (Replan, error)

func (f replanFunc) AfterBootstrap(ctx context.Context, p plan.Plan) (Replan, error) {
	return f(ctx, p)
}

type harness struct {
	j        *journal
	runner   *fakeRunner
	inv      *fakeInventory
	state    *fakeState
	progress *fakeProgress
	inst     *fakeInstaller
	svc      *Service
}

func newHarness() *harness {
	j := &journal{}
	h := &harness{
		j: j, runner: &fakeRunner{j: j, exit: map[string]int{}, errs: map[string]error{}}, inv: newInventory(j),
		state: &fakeState{logs: map[string]*logFile{}}, progress: &fakeProgress{},
		inst: &fakeInstaller{path: "/tmp/facman-install-123.sh"},
	}
	h.svc = &Service{
		Runner: h.runner, Inventory: h.inv, Installer: h.inst, State: h.state, Clock: fakeClock{t0}, User: fakeUser("j"),
		Progress: h.progress, Confirmer: fakeConfirmer{ok: true}, Env: map[string]string{"PATH": "/opt/homebrew/bin:/usr/bin", "HOMEBREW_PREFIX": "/opt/homebrew"},
	}
	return h
}

func brew(args ...string) plan.Command {
	return plan.Command{Argv: append([]string{"brew"}, args...), Mutating: true}
}

func install(id int, pkg string, cmd plan.Command, deps ...int) plan.Action {
	return plan.Action{ID: id, Kind: plan.ActionInstall, PackageID: pkg, Commands: []plan.Command{cmd}, DependsOn: deps}
}

var orcaTap = &plan.TapTrustStep{Tap: "stablyai/orca", ItemKind: plan.ItemCask, Item: "stablyai/orca/orca", PackageID: "orca"}

// basePlan: gh, then orca (tap, trust, sudo cask install).
func basePlan() plan.Plan {
	sudoCask := brew("install", "--cask", "stablyai/orca/orca")
	sudoCask.NeedsSudo, sudoCask.Interactive = true, true
	return plan.Plan{CatalogRevision: "rev-1", Actions: []plan.Action{
		install(1, "gh", brew("install", "gh")),
		{ID: 2, Kind: plan.ActionTap, PackageID: "orca", Commands: []plan.Command{brew("tap", "stablyai/orca")}, TapTrust: orcaTap},
		{ID: 3, Kind: plan.ActionTrust, PackageID: "orca", Commands: []plan.Command{brew("trust", "--cask", "stablyai/orca/orca")}, DependsOn: []int{2}, TapTrust: orcaTap},
		{ID: 4, Kind: plan.ActionInstall, PackageID: "orca", Commands: []plan.Command{sudoCask}, DependsOn: []int{3}},
	}}
}

var consent = plan.NewConsent("j", t0)

func statuses(o Outcome) []ActionStatus {
	var out []ActionStatus
	for _, r := range o.Results {
		out = append(out, r.Status)
	}
	return out
}

func TestRunRecordsOperations(t *testing.T) {
	h := newHarness()
	out, err := h.svc.Run(context.Background(), basePlan(), consent)
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(out); !slices.Equal(got, []ActionStatus{ActionSucceeded, ActionSucceeded, ActionSucceeded, ActionSucceeded}) {
		t.Fatalf("statuses = %v", got)
	}
	// in_progress is written before each command runs, finished after.
	for _, ev := range [][3]string{
		{"start gh install", "run brew install gh", "finish gh succeeded"},
		{"start orca tap", "run brew tap stablyai/orca", "finish orca succeeded"},
		{"start orca trust", "run brew trust --cask stablyai/orca/orca", ""},
		{"start orca install", "run brew install --cask stablyai/orca/orca", ""},
	} {
		if a, b := h.j.index(t, ev[0]), h.j.index(t, ev[1]); a > b {
			t.Errorf("%q after %q", ev[0], ev[1])
		}
		if ev[2] != "" && h.j.index(t, ev[1]) > h.j.index(t, ev[2]) {
			t.Errorf("%q after %q", ev[1], ev[2])
		}
	}
	for id, op := range h.inv.ops {
		if op.ConsentBy != "j@2026-11-14T19:00:00Z" || op.Status != inventory.StatusInProgress || !op.StartedAt.Equal(t0) {
			t.Errorf("op %d = %+v", id, op)
		}
		if !slices.Equal(op.Commands[0].EnvNames, []string{"HOMEBREW_PREFIX", "PATH"}) {
			t.Errorf("op %d env names = %v", id, op.Commands[0].EnvNames)
		}
		d := h.inv.done[id]
		if d.exit != 0 || d.status != inventory.StatusSucceeded || !strings.HasPrefix(d.log, "/state/logs/20261114-190000-") {
			t.Errorf("finish %d = %+v", id, d)
		}
		if !strings.Contains(h.state.logs[d.log].String(), "==> brew") {
			t.Errorf("log %s = %q", d.log, h.state.logs[d.log].String())
		}
	}
	gh, ok := h.inv.record("gh")
	orca, ok2 := h.inv.record("orca")
	if !ok || !ok2 || gh.BackendRef != "brew-formula:gh" || orca.BackendRef != "brew-cask:stablyai/orca/orca" || orca.State != inventory.StateManaged ||
		orca.InstalledBy != consent.String() || orca.ManifestRev != "rev-1" || orca.Bootstrap {
		t.Errorf("records = %+v", h.inv.records)
	}
	if len(h.inv.grants) != 1 || h.inv.grants[0].Item != "stablyai/orca/orca" || h.inv.grants[0].OperationID != 3 {
		t.Errorf("grants = %+v", h.inv.grants)
	}
	for i, c := range h.runner.cmds {
		if c.Interactive != (i == 3) || c.Env["PATH"] != "/opt/homebrew/bin:/usr/bin" {
			t.Errorf("cmd %d = %+v", i, c)
		}
	}
	if !slices.Equal(h.progress.started, []int{1, 2, 3, 4}) || len(h.progress.finished) != 4 ||
		!slices.Contains(h.progress.lines, "==> brew install gh") || !slices.Contains(h.progress.lines, "partial") {
		t.Errorf("progress = %+v", h.progress)
	}
	if h.state.pruned != 1 || out.Consent != consent || out.BrewInstalled {
		t.Errorf("pruned = %d, outcome = %+v", h.state.pruned, out)
	}
	if r, ok := out.Result(4); !ok || r.OperationID != 4 || r.LogPath == "" {
		t.Errorf("Result(4) = %+v", r)
	}
	if _, ok := out.Result(9); ok {
		t.Error("Result(9) found")
	}
}

func TestFailureSkipsDependentsOnly(t *testing.T) {
	h := newHarness()
	h.runner.exit["brew tap stablyai/orca"] = 1
	out, err := h.svc.Run(context.Background(), basePlan(), consent)
	if failure.ExitCode(err) != 1 {
		t.Fatalf("err = %v (exit %d)", err, failure.ExitCode(err))
	}
	if got := statuses(out); !slices.Equal(got, []ActionStatus{ActionSucceeded, ActionFailed, ActionSkipped, ActionSkipped}) {
		t.Fatalf("statuses = %v", got)
	}
	if h.inv.done[2] != (finished{1, inventory.StatusFailed, h.inv.done[2].log}) || !strings.Contains(out.Results[1].Detail, "exited 1") {
		t.Errorf("tap finish = %+v, %q", h.inv.done[2], out.Results[1].Detail)
	}
	if !strings.Contains(out.Results[2].Detail, "depends on action 2, which failed") || !strings.Contains(out.Results[3].Detail, "which was skipped") {
		t.Errorf("details = %q, %q", out.Results[2].Detail, out.Results[3].Detail)
	}
	// no retry; skipped actions write nothing.
	if got := h.runner.argvs(); !slices.Equal(got, []string{"brew install gh", "brew tap stablyai/orca"}) {
		t.Errorf("ran %v", got)
	}
	if len(h.inv.ops) != 2 || len(h.inv.records) != 1 || len(h.inv.grants) != 0 {
		t.Errorf("writes: %d ops, %+v", len(h.inv.ops), h.inv.records)
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name     string
		plan     plan.Plan
		opts     Options
		conf     fakeConfirmer
		asked    int
		ran      bool
		declined bool
		wantErr  bool
	}{
		{"dry run", basePlan(), Options{DryRun: true}, fakeConfirmer{ok: true}, 0, false, false, false},
		{"empty plan", plan.Plan{}, Options{}, fakeConfirmer{ok: true}, 0, false, false, false},
		{"decline", basePlan(), Options{}, fakeConfirmer{}, 1, false, true, false},
		{"prompt fails", basePlan(), Options{}, fakeConfirmer{err: errors.New("eof")}, 1, false, false, true},
		{"consent", basePlan(), Options{}, fakeConfirmer{ok: true}, 1, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness()
			asked := 0
			tt.conf.asked = &asked
			h.svc.Confirmer = tt.conf
			out, err := h.svc.Apply(context.Background(), tt.plan, tt.opts)
			if (err != nil) != tt.wantErr || asked != tt.asked || out.Declined != tt.declined || out.DryRun != tt.opts.DryRun {
				t.Fatalf("Apply = %+v, %v (asked %d)", out, err, asked)
			}
			if tt.ran {
				if len(h.runner.cmds) != 4 || out.Consent.String() != "j@2026-11-14T19:00:00Z" {
					t.Errorf("outcome = %+v", out)
				}
				return
			}
			if len(h.runner.cmds) != 0 || h.inv.writes() != 0 || h.state.pruned != 0 || len(out.Results) != 0 {
				t.Errorf("nothing may run or be written: %v, %d writes", h.runner.argvs(), h.inv.writes())
			}
		})
	}
}

// bootstrapPlan is the planner's shape when Homebrew is missing.
func bootstrapPlan() plan.Plan {
	installer := plan.Command{Argv: []string{"/bin/bash", "<temp file>"}, NeedsSudo: true, Interactive: true, Unverified: true, Mutating: true}
	return plan.Plan{CatalogRevision: "rev-1", Actions: []plan.Action{
		{ID: 1, Kind: plan.ActionBootstrapHomebrew, PackageID: plan.HomebrewPackageID, Commands: []plan.Command{installer}},
		install(2, "node", brew("install", "node@24"), 1),
		install(3, "gh", brew("install", "gh"), 1),
		install(4, "claude-code", brew("install", "--cask", "claude-code"), 1),
		install(5, "paperclip", brew("install", "paperclip"), 1, 2),
		install(6, "jq", brew("install", "jq"), 1, 3),
	}}
}

func consented(p plan.Plan, installerPath string) []string {
	var out []string
	for _, c := range p.Commands() {
		argv := slices.Clone(c.Argv)
		if argv[0] == "/bin/bash" {
			argv[1] = installerPath
		}
		out = append(out, strings.Join(argv, " "))
	}
	return out
}

func TestBootstrapReplan(t *testing.T) {
	h := newHarness()
	h.svc.Env = map[string]string{"PATH": "/usr/bin"}
	newEnv := map[string]string{"PATH": "/home/linuxbrew/.linuxbrew/opt/node@24/bin:/home/linuxbrew/.linuxbrew/bin:/usr/bin", "HOMEBREW_PREFIX": "/home/linuxbrew/.linuxbrew"}
	var asked []string
	h.svc.Replanner = replanFunc(func(ctx context.Context, rest plan.Plan) (Replan, error) {
		if ctx.Err() != nil {
			t.Error("replan context cancelled")
		}
		for _, a := range rest.Actions {
			asked = append(asked, a.PackageID)
		}
		return Replan{Env: newEnv, Present: []string{"node"},
			Conflicts: map[string]*plan.Conflict{"gh": {PackageID: "gh", FixHint: "remove /home/linuxbrew/.linuxbrew/bin/gh yourself"}}}, nil
	})
	p := bootstrapPlan()
	out, err := h.svc.Run(context.Background(), p, consent)
	if failure.ExitCode(err) != 1 {
		t.Fatalf("a new conflict must end with exit 1: %v", err)
	}
	if !slices.Equal(asked, []string{"node", "gh", "claude-code", "paperclip", "jq"}) {
		t.Errorf("replanner asked about %v", asked)
	}
	want := []ActionStatus{ActionSucceeded, ActionAlreadyPresent, ActionSkipped, ActionSucceeded, ActionSucceeded, ActionSkipped}
	if got := statuses(out); !slices.Equal(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if !strings.Contains(out.Results[2].Detail, "conflict; remove /home/linuxbrew") || !strings.Contains(out.Results[5].Detail, "dependency-excluded") {
		t.Errorf("details = %q / %q", out.Results[2].Detail, out.Results[5].Detail)
	}
	var reasons []string
	for _, ex := range out.Plan.Excluded {
		reasons = append(reasons, ex.PackageID+":"+string(ex.Reason))
	}
	if !slices.Equal(reasons, []string{"gh:conflict", "jq:dependency-excluded"}) {
		t.Errorf("excluded = %v", reasons)
	}
	// only consented commands ever run.
	allowed := consented(p, h.inst.path)
	for _, argv := range h.runner.argvs() {
		if !slices.Contains(allowed, argv) {
			t.Errorf("ran %q outside the consented plan", argv)
		}
	}
	if got := h.runner.argvs(); !slices.Equal(got, []string{"/bin/bash /tmp/facman-install-123.sh", "brew install --cask claude-code", "brew install paperclip"}) {
		t.Errorf("ran %v", got)
	}
	if c := h.runner.cmds[0]; !c.Interactive || c.Env["PATH"] != "/usr/bin" {
		t.Errorf("installer cmd = %+v", c)
	}
	if c := h.runner.cmds[1]; c.Env["PATH"] != newEnv["PATH"] || c.Env["HOMEBREW_PREFIX"] == "" {
		t.Errorf("later commands must get the refreshed env: %+v", c.Env)
	}
	if op := h.inv.ops[1]; op.Op != inventory.OpBootstrap || op.Commands[0].Argv[1] != h.inst.path || !op.Commands[0].Unverified {
		t.Errorf("bootstrap op = %+v", op)
	}
	if r, ok := h.inv.record("homebrew"); !ok || !r.Bootstrap || r.Backend != inventory.BackendScript || r.BackendRef != inventory.HomebrewRef {
		t.Errorf("homebrew record = %+v", r)
	}
	if !out.BrewInstalled || out.Env["HOMEBREW_PREFIX"] == "" {
		t.Errorf("outcome = %+v", out)
	}
}

func TestBootstrapFailures(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(h *harness)
		exit    int
		ran     []string
		statusN ActionStatus // status of every other action
		detail  string
	}{
		{"download fails", func(h *harness) { h.inst.err = errors.New("3 attempts failed: 503") }, 4, nil, ActionSkipped, "503"},
		{"installer is not the consented program", func(h *harness) { h.inst.argv0 = "/bin/sh" }, 4, nil, ActionSkipped, "not the consented one"},
		{"installer exits non-zero", func(h *harness) { h.runner.exit["/bin/bash /tmp/facman-install-123.sh"] = 1 }, 1,
			[]string{"/bin/bash /tmp/facman-install-123.sh"}, ActionSkipped, "exited 1"},
		{"re-scan fails", func(h *harness) {
			h.svc.Replanner = replanFunc(func(context.Context, plan.Plan) (Replan, error) { return Replan{}, errors.New("brew info: exit 1") })
		}, 1, []string{"/bin/bash /tmp/facman-install-123.sh"}, ActionSkipped, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness()
			tt.setup(h)
			out, err := h.svc.Run(context.Background(), bootstrapPlan(), consent)
			if failure.ExitCode(err) != tt.exit {
				t.Fatalf("exit = %d (%v)", failure.ExitCode(err), err)
			}
			if !slices.Equal(h.runner.argvs(), tt.ran) {
				t.Errorf("ran %v", h.runner.argvs())
			}
			for _, r := range out.Results[1:] {
				if r.Status != tt.statusN {
					t.Errorf("action %d = %+v", r.ActionID, r)
				}
			}
			if !strings.Contains(out.Results[0].Detail, tt.detail) {
				t.Errorf("bootstrap detail = %q", out.Results[0].Detail)
			}
			if tt.ran == nil && h.inv.writes() != 0 {
				t.Errorf("nothing may be written: %+v", h.inv.ops)
			}
		})
	}
	t.Run("re-scan failure excludes the rest as incomplete", func(t *testing.T) {
		h := newHarness()
		h.svc.Replanner = replanFunc(func(context.Context, plan.Plan) (Replan, error) { return Replan{}, errors.New("brew info: exit 1") })
		out, _ := h.svc.Run(context.Background(), bootstrapPlan(), consent)
		for _, ex := range out.Plan.Excluded {
			ok := ex.Reason == plan.ExcludedDetectionIncomplete && strings.Contains(ex.Detail, "brew info: exit 1") ||
				ex.Reason == plan.ExcludedDependency && strings.Contains(ex.Detail, "(detection-incomplete)")
			if !ok {
				t.Errorf("exclusion = %+v", ex)
			}
		}
		if len(out.Plan.Excluded) != 5 {
			t.Errorf("excluded = %+v", out.Plan.Excluded)
		}
	})
	t.Run("conflict without detail and incomplete", func(t *testing.T) {
		h := newHarness()
		h.svc.Replanner = replanFunc(func(context.Context, plan.Plan) (Replan, error) {
			return Replan{Conflicts: map[string]*plan.Conflict{"claude-code": nil}, Incomplete: map[string]string{"node": "timeout"}}, nil
		})
		out, _ := h.svc.Run(context.Background(), bootstrapPlan(), consent)
		want := []ActionStatus{ActionSucceeded, ActionSkipped, ActionSucceeded, ActionSkipped, ActionSkipped, ActionSucceeded}
		if got := statuses(out); !slices.Equal(got, want) {
			t.Errorf("statuses = %v", got)
		}
		if !strings.Contains(out.Results[3].Detail, "held by something else") || !strings.Contains(out.Results[1].Detail, "detection incomplete: timeout") {
			t.Errorf("details = %q / %q", out.Results[3].Detail, out.Results[1].Detail)
		}
	})
	t.Run("no replanner keeps the plan", func(t *testing.T) {
		h := newHarness()
		out, err := h.svc.Run(context.Background(), bootstrapPlan(), consent)
		if err != nil || len(h.runner.cmds) != 6 || !out.BrewInstalled {
			t.Errorf("Run = %+v, %v", out, err)
		}
	})
}

func TestCancel(t *testing.T) {
	t.Run("during a command", func(t *testing.T) {
		h := newHarness()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.runner.hook = func(c process.Cmd) {
			if c.Argv[1] == "tap" {
				cancel() // Ctrl+C while brew tap runs
			}
		}
		out, err := h.svc.Run(ctx, basePlan(), consent)
		if failure.ExitCode(err) != 1 || !out.Cancelled {
			t.Fatalf("Run = %+v, %v", out, err)
		}
		if got := statuses(out); !slices.Equal(got, []ActionStatus{ActionSucceeded, ActionSucceeded, ActionNotStarted, ActionNotStarted}) {
			t.Errorf("statuses = %v", got)
		}
		if !slices.Equal(h.runner.ctxOK, []bool{true, true}) {
			t.Errorf("a running command must not see the cancel: %v", h.runner.ctxOK)
		}
		if h.inv.done[2].status != inventory.StatusSucceeded || len(h.inv.ops) != 2 {
			t.Errorf("the current command is finished and recorded: %+v", h.inv.done)
		}
	})
	t.Run("between commands of one action", func(t *testing.T) {
		h := newHarness()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.runner.hook = func(process.Cmd) { cancel() }
		p := plan.Plan{Actions: []plan.Action{{ID: 1, Kind: plan.ActionTap, PackageID: "orca", Commands: []plan.Command{brew("tap", "a/b"), brew("tap", "c/d")}}}}
		out, err := h.svc.Run(ctx, p, consent)
		if err == nil || out.Results[0].Status != ActionFailed || h.inv.done[1].status != inventory.StatusInterrupted || len(h.runner.cmds) != 1 {
			t.Errorf("Run = %+v, %v, %+v", out, err, h.inv.done)
		}
	})
	t.Run("during the installer download", func(t *testing.T) {
		h := newHarness()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.inst.hook = cancel
		h.inst.err = context.Canceled
		out, err := h.svc.Run(ctx, bootstrapPlan(), consent)
		if failure.ExitCode(err) != 1 || !out.Cancelled || h.inv.writes() != 0 || len(h.runner.cmds) != 0 {
			t.Errorf("Run = %+v, %v", out, err)
		}
		if got := statuses(out); len(got) != 6 || got[0] != ActionNotStarted {
			t.Errorf("statuses = %v", got)
		}
	})
	t.Run("before the run keeps re-plan results", func(t *testing.T) {
		h := newHarness()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.svc.Replanner = replanFunc(func(context.Context, plan.Plan) (Replan, error) {
			cancel()
			return Replan{Present: []string{"gh"}}, nil
		})
		out, _ := h.svc.Run(ctx, bootstrapPlan(), consent)
		want := []ActionStatus{ActionSucceeded, ActionNotStarted, ActionAlreadyPresent, ActionNotStarted, ActionNotStarted, ActionNotStarted}
		if got := statuses(out); !slices.Equal(got, want) {
			t.Errorf("statuses = %v", got)
		}
	})
}

func TestStoreAndRunnerErrors(t *testing.T) {
	gh := plan.Plan{Actions: []plan.Action{install(1, "gh", brew("install", "gh"))}}
	tests := []struct {
		name   string
		plan   plan.Plan
		setup  func(h *harness)
		ran    int
		exit   int
		detail string
	}{
		{"start fails: nothing runs", gh, func(h *harness) { h.inv.startErr = errors.New("disk full") }, 0, 0, "nothing was run"},
		{"log cannot open: nothing runs", gh, func(h *harness) { h.state.openErr = errors.New("eacces") }, 0, NotRunExitCode, "opening the log"},
		{"finish fails", gh, func(h *harness) { h.inv.finishErr = errors.New("locked") }, 1, 0, "finishing the operation"},
		{"record fails", gh, func(h *harness) { h.inv.upsertErr = errors.New("constraint") }, 1, 0, "recording it failed"},
		{"runner cannot start", gh, func(h *harness) { h.runner.errs["brew install gh"] = process.ErrNotFound }, 1, NotRunExitCode, "program not found"},
		{"trust grant fails", plan.Plan{Actions: []plan.Action{{ID: 1, Kind: plan.ActionTrust, PackageID: "orca", Commands: []plan.Command{brew("trust", "x/y/z")}, TapTrust: orcaTap}}},
			func(h *harness) { h.inv.trErr = errors.New("constraint") }, 1, 0, "recording it failed"},
		{"trust without step", plan.Plan{Actions: []plan.Action{{ID: 1, Kind: plan.ActionTrust, PackageID: "orca", Commands: []plan.Command{brew("trust", "x/y/z")}}}},
			func(*harness) {}, 1, 0, "without its tap"},
		{"install without brew install", plan.Plan{Actions: []plan.Action{install(1, "pc", plan.Command{Argv: []string{"npm", "install", "-g", "pc"}})}},
			func(*harness) {}, 1, 0, "no brew install command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness()
			tt.setup(h)
			out, err := h.svc.Run(context.Background(), tt.plan, consent)
			r := out.Results[0]
			if failure.ExitCode(err) != 1 || r.Status != ActionFailed || len(h.runner.cmds) != tt.ran || r.ExitCode != tt.exit || !strings.Contains(r.Detail, tt.detail) {
				t.Errorf("Run = %+v, %v (ran %d)", r, err, len(h.runner.cmds))
			}
		})
	}
	t.Run("warnings do not fail the run", func(t *testing.T) {
		h := newHarness()
		h.state.pruneErr, h.state.closeErr = errors.New("eperm"), errors.New("enospc")
		out, err := h.svc.Run(context.Background(), gh, consent)
		if err != nil || len(out.Warnings) != 2 {
			t.Errorf("Run = %+v, %v", out, err)
		}
	})
	t.Run("no progress sink", func(t *testing.T) {
		h := newHarness()
		h.svc.Progress = nil
		if _, err := h.svc.Run(context.Background(), gh, consent); err != nil {
			t.Error(err)
		}
	})
}

func TestOutcomeErr(t *testing.T) {
	tests := []struct {
		name string
		out  Outcome
		exit int
	}{
		{"clean", Outcome{Results: []ActionResult{{Status: ActionSucceeded}, {Status: ActionAlreadyPresent}}}, 0},
		{"unavailable here is not an error", Outcome{Plan: plan.Plan{Excluded: []plan.Exclusion{{Reason: plan.ExcludedUnavailableHere}}}}, 0},
		{"planned conflict", Outcome{Plan: plan.Plan{Excluded: []plan.Exclusion{{PackageID: "gh", Reason: plan.ExcludedConflict}}}}, 1},
		{"cancelled", Outcome{Cancelled: true}, 1},
		{"bootstrap", Outcome{BootstrapErr: errors.New("x"), Results: []ActionResult{{Status: ActionFailed}}}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failure.ExitCode(tt.out.Err()); got != tt.exit {
				t.Errorf("exit = %d", got)
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	if describe(ActionNotStarted) != "did not run" || describe(ActionSkipped) != "was skipped" {
		t.Error("describe")
	}
}
