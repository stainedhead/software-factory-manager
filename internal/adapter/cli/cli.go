// Package cli is facman's headless front end (AD-2, AD-3): the cobra
// command tree, flag rules, exit codes, text and --json output, the
// [y/N] consent prompt and signal handling. It runs commands through an
// Engine (adapter/session) and hands the no-command case to a TUIRunner
// (adapter/tui).
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/stainedhead/software-factory-manager/internal/adapter/catalogyaml"
	"github.com/stainedhead/software-factory-manager/internal/adapter/report"
	"github.com/stainedhead/software-factory-manager/internal/adapter/session"
	"github.com/stainedhead/software-factory-manager/internal/domain/appinfo"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
)

// DefaultProfile is the profile the TUI starts with (R-6).
const DefaultProfile = "agentic-factory"

// Engine runs one command. Implemented by adapter/session.
type Engine interface {
	Run(ctx context.Context, req session.Request) (session.Result, error)
}

// TUIRequest is what the TUI starts with: the root command's flags.
type TUIRequest struct {
	// Profiles default to agentic-factory.
	Profiles []string
	// With are optional packages and choice picks to pre-select.
	With []string
	// CatalogDir is --catalog; empty means embedded.
	CatalogDir string
	// Stdio is the terminal.
	Stdio Stdio
}

// TUIRunner opens the interactive TUI. Implemented by adapter/tui; when
// App.TUI is nil, facman says so and points to the headless commands.
type TUIRunner interface {
	Run(ctx context.Context, req TUIRequest) error
}

// Stdio is the process's standard streams.
type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// App is the CLI.
type App struct {
	// Engine runs commands.
	Engine Engine
	// TUI opens the TUI (adapter/tui, wired in cmd/facman).
	TUI TUIRunner
	// Stdio are the streams.
	Stdio Stdio
	// Version is the facman version.
	Version string
	// Now tells the report time; nil means time.Now.
	Now func() time.Time
	// Signals delivers SIGINT/SIGTERM; nil means signal.Notify. stop
	// releases them.
	Signals func() (ch <-chan os.Signal, stop func())
}

// Execute runs facman with args (without the program name) and returns
// the exit code (PRD Appendix H): 64 usage, 4 bootstrap, 3 validation,
// 1 operation, 2 drift (doctor), else 0.
func (a *App) Execute(ctx context.Context, args []string) int {
	err := a.execute(ctx, args)
	if err != nil {
		a.printError(err)
	}
	return failure.ExitCode(err)
}

func (a *App) now() time.Time {
	if a.Now == nil {
		return time.Now()
	}
	return a.Now()
}

func (a *App) execute(ctx context.Context, args []string) error {
	norm, err := Normalize(args)
	if err != nil {
		return err
	}
	var runErr error
	root := a.command(&runErr)
	root.SetArgs(norm)
	root.SetIn(a.Stdio.In)
	root.SetOut(a.Stdio.Out)
	root.SetErr(a.Stdio.Err)
	if err := root.ExecuteContext(ctx); err != nil {
		if runErr != nil {
			return runErr
		}
		if _, typed := failure.KindOf(err); typed {
			return err
		}
		// cobra's own errors: unknown command, bad flag, bad args.
		return failure.Usage("%s", err.Error()).WithFix("run facman --help to see the commands and flags")
	}
	return runErr
}

// deferredCommands are full-product commands that arrive later (FR-004).
var deferredCommands = map[string]string{
	"upgrade": "Increment 2", "uninstall": "Increment 2", "adopt": "Increment 2", "forget": "Increment 2",
	"restore": "Increment 2", "service": "Increment 2", "config": "Increment 3", "template": "Increment 4",
	"validate": "Increment 4", "setup": "Increment 5", "export": "Increment 5",
}

// flags shared by the commands.
type flags struct {
	profiles []string
	with     []string
	catalog  string
	json     bool
	dryRun   bool
	version  bool
	about    bool
}

