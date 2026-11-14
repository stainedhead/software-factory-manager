package execute

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// NotRunExitCode is the exit code recorded for an operation whose command
// could not be started (or whose log could not be opened).
const NotRunExitCode = -1

// Service runs consented plans (FR-026 to FR-031, FR-050).
//
// The caller holds the state lock for the whole run (it also held it for
// the scan the plan came from); Service never takes it.
type Service struct {
	// Runner runs every plan command.
	Runner CommandRunner
	// Inventory receives the operations log, records and tap-trust grants.
	Inventory Inventory
	// Installer downloads the Homebrew installer for a bootstrap action.
	Installer HomebrewInstaller
	// State names and opens the per-operation logs and prunes old ones.
	State StateDir
	// Clock tells the time of rows and logs.
	Clock Clock
	// User names the consenting OS user.
	User UserInfo
	// Progress receives run events; nil means none.
	Progress Progress
	// Confirmer asks for consent in Apply.
	Confirmer Confirmer
	// Replanner re-scans after the Homebrew bootstrap; nil keeps the plan
	// as it is (used only where Homebrew cannot be bootstrapped).
	Replanner Replanner
	// Env is the child environment (shellenv, node@24 bin on PATH) passed
	// to every command; the bootstrap re-plan replaces it.
	Env map[string]string
}

// Options change how Apply behaves.
type Options struct {
	// DryRun returns the plan without asking or running anything.
	DryRun bool
}

// Outcome is the result of Apply or Run.
type Outcome struct {
	// Plan is the plan as run: exclusions added by the re-plan included.
	Plan plan.Plan
	// Consent is who agreed and when; zero for a dry run or a decline.
	Consent plan.Consent
	// Declined is true when the user said no.
	Declined bool
	// DryRun is true for a dry run.
	DryRun bool
	// Results has one entry per consented action, in plan order.
	Results []ActionResult
	// Cancelled is true when a cancel stopped the run early.
	Cancelled bool
	// Env is the child environment at the end of the run.
	Env map[string]string
	// BrewInstalled is true when the Homebrew bootstrap succeeded.
	BrewInstalled bool
	// BootstrapErr is set when the Homebrew installer could not be
	// downloaded or did not match the consented command.
	BootstrapErr error
	// Warnings are problems that did not change any result (log pruning).
	Warnings []string
}

// Result returns the result for the action with the given ID.
func (o Outcome) Result(actionID int) (ActionResult, bool) {
	for _, r := range o.Results {
		if r.ActionID == actionID {
			return r, true
		}
	}
	return ActionResult{}, false
}

// Err is the error the run ends with: a bootstrap failure (exit 4) when
// the installer could not be fetched, an operation failure (exit 1) when
// any action failed, was skipped or not started, the run was cancelled or
// a package was excluded for a conflict, a dependency or incomplete
// detection; nil otherwise (a decline and a dry run included).
func (o Outcome) Err() error {
	if o.BootstrapErr != nil {
		return failure.Bootstrap("installing Homebrew: %w", o.BootstrapErr)
	}
	var problems []string
	for _, r := range o.Results {
		switch r.Status {
		case ActionFailed, ActionSkipped, ActionNotStarted:
			problems = append(problems, fmt.Sprintf("action %d (%s) %s", r.ActionID, r.PackageID, r.Status))
		}
	}
	for _, ex := range o.Plan.Excluded {
		switch ex.Reason {
		case plan.ExcludedConflict, plan.ExcludedDependency, plan.ExcludedDetectionIncomplete:
			problems = append(problems, fmt.Sprintf("%s not installed (%s)", ex.PackageID, ex.Reason))
		}
	}
	if o.Cancelled {
		problems = append(problems, "the run was cancelled")
	}
	if len(problems) == 0 {
		return nil
	}
	return failure.Operation("not everything was installed").WithDetails(problems...)
}

// Apply asks for consent and runs p. A dry run returns at once and an
// empty plan needs no consent; neither runs or writes anything, and
// neither does a decline.
func (s *Service) Apply(ctx context.Context, p plan.Plan, opts Options) (Outcome, error) {
	out := Outcome{Plan: p, DryRun: opts.DryRun, Env: s.Env}
	if opts.DryRun || p.Empty() {
		return out, nil
	}
	ok, err := s.Confirmer.Confirm(ctx, p)
	if err != nil {
		return out, fmt.Errorf("asking for consent: %w", err)
	}
	if !ok {
		out.Declined = true
		return out, nil
	}
	return s.Run(ctx, p, plan.NewConsent(s.User.Username(), s.Clock.Now()))
}

