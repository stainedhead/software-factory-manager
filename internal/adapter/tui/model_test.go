package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

func started(t *testing.T) *driver {
	t.Helper()
	return newDriver(t, &fakeEngine{boot: brewFound()})
}

// T-G1: the state machine and the keys every screen shares.
func TestFlowKeys(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		screen Screen
		quit   bool
		status string
	}{
		{name: "starts on Scan", screen: ScreenScan},
		{name: "enter goes to Plan", keys: []string{"enter"}, screen: ScreenPlan},
		{name: "esc on Plan goes back to Scan", keys: []string{"enter", "esc"}, screen: ScreenScan},
		{name: "enter on Plan goes to Review", keys: []string{"enter", "enter"}, screen: ScreenReview},
		{name: "esc on Review goes back to Plan", keys: []string{"enter", "enter", "esc"}, screen: ScreenPlan},
		{name: "esc on Scan does nothing", keys: []string{"esc"}, screen: ScreenScan},
		{name: "q on Scan quits", keys: []string{"q"}, screen: ScreenScan, quit: true},
		{name: "q on Plan quits", keys: []string{"enter", "q"}, screen: ScreenPlan, quit: true},
		{name: "q on Review quits", keys: []string{"enter", "enter", "q"}, screen: ScreenReview, quit: true},
		{name: "ctrl+c outside Run quits", keys: []string{"enter", "ctrl+c"}, screen: ScreenPlan, quit: true},
		{name: "tab says the tabs come later", keys: []string{"tab"}, screen: ScreenScan, status: "Manage and Files tabs arrive in a later increment"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := started(t)
			d.key(tc.keys...)
			if d.m.Screen() != tc.screen || d.quit != tc.quit {
				t.Errorf("screen %s quit %v, want %s %v\n%s", d.m.Screen(), d.quit, tc.screen, tc.quit, d.screen())
			}
			if tc.status != "" && !strings.Contains(d.screen(), tc.status) {
				t.Errorf("status missing %q:\n%s", tc.status, d.screen())
			}
			if len(d.runner.runs) != 0 || len(d.eng.steps.inv.ops) != 0 {
				t.Errorf("something ran before consent: %v %v", d.runner.runs, d.eng.steps.inv.ops)
			}
			if d.quit && d.m.Err() != nil {
				t.Errorf("quit before consent err = %v", d.m.Err())
			}
		})
	}
}

func TestScanScreen(t *testing.T) {
	d := started(t)
	s := d.screen()
	for _, want := range []string{"facman · Agentic Software Factory Manager", "Profile: agentic-factory", "[Scan] > Plan",
		"Homebrew: /opt/homebrew/bin/brew", "superseded by antigravity-cli", "Nothing has changed"} {
		if !strings.Contains(s, want) {
			t.Errorf("Scan screen misses %q", want)
		}
	}
	golden(t, "scan", s)
}

func TestHelpOverlay(t *testing.T) {
	d := started(t)
	d.key("enter", "?")
	if s := d.screen(); !strings.Contains(s, "Keys on Plan") || !strings.Contains(s, "space") {
		t.Errorf("help:\n%s", s)
	}
	d.key("enter") // ignored while help is open
	if d.m.Screen() != ScreenPlan {
		t.Error("keys reached the screen under the help")
	}
	d.key("?")
	if strings.Contains(d.screen(), "Keys on Plan") {
		t.Error("? did not close help")
	}
	golden(t, "help-review", func() string { d.key("enter", "?"); return d.screen() }())
}

func TestStartFailure(t *testing.T) {
	err := failure.Bootstrap("another facman (PID 42) is changing this machine").WithFix("wait for it to finish, then run facman again")
	d := newDriver(t, &fakeEngine{boot: brewFound(), err: err})
	d.key("enter")
	if d.m.Screen() != ScreenScan {
		t.Error("enter left Scan after a failed start")
	}
	s := d.screen()
	if !strings.Contains(s, "PID 42") || !strings.Contains(s, "fix: wait for it") {
		t.Errorf("error screen:\n%s", s)
	}
	d.key("q")
	if !d.quit || failure.ExitCode(d.m.Err()) != 4 {
		t.Errorf("quit %v exit %d", d.quit, failure.ExitCode(d.m.Err()))
	}
}

