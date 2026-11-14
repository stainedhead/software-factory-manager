package tui

import (
	"context"
	"os/exec"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// Screen is one step of the install flow.
type Screen int

// The screens, in flow order (PRD Appendix C).
const (
	ScreenScan Screen = iota
	ScreenPlan
	ScreenReview
	ScreenRun
	ScreenSummary
)

var screenNames = []string{"Scan", "Plan", "Review", "Run", "Summary"}

// String names the screen.
func (s Screen) String() string { return screenNames[s] }

// Config is what one TUI run starts with.
type Config struct {
	// Engine starts the session.
	Engine Engine
	// Runner runs non-interactive plan commands.
	Runner execute.CommandRunner
	// Handoff prepares interactive commands for tea.ExecProcess.
	Handoff Handoff
	// User is the OS user consent is recorded for.
	User string
	// Profiles are the selected profiles (default agentic-factory).
	Profiles []string
	// With pre-selects optional items and choice members.
	With []string
	// CatalogDir is --catalog; empty means embedded.
	CatalogDir string
}

// spinnerInterval is how often the spinner turns.
const spinnerInterval = 120 * time.Millisecond

// logTailMax bounds the run log kept in memory.
const logTailMax = 200

// Messages from the session steps.
type (
	startedMsg struct {
		steps Steps
		res   sessionResult
		err   error
	}
	plannedMsg struct {
		res sessionResult
		err error
	}
	runDoneMsg struct {
		res sessionResult
		err error
	}
	tickMsg struct{}
)

// Model is the Bubble Tea model of the install flow. Its fields change
// only in Update; the session work runs in tea.Cmds.
type Model struct {
	ctx     context.Context
	cfg     Config
	b       *bridge
	consent *consent

	steps Steps
	// res is the session's result as of the last finished step: the
	// view reads only this copy, never the session while a step runs.
	res    sessionResult
	screen Screen
	width  int
	height int
	frame  int
	busy   string
	tick   bool
	err    error
	status string
	help   bool

	// Plan screen.
	sel       catalog.Selection
	desired   catalog.DesiredSet
	cursor    int
	offset    int
	filter    string
	filtering bool
	info      bool

	// Run screen.
	running     bool
	ran         bool
	confirmStop bool
	stopping    bool
	runCancel   context.CancelFunc
	runWG       sync.WaitGroup
	current     int
	results     map[int]execute.ActionResult
	logTail     []string
	handingOff  string

	// tickEvery is the spinner interval; tests set it to zero.
	tickEvery time.Duration
	// execProcess is tea.ExecProcess; tests replace it.
	execProcess func(*exec.Cmd, tea.ExecCallback) tea.Cmd
}

// NewModel returns the model for one TUI run.
func NewModel(ctx context.Context, cfg Config) *Model {
	return &Model{ctx: ctx, cfg: cfg, b: newBridge(), consent: &consent{}, busy: "Checking the machine and scanning",
		sel: catalog.Selection{Profiles: slices.Clone(cfg.Profiles), With: slices.Clone(cfg.With)}, results: map[int]execute.ActionResult{},
		execProcess: tea.ExecProcess, tickEvery: spinnerInterval}
}

// Screen returns the current screen.
func (m *Model) Screen() Screen { return m.screen }

// Err is the error the TUI ends with (it decides the exit code).
func (m *Model) Err() error { return m.err }

// Shutdown stops a run in progress after its current command, waits for
// it and releases the session (and with it the state lock).
func (m *Model) Shutdown() {
	if m.runCancel != nil {
		m.runCancel()
	}
	m.b.close()
	m.runWG.Wait()
	if m.steps != nil {
		m.steps.Close()
	}
}

// Init starts the scan.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.startCmd(), m.startTick())
}

func (m *Model) startCmd() tea.Cmd {
	ctx, cfg, sel := m.ctx, m.cfg, m.sel
	req := tuiRequest(cfg, m.b, m.consent)
	return func() tea.Msg {
		steps, err := cfg.Engine.Start(ctx, req)
		if err == nil {
			// The first plan runs the conflict pre-flight, so the Plan
			// screen shows conflicts for everything pre-selected.
			err = steps.PlanSelection(ctx, sel)
		}
		msg := startedMsg{steps: steps, err: err}
		if steps != nil {
			msg.res = steps.Result()
		}
		return msg
	}
}