// Run executes the consented plan p in order. Each operation is written
// in_progress before its command runs and finished after it. Commands run
// under a context that a cancel does not reach, so a cancel stops the run
// after the current command; the actions left are not started. A failure
// skips the actions that depend on it and nothing is retried. The returned
// error is Outcome.Err.
func (s *Service) Run(ctx context.Context, p plan.Plan, consent plan.Consent) (Outcome, error) {
	r := &run{s: s, out: Outcome{Plan: p, Consent: consent, Env: s.Env}, preset: map[int]ActionResult{}, status: map[int]ActionStatus{}}
	for i, a := range p.Actions {
		if res, ok := r.preset[a.ID]; ok {
			r.finish(a, res)
			continue
		}
		if ctx.Err() != nil {
			r.cancelFrom(p.Actions[i:])
			break
		}
		if dep, st, ok := r.unmetDependency(a); !ok {
			r.finish(a, ActionResult{ActionID: a.ID, PackageID: a.PackageID, Status: ActionSkipped,
				Detail: fmt.Sprintf("depends on action %d, which %s", dep, describe(st))})
			continue
		}
		s.started(a)
		res, cancelled := r.action(ctx, a)
		if cancelled {
			r.cancelFrom(p.Actions[i:])
			break
		}
		r.finish(a, res)
		if a.Kind == plan.ActionBootstrapHomebrew && res.Status == ActionSucceeded {
			r.out.BrewInstalled = true
			r.replan(ctx, p.Actions[i+1:])
		}
	}
	if err := s.State.PruneLogs(s.Clock.Now()); err != nil {
		r.out.Warnings = append(r.out.Warnings, fmt.Sprintf("pruning old logs: %v", err))
	}
	return r.out, r.out.Err()
}

type run struct {
	s      *Service
	out    Outcome
	preset map[int]ActionResult // results decided by the re-plan
	status map[int]ActionStatus
}

func (r *run) finish(a plan.Action, res ActionResult) {
	r.status[a.ID] = res.Status
	r.out.Results = append(r.out.Results, res)
	r.s.finished(a, res)
}

func (r *run) cancelFrom(rest []plan.Action) {
	r.out.Cancelled = true
	for _, a := range rest {
		res, ok := r.preset[a.ID]
		if !ok {
			res = ActionResult{ActionID: a.ID, PackageID: a.PackageID, Status: ActionNotStarted, Detail: "the run was cancelled"}
		}
		r.finish(a, res)
	}
}

func (r *run) unmetDependency(a plan.Action) (int, ActionStatus, bool) {
	for _, d := range a.DependsOn {
		if st := r.status[d]; st != ActionSucceeded && st != ActionAlreadyPresent {
			return d, st, false
		}
	}
	return 0, "", true
}

func describe(st ActionStatus) string {
	switch st {
	case ActionFailed:
		return "failed"
	case ActionSkipped:
		return "was skipped"
	default:
		return "did not run"
	}
}

// action runs one action. cancelled is true when a cancel arrived before
// anything was written for it (during the installer download).
func (r *run) action(ctx context.Context, a plan.Action) (ActionResult, bool) {
	res := ActionResult{ActionID: a.ID, PackageID: a.PackageID, Status: ActionFailed}
	cmds := a.Commands
	if a.Kind == plan.ActionBootstrapHomebrew {
		cmd, script, err := r.installerCommand(ctx, a)
		res.Installer = script
		if err != nil {
			if ctx.Err() != nil {
				return res, true
			}
			r.out.BootstrapErr = err
			res.Detail = err.Error()
			return res, false
		}
		cmds = []plan.Command{cmd}
	}
	recorded := r.withEnvNames(cmds)
	opID, err := r.s.Inventory.StartOperation(ctx, inventory.Operation{
		StartedAt: r.s.Clock.Now(), PackageID: a.PackageID, Op: inventory.OpFor(a.Kind),
		Commands: recorded, Status: inventory.StatusInProgress, ConsentBy: r.out.Consent.String(),
	})
	if err != nil {
		res.Detail = fmt.Sprintf("recording the operation (nothing was run): %v", err)
		return res, false
	}
	res.OperationID = opID
	res.LogPath = r.s.State.LogPath(opID, a.PackageID, r.s.Clock.Now())
	log, err := r.s.State.OpenLog(res.LogPath)
	if err != nil {
		res.ExitCode = NotRunExitCode
		res.Detail = fmt.Sprintf("opening the log (nothing was run): %v", err)
		r.finishOp(ctx, &res, inventory.StatusFailed)
		return res, false
	}
	if s := res.Installer; s != nil {
		// Which installer ran: Homebrew publishes no checksum, so record
		// the fingerprint of the exact bytes (review FR-009).
		if _, err := fmt.Fprintf(log, "# installer: sha256=%s bytes=%d\n", s.SHA256, s.Size); err != nil {
			r.out.Warnings = append(r.out.Warnings, fmt.Sprintf("writing %s: %v", res.LogPath, err))
		}
	}
	status := inventory.StatusSucceeded
	res.ExitCode = 0
	for i, c := range cmds {
		if i > 0 && ctx.Err() != nil {
			status = inventory.StatusInterrupted
			res.Detail = fmt.Sprintf("cancelled before command %d of %d", i+1, len(cmds))
			break
		}
		code, err := r.command(ctx, a, c, log)
		res.ExitCode = code
		if err != nil || code != 0 {
			status = inventory.StatusFailed
			res.Detail = commandDetail(c, code, err)
			break
		}
	}
	if err := log.Close(); err != nil {
		r.out.Warnings = append(r.out.Warnings, fmt.Sprintf("closing %s: %v", res.LogPath, err))
	}
	if status == inventory.StatusSucceeded {
		res.Status = ActionSucceeded
	}
	r.finishOp(ctx, &res, status)
	if res.Status == ActionSucceeded {
		if err := r.record(ctx, a, opID); err != nil {
			res.Status = ActionFailed
			res.Detail = fmt.Sprintf("it ran, but recording it failed: %v", err)
		}
	}
	return res, false
}