// T-G2: the Plan screen.
func TestPlanScreen(t *testing.T) {
	d := started(t)
	d.key("enter")
	golden(t, "plan", d.screen())

	rows := map[string]row{}
	for _, r := range d.m.rows() {
		if r.kind != rowHeader {
			rows[r.id] = r
		}
	}
	tests := []struct {
		id                         string
		checked, selectable, greyd bool
		note                       string
	}{
		{id: "jq", checked: true, selectable: true, note: "required by agentic-factory"},
		{id: "codex", checked: true, selectable: true},
		{id: "gcloud-cli", selectable: true, note: "optional in agentic-factory"},
		{id: "git", note: "adopt arrives in Increment 2"},
		{id: "node", note: "24.1.0"},
		{id: "paperclip", greyd: true, note: "needs the npm backend (Increment 2)"},
		{id: "claude-code", greyd: true, note: "/opt/homebrew/bin/claude held by orphan (dangling symlink)"},
		{id: "awscli", greyd: true, note: "brew timed out"},
		{id: "orbstack", selectable: true},
		{id: "colima", selectable: true},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			r, ok := rows[tc.id]
			if !ok {
				t.Fatalf("no row for %s", tc.id)
			}
			if r.checked != tc.checked || r.selectable != tc.selectable || r.greyed != tc.greyd || !strings.Contains(r.note, tc.note) {
				t.Errorf("row = %+v", r)
			}
		})
	}
}

func TestPlanToggle(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		check  func(t *testing.T, d *driver)
		status string
	}{
		{name: "space ticks an optional item", id: "gcloud-cli", check: func(t *testing.T, d *driver) {
			if it, _ := d.m.desired.Item("gcloud-cli"); !it.Selected || !slices.Contains(d.m.sel.With, "gcloud-cli") {
				t.Errorf("gcloud-cli = %+v, sel %+v", it, d.m.sel)
			}
		}},
		{name: "space unticks a required item", id: "jq", check: func(t *testing.T, d *driver) {
			if it, _ := d.m.desired.Item("jq"); it.Selected || !slices.Contains(d.m.sel.Without, "jq") {
				t.Errorf("jq = %+v", it)
			}
		}},
		{name: "foreign is not selectable", id: "git", status: "never touches it"},
		{name: "unavailable is not selectable", id: "paperclip", status: "not installable here"},
		{name: "conflict is not selectable", id: "claude-code", status: "press i for the evidence"},
		{name: "incomplete is not selectable", id: "awscli", status: "never installs on top of the unknown"},
		{name: "choice member is picked and pulls its requires", id: "colima", check: func(t *testing.T, d *driver) {
			g, _ := d.m.desired.Item("container-runtime")
			dock, ok := d.m.desired.Item("docker")
			if g.ChoicePick != "colima" || !ok || !dock.Selected {
				t.Errorf("group %+v docker %+v", g, dock)
			}
			if !strings.Contains(d.screen(), "needed by colima") {
				t.Errorf("docker row missing:\n%s", d.screen())
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := started(t)
			d.key("enter")
			before := cloneSelection(d.m.sel)
			d.moveTo(tc.id)
			d.key("space")
			if tc.status != "" {
				if !strings.Contains(d.screen(), tc.status) {
					t.Errorf("status missing %q:\n%s", tc.status, d.screen())
				}
				if !slices.Equal(before.With, d.m.sel.With) || !slices.Equal(before.Without, d.m.sel.Without) {
					t.Errorf("selection changed: %+v", d.m.sel)
				}
				return
			}
			tc.check(t, d)
		})
	}
}