// command builds the command tree. RunE errors are stored in runErr so
// they are told apart from cobra's own parse errors.
func (a *App) command(runErr *error) *cobra.Command {
	if runErr == nil {
		runErr = new(error)
	}
	wrap := func(f func(*cobra.Command, *flags) error, fl *flags) func(*cobra.Command, []string) error {
		return func(c *cobra.Command, _ []string) error {
			*runErr = f(c, fl)
			return *runErr
		}
	}
	rootFlags := &flags{}
	root := &cobra.Command{
		Use:           appinfo.Name,
		Short:         appinfo.Title,
		Long:          appinfo.Title + ": detect first, propose only what is missing, ask before acting, manage only what it installed.\n\nRun facman with no command to open the TUI.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          wrap(a.root, rootFlags),
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return failure.Usage("%s", err.Error()).WithFix("run facman --help to see the flags")
	})
	root.Flags().BoolVar(&rootFlags.version, "version", false, "print the version and exit")
	root.Flags().BoolVar(&rootFlags.about, "about", false, "print application information and exit")
	root.Flags().StringArrayVar(&rootFlags.profiles, "profile", []string{DefaultProfile}, "profile for the TUI (repeatable)")
	root.Flags().StringArrayVar(&rootFlags.with, "with", nil, "optional package or choice member to pre-select in the TUI (repeatable)")
	root.Flags().StringVar(&rootFlags.catalog, "catalog", "", "use the catalog in this directory instead of the embedded one")

	type sub struct {
		cmd         platform.Command
		short       string
		profile     bool
		dryRun      bool
		catalogFlag bool
	}
	subs := []sub{
		{platform.CommandScan, "Detect every catalog package and how it got there (read-only)", false, false, true},
		{platform.CommandPlan, "Print the exact commands that would install what a profile is missing (read-only)", true, true, true},
		{platform.CommandApply, "Install what a profile is missing, after you consent", true, true, true},
		{platform.CommandStatus, "List managed, adopted, foreign, drifted and duplicate packages (read-only)", false, false, true},
		{platform.CommandDoctor, "Check every catalog item for link conflicts and untrusted taps (read-only; exit 2 when something needs fixing)", false, false, true},
	}
	for _, s := range subs {
		fl := &flags{}
		c := &cobra.Command{Use: string(s.cmd), Short: s.short, Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true}
		cmd := s.cmd
		c.RunE = wrap(func(c *cobra.Command, fl *flags) error { return a.run(c.Context(), cmd, fl) }, fl)
		c.Flags().BoolVar(&fl.json, "json", false, "write a report schema v1 document to stdout")
		if s.catalogFlag {
			c.Flags().StringVar(&fl.catalog, "catalog", "", "use the catalog in this directory instead of the embedded one")
		}
		if s.profile {
			c.Flags().StringArrayVar(&fl.profiles, "profile", nil, "profile to use (required, repeatable; the union is wanted)")
			c.Flags().StringArrayVar(&fl.with, "with", nil, "optional package, or choice member such as colima, to include (repeatable)")
		}
		if s.dryRun {
			c.Flags().BoolVar(&fl.dryRun, "dry-run", false, "print the plan and change nothing")
		}
		root.AddCommand(c)
	}
	for name, inc := range deferredCommands {
		root.AddCommand(&cobra.Command{Use: name, Hidden: true, DisableFlagParsing: true, SilenceUsage: true, SilenceErrors: true,
			RunE: wrap(func(*cobra.Command, *flags) error {
				return failure.Usage("facman %s arrives in a later increment (%s)", name, inc).
					WithFix("Increment 1 has scan, plan, apply, status and doctor; run facman --help")
			}, nil)})
	}
	return root
}

// root handles --version, --about and the no-command case (AD-14): the TUI
// when stdout is a terminal, else the about text and a hint.
func (a *App) root(c *cobra.Command, fl *flags) error {
	switch {
	case fl.version:
		return a.write(a.Stdio.Out, appinfo.VersionLine(a.Version)+"\n")
	case fl.about:
		return a.write(a.Stdio.Out, appinfo.About(a.Version))
	}
	profiles, err := cleanValues("--profile", fl.profiles)
	if err != nil {
		return err
	}
	with, err := cleanValues("--with", fl.with)
	if err != nil {
		return err
	}
	if err := checkCatalog(fl.catalog); err != nil {
		return err
	}
	if !isTerminal(a.Stdio.Out) {
		return a.write(a.Stdio.Out, appinfo.About(a.Version)+"\nstdout is not a terminal, so the TUI was not opened; use facman scan, plan or apply (see facman --help).\n")
	}
	if !isTerminal(a.Stdio.In) {
		// Keys, and with them consent, only ever come from a terminal.
		return failure.Usage("the TUI reads keys from the terminal, and stdin is not a terminal").
			WithFix("run facman in a terminal, or use facman scan, plan --profile agentic-factory or apply --dry-run")
	}
	if a.TUI == nil {
		return a.write(a.Stdio.Out, appinfo.About(a.Version)+"\nThe interactive TUI is not available in this build; use facman scan, plan --profile agentic-factory or apply --profile agentic-factory.\n")
	}
	if err := a.TUI.Run(c.Context(), TUIRequest{Profiles: profiles, With: with, CatalogDir: fl.catalog, Stdio: a.Stdio}); err != nil {
		return asTyped(err)
	}
	return nil
}