// installerCommand downloads the installer and checks that the command
// built for the file runs the consented program.
func (r *run) installerCommand(ctx context.Context, a plan.Action) (plan.Command, *InstallerScript, error) {
	script, err := r.s.Installer.Download(ctx)
	if err != nil {
		return plan.Command{}, nil, fmt.Errorf("downloading the Homebrew installer: %w", err)
	}
	cmd := r.s.Installer.Command(script.Path)
	if len(a.Commands) != 1 || len(cmd.Argv) == 0 || len(cmd.Argv) != len(a.Commands[0].Argv) || cmd.Argv[0] != a.Commands[0].Argv[0] {
		return plan.Command{}, nil, fmt.Errorf("the installer command %q is not the consented one", cmd.String())
	}
	planned := a.Commands[0]
	cmd.NeedsSudo, cmd.Interactive, cmd.Unverified, cmd.Mutating = planned.NeedsSudo, planned.Interactive, planned.Unverified, planned.Mutating
	return cmd, &script, nil
}

func (r *run) withEnvNames(cmds []plan.Command) []plan.Command {
	names := slices.Sorted(maps.Keys(r.out.Env))
	out := make([]plan.Command, len(cmds))
	for i, c := range cmds {
		c.EnvNames = append(slices.Clone(c.EnvNames), names...)
		slices.Sort(c.EnvNames)
		c.EnvNames = slices.Compact(c.EnvNames)
		out[i] = c
	}
	return out
}

// command runs c to completion: the context it runs under is never
// cancelled, so a cancel cannot kill it half-way.
func (r *run) command(ctx context.Context, a plan.Action, c plan.Command, log io.Writer) (int, error) {
	lines := &lineWriter{emit: func(line string) { r.s.output(a, line) }}
	res, err := r.s.Runner.Run(context.WithoutCancel(ctx), process.Cmd{
		Argv: c.Argv, Env: maps.Clone(r.out.Env), Interactive: c.NeedsSudo || c.Interactive,
		Log: io.MultiWriter(log, lines),
	})
	lines.flush()
	if err != nil && res.ExitCode == 0 {
		return NotRunExitCode, err
	}
	return res.ExitCode, err
}

func commandDetail(c plan.Command, code int, err error) string {
	if err != nil {
		return fmt.Sprintf("%s: %v", c.String(), err)
	}
	return fmt.Sprintf("%s exited %d", c.String(), code)
}

func (r *run) finishOp(ctx context.Context, res *ActionResult, status inventory.OperationStatus) {
	if err := r.s.Inventory.FinishOperation(context.WithoutCancel(ctx), res.OperationID, res.ExitCode, status, res.LogPath); err != nil {
		res.Status = ActionFailed
		res.Detail = joinDetail(res.Detail, fmt.Sprintf("finishing the operation record: %v", err))
	}
}