func (m *Model) startTick() tea.Cmd {
	if m.tick {
		return nil
	}
	m.tick = true
	if m.tickEvery <= 0 {
		return func() tea.Msg { return tickMsg{} }
	}
	return tea.Tick(m.tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// Update handles one message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.tick = false
		if m.busy == "" && !m.running {
			return m, nil
		}
		m.frame++
		return m, m.startTick()
	case startedMsg:
		return m, m.started(msg)
	case plannedMsg:
		return m, m.planned(msg)
	case tea.InterruptMsg:
		return m, m.interrupt()
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, m.runEvent(msg)
}

func (m *Model) started(msg startedMsg) tea.Cmd {
	m.busy, m.steps, m.res, m.err = "", msg.steps, msg.res, msg.err
	if m.res.Desired != nil {
		m.desired = *m.res.Desired
	}
	return nil
}

func (m *Model) planned(msg plannedMsg) tea.Cmd {
	m.busy, m.err, m.res = "", msg.err, msg.res
	if msg.err != nil {
		m.status = errorText(msg.err)
		return nil
	}
	if m.res.Desired != nil {
		m.desired = *m.res.Desired
	}
	m.screen, m.status = ScreenReview, ""
	return nil
}

// interrupt is Ctrl+C: quit outside Run; during Run, ask first.
func (m *Model) interrupt() tea.Cmd {
	if m.running {
		if !m.stopping {
			m.confirmStop = true
		}
		return nil
	}
	return tea.Quit
}

func (m *Model) key(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return m.interrupt()
	}
	if m.screen == ScreenRun {
		return m.runKey(key)
	}
	if m.filtering {
		m.filterKey(k)
		return nil
	}
	if m.help {
		if key == "?" || key == "esc" || key == "q" {
			m.help = false
		}
		return nil
	}
	if m.info {
		if key == "i" || key == "esc" || key == "q" || key == "enter" {
			m.info = false
		}
		return nil
	}
	if m.busy != "" {
		if key == "q" {
			return tea.Quit
		}
		return nil
	}
	m.status = ""
	switch key {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
		return nil
	case "tab", "shift+tab":
		m.status = "The Manage and Files tabs arrive in a later increment."
		return nil
	}
	switch m.screen {
	case ScreenScan:
		if key == "enter" && m.steps != nil && m.err == nil {
			m.screen = ScreenPlan
		}
	case ScreenPlan:
		return m.planKey(key)
	case ScreenReview:
		return m.reviewKey(key)
	case ScreenSummary:
		if key == "esc" {
			if m.ran {
				m.status = "The run is over; quit with q and run facman again to see the new state."
			} else {
				m.screen = ScreenReview
			}
		}
	}
	return nil
}

func (m *Model) planKey(key string) tea.Cmd {
	rows := m.visibleRows()
	switch key {
	case "up", "k":
		m.cursor = moveCursor(rows, m.cursor, -1)
	case "down", "j":
		m.cursor = moveCursor(rows, m.cursor, +1)
	case "space":
		if r, ok := cursorRow(rows, m.cursor); ok {
			m.toggle(r)
		}
	case "i":
		if _, ok := cursorRow(rows, m.cursor); ok {
			m.info = true
		}
	case "/":
		m.filtering = true
	case "esc":
		if m.filter != "" {
			m.filter, m.cursor = "", 0
			return nil
		}
		m.screen = ScreenScan
	case "enter":
		m.busy = "Planning"
		ctx, steps, sel := m.ctx, m.steps, cloneSelection(m.sel)
		return tea.Batch(func() tea.Msg {
			err := steps.PlanSelection(ctx, sel)
			return plannedMsg{res: steps.Result(), err: err}
		}, m.startTick())
	}
	return nil
}

