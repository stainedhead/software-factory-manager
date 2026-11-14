// Package tui is facman's interactive front end (AD-2, PRD Appendix C): a
// Bubble Tea program that walks Scan → Plan → Review → Run → Summary,
// install only. It is a driving adapter with no business logic: every step
// is a call on the session (adapter/session), which composes the use
// cases. The TUI only turns keys into a selection, shows what the session
// found and planned, and hands the terminal to sudo commands through
// tea.ExecProcess while execute waits on a reply channel (AD-16).
package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/stainedhead/software-factory-manager/internal/adapter/cli"
	"github.com/stainedhead/software-factory-manager/internal/adapter/session"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// Steps is one TUI session in progress: the scan is done when Start
// returns, and the state lock is held until Close. Implemented by
// *session.Run.
type Steps interface {
	// Result is what the session produced so far.
	Result() session.Result
	// Resolve resolves a selection without planning (no I/O).
	Resolve(sel catalog.Selection) (catalog.DesiredSet, error)
	// PlanSelection runs the conflict pre-flight and builds the plan.
	PlanSelection(ctx context.Context, sel catalog.Selection) error
	// Execute asks the request's Confirmer and runs the plan.
	Execute(ctx context.Context) error
	// Close releases the lock and the state.
	Close()
}

// Engine starts a TUI session: bootstrap check, state lock, catalog and
// scan. On error the returned Steps (possibly nil) hold what was found
// and are already closed.
type Engine interface {
	Start(ctx context.Context, req session.Request) (Steps, error)
}

// SessionEngine adapts *session.Session to Engine.
type SessionEngine struct {
	// Session runs the steps.
	Session *session.Session
}

// Start starts the session.
func (e SessionEngine) Start(ctx context.Context, req session.Request) (Steps, error) {
	r, err := e.Session.Start(ctx, req)
	return r, err
}

// Handoff prepares an interactive command for tea.ExecProcess and turns
// its run error into a result. Implemented by execrunner.Runner.Handoff.
type Handoff func(cmd process.Cmd) (*exec.Cmd, func(runErr error) (process.Result, error), error)

// Options are what the TUI is built with, wired in cmd/facman.
type Options struct {
	// Engine starts sessions.
	Engine Engine
	// Runner is the system runner for non-interactive plan commands.
	Runner execute.CommandRunner
	// Handoff prepares interactive (sudo) commands.
	Handoff Handoff
	// User names the OS user consent is recorded for (shown on Review).
	User func() string
	// ProgramOptions are extra Bubble Tea options (tests).
	ProgramOptions []tea.ProgramOption
}

// Runner opens the TUI. It implements cli.TUIRunner.
type Runner struct {
	o Options
}

var _ cli.TUIRunner = (*Runner)(nil)

// New returns a Runner over o.
func New(o Options) *Runner { return &Runner{o: o} }

// Run runs the TUI on req.Stdio until the developer quits, and returns the
// error that decides the exit code: nil when nothing was changed or
// everything was installed, the session's error otherwise. A run in
// progress when the program ends is stopped after its current command.
func (r *Runner) Run(ctx context.Context, req cli.TUIRequest) error {
	m := NewModel(ctx, r.config(req))
	opts := append([]tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(req.Stdio.In), tea.WithOutput(req.Stdio.Out)}, r.o.ProgramOptions...)
	final, err := tea.NewProgram(m, opts...).Run()
	if fm, ok := final.(*Model); ok && fm != nil {
		m = fm
	}
	m.Shutdown()
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return failure.Operation("running the TUI: %w", err)
	}
	return m.Err()
}

func (r *Runner) config(req cli.TUIRequest) Config {
	user := ""
	if r.o.User != nil {
		user = r.o.User()
	}
	return Config{Engine: r.o.Engine, Runner: r.o.Runner, Handoff: r.o.Handoff, User: user,
		Profiles: req.Profiles, With: req.With, CatalogDir: req.CatalogDir}
}

// bridge connects the execute goroutine to the Bubble Tea program: run
// events and hand-off requests travel as messages on events, which the
// model reads one at a time (listen). done is closed when the program
// ends, so nothing blocks on a model that is gone.
type bridge struct {
	events chan tea.Msg
	done   chan struct{}
	once   sync.Once
}

func newBridge() *bridge {
	return &bridge{events: make(chan tea.Msg, 64), done: make(chan struct{})}
}

