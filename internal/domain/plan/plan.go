// Package plan holds what facman intends to do: commands as exact argv,
// actions ordered by dependencies, exclusions with reasons, conflicts with
// their evidence, and the consent value object. Validate enforces the safety
// invariants that every produced plan must meet.
package plan

import (
	"strconv"
	"strings"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// OwnerClass classifies who owns a conflicting path (FR-015).
type OwnerClass string

// Owner classes.
const (
	OwnerThisPackage    OwnerClass = "this-package"
	OwnerOtherBrew      OwnerClass = "other-brew"
	OwnerNPM            OwnerClass = "npm"
	OwnerOtherInstaller OwnerClass = "other-installer"
	OwnerOrphan         OwnerClass = "orphan"
)

// ActionKind is what an action does. Increment 1 only installs.
type ActionKind string

// Action kinds.
const (
	ActionBootstrapHomebrew ActionKind = "bootstrap-homebrew"
	ActionTap               ActionKind = "tap"
	ActionTrust             ActionKind = "trust"
	ActionInstall           ActionKind = "install"
)

// ExclusionReason says why a wanted package is not in the plan.
type ExclusionReason string

// Exclusion reasons.
const (
	ExcludedConflict            ExclusionReason = "conflict"
	ExcludedDependency          ExclusionReason = "dependency-excluded"
	ExcludedUnavailableHere     ExclusionReason = "unavailable-here"
	ExcludedDetectionIncomplete ExclusionReason = "detection-incomplete"
	ExcludedNotSelected         ExclusionReason = "not-selected"
)

// HomebrewPackageID is the package ID of the Homebrew bootstrap install.
const HomebrewPackageID = "homebrew"

// Command is one exact subprocess invocation (argv, never a shell string).
type Command struct {
	// Argv is the exact argv; Argv[0] is the program.
	Argv []string
	// EnvNames are the names of variables set for this command (values are never logged).
	EnvNames []string
	// NeedsSudo marks cask pkg/installer artifacts and the Homebrew installer.
	NeedsSudo bool
	// Unverified marks the Homebrew installer script (no published checksum).
	Unverified bool
	// Interactive hands the terminal to the child (set with NeedsSudo).
	Interactive bool
	// Mutating is true for every plan command.
	Mutating bool
}

// String renders argv for display, quoting arguments that need it.
func (c Command) String() string {
	parts := make([]string, len(c.Argv))
	for i, a := range c.Argv {
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\$`*?[]{}()<>|&;#~") {
			a = strconv.Quote(a)
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

// TapTrustStep describes a tap or item-trust step for a third-party tap item.
type TapTrustStep struct {
	// Tap is the tap, for example stablyai/orca.
	Tap string
	// ItemKind is "cask" or "formula".
	ItemKind string
	// Item is the full item path, for example stablyai/orca/orca.
	Item string
	// PackageID is the catalog package the step serves.
	PackageID string
}

// Item kinds for TapTrustStep.
const (
	ItemCask    = "cask"
	ItemFormula = "formula"
)

// Action is one step of the plan.
type Action struct {
	// ID is the 1-based position in the plan.
	ID int
	// Kind says what the action does.
	Kind ActionKind
	// PackageID is the catalog package (homebrew for the bootstrap).
	PackageID string
	// Commands run in order.
	Commands []Command
	// DependsOn lists the IDs of earlier actions this one needs.
	DependsOn []int
	// TapTrust is set for tap and trust actions.
	TapTrust *TapTrustStep
}

// Exclusion is a wanted package that is left out, with the reason.
type Exclusion struct {
	// PackageID is the catalog package.
	PackageID string
	// Reason is why it is left out.
	Reason ExclusionReason
	// Detail explains the reason for people.
	Detail string
}

// Plan is the ordered list of actions facman proposes.
type Plan struct {
	// Profiles are the selected profiles.
	Profiles []string
	// Actions are topologically ordered.
	Actions []Action
	// Excluded lists wanted packages left out.
	Excluded []Exclusion
	// Warnings are bootstrap findings to show with the plan (glibc, ...).
	Warnings []platform.Finding
	// CatalogRevision identifies the catalog the plan came from.
	CatalogRevision string
}

// Empty reports whether the plan has no actions.
func (p Plan) Empty() bool { return len(p.Actions) == 0 }

// Commands returns every command in order.
func (p Plan) Commands() []Command {
	var out []Command
	for _, a := range p.Actions {
		out = append(out, a.Commands...)
	}
	return out
}

// Action returns the action with the given ID.
func (p Plan) Action(id int) (Action, bool) {
	for _, a := range p.Actions {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}

// ConflictEvidence is what sits at a conflicting path (FR-016).
type ConflictEvidence struct {
	// LsLine is an ls -l style line built in Go.
	LsLine string
	// FileType is `file -b` output or a Go-derived type.
	FileType string
	// IsSymlink is true for a symlink.
	IsSymlink bool
	// SymlinkTarget is the link target.
	SymlinkTarget string
	// Dangling is true when the link target does not exist.
	Dangling bool
	// Executable is true when any execute bit is set.
	Executable bool
	// Size in bytes.
	Size int64
	// ModTime is the modification time.
	ModTime time.Time
}

// Conflict is a path a backend would write that something else holds.
type Conflict struct {
	// PackageID is the package that cannot be installed.
	PackageID string
	// Path is the conflicting path.
	Path string
	// Owner classifies who owns the path.
	Owner OwnerClass
	// OwnerDetail names the owner (brew formula gemini-cli, npm @anthropic-ai/claude-code).
	OwnerDetail string
	// Evidence describes what is there.
	Evidence ConflictEvidence
	// FailingCommand is the install command that would fail.
	FailingCommand []string
	// FixHint is the manual fix.
	FixHint string
}

// Consent records who agreed to a plan and when (FR-027).
type Consent struct {
	// By is the OS user name.
	By string
	// At is the consent time.
	At time.Time
}

// NewConsent returns the consent of user at t.
func NewConsent(user string, t time.Time) Consent { return Consent{By: user, At: t} }

// String returns <user>@<RFC 3339 UTC>, as stored in operations.consent_by.
func (c Consent) String() string { return c.By + "@" + c.At.UTC().Format(time.RFC3339) }
