package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/stainedhead/software-factory-manager/internal/adapter/session"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/doctor"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/status"
	"github.com/stainedhead/software-factory-manager/internal/usecase/summary"
)

// fakeEngine records requests and returns a canned result.
type fakeEngine struct {
	reqs []session.Request
	res  session.Result
	err  error
	// confirm, when set, is called with the request's Confirmer.
	confirm func(req session.Request) (bool, error)
}

func (e *fakeEngine) Run(_ context.Context, req session.Request) (session.Result, error) {
	e.reqs = append(e.reqs, req)
	if e.confirm != nil {
		if _, err := e.confirm(req); err != nil {
			return e.res, err
		}
	}
	return e.res, e.err
}

type fakeTUI struct{ got *TUIRequest }

func (f *fakeTUI) Run(_ context.Context, req TUIRequest) error { f.got = &req; return nil }

func newApp(e Engine) (*App, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return &App{Engine: e, Stdio: Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}, Version: "1.2.3",
		Now: func() time.Time { return time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC) }}, &out, &errOut
}

func boot() *platform.BootstrapReport {
	return &platform.BootstrapReport{Platform: platform.Platform{OS: "linux", Arch: "amd64", Distro: platform.Distro{ID: "ubuntu"}},
		Homebrew: platform.HomebrewInfo{Found: true, Path: "/hb/bin/brew", Prefix: "/hb"},
		Findings: []platform.Finding{{Code: "glibc-old", Severity: platform.SeverityWarning, Message: "glibc is old", Fix: "upgrade"},
			{Code: "uid0", Severity: platform.SeverityFatal, Message: "root"}}}
}