func TestChoiceSingleSelectOrNone(t *testing.T) {
	d := started(t)
	d.key("enter")
	d.moveTo("colima")
	d.key("space")
	d.moveTo("orbstack")
	d.key("space")
	if g, _ := d.m.desired.Item("container-runtime"); g.ChoicePick != "orbstack" || slices.Contains(d.m.sel.With, "colima") {
		t.Fatalf("switch: group %+v sel %+v", g, d.m.sel)
	}
	if _, ok := d.m.desired.Item("docker"); ok {
		t.Error("docker still wanted after switching away from colima")
	}
	d.key("space")
	if g, _ := d.m.desired.Item("container-runtime"); g.ChoicePick != "" || len(d.m.sel.With) != 0 {
		t.Errorf("none: group %+v sel %+v", g, d.m.sel)
	}
	// a rejected selection is undone
	d.m.sel.With = append(d.m.sel.With, "nope")
	d.moveTo("gcloud-cli")
	d.key("space")
	if !strings.Contains(d.screen(), "unknown package") {
		t.Errorf("rejection not shown:\n%s", d.screen())
	}
}

func TestPlanInfoAndFilter(t *testing.T) {
	d := started(t)
	d.key("enter")
	d.moveTo("claude-code")
	d.key("i")
	s := d.screen()
	for _, want := range []string{"Claude Code (claude-code)", "Source:  brew-cask claude-code", "required by profile agentic-factory",
		"owner:   orphan", "/Users/j/.local/bin/claude (dangling)", "would fail: brew install --cask claude-code", "rm /opt/homebrew/bin/claude"} {
		if !strings.Contains(s, want) {
			t.Errorf("info misses %q", want)
		}
	}
	golden(t, "plan-info", s)
	d.key("esc")
	for id, want := range map[string]string{"awscli": "Note:    detection incomplete: brew timed out", "paperclip": "Reason:  needs the npm backend",
		"git": "brew 2.47.0 /opt/homebrew/bin/git"} {
		d.moveTo(id)
		d.key("i")
		if !strings.Contains(d.screen(), want) {
			t.Errorf("info on %s misses %q:\n%s", id, want, d.screen())
		}
		d.key("i")
	}
	d.moveTo("orca")
	d.key("i")
	if !strings.Contains(d.screen(), "tap stablyai/orca, item-level trust") {
		t.Errorf("orca source:\n%s", d.screen())
	}
	d.key("i", "/", "o", "r", "x", "backspace", "enter")
	if d.m.filter != "or" {
		t.Fatalf("filter = %q", d.m.filter)
	}
	ids := []string{}
	for _, r := range d.m.visibleRows() {
		if r.kind != rowHeader {
			ids = append(ids, r.id)
		}
	}
	if !slices.Equal(ids, []string{"orbstack", "orca"}) {
		t.Errorf("filtered = %v", ids)
	}
	d.key("q") // q inside a kept filter quits: the filter is not being typed
	if !d.quit {
		t.Error("q did not quit")
	}
	d2 := started(t)
	d2.key("enter", "/", "q")
	if d2.quit || d2.m.filter != "q" {
		t.Errorf("q while typing a filter must be text: quit %v filter %q", d2.quit, d2.m.filter)
	}
	d2.key("esc")
	if d2.m.filter != "" || d2.m.Screen() != ScreenPlan {
		t.Error("esc while typing must clear the filter only")
	}
	d2.key("/", "zzz")
	if s := d2.screen(); !strings.Contains(s, "No package matches.") || !strings.Contains(s, "/ zzz_") {
		t.Errorf("empty filter:\n%s", s)
	}
	d2.key("enter", "esc")
	if d2.m.filter != "" || d2.m.Screen() != ScreenPlan {
		t.Error("esc with a kept filter must clear it first")
	}
}

// T-G3: the Review screen.
func TestReviewScreen(t *testing.T) {
	d := started(t)
	d.key("enter")
	d.moveTo("colima")
	d.key("space", "enter")
	s := d.screen()
	for _, want := range []string{"brew install jq", "brew tap stablyai/orca", "brew trust --cask stablyai/orca/orca",
		"trusts only stablyai/orca/orca, not the whole tap", "may ask for your sudo password", "brew install docker", "after ",
		"claude-code: conflict", "paperclip: unavailable-here", "recorded as j with the time"} {
		if !strings.Contains(s, want) {
			t.Errorf("Review misses %q", want)
		}
	}
	if strings.Index(s, "brew install docker") > strings.Index(s, "brew install colima") {
		t.Error("docker must come before colima")
	}
	golden(t, "review", s)
	if got := d.eng.steps.planned; len(got) != 2 || !slices.Contains(got[1].With, "colima") {
		t.Errorf("plans = %+v", got)
	}
}

