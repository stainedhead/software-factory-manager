package cli

import (
	"fmt"
	"io"
	"sync"

	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
)

// textProgress prints run events for headless apply.
type textProgress struct {
	mu      sync.Mutex
	w       io.Writer
	current *plan.Command // the command running now, for the interrupt message
}

var _ execute.Progress = (*textProgress)(nil)

func (p *textProgress) printf(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(p.w, format, args...)
}

// ActionStarted prints the action and its commands.
func (p *textProgress) ActionStarted(a plan.Action) {
	for _, c := range a.Commands {
		p.printf("==> [%d] %s: %s\n", a.ID, a.PackageID, c.String())
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(a.Commands) > 0 {
		c := a.Commands[len(a.Commands)-1]
		p.current = &c
	}
}

// Running returns the command running now, if any.
func (p *textProgress) Running() (plan.Command, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return plan.Command{}, false
	}
	return *p.current, true
}

// interruptMessage is what the first Ctrl+C prints: what stops, and
// whether the running command got the interrupt too (review FR-006 AC2).
// A non-interactive command runs in its own process group, so the
// terminal's SIGINT never reaches it; an interactive (sudo) command shares
// the terminal and receives the interrupt directly.
func interruptMessage(c plan.Command, running bool) string {
	switch {
	case !running:
		return "\nfacman: stopping; nothing else will start"
	case c.Interactive || c.NeedsSudo:
		return "\nfacman: stopping after the current command (" + c.String() + "); it shares your terminal, so it received this interrupt directly and may stop now"
	default:
		return "\nfacman: stopping after the current command (" + c.String() + ") finishes; it runs on its own and did not receive this interrupt"
	}
}

// CommandOutput prints one line of output, indented.
func (p *textProgress) CommandOutput(_ plan.Action, line string) { p.printf("    %s\n", line) }

// ActionFinished prints how the action ended.
func (p *textProgress) ActionFinished(a plan.Action, r execute.ActionResult) {
	p.mu.Lock()
	p.current = nil
	p.mu.Unlock()
	switch r.Status {
	case execute.ActionSucceeded:
		p.printf("    [%d] %s: done\n", a.ID, a.PackageID)
	case execute.ActionFailed:
		p.printf("    [%d] %s: failed (exit %d): %s; log: %s\n", a.ID, a.PackageID, r.ExitCode, r.Detail, r.LogPath)
	default:
		p.printf("    [%d] %s: %s: %s\n", a.ID, a.PackageID, r.Status, r.Detail)
	}
}