func joinDetail(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// record stores what a succeeded action changed.
func (r *run) record(ctx context.Context, a plan.Action, opID int64) error {
	ctx = context.WithoutCancel(ctx)
	now := r.s.Clock.Now()
	switch a.Kind {
	case plan.ActionBootstrapHomebrew:
		return r.s.Inventory.UpsertRecord(ctx, inventory.Record{ID: plan.HomebrewPackageID, State: inventory.StateManaged,
			Backend: inventory.BackendScript, BackendRef: inventory.HomebrewRef, InstalledAt: now,
			InstalledBy: r.out.Consent.String(), ManifestRev: r.out.Plan.CatalogRevision, Scope: inventory.ScopeUser, Bootstrap: true})
	case plan.ActionTrust:
		if a.TapTrust == nil {
			return errors.New("trust action without its tap and item")
		}
		return r.s.Inventory.RecordTapTrust(ctx, inventory.TapTrustGrant{PackageID: a.PackageID, Tap: a.TapTrust.Tap,
			Item: a.TapTrust.Item, GrantedAt: now, OperationID: opID})
	case plan.ActionInstall:
		backend, ref, ok := inventory.InstallRef(a.Commands)
		if !ok {
			return fmt.Errorf("no brew install command in action %d", a.ID)
		}
		return r.s.Inventory.UpsertRecord(ctx, inventory.Record{ID: a.PackageID, State: inventory.StateManaged,
			Backend: backend, BackendRef: ref, InstalledAt: now, InstalledBy: r.out.Consent.String(),
			ManifestRev: r.out.Plan.CatalogRevision, Scope: inventory.ScopeUser})
	default:
		return nil // a tap leaves nothing to record beyond its operation
	}
}

// replan asks the Replanner about the actions left after the bootstrap
// and applies what it found: the new environment, packages now present
// (dropped, satisfied for dependents) and packages that may no longer be
// installed (excluded with their dependents).
func (r *run) replan(ctx context.Context, rest []plan.Action) {
	if r.s.Replanner == nil || len(rest) == 0 {
		return
	}
	remaining := r.out.Plan
	remaining.Actions = rest
	rp, err := r.s.Replanner.AfterBootstrap(context.WithoutCancel(ctx), remaining)
	if err != nil {
		rp = Replan{Incomplete: map[string]string{}}
		for _, a := range rest {
			rp.Incomplete[a.PackageID] = fmt.Sprintf("the re-scan after installing Homebrew failed: %v", err)
		}
	}
	if rp.Env != nil {
		r.out.Env = rp.Env
	}
	for _, a := range rest {
		if slices.Contains(rp.Present, a.PackageID) {
			r.preset[a.ID] = ActionResult{ActionID: a.ID, PackageID: a.PackageID, Status: ActionAlreadyPresent,
				Detail: "found present after Homebrew was installed"}
		}
	}
	excl := remaining
	excl.Actions = slices.DeleteFunc(slices.Clone(rest), func(a plan.Action) bool { _, ok := r.preset[a.ID]; return ok })
	for _, id := range sortedKeys(rp.Conflicts) {
		detail := "a path it would write is held by something else"
		if c := rp.Conflicts[id]; c != nil {
			detail = c.FixHint
		}
		excl = plan.ExcludeWithDependents(excl, id, plan.ExcludedConflict, detail)
	}
	for _, id := range sortedKeys(rp.Incomplete) {
		excl = plan.ExcludeWithDependents(excl, id, plan.ExcludedDetectionIncomplete, "detection incomplete: "+rp.Incomplete[id])
	}
	kept := map[int]bool{}
	for _, a := range excl.Actions {
		kept[a.ID] = true
	}
	for _, a := range rest {
		if _, ok := r.preset[a.ID]; ok || kept[a.ID] {
			continue
		}
		res := ActionResult{ActionID: a.ID, PackageID: a.PackageID, Status: ActionSkipped, Detail: "excluded after Homebrew was installed"}
		for _, ex := range excl.Excluded {
			if ex.PackageID == a.PackageID {
				res.Detail = fmt.Sprintf("excluded after Homebrew was installed: %s", joinDetail(string(ex.Reason), ex.Detail))
			}
		}
		r.preset[a.ID] = res
	}
	r.out.Plan.Excluded = excl.Excluded
}

func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

func (s *Service) started(a plan.Action) {
	if s.Progress != nil {
		s.Progress.ActionStarted(a)
	}
}

func (s *Service) output(a plan.Action, line string) {
	if s.Progress != nil {
		s.Progress.CommandOutput(a, line)
	}
}

func (s *Service) finished(a plan.Action, res ActionResult) {
	if s.Progress != nil {
		s.Progress.ActionFinished(a, res)
	}
}

// lineWriter turns written output into lines.
type lineWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	emit func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(bytes.TrimRight(w.buf.Next(i+1), "\r\n"))
		w.emit(line)
	}
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() > 0 {
		w.emit(w.buf.String())
		w.buf.Reset()
	}
}