// Review FR-001c: a control character in a command is shown escaped on the
// consent screen, never raw.
func TestReviewEscapesControlCharacters(t *testing.T) {
	d := newDriver(t, &fakeEngine{boot: brewFound(), editPlan: func(p *plan.Plan) {
		p.Actions[0].Commands[0].Argv = []string{"brew", "install", "jq\x1b7hidden"}
	}})
	d.key("enter", "enter")
	raw := d.m.Render()
	if strings.Contains(raw, "jq\x1b") {
		t.Error("Review renders the raw ESC from the command")
	}
	s := d.screen()
	if !strings.Contains(s, `brew install "jq\x1b7hidden"`) {
		t.Errorf("Review does not show the escaped command:\n%s", s)
	}
	golden(t, "review-escaped", s)
}

func TestReviewHomebrewMissing(t *testing.T) {
	boot := brewFound()
	boot.Homebrew = platform.HomebrewInfo{}
	boot.Findings = []platform.Finding{{Code: "glibc", Severity: platform.SeverityWarning, Message: "glibc 2.31 is older than Homebrew needs"}}
	d := newDriver(t, &fakeEngine{boot: boot})
	d.key("enter", "enter")
	s := d.screen()
	if !strings.Contains(s, " 1. homebrew") || !strings.Contains(s, "unverified script") || !strings.Contains(s, "warning: glibc") {
		t.Errorf("Review:\n%s", s)
	}
	if !strings.Contains(s, "Homebrew: not installed") {
		d.key("esc", "esc")
		if !strings.Contains(d.screen(), "Homebrew: not installed") {
			t.Errorf("Scan:\n%s", d.screen())
		}
	}
}

func TestReviewNothingToInstall(t *testing.T) {
	d := started(t)
	d.key("enter")
	for _, id := range []string{"jq", "codex", "orca"} {
		d.moveTo(id)
		d.key("space")
	}
	d.key("enter")
	if s := d.screen(); !strings.Contains(s, "Nothing to install.") || !strings.Contains(s, "jq: not-selected") {
		t.Fatalf("Review:\n%s", s)
	}
	d.key("enter")
	if d.m.Screen() != ScreenSummary || len(d.eng.steps.inv.ops) != 0 || len(d.runner.runs) != 0 {
		t.Fatalf("screen %s ops %v", d.m.Screen(), d.eng.steps.inv.ops)
	}
	if s := d.screen(); !strings.Contains(s, "jq: not installed: not selected") {
		t.Errorf("Summary:\n%s", s)
	}
	d.key("esc")
	if d.m.Screen() != ScreenReview {
		t.Error("esc from Summary must go back when nothing ran")
	}
}

func TestPlanError(t *testing.T) {
	d := newDriver(t, &fakeEngine{boot: brewFound()})
	d.key("enter")
	d.eng.steps.planErr = failure.Operation("internal error: the plan breaks a safety rule")
	d.key("enter")
	if d.m.Screen() != ScreenPlan || !strings.Contains(d.screen(), "safety rule") {
		t.Errorf("screen %s:\n%s", d.m.Screen(), d.screen())
	}
}

