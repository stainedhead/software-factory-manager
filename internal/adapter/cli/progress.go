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
	mu sync.Mutex
	w  io.Writer
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
}

// CommandOutput prints one line of output, indented.
func (p *textProgress) CommandOutput(_ plan.Action, line string) { p.printf("    %s\n", line) }

// ActionFinished prints how the action ended.
func (p *textProgress) ActionFinished(a plan.Action, r execute.ActionResult) {
	switch r.Status {
	case execute.ActionSucceeded:
		p.printf("    [%d] %s: done\n", a.ID, a.PackageID)
	case execute.ActionFailed:
		p.printf("    [%d] %s: failed (exit %d): %s; log: %s\n", a.ID, a.PackageID, r.ExitCode, r.Detail, r.LogPath)
	default:
		p.printf("    [%d] %s: %s: %s\n", a.ID, a.PackageID, r.Status, r.Detail)
	}
}