func (b *bridge) send(msg tea.Msg) bool {
	if b.closed() {
		return false
	}
	select {
	case b.events <- msg:
		return true
	case <-b.done:
		return false
	}
}

func (b *bridge) closed() bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

func (b *bridge) close() { b.once.Do(func() { close(b.done) }) }

// listen waits for the next event.
func (b *bridge) listen() tea.Cmd {
	return func() tea.Msg {
		if b.closed() {
			return nil
		}
		select {
		case msg := <-b.events:
			return msg
		case <-b.done:
			return nil
		}
	}
}

// Run-event messages.
type (
	actionStartedMsg  struct{ action plan.Action }
	commandOutputMsg  struct{ line string }
	actionFinishedMsg struct {
		action plan.Action
		result execute.ActionResult
	}
	// execRequestMsg asks the model to hand the terminal to cmd; the
	// result goes to reply, on which execute is blocked (AD-16).
	execRequestMsg struct {
		cmd   process.Cmd
		reply chan execReply
	}
	execReply struct {
		res process.Result
		err error
	}
	execDoneMsg struct{}
)

// progress implements execute.Progress over the bridge.
type progress struct{ b *bridge }

var _ execute.Progress = progress{}

func (p progress) ActionStarted(a plan.Action) { p.b.send(actionStartedMsg{action: a}) }
func (p progress) CommandOutput(_ plan.Action, line string) {
	p.b.send(commandOutputMsg{line: line})
}
func (p progress) ActionFinished(a plan.Action, r execute.ActionResult) {
	p.b.send(actionFinishedMsg{action: a, result: r})
}

// handoffRunner runs non-interactive commands on the system runner and
// sends interactive ones to the model, waiting for their result.
type handoffRunner struct {
	base execute.CommandRunner
	b    *bridge
}

var _ execute.CommandRunner = handoffRunner{}

// errTUIClosed is returned for a hand-off when the program has ended.
var errTUIClosed = errors.New("the TUI closed before the command could run")

func (h handoffRunner) Run(ctx context.Context, cmd process.Cmd) (process.Result, error) {
	if !cmd.Interactive {
		res, err := h.base.Run(ctx, cmd)
		if err != nil {
			return res, fmt.Errorf("running %s: %w", argv0(cmd), err)
		}
		return res, nil
	}
	reply := make(chan execReply, 1)
	if !h.b.send(execRequestMsg{cmd: cmd, reply: reply}) {
		return process.Result{ExitCode: execute.NotRunExitCode}, errTUIClosed
	}
	select {
	case r := <-reply:
		return r.res, r.err
	case <-h.b.done:
		return process.Result{ExitCode: execute.NotRunExitCode}, errTUIClosed
	}
}

func argv0(cmd process.Cmd) string {
	if len(cmd.Argv) == 0 {
		return "command"
	}
	return cmd.Argv[0]
}

// consent implements execute.Confirmer for the TUI: the developer already
// pressed Enter on Review, so it agrees only to exactly the plan that was
// reviewed.
type consent struct {
	mu       sync.Mutex
	reviewed *plan.Plan
}

var _ execute.Confirmer = (*consent)(nil)

func (c *consent) set(p plan.Plan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reviewed = &p
}

// Confirm agrees when p has the same actions as the reviewed plan.
func (c *consent) Confirm(_ context.Context, p plan.Plan) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reviewed == nil {
		return false, nil
	}
	return sameActions(*c.reviewed, p), nil
}

func sameActions(a, b plan.Plan) bool {
	if len(a.Actions) != len(b.Actions) {
		return false
	}
	for i := range a.Actions {
		x, y := a.Actions[i], b.Actions[i]
		if x.ID != y.ID || x.PackageID != y.PackageID || len(x.Commands) != len(y.Commands) {
			return false
		}
		for j := range x.Commands {
			if x.Commands[j].String() != y.Commands[j].String() || x.Commands[j].NeedsSudo != y.Commands[j].NeedsSudo {
				return false
			}
		}
	}
	return true
}

// tuiRequest builds the session request for the TUI's Run step.
func tuiRequest(cfg Config, b *bridge, c *consent) session.Request {
	return session.Request{Command: platform.CommandTUIRun, Profiles: cfg.Profiles, With: cfg.With, CatalogDir: cfg.CatalogDir,
		Confirmer: c, Progress: progress{b: b}, Runner: handoffRunner{base: cfg.Runner, b: b}}
}

// sessionResult is what the session produced so far.
type sessionResult = session.Result
