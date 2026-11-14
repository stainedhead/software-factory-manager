package tui

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stainedhead/software-factory-manager/internal/adapter/cli"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// firstFrameBudget is the NFR: the TUI draws its first frame in under
// 500 ms (PRD NFR, T-F6).
const firstFrameBudget = 500 * time.Millisecond

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ansi.Strip(s.b.String())
}

// program runs the real Bubble Tea program over pipes.
type program struct {
	t     *testing.T
	out   *syncBuffer
	in    *os.File
	errc  chan error
	start time.Time
}

func runProgram(t *testing.T, eng Engine, handoff Handoff) *program {
	t.Helper()
	// An *os.File, as a terminal is: Bubble Tea can cancel reads on it
	// when it hands the terminal to a child.
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	p := &program{t: t, out: &syncBuffer{}, in: inW, errc: make(chan error, 1)}
	r := New(Options{Engine: eng, Runner: &fakeRunner{}, Handoff: handoff, User: func() string { return "j" },
		ProgramOptions: []tea.ProgramOption{tea.WithWindowSize(120, 60), tea.WithoutSignals()}})
	p.start = time.Now()
	go func() {
		p.errc <- r.Run(context.Background(), cli.TUIRequest{Profiles: []string{"agentic-factory"}, Stdio: cli.Stdio{In: inR, Out: p.out, Err: io.Discard}})
	}()
	t.Cleanup(func() { _ = inW.Close(); _ = inR.Close() })
	return p
}

// waitFor waits until the screen shows s and returns how long it took.
func (p *program) waitFor(s string) time.Duration {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(p.out.String(), s) {
		if time.Now().After(deadline) {
			p.t.Fatalf("screen never showed %q:\n%s", s, p.out.String())
		}
		time.Sleep(time.Millisecond)
	}
	return time.Since(p.start)
}

func (p *program) keys(s string) {
	p.t.Helper()
	if _, err := p.in.Write([]byte(s)); err != nil {
		p.t.Fatal(err)
	}
}

func (p *program) exit() error {
	p.t.Helper()
	select {
	case err := <-p.errc:
		return err
	case <-time.After(10 * time.Second):
		p.t.Fatal("the TUI did not exit")
		return nil
	}
}

const helperEnv = "TUI_TEST_HELPER"

// TestHelperProcess is the harmless child the hand-off test runs. It is
// not a real test.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		t.Skip("helper process only")
	}
	os.Exit(0)
}

func TestFirstFrameAndQuit(t *testing.T) {
	wait := make(chan struct{})
	eng := &fakeEngine{boot: brewFound(), wait: wait}
	p := runProgram(t, eng, nil)
	// The scan is held back: the first frame must not wait for it.
	if took := p.waitFor("Checking the machine and scanning"); took > firstFrameBudget {
		t.Errorf("first frame after %s, budget %s", took, firstFrameBudget)
	} else {
		t.Logf("first frame after %s", took)
	}
	close(wait)
	p.waitFor("Press enter to see what is missing")
	p.keys("q")
	if err := p.exit(); err != nil {
		t.Errorf("quit before consent = %v", err)
	}
	if !eng.steps.closed {
		t.Error("the session (and its lock) was not released")
	}
	if len(eng.steps.inv.ops) != 0 {
		t.Error("something was recorded")
	}
}

func TestProgramStartFailureExitCode(t *testing.T) {
	eng := &fakeEngine{boot: brewFound(), err: failure.Bootstrap("another facman (PID 7) is changing this machine")}
	p := runProgram(t, eng, nil)
	p.waitFor("PID 7")
	p.keys("q")
	if err := p.exit(); failure.ExitCode(err) != 4 {
		t.Errorf("exit = %d (%v)", failure.ExitCode(err), err)
	}
}

// TestProgramHandsTheTerminalOver runs the whole flow in a real program:
// the sudo command goes through tea.ExecProcess (a harmless child: this
// test binary running no tests) and its exit code reaches execute.
func TestProgramHandsTheTerminalOver(t *testing.T) {
	eng := &fakeEngine{boot: brewFound()}
	var handed []string
	var mu sync.Mutex
	handoff := func(c process.Cmd) (*exec.Cmd, func(error) (process.Result, error), error) {
		mu.Lock()
		handed = append(handed, strings.Join(c.Argv, " "))
		mu.Unlock()
		child := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$") //nolint:gosec // the test binary itself
		child.Env = append(os.Environ(), helperEnv+"=1")
		return child, func(err error) (process.Result, error) {
			if err != nil {
				return process.Result{ExitCode: -1}, err
			}
			return process.Result{ExitCode: child.ProcessState.ExitCode()}, nil
		}, nil
	}
	p := runProgram(t, eng, handoff)
	p.waitFor("Press enter to see what is missing")
	p.keys("\r")
	p.waitFor("Tick what to install")
	p.keys("\r")
	p.waitFor("Pressing enter is your consent")
	p.keys("\r")
	p.waitFor("Next steps (installed, sign-in required)")
	p.keys("q")
	if err := p.exit(); failure.ExitCode(err) != 1 {
		t.Errorf("exit = %d (%v): the conflict and the incomplete detection are left out", failure.ExitCode(err), err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(handed) != 1 || handed[0] != "brew install --cask stablyai/orca/orca" {
		t.Errorf("handed over = %v", handed)
	}
	for _, r := range eng.steps.res.Outcome.Results {
		if r.Status != execute.ActionSucceeded {
			t.Errorf("result %+v", r)
		}
	}
}