// T-G4 and T-G5: consent, run, hand-off and summary, end to end.
func TestFullFlow(t *testing.T) {
	d := started(t)
	golden(t, "flow-1-scan", d.screen())
	d.key("enter")
	d.moveTo("colima")
	d.key("space")
	golden(t, "flow-2-plan", d.screen())
	d.key("enter")
	golden(t, "flow-3-review", d.screen())
	d.key("enter")
	if d.m.Screen() != ScreenSummary {
		t.Fatalf("screen %s:\n%s", d.m.Screen(), d.screen())
	}
	golden(t, "flow-4-summary", d.screen())
	// Like headless apply: a conflict or an incomplete detection left
	// out of the plan is exit 1.
	if failure.ExitCode(d.m.Err()) != 1 || !strings.Contains(d.m.Err().Error(), "claude-code not installed (conflict)") {
		t.Errorf("err = %v", d.m.Err())
	}
	ops := d.eng.steps.inv.ops
	if len(ops) == 0 {
		t.Fatal("no operations recorded")
	}
	for _, op := range ops {
		if op.ConsentBy != "j@2026-11-14T19:00:00Z" {
			t.Errorf("consent = %q", op.ConsentBy)
		}
	}
	if len(d.handoff) != 1 || strings.Join(d.handoff[0].Argv, " ") != "brew install --cask stablyai/orca/orca" || !d.handoff[0].Interactive {
		t.Errorf("hand-offs = %+v", d.handoff)
	}
	for _, r := range d.runner.runs {
		if strings.Contains(r, "stablyai/orca/orca") && strings.Contains(r, "install") {
			t.Errorf("sudo command ran on the system runner: %s", r)
		}
	}
	s := d.screen()
	for _, want := range []string{"Codex CLI: installed, sign-in required", "Sign in to Codex CLI", "Claude Code: not installed: conflict", "Logs:"} {
		if !strings.Contains(s, want) {
			t.Errorf("Summary misses %q", want)
		}
	}
	d.key("esc")
	if d.m.Screen() != ScreenSummary || !strings.Contains(d.screen(), "The run is over") {
		t.Error("esc after a run must not go back")
	}
}

func TestRunFailureAndHandoffError(t *testing.T) {
	d := started(t)
	d.runner.fail = map[string]int{"brew install jq": 1}
	d.handoffExit = 1
	d.key("enter", "enter", "enter")
	s := d.screen()
	if failure.ExitCode(d.m.Err()) != 1 || !strings.Contains(s, "jq: not installed: failed") || !strings.Contains(s, "log: /state/logs/") ||
		!strings.Contains(s, "Orca: not installed: failed") {
		t.Errorf("exit %d:\n%s", failure.ExitCode(d.m.Err()), s)
	}

	// The hand-off factory cannot prepare the command.
	d2 := started(t)
	d2.m.cfg.Handoff = func(process.Cmd) (*execCmd, func(error) (process.Result, error), error) {
		return nil, nil, errors.New("sudo not found")
	}
	d2.key("enter", "enter", "enter")
	if s := d2.screen(); !strings.Contains(s, "Orca: not installed") {
		t.Errorf("Summary:\n%s", s)
	}
	d3 := started(t)
	d3.m.cfg.Handoff = nil
	d3.key("enter", "enter", "enter")
	if failure.ExitCode(d3.m.Err()) != 1 {
		t.Errorf("no hand-off: exit %d", failure.ExitCode(d3.m.Err()))
	}
}

// T-G4: the Run screen while it runs, and Ctrl+C.
func TestRunScreenAndStop(t *testing.T) {
	block := make(chan struct{})
	d := newDriver(t, &fakeEngine{boot: brewFound(), block: block})
	d.key("enter", "enter")
	_, listen := d.m.Update(keyMsg("enter")) // consent; Execute waits on block
	if d.m.Screen() != ScreenRun {
		t.Fatalf("screen %s", d.m.Screen())
	}
	p := *d.eng.steps.res.Plan
	a1, a2 := p.Actions[0], p.Actions[1]
	d.m.Update(actionStartedMsg{action: a1})
	d.m.Update(commandOutputMsg{line: "==> Pouring jq--1.7.1.arm64_sequoia.bottle.tar.gz"})
	d.m.Update(actionFinishedMsg{action: a1, result: execute.ActionResult{ActionID: a1.ID, Status: execute.ActionSucceeded}})
	d.m.Update(actionStartedMsg{action: a2})
	d.m.Update(execRequestMsg{cmd: process.Cmd{Argv: []string{"brew", "install", "--cask", "x"}, Interactive: true}, reply: make(chan execReply, 1)})
	golden(t, "run", d.screen())

	d.key("q")
	if d.quit || d.m.Screen() != ScreenRun {
		t.Fatal("q must be ignored during Run")
	}
	d.key("ctrl+c")
	if !strings.Contains(d.screen(), "Stop after the current command?") {
		t.Fatalf("no confirmation:\n%s", d.screen())
	}
	d.key("n")
	if d.m.confirmStop || d.m.stopping {
		t.Fatal("n must keep going")
	}
	d.send(tea.InterruptMsg{})
	d.key("y")
	if !d.m.stopping || !strings.Contains(d.screen(), "Stopping after the current command") {
		t.Fatalf("not stopping:\n%s", d.screen())
	}
	d.key("ctrl+c")
	if d.m.confirmStop {
		t.Error("a second ctrl+c while stopping must not ask again")
	}
	close(block)
	d.do(listen)
	if d.m.Screen() != ScreenSummary || failure.ExitCode(d.m.Err()) != 1 || !strings.Contains(d.screen(), "not installed: cancelled") {
		t.Errorf("after stop: %s exit %d\n%s", d.m.Screen(), failure.ExitCode(d.m.Err()), d.screen())
	}
	if len(d.runner.runs) != 0 {
		t.Errorf("commands ran after the stop: %v", d.runner.runs)
	}
}

