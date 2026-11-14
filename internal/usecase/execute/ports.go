// Package execute runs a consented plan: it writes each operation as
// in_progress before running it, finishes it with the exit code and log
// path, records managed packages and tap-trust grants, skips dependents of
// failed actions, re-plans after the Homebrew bootstrap and stops after the
// current command on cancel. This file declares the ports it consumes.
package execute

import (
	"context"
	"io"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// CommandRunner runs plan commands. Interactive commands (NeedsSudo) get
// the terminal. Implemented by adapter/execrunner, and wrapped by
// adapter/tui for the tea.ExecProcess hand-off (AD-16).
type CommandRunner interface {
	Run(ctx context.Context, cmd process.Cmd) (process.Result, error)
}

// Inventory is the subset of the inventory store execute writes.
// Implemented by adapter/sqlite.
type Inventory interface {
	StartOperation(ctx context.Context, op inventory.Operation) (int64, error)
	FinishOperation(ctx context.Context, id int64, exitCode int, status inventory.OperationStatus, logPath string) error
	UpsertRecord(ctx context.Context, r inventory.Record) error
	RecordTapTrust(ctx context.Context, g inventory.TapTrustGrant) error
}

// HomebrewInstaller downloads the official installer to a temp file (never
// piped; 3 attempts with backoff) and builds its command. Implemented by
// adapter/installer.
type HomebrewInstaller interface {
	Download(ctx context.Context) (InstallerScript, error)
	Command(path string) plan.Command
}

// InstallerScript is the downloaded installer: where it was saved and the
// fingerprint of the exact bytes that run (review FR-009).
type InstallerScript struct {
	// Path is the temp file.
	Path string
	// SHA256 is the hex SHA-256 of the file.
	SHA256 string
	// Size is the file size in bytes.
	Size int64
}

// StateDir is the subset of the state directory execute uses.
// Implemented by adapter/statedir.
type StateDir interface {
	// LogPath returns <state>/logs/<YYYYMMDD-HHMMSS>-<opID>-<pkgID>.log.
	LogPath(opID int64, pkgID string, t time.Time) string
	// Lock takes the exclusive lock, waiting until ctx is done.
	Lock(ctx context.Context) (unlock func(), err error)
	// TryLock takes the lock without waiting; when another process holds
	// it, unlock is nil and holder is that process's PID.
	TryLock() (unlock func(), holder int, err error)
	// PruneLogs deletes logs older than 90 days.
	PruneLogs(now time.Time) error
	// OpenLog creates (or appends to) the log file at path, creating its
	// directory, readable by the user only.
	OpenLog(path string) (io.WriteCloser, error)
}

// Replan is what changed once Homebrew was bootstrapped: the refreshed
// child environment and the re-scan of the packages still to install.
type Replan struct {
	// Env is the new child environment (brew shellenv plus the node@24 keg
	// on PATH); it replaces the environment of every later command.
	Env map[string]string
	// Present are packages now found present: their actions are dropped.
	Present []string
	// Conflicts by package ID: these and their dependents are excluded.
	Conflicts map[string]*plan.Conflict
	// Incomplete holds packages whose detection or pre-flight could not
	// finish, with the reason: these and their dependents are excluded.
	Incomplete map[string]string
}

// Replanner re-locates brew, refreshes shellenv and re-scans the packages
// of remaining right after the Homebrew bootstrap succeeds. Implemented in
// cmd/facman wiring over the bootstrap, scan and conflicts use cases.
type Replanner interface {
	AfterBootstrap(ctx context.Context, remaining plan.Plan) (Replan, error)
}

// Clock tells the time. Implemented by adapter/execrunner (system clock).
type Clock interface {
	Now() time.Time
}

// UserInfo names the invoking OS user. Implemented by adapter/execrunner.
type UserInfo interface {
	Username() string
}

// ActionStatus is how an action ended.
type ActionStatus string

// Action statuses.
const (
	ActionSucceeded ActionStatus = "succeeded"
	ActionFailed    ActionStatus = "failed"
	// ActionSkipped means a dependency failed or was excluded.
	ActionSkipped ActionStatus = "skipped"
	// ActionNotStarted means the run was cancelled before it.
	ActionNotStarted ActionStatus = "not-started"
	// ActionAlreadyPresent means the re-scan after the Homebrew bootstrap
	// found the package present, so its action was dropped; dependents
	// treat it as satisfied.
	ActionAlreadyPresent ActionStatus = "already-present"
)

// ActionResult is the outcome of one action.
type ActionResult struct {
	// ActionID is the plan action.
	ActionID int
	// PackageID is the catalog package.
	PackageID string
	// Status is how it ended.
	Status ActionStatus
	// ExitCode of the last command run.
	ExitCode int
	// LogPath is the operation log.
	LogPath string
	// OperationID is the operations row, 0 when none was written.
	OperationID int64
	// Detail explains a failure or skip.
	Detail string
	// Installer is the downloaded Homebrew installer, for the bootstrap
	// action only (nil otherwise).
	Installer *InstallerScript
}

// Progress receives run events. Implemented by adapter/cli (text) and
// adapter/tui (messages).
type Progress interface {
	ActionStarted(a plan.Action)
	CommandOutput(a plan.Action, line string)
	ActionFinished(a plan.Action, r ActionResult)
}

// Confirmer asks for consent to a plan. Implemented by adapter/cli
// ([y/N] prompt) and adapter/tui (Enter on Review).
type Confirmer interface {
	Confirm(ctx context.Context, p plan.Plan) (bool, error)
}