func (m *Model) filterKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "enter":
		m.filtering = false
	case "esc":
		m.filtering, m.filter = false, ""
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	default:
		if k.Text != "" {
			m.filter += k.Text
		}
	}
	m.cursor, m.offset = moveCursor(m.visibleRows(), -1, +1), 0
}

func (m *Model) reviewKey(key string) tea.Cmd {
	switch key {
	case "esc":
		m.screen = ScreenPlan
	case "enter":
		return m.consentAndRun()
	}
	return nil
}

// consentAndRun is Enter on Review, the only key that consents: the
// reviewed plan is what the Confirmer agrees to; execute records the
// consent with the OS user and the time. An empty plan needs no consent
// and goes straight to Summary.
func (m *Model) consentAndRun() tea.Cmd {
	p := m.res.Plan
	if p == nil {
		return nil
	}
	m.consent.set(*p)
	ctx, cancel := context.WithCancel(m.ctx)
	m.runCancel = cancel
	steps, b := m.steps, m.b
	m.runWG.Add(1)
	go func() {
		defer m.runWG.Done()
		err := steps.Execute(ctx)
		b.send(runDoneMsg{res: steps.Result(), err: err})
	}()
	if p.Empty() {
		m.busy = "Finishing"
	} else {
		m.screen, m.running, m.ran = ScreenRun, true, true
	}
	return tea.Batch(b.listen(), m.startTick())
}

func (m *Model) runKey(key string) tea.Cmd {
	if !m.confirmStop {
		return nil // q and everything else are ignored during Run
	}
	switch key {
	case "y", "Y":
		m.confirmStop, m.stopping = false, true
		m.runCancel()
	case "n", "N", "esc":
		m.confirmStop = false
	}
	return nil
}

// runEvent handles the execute goroutine's messages; each one except the
// last re-arms the listener.
func (m *Model) runEvent(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case actionStartedMsg:
		m.current = msg.action.ID
	case commandOutputMsg:
		m.logTail = append(m.logTail, msg.line)
		if len(m.logTail) > logTailMax {
			m.logTail = m.logTail[len(m.logTail)-logTailMax:]
		}
	case actionFinishedMsg:
		m.results[msg.action.ID] = msg.result
		if m.current == msg.action.ID {
			m.current = 0
		}
	case execRequestMsg:
		return tea.Batch(m.handoff(msg), m.b.listen())
	case execDoneMsg:
		m.handingOff = ""
		return nil
	case runDoneMsg:
		m.running, m.confirmStop, m.busy, m.current = false, false, "", 0
		m.err, m.res = msg.err, msg.res
		m.screen = ScreenSummary
		return nil
	default:
		return nil
	}
	return m.b.listen()
}

// handoff gives the terminal to an interactive (sudo) command with
// tea.ExecProcess; its callback writes the result to the reply channel
// execute is blocked on (AD-16). facman never sees the password.
func (m *Model) handoff(req execRequestMsg) tea.Cmd {
	if m.cfg.Handoff == nil {
		req.reply <- execReply{res: process.Result{ExitCode: execute.NotRunExitCode}, err: errNoHandoff}
		return nil
	}
	c, finish, err := m.cfg.Handoff(req.cmd)
	if err != nil {
		req.reply <- execReply{res: process.Result{ExitCode: execute.NotRunExitCode}, err: err}
		return nil
	}
	m.handingOff = commandLine(req.cmd)
	return m.execProcess(c, execCallback(req.reply, finish))
}

// execCallback is the tea.ExecProcess callback: it turns the run error
// into a result, hands it to execute and tells the model the terminal is
// back.
func execCallback(reply chan<- execReply, finish func(error) (process.Result, error)) tea.ExecCallback {
	return func(runErr error) tea.Msg {
		res, err := finish(runErr)
		reply <- execReply{res: res, err: err}
		return execDoneMsg{}
	}
}

func cloneSelection(s catalog.Selection) catalog.Selection {
	return catalog.Selection{Profiles: slices.Clone(s.Profiles), With: slices.Clone(s.With), Without: slices.Clone(s.Without)}
}