func TestExecCallbackRepliesToExecute(t *testing.T) {
	tests := []struct {
		name   string
		runErr error
		code   int
	}{
		{name: "exit code goes back", code: 3},
		{name: "a start failure goes back", runErr: errors.New("no tty"), code: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reply := make(chan execReply, 1)
			finish := func(err error) (process.Result, error) {
				if err != nil {
					return process.Result{ExitCode: -1}, err
				}
				return process.Result{ExitCode: tc.code}, nil
			}
			msg := execCallback(reply, finish)(tc.runErr)
			if _, ok := msg.(execDoneMsg); !ok {
				t.Errorf("msg = %T", msg)
			}
			r := <-reply
			if r.res.ExitCode != tc.code || (r.err != nil) != (tc.runErr != nil) {
				t.Errorf("reply = %+v", r)
			}
		})
	}
	// The model builds the ExecProcess command from the injected factory.
	d := started(t)
	var got []string
	d.m.execProcess = func(c *execCmd, _ tea.ExecCallback) tea.Cmd {
		got = c.Args
		return nil
	}
	d.m.Update(execRequestMsg{cmd: process.Cmd{Argv: []string{"brew", "install", "--cask", "x"}, Interactive: true}, reply: make(chan execReply, 1)})
	if strings.Join(got, " ") != "brew install --cask x" || d.m.handingOff == "" {
		t.Errorf("exec args = %v", got)
	}
	d.m.Update(execDoneMsg{})
	if d.m.handingOff != "" {
		t.Error("execDone must clear the hand-off line")
	}
}