// run validates the flags (usage errors before any I/O), runs the command
// and writes its output.
func (a *App) run(ctx context.Context, cmd platform.Command, fl *flags) error {
	if err := checkCatalog(fl.catalog); err != nil {
		return err
	}
	req := session.Request{Command: cmd, CatalogDir: fl.catalog, DryRun: fl.dryRun}
	if cmd == platform.CommandPlan || cmd == platform.CommandApply {
		var err error
		if req.Profiles, err = cleanValues("--profile", fl.profiles); err != nil {
			return err
		}
		if len(req.Profiles) == 0 {
			return failure.Usage("facman %s needs --profile <name>", cmd).
				WithFix(fmt.Sprintf("for example: facman %s --profile agentic-factory", cmd))
		}
		if req.With, err = cleanValues("--with", fl.with); err != nil {
			return err
		}
	}
	// One lock for both streams: the prompt, the progress, the signal
	// watcher and the final output may write concurrently.
	mu := &sync.Mutex{}
	stdout, stderr := &lockedWriter{mu: mu, w: a.Stdio.Out}, &lockedWriter{mu: mu, w: a.Stdio.Err}
	var out io.Writer = stdout
	if fl.json {
		out = stderr // human text goes to stderr; stdout carries only the report (R-8)
	}
	var prompt *Prompt
	if cmd == platform.CommandApply && !fl.dryRun {
		if !isTerminal(a.Stdio.In) {
			return failure.Usage("facman apply asks for consent on the terminal, and stdin is not a terminal").
				WithFix("run facman (the TUI) or facman apply in a terminal; unattended runs get configuration lists (-cl) in Increment 3; facman apply --dry-run prints the plan anywhere")
		}
		sigs, stop := a.signals()
		defer stop()
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		done := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); watchSignals(sigs, done, cancel, out) }()
		defer wg.Wait()
		defer close(done)
		// The command list and the prompt always go to stderr (R-8).
		prompt = &Prompt{In: a.Stdio.In, Out: stderr}
		req.Confirmer = prompt
		req.Progress = &textProgress{w: out}
	}
	res, err := a.Engine.Run(ctx, req)
	if prompt != nil && !prompt.Agreed() && errors.Is(err, context.Canceled) {
		// Ctrl+C before consent (during the scan or planning): nothing
		// ran, so it is a decline like Ctrl+C at the prompt (D-5, I-54).
		err, res.Outcome = nil, &execute.Outcome{Declined: true}
	}
	err = asTyped(err)
	if fl.json {
		doc := report.Build(reportInput(cmd, a.Version, a.now(), res, err))
		if werr := report.Encode(stdout, doc); werr != nil {
			return failure.Worst(err, failure.Operation("%w", werr))
		}
	} else if werr := render(out, cmd, req, res); werr != nil {
		return failure.Worst(err, failure.Operation("writing the output: %w", werr))
	}
	return err
}

// watchSignals cancels on the first signal (execute then stops after the
// current command) and swallows later ones, so the operations log stays
// consistent.
func watchSignals(sigs <-chan os.Signal, done <-chan struct{}, cancel context.CancelFunc, w io.Writer) {
	first := true
	for {
		select {
		case <-done:
			return
		case <-sigs:
			if first {
				first = false
				cancel()
				_, _ = fmt.Fprintln(w, "\nfacman: stopping after the current command finishes (it is never killed midway)")
			}
		}
	}
}

func (a *App) signals() (<-chan os.Signal, func()) {
	if a.Signals != nil {
		return a.Signals()
	}
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch, func() { signal.Stop(ch) }
}

func reportInput(cmd platform.Command, version string, now time.Time, res session.Result, err error) report.Input {
	in := report.Input{Command: cmd, Version: version, Now: now, Bootstrap: res.Bootstrap, Catalog: res.Catalog, Desired: res.Desired,
		Plan: res.Plan, Outcome: res.Outcome, Status: res.Status, Doctor: res.Doctor, Summary: res.Summary, Err: err}
	if res.Scan != nil {
		in.Statuses, in.Findings = res.Scan.Statuses, res.Scan.Findings
	}
	return in
}

// asTyped gives an untyped error the operation kind (exit 1), so only
// cobra's own errors become usage errors.
func asTyped(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := failure.KindOf(err); ok {
		return err
	}
	return failure.Operation("%w", err)
}

// cleanValues trims, rejects empty values (exit 64) and de-duplicates.
func cleanValues(flag string, vals []string) ([]string, error) {
	var out []string
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, failure.Usage("%s needs a value", flag)
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out, nil
}

// checkCatalog rejects a git URL (exit 64) before any I/O.
func checkCatalog(dir string) error {
	if dir == "" {
		return nil
	}
	_, err := catalogyaml.Dir(dir)
	if k, ok := failure.KindOf(err); ok && k == failure.KindUsage {
		return err
	}
	return nil
}

func (a *App) write(w io.Writer, s string) error {
	if _, err := io.WriteString(w, s); err != nil {
		return failure.Operation("writing the output: %w", err)
	}
	return nil
}

func (a *App) printError(err error) {
	msg := err.Error()
	var fe *failure.Error
	fix := ""
	if errors.As(err, &fe) {
		fix = fe.Fix
	}
	_, _ = fmt.Fprintf(a.Stdio.Err, "%s: %s\n", appinfo.Name, msg)
	if fix != "" {
		_, _ = fmt.Fprintf(a.Stdio.Err, "  fix: %s\n", fix)
	}
}

// isTerminal reports whether s is a terminal (an *os.File on a tty).
func isTerminal(s any) bool {
	f, ok := s.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd())) //nolint:gosec // a file descriptor fits in an int
}

// lockedWriter serializes writes that share mu.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, err := l.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("writing output: %w", err)
	}
	return n, nil
}
