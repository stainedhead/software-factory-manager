package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
)

// Prompt is the headless consent gate (PRD Appendix L, 5): it prints the
// full command list, then asks "Proceed? [y/N]". Only y, Y or yes (any
// case) consent; n, an empty line, EOF and a cancel (Ctrl+C) decline.
type Prompt struct {
	// In is the terminal the answer is read from.
	In io.Reader
	// Out receives the command list and the prompt (stderr).
	Out io.Writer
	// agreed records a yes, so the caller can tell a cancel before
	// consent (nothing ran) from one during the run.
	agreed bool
}

// Agreed reports whether Confirm returned true.
func (pr *Prompt) Agreed() bool { return pr.agreed }

var _ execute.Confirmer = (*Prompt)(nil)

// Confirm prints p and reads one answer. A cancelled ctx declines.
func (pr *Prompt) Confirm(ctx context.Context, p plan.Plan) (bool, error) {
	if err := writePlan(pr.Out, p); err != nil {
		return false, err
	}
	if _, err := io.WriteString(pr.Out, "\nProceed? [y/N] "); err != nil {
		return false, fmt.Errorf("writing the prompt: %w", err)
	}
	answer := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(pr.In).ReadString('\n')
		answer <- line
	}()
	select {
	case <-ctx.Done():
		_, _ = io.WriteString(pr.Out, "\n")
		return false, nil
	case line := <-answer:
		if line == "" || !strings.HasSuffix(line, "\n") {
			_, _ = io.WriteString(pr.Out, "\n")
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			pr.agreed = true
			return true, nil
		}
		return false, nil
	}
}
