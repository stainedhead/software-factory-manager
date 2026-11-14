// Package inventory holds the records facman keeps about what it installed:
// package records (managed or adopted only), the append-only operations log
// with its status transitions, detection snapshots and tap-trust grants.
package inventory

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

// ErrInvalid is wrapped by record and transition validation errors.
var ErrInvalid = errors.New("invalid inventory data")

// BackendScript is the backend recorded for the Homebrew bootstrap install.
const BackendScript catalog.BackendID = "script"

// ScopeUser is the only scope in Increment 1.
const ScopeUser = "user"

// RecordState is the stored ownership state: only managed or adopted are
// ever stored; every other state is computed per scan.
type RecordState string

// Stored states.
const (
	StateManaged RecordState = "managed"
	StateAdopted RecordState = "adopted"
)

// Record is one row of the packages table.
type Record struct {
	// ID is the catalog package ID (homebrew for the bootstrap).
	ID string
	// State is managed or adopted.
	State RecordState
	// Backend is the backend that installed it.
	Backend catalog.BackendID
	// BackendRef identifies the item in the backend, for example brew-cask:claude-code.
	BackendRef string
	// InstalledVersion is the version at install time.
	InstalledVersion string
	// InstallPath is where it was installed.
	InstallPath string
	// InstalledAt is the install time.
	InstalledAt time.Time
	// InstalledBy is the consent string of the install.
	InstalledBy string
	// AdoptedAt is set for adopted records.
	AdoptedAt *time.Time
	// ManifestRev is the catalog revision at install time.
	ManifestRev string
	// Scope is "user" in Increment 1.
	Scope string
	// Bootstrap is true for the Homebrew bootstrap record.
	Bootstrap bool
}

// Validate checks the record can be stored.
func (r Record) Validate() error {
	var problems []string
	if r.ID == "" {
		problems = append(problems, "missing id")
	}
	switch r.State {
	case StateManaged:
	case StateAdopted:
		if r.AdoptedAt == nil {
			problems = append(problems, "adopted record without adopted_at")
		}
	default:
		problems = append(problems, fmt.Sprintf("state %q cannot be stored (only managed or adopted)", r.State))
	}
	if r.Backend == "" {
		problems = append(problems, "missing backend")
	}
	if r.InstalledAt.IsZero() {
		problems = append(problems, "missing installed_at")
	}
	if r.InstalledBy == "" {
		problems = append(problems, "missing installed_by")
	}
	if r.Scope != "" && r.Scope != ScopeUser {
		problems = append(problems, fmt.Sprintf("unknown scope %q", r.Scope))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: record %s: %v", ErrInvalid, r.ID, problems)
	}
	return nil
}

// OperationOp is the kind of an operations row.
type OperationOp string

// Operation kinds in Increment 1.
const (
	OpInstall   OperationOp = "install"
	OpTap       OperationOp = "tap"
	OpTrust     OperationOp = "trust"
	OpBootstrap OperationOp = "bootstrap"
)

// OpFor maps a plan action kind to the operation recorded for it.
func OpFor(k plan.ActionKind) OperationOp {
	switch k {
	case plan.ActionBootstrapHomebrew:
		return OpBootstrap
	case plan.ActionTap:
		return OpTap
	case plan.ActionTrust:
		return OpTrust
	default:
		return OpInstall
	}
}

// OperationStatus is the status of an operations row.
type OperationStatus string

// Operation statuses.
const (
	StatusInProgress  OperationStatus = "in_progress"
	StatusSucceeded   OperationStatus = "succeeded"
	StatusFailed      OperationStatus = "failed"
	StatusInterrupted OperationStatus = "interrupted"
)

// Terminal reports whether no transition leaves s.
func (s OperationStatus) Terminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusInterrupted
}

// CanTransition reports whether an operation may move from one status to
// another: only in_progress moves, to succeeded, failed or interrupted.
func CanTransition(from, to OperationStatus) bool {
	return from == StatusInProgress && to.Terminal()
}

// Operation is one row of the append-only operations log.
type Operation struct {
	// ID is assigned by the store.
	ID int64
	// StartedAt is written before the command runs.
	StartedAt time.Time
	// FinishedAt is nil while in progress.
	FinishedAt *time.Time
	// PackageID is the catalog package.
	PackageID string
	// Op is the operation kind.
	Op OperationOp
	// Commands are the exact commands (stored as JSON).
	Commands []plan.Command
	// ExitCode is nil while in progress.
	ExitCode *int
	// Status is the operation status.
	Status OperationStatus
	// LogPath is the per-operation log file.
	LogPath string
	// ConsentBy is <user>@<RFC 3339 UTC>.
	ConsentBy string
}

// Finish returns op moved to status with its exit code, finish time and log
// path, or an error when the transition is not allowed.
func (op Operation) Finish(at time.Time, exitCode int, status OperationStatus, logPath string) (Operation, error) {
	if !CanTransition(op.Status, status) {
		return op, fmt.Errorf("%w: operation %d cannot move from %s to %s", ErrInvalid, op.ID, op.Status, status)
	}
	op.FinishedAt = &at
	op.ExitCode = &exitCode
	op.Status = status
	if logPath != "" {
		op.LogPath = logPath
	}
	return op, nil
}

// DetectionSnapshot is one row of the detections table.
type DetectionSnapshot struct {
	// ScannedAt is the scan time.
	ScannedAt time.Time
	// PackageID is the catalog package.
	PackageID string
	// Found is true when the package was detected.
	Found bool
	// Version is the detected version.
	Version string
	// Provenance is where it came from.
	Provenance catalog.Provenance
	// Path is the detected path.
	Path string
}

// TapTrustGrant is one row of the tap_trust table.
type TapTrustGrant struct {
	// PackageID is the catalog package.
	PackageID string
	// Tap is the tap.
	Tap string
	// Item is the full trusted item path.
	Item string
	// GrantedAt is the grant time.
	GrantedAt time.Time
	// OperationID is the trust operation.
	OperationID int64
}

// HomebrewRef is the backend reference of the Homebrew bootstrap record.
const HomebrewRef = "script:homebrew-install.sh"

// InstallRef reads the backend and item reference from the brew install
// command among cmds (brew install [--cask] <id>), for example
// brew-cask:claude-code. ok is false when there is none.
func InstallRef(cmds []plan.Command) (backend catalog.BackendID, ref string, ok bool) {
	for _, c := range cmds {
		if len(c.Argv) < 3 || !strings.HasSuffix(c.Argv[0], "brew") || c.Argv[1] != "install" {
			continue
		}
		backend = catalog.BackendBrewFormula
		if slices.Contains(c.Argv, "--cask") {
			backend = catalog.BackendBrewCask
		}
		return backend, string(backend) + ":" + c.Argv[len(c.Argv)-1], true
	}
	return "", "", false
}