// TestExitCodePrecedence: 64 > 4 > 3 > 1 (FR-075, D-6).
func TestExitCodePrecedence(t *testing.T) {
	tests := []struct {
		name string
		errs []error
		want int
	}{
		{"none", nil, 0},
		{"operation", []error{failure.Operation("x")}, 1},
		{"untyped is operation", []error{errors.New("x")}, 1},
		{"drift", []error{failure.Drift("x")}, 2},
		{"validation beats operation", []error{failure.Operation("x"), failure.Validation("y")}, 3},
		{"bootstrap beats validation", []error{failure.Validation("y"), failure.Bootstrap("z")}, 4},
		{"usage beats everything", []error{failure.Bootstrap("z"), failure.Usage("u"), failure.Validation("y"), failure.Operation("x")}, 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failure.ExitCode(failure.Worst(tt.errs...)); got != tt.want {
				t.Errorf("exit = %d, want %d", got, tt.want)
			}
			e := &fakeEngine{err: failure.Worst(tt.errs...)}
			a, _, _ := newApp(e)
			if got := a.Execute(context.Background(), []string{"scan"}); got != tt.want {
				t.Errorf("Execute exit = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUsageErrorsNeverReachTheEngine(t *testing.T) {
	for _, args := range [][]string{
		{"plan"}, {"apply"}, {"plan", "--profile"}, {"plan", "--profile", " "}, {"plan", "--profile", "p", "--with", ""},
		{"scan", "extra"}, {"bogus"}, {"--nope"}, {"export"}, {"config", "init", "--x"}, {"plan", "-nop"},
		{"apply", "--profile", "p"}, // stdin is not a terminal
		{"doctor", "--catalog", "git@github.com:x/y.git"}, {"--catalog", "https://x"}, {"--profile", ""}, {"--with", " "},
	} {
		e := &fakeEngine{}
		a, _, errOut := newApp(e)
		if got := a.Execute(context.Background(), args); got != 64 {
			t.Errorf("%q: exit %d, want 64 (%s)", args, got, errOut.String())
		}
		if len(e.reqs) != 0 {
			t.Errorf("%q reached the engine", args)
		}
		if !strings.HasPrefix(errOut.String(), "facman: ") {
			t.Errorf("%q: stderr = %q", args, errOut.String())
		}
	}
}

func TestRequestsCarryFlags(t *testing.T) {
	e := &fakeEngine{}
	a, _, _ := newApp(e)
	if got := a.Execute(context.Background(), []string{"plan", "--profile", "a", "--profile", " a ", "--profile", "b", "--with", "colima", "--catalog", "./cat", "--dry-run"}); got != 0 {
		t.Fatalf("exit %d", got)
	}
	r := e.reqs[0]
	if r.Command != platform.CommandPlan || strings.Join(r.Profiles, ",") != "a,b" || strings.Join(r.With, ",") != "colima" || r.CatalogDir != "./cat" || !r.DryRun {
		t.Errorf("request = %+v", r)
	}
	if r.Confirmer != nil || r.Progress != nil {
		t.Error("plan asks for no consent")
	}
	for _, cmd := range []string{"scan", "status", "doctor"} {
		if got := a.Execute(context.Background(), []string{cmd, "--json"}); got != 0 {
			t.Errorf("%s exit %d", cmd, got)
		}
	}
}

func TestRootVersionAboutAndTUI(t *testing.T) {
	a, out, _ := newApp(&fakeEngine{})
	if a.Execute(context.Background(), []string{"--version"}) != 0 || out.String() != "facman 1.2.3 (Agentic Software Factory Manager)\n" {
		t.Errorf("--version = %q", out.String())
	}
	out.Reset()
	if a.Execute(context.Background(), []string{"--about"}) != 0 || !strings.Contains(out.String(), "Agentic Software Factory Manager") {
		t.Errorf("--about = %q", out.String())
	}
	out.Reset()
	if a.Execute(context.Background(), []string{"--help"}) != 0 || !strings.Contains(out.String(), "apply") {
		t.Errorf("--help = %q", out.String())
	}

	// stdout a terminal: the TUI seam.
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ptmx.Close(); _ = tty.Close() }()
	go drain(ptmx)
	a.Stdio.Out = tty
	tui := &fakeTUI{}
	a.TUI = tui
	// stdin not a terminal: keys (and consent) only come from a terminal.
	errBuf := &strings.Builder{}
	a.Stdio.Err = errBuf
	if got := a.Execute(context.Background(), nil); got != 64 || tui.got != nil || !strings.Contains(errBuf.String(), "stdin is not a terminal") {
		t.Errorf("stdin not a tty: exit %d, tui %+v, stderr %q", got, tui.got, errBuf.String())
	}
	a.Stdio.In = tty
	a.TUI = nil
	if got := a.Execute(context.Background(), nil); got != 0 {
		t.Errorf("no TUI wired: exit %d", got)
	}
	a.TUI = tui
	if got := a.Execute(context.Background(), []string{"--profile", "x", "--with", "colima", "--catalog", "./c"}); got != 0 {
		t.Fatalf("tui exit %d", got)
	}
	if tui.got == nil || strings.Join(tui.got.Profiles, ",") != "x" || tui.got.With[0] != "colima" || tui.got.CatalogDir != "./c" {
		t.Errorf("tui request = %+v", tui.got)
	}
	tui.got = nil
	_ = a.Execute(context.Background(), nil)
	if tui.got == nil || tui.got.Profiles[0] != DefaultProfile {
		t.Errorf("default profile = %+v", tui.got)
	}
}

func drain(f *os.File) {
	buf := make([]byte, 1024)
	for {
		if _, err := f.Read(buf); err != nil {
			return
		}
	}
}

func TestPrompt(t *testing.T) {
	p := plan.Plan{Profiles: []string{"p"}, Actions: []plan.Action{{ID: 1, Kind: plan.ActionInstall, PackageID: "jq", Commands: []plan.Command{{Argv: []string{"brew", "install", "jq"}}}}}}
	for _, tc := range []struct {
		answer string
		want   bool
	}{{"y\n", true}, {"Y\n", true}, {"yes\n", true}, {" YES \n", true}, {"n\n", false}, {"\n", false}, {"", false}, {"sure\n", false}, {"y", true}} {
		answer, want := tc.answer, tc.want
		var out bytes.Buffer
		ok, err := (&Prompt{In: strings.NewReader(answer), Out: &out}).Confirm(context.Background(), p)
		if err != nil || ok != want {
			t.Errorf("answer %q: %v, %v; want %v", answer, ok, err, want)
		}
		if !strings.Contains(out.String(), "brew install jq") || !strings.Contains(out.String(), "Proceed? [y/N] ") {
			t.Errorf("prompt output = %q", out.String())
		}
	}
	// a cancel (Ctrl+C) declines.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, w, _ := os.Pipe()
	defer func() { _ = r.Close(); _ = w.Close() }()
	if ok, err := (&Prompt{In: r, Out: &bytes.Buffer{}}).Confirm(ctx, p); ok || err != nil {
		t.Errorf("cancelled prompt = %v, %v", ok, err)
	}
	if _, err := (&Prompt{In: r, Out: failWriter{}}).Confirm(context.Background(), p); err == nil {
		t.Error("want a write error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestApplyNeedsTerminalAndPassesConsentAndProgress(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ptmx.Close(); _ = tty.Close() }()
	e := &fakeEngine{}
	a, out, _ := newApp(e)
	a.Stdio.In = tty
	sigs := make(chan os.Signal, 2)
	a.Signals = func() (<-chan os.Signal, func()) { return sigs, func() {} }
	e.confirm = func(req session.Request) (bool, error) {
		if req.Confirmer == nil || req.Progress == nil {
			t.Error("apply has no consent gate or progress")
		}
		pr := req.Progress
		act := plan.Action{ID: 1, PackageID: "jq", Commands: []plan.Command{{Argv: []string{"brew", "install", "jq"}}}}
		pr.ActionStarted(act)
		pr.CommandOutput(act, "==> Installed jq")
		pr.ActionFinished(act, execute.ActionResult{Status: execute.ActionSucceeded})
		pr.ActionFinished(act, execute.ActionResult{Status: execute.ActionFailed, ExitCode: 1, Detail: "boom", LogPath: "/l"})
		pr.ActionFinished(act, execute.ActionResult{Status: execute.ActionSkipped, Detail: "dep"})
		sigs <- os.Interrupt
		time.Sleep(50 * time.Millisecond)
		return true, nil
	}
	if got := a.Execute(context.Background(), []string{"apply", "--profile", "p"}); got != 0 {
		t.Fatalf("exit %d", got)
	}
	for _, s := range []string{"==> [1] jq: brew install jq", "    ==> Installed jq", "jq: done", "failed (exit 1): boom; log: /l", "skipped: dep", "stopping after the current command"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("progress output misses %q:\n%s", s, out.String())
		}
	}
}

func TestRenderEveryCommand(t *testing.T) {
	jq := ownership.Detection{PackageID: "jq", Detector: ownership.DetectorPath, Status: ownership.DetectionOK, Found: true, Path: "/usr/bin/jq", Provenance: "os", Version: "1.7"}
	sr := &scan.Result{Statuses: []ownership.PackageStatus{
		{PackageID: "jq", State: ownership.StateForeign, OSBundled: true, Winner: &jq, Offered: true},
		{PackageID: "claude-code", State: ownership.StateDuplicate, Winner: &jq, Offered: true, Incomplete: true},
		{PackageID: "t3", State: ownership.StateUnavailableHere, Offered: true, UnavailableReason: "no display", Hint: "use a Mac"},
		{PackageID: "hidden", State: ownership.StateAbsent},
	}, Findings: []platform.Finding{{Code: "superseded", Severity: platform.SeverityInfo, Message: "Gemini CLI found"}}}
	p := &plan.Plan{Profiles: []string{"p"}, Warnings: []platform.Finding{{Severity: platform.SeverityWarning, Message: "glibc is old"}},
		Actions: []plan.Action{
			{ID: 1, PackageID: "homebrew", Commands: []plan.Command{{Argv: []string{"/bin/bash", "<temp file>"}, NeedsSudo: true, Unverified: true}}},
			{ID: 2, PackageID: "jq", Commands: []plan.Command{{Argv: []string{"brew", "install", "jq"}}}, DependsOn: []int{1}}},
		Excluded: []plan.Exclusion{{PackageID: "claude-code", Reason: plan.ExcludedConflict, Detail: "held"}}}
	rec := inventory.Record{ID: "homebrew", InstalledBy: "j@t", Bootstrap: true}
	tests := []struct {
		cmd  platform.Command
		req  session.Request
		res  session.Result
		want []string
	}{
		{platform.CommandScan, session.Request{}, session.Result{Bootstrap: boot(), Scan: sr},
			[]string{"linux/amd64 (no display), ubuntu", "warning: glibc is old", "fix: upgrade", "present (OS)", "duplicate (detection incomplete)", "t3: no display; use a Mac", "wins on PATH", "Gemini CLI found"}},
		{platform.CommandPlan, session.Request{}, session.Result{Bootstrap: boot(), Plan: p},
			[]string{"2 command(s)", `/bin/bash "<temp file>"`, "may ask for sudo", "unverified script", "after 1", "claude-code: conflict; held", "warning: glibc is old"}},
		{platform.CommandPlan, session.Request{}, session.Result{Plan: &plan.Plan{}}, []string{"Nothing to install."}},
		{platform.CommandApply, session.Request{DryRun: true}, session.Result{Plan: p}, []string{"Dry run: nothing was changed."}},
		{platform.CommandApply, session.Request{}, session.Result{Plan: p, Outcome: &execute.Outcome{Declined: true}}, []string{"Nothing was changed."}},
		{platform.CommandApply, session.Request{}, session.Result{Plan: &plan.Plan{Excluded: p.Excluded}, Outcome: &execute.Outcome{Warnings: []string{"pruning"}},
			Summary: &summary.Summary{Lines: []summary.Line{{Display: "Claude Code", Text: "not installed: conflict", LogPath: "/l"}}}},
			[]string{"Nothing to install.", "Claude Code: not installed: conflict", "log: /l", "warning: pruning"}},
		{platform.CommandApply, session.Request{}, session.Result{Plan: p, Outcome: &execute.Outcome{Consent: plan.NewConsent("j", time.Now())},
			Summary: &summary.Summary{Lines: []summary.Line{{Display: "jq", Installed: true, Text: "installed"}}, NextSteps: []string{"Sign in"}, ProfileLines: []string{"eval x"}}},
			[]string{"Summary:", "jq: installed", "Next steps:", "Sign in", "Add to your shell profile:", "eval x"}},
		{platform.CommandApply, session.Request{}, session.Result{}, nil},
		{platform.CommandStatus, session.Request{}, session.Result{Bootstrap: &platform.BootstrapReport{Homebrew: platform.HomebrewInfo{Found: true, Path: "/x/brew"}}, Status: &status.Report{
			Rows:     []status.Row{{PackageID: "jq", State: ownership.StateDrifted, Detail: "recorded, not found"}},
			Homebrew: &rec, TapTrust: []inventory.TapTrustGrant{{PackageID: "orca", Tap: "stablyai/orca", Item: "stablyai/orca/orca"}}}},
			[]string{"(not on PATH", "drifted", "jq: recorded, not found", "installed by facman (j@t)", "Trusted stablyai/orca/orca"}},
		{platform.CommandStatus, session.Request{}, session.Result{Status: &status.Report{}}, []string{"No catalog packages are installed."}},
		{platform.CommandStatus, session.Request{}, session.Result{}, nil},
		{platform.CommandDoctor, session.Request{}, session.Result{Doctor: &doctor.Report{Checked: []string{"a"}}}, []string{"No problems found (1 catalog items checked)."}},
		{platform.CommandDoctor, session.Request{}, session.Result{Doctor: &doctor.Report{Findings: []platform.Finding{{Severity: platform.SeverityWarning, Message: "conflict"}}}}, []string{"1 problem(s) to fix"}},
		{platform.CommandDoctor, session.Request{}, session.Result{}, nil},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if err := render(&out, tt.cmd, tt.req, tt.res); err != nil {
			t.Fatal(err)
		}
		for _, w := range tt.want {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%s output misses %q:\n%s", tt.cmd, w, out.String())
			}
		}
		if strings.Contains(out.String(), "hidden") {
			t.Error("a package not offered and not installed was listed")
		}
		if tt.cmd == platform.CommandScan && strings.Contains(out.String(), "fatal") {
			t.Error("a fatal finding is shown as the error, not in the listing")
		}
		if err := render(failWriter{}, tt.cmd, tt.req, tt.res); err == nil && tt.want != nil {
			t.Errorf("%s: want a write error", tt.cmd)
		}
	}
}

func TestJSONOutputAndWriteFailures(t *testing.T) {
	e := &fakeEngine{res: session.Result{Bootstrap: boot()}, err: failure.Bootstrap("root").WithFix("do not use sudo")}
	a, out, errOut := newApp(e)
	if got := a.Execute(context.Background(), []string{"scan", "--json"}); got != 4 {
		t.Fatalf("exit %d", got)
	}
	if !strings.Contains(out.String(), `"exit_code": 4`) || !strings.Contains(errOut.String(), "fix: do not use sudo") {
		t.Errorf("stdout %q stderr %q", out.String(), errOut.String())
	}
	a.Stdio.Out = failWriter{}
	e.err = nil
	if got := a.Execute(context.Background(), []string{"scan", "--json"}); got != 1 {
		t.Errorf("json write failure exit %d", got)
	}
	if got := a.Execute(context.Background(), []string{"scan"}); got != 1 {
		t.Errorf("text write failure exit %d", got)
	}
	if got := a.Execute(context.Background(), []string{"--version"}); got != 1 {
		t.Errorf("--version write failure exit %d", got)
	}
	e.err = errors.New("untyped")
	a.Stdio.Out = &bytes.Buffer{}
	if got := a.Execute(context.Background(), []string{"status"}); got != 1 {
		t.Errorf("untyped error exit %d", got)
	}
	if a.now().IsZero() || (&App{}).now().IsZero() {
		t.Error("now")
	}
	ch, stop := (&App{}).signals()
	stop()
	if ch == nil {
		t.Error("default signals")
	}
}
