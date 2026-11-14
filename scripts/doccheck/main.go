// Command doccheck checks that the docs keep up with the repo (review
// FR-015, FR-018): every .PHONY target of the Makefile has a row in
// AGENTS.md's Commands table, and every user guide for a later increment
// says so within its first three lines. Run it from the repository root
// (make doccheck); it exits 1 naming each problem.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

// FutureGuides are the user guides of features that arrive after
// Increment 1. Remove a guide from the list when its feature ships.
var FutureGuides = []string{
	"user-docs/configuration-list.md",
	"user-docs/hermes-setup.md",
	"user-docs/orca-setup.md",
	"user-docs/paperclip-setup.md",
	"user-docs/t3-setup.md",
	"user-docs/templates-and-validation.md",
}

var (
	phonyRe  = regexp.MustCompile(`(?m)^\.PHONY:(.*)$`)
	statusRe = regexp.MustCompile(`\*\*Status: planned for Increment \d+\.\*\*.*exits? 64 until then`)
)

func main() {
	problems, err := check(os.DirFS("."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "doccheck:", err)
		os.Exit(2)
	}
	os.Exit(report(os.Stdout, problems))
}

func report(w io.Writer, problems []string) int {
	for _, p := range problems {
		_, _ = fmt.Fprintln(w, p)
	}
	if len(problems) > 0 {
		_, _ = fmt.Fprintf(w, "doccheck: %d problem(s)\n", len(problems))
		return 1
	}
	return 0
}

// check reads the Makefile, AGENTS.md and the future guides from fsys.
func check(fsys fs.FS) ([]string, error) {
	mk, err := fs.ReadFile(fsys, "Makefile")
	if err != nil {
		return nil, fmt.Errorf("reading the Makefile: %w", err)
	}
	agents, err := fs.ReadFile(fsys, "AGENTS.md")
	if err != nil {
		return nil, fmt.Errorf("reading AGENTS.md: %w", err)
	}
	problems := commandProblems(string(mk), commandsTable(string(agents)))
	for _, g := range FutureGuides {
		b, err := fs.ReadFile(fsys, g)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", g, err)
		}
		if p := statusProblem(g, string(b)); p != "" {
			problems = append(problems, p)
		}
	}
	return problems, nil
}

// commandsTable returns the text of AGENTS.md's "## Commands" section.
func commandsTable(agents string) string {
	_, rest, ok := strings.Cut(agents, "\n## Commands\n")
	if !ok {
		return ""
	}
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// commandProblems names every .PHONY target without a "| `make <t>`" row.
func commandProblems(makefile, table string) []string {
	var problems []string
	for _, m := range phonyRe.FindAllStringSubmatch(makefile, -1) {
		for _, t := range strings.Fields(m[1]) {
			if !strings.Contains(table, "| `make "+t+"`") && !strings.Contains(table, "| `make "+t+" ") {
				problems = append(problems, fmt.Sprintf("AGENTS.md: the Commands table has no row for make %s (FR-015)", t))
			}
		}
	}
	if len(problems) == 0 && !phonyRe.MatchString(makefile) {
		problems = append(problems, "Makefile: no .PHONY line")
	}
	return problems
}

// statusProblem checks a future guide's first three lines.
func statusProblem(name, text string) string {
	lines := strings.SplitN(text, "\n", 4)
	if len(lines) > 3 {
		lines = lines[:3]
	}
	if !statusRe.MatchString(strings.Join(lines, "\n")) {
		return fmt.Sprintf("%s: no status note in the first three lines (\"**Status: planned for Increment N.** ... exit(s) 64 until then\", FR-018)", name)
	}
	return ""
}