func TestHandoffRunnerAndConsent(t *testing.T) {
	b := newBridge()
	base := &fakeRunner{}
	h := handoffRunner{base: base, b: b}
	if res, err := h.Run(context.Background(), process.Cmd{Argv: []string{"brew", "install", "jq"}}); err != nil || res.ExitCode != 0 || len(base.runs) != 1 {
		t.Errorf("non-interactive = %+v, %v", res, err)
	}
	if _, err := (handoffRunner{base: &fakeRunner{fail: nil}, b: b}).Run(context.Background(), process.Cmd{Argv: []string{"x"}, Interactive: false}); err != nil {
		t.Error(err)
	}
	if _, err := (handoffRunner{base: errRunner{}, b: b}).Run(context.Background(), process.Cmd{Argv: []string{"x"}}); err == nil || !strings.Contains(err.Error(), "running x") {
		t.Errorf("base error = %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := h.Run(context.Background(), process.Cmd{Argv: []string{"sudo-thing"}, Interactive: true})
		done <- err
	}()
	if _, ok := (<-b.events).(execRequestMsg); !ok {
		t.Fatal("no exec request")
	}
	b.close()
	if err := <-done; !errors.Is(err, errTUIClosed) {
		t.Errorf("closed while waiting = %v", err)
	}
	if _, err := h.Run(context.Background(), process.Cmd{Argv: []string{"sudo-thing"}, Interactive: true}); !errors.Is(err, errTUIClosed) {
		t.Errorf("closed before = %v", err)
	}
	if b.listen()() != nil {
		t.Error("listen after close must return nil")
	}
	progress{b: b}.ActionStarted(plan.Action{}) // must not block after close

	c := &consent{}
	p := plan.Plan{Actions: []plan.Action{{ID: 1, PackageID: "jq", Commands: []plan.Command{{Argv: []string{"brew", "install", "jq"}}}}}}
	if ok, _ := c.Confirm(context.Background(), p); ok {
		t.Error("consent without Review")
	}
	c.set(p)
	if ok, _ := c.Confirm(context.Background(), p); !ok {
		t.Error("reviewed plan refused")
	}
	other := plan.Plan{Actions: []plan.Action{{ID: 1, PackageID: "jq", Commands: []plan.Command{{Argv: []string{"brew", "install", "jq", "--force"}}}}}}
	if ok, _ := c.Confirm(context.Background(), other); ok {
		t.Error("a different command was consented")
	}
	for _, q := range []plan.Plan{{}, {Actions: []plan.Action{{ID: 2, PackageID: "jq", Commands: p.Actions[0].Commands}}},
		{Actions: []plan.Action{{ID: 1, PackageID: "jq"}}}} {
		if sameActions(p, q) {
			t.Errorf("sameActions(%+v)", q)
		}
	}
	if argv0(process.Cmd{}) != "command" {
		t.Error("argv0 of nothing")
	}
}

type errRunner struct{}

func (errRunner) Run(context.Context, process.Cmd) (process.Result, error) {
	return process.Result{ExitCode: -1}, errors.New("boom")
}

func TestNarrowTerminal(t *testing.T) {
	d := started(t)
	d.send(tea.WindowSizeMsg{Width: 40, Height: 12})
	for _, keys := range [][]string{nil, {"enter"}, {"j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "k"}, {"i"}, {"esc", "enter"}} {
		d.key(keys...)
		for _, l := range strings.Split(d.screen(), "\n") {
			if w := len([]rune(l)); w > 40 {
				t.Errorf("line of %d cells on %s: %q", w, d.m.Screen(), l)
			}
		}
	}
	if !strings.Contains(d.screen(), "…") {
		t.Error("long cells must end in …")
	}
}

func TestSpinnerTicks(t *testing.T) {
	m := NewModel(context.Background(), Config{Engine: &fakeEngine{boot: brewFound()}, Profiles: []string{"p"}})
	t.Cleanup(m.Shutdown)
	if m.startTick() == nil || m.startTick() != nil {
		t.Fatal("one ticker at a time")
	}
	_, cmd := m.Update(tickMsg{})
	if cmd == nil || m.frame != 1 || !strings.Contains(stripped(m), "/ Checking the machine") {
		t.Errorf("busy tick: frame %d", m.frame)
	}
	m.busy = ""
	if _, cmd := m.Update(tickMsg{}); cmd != nil {
		t.Error("idle ticks must stop")
	}
	if v := m.View(); !v.AltScreen || v.WindowTitle != "facman" {
		t.Errorf("view = %+v", v)
	}
	for s := ScreenScan; s <= ScreenSummary; s++ {
		m.screen, m.help = s, true
		if !strings.Contains(m.Render(), "Keys on "+s.String()) {
			t.Errorf("help on %s", s)
		}
	}
}

// Review FR-013: Review names the HOMEBREW_* variables brew inherits.
func TestReviewNamesInheritedHomebrewEnv(t *testing.T) {
	boot := brewFound()
	boot.Homebrew.InheritedEnv = []string{"HOMEBREW_CASK_OPTS", "HOMEBREW_NO_INSTALL_FROM_API"}
	d := newDriver(t, &fakeEngine{boot: boot})
	d.key("enter", "enter")
	s := d.screen()
	if !strings.Contains(s, "and inherits from your environment: HOMEBREW_CASK_OPTS HOMEBREW_NO_INSTALL_FROM_API") {
		t.Errorf("Review:\n%s", s)
	}
	golden(t, "review-inherited-env", s)
}
