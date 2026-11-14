// Package platform describes the machine facman runs on and the result of the
// startup bootstrap check: platform facts, Homebrew and Node state, and the
// findings (with severities) that decide whether a command may proceed.
package platform

import "github.com/stainedhead/software-factory-manager/internal/domain/version"

// Key selects a catalog install block.
type Key string

// Platform keys.
const (
	Darwin Key = "darwin"
	Linux  Key = "linux"
	WSL    Key = "wsl"
)

// Valid reports whether k is a known platform key.
func (k Key) Valid() bool { return k == Darwin || k == Linux || k == WSL }

// Package managers named in Distro.PkgManager.
const (
	PkgManagerApt = "apt"
	PkgManagerDnf = "dnf"
)

// Distro is the Linux distribution read from /etc/os-release.
type Distro struct {
	// ID is the os-release ID (ubuntu, debian, fedora, ...).
	ID string
	// IDLike is the os-release ID_LIKE list.
	IDLike []string
	// PkgManager is "apt", "dnf" or "" when unknown.
	PkgManager string
}

// Platform is what the bootstrap probe found about this machine.
type Platform struct {
	// OS is "darwin" or "linux".
	OS string
	// Arch is "amd64" or "arm64".
	Arch string
	// WSL is true inside Windows Subsystem for Linux.
	WSL bool
	// Display is true when GUI apps can run (always true on darwin).
	Display bool
	// Glibc is nil on darwin or when it could not be determined.
	Glibc *version.Version
	// Distro is empty on darwin.
	Distro Distro
	// UID is the invoking user's numeric ID.
	UID int
}

// Key returns the platform key used to select catalog install blocks.
func (p Platform) Key() Key {
	switch {
	case p.OS == "darwin":
		return Darwin
	case p.WSL:
		return WSL
	default:
		return Linux
	}
}

// GlibcBelow reports whether glibc is known and below minimum. An unknown glibc
// or an unparsable minimum reports false.
func (p Platform) GlibcBelow(minimum string) bool {
	if p.Glibc == nil {
		return false
	}
	m, err := version.Parse(minimum)
	if err != nil {
		return false
	}
	return p.Glibc.Known() && version.Compare(*p.Glibc, m) < 0
}

// Command names the facman command a bootstrap decision is made for.
type Command string

// Commands that run the bootstrap check.
const (
	CommandScan   Command = "scan"
	CommandPlan   Command = "plan"
	CommandApply  Command = "apply"
	CommandStatus Command = "status"
	CommandDoctor Command = "doctor"
	// CommandTUIRun is the TUI's Run step (consented execution).
	CommandTUIRun Command = "tui-run"
)

// Mutates reports whether the command changes the machine.
func (c Command) Mutates() bool { return c == CommandApply || c == CommandTUIRun }

// Severity of a finding.
type Severity string

// Severities.
const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityFatal   Severity = "fatal"
)

// Finding codes produced by the bootstrap check and doctor.
const (
	CodeUID0            = "uid0"
	CodeGlibcOld        = "glibc-old"
	CodeGlibcUnknown    = "glibc-unknown"
	CodeBrewMissing     = "brew-missing"
	CodeBrewNotOnPath   = "brew-not-on-path"
	CodeLinuxPrereqs    = "linux-prereqs"
	CodeBrewPrefixOwner = "brew-prefix-owner"
	CodeCLTMissing      = "clt-missing"
	CodeNodeOld         = "node-old"
	CodeNoDisplay       = "no-display"
	// CodeSuperseded marks a found tool the catalog says is replaced by
	// another (Gemini CLI by Antigravity CLI); info only.
	CodeSuperseded = "superseded"
	// CodeBrewShellenv means brew shellenv could not be read.
	CodeBrewShellenv = "brew-shellenv"
	// CodeStateLocked means another facman holds the state lock, so a
	// read-only command did not save its detections.
	CodeStateLocked = "state-locked"
	// CodeConflict is a doctor finding: a path a brew item would write is
	// held by something else.
	CodeConflict = "conflict"
	// CodeUntrustedTap is a doctor finding: a catalog tap item is not
	// trusted although its tap is added or the item is installed.
	CodeUntrustedTap = "untrusted-tap"
	// CodeTapPending is a doctor info: a catalog tap is not added yet;
	// apply adds and trusts it as reviewed steps.
	CodeTapPending = "tap-not-added"
	// CodeCheckFailed is a doctor info: a check could not be completed.
	CodeCheckFailed = "check-failed"
	// CodePathRelativeEntry is a scan warning: PATH has a relative or empty
	// entry, which facman ignores when it looks for programs (review FR-002).
	CodePathRelativeEntry = "path-relative-entry"
	// CodeHomebrewCaskOpts is a warning: the inherited HOMEBREW_CASK_OPTS
	// contains --no-quarantine (review FR-013).
	CodeHomebrewCaskOpts = "homebrew-cask-opts"
)

// Finding is one bootstrap or doctor observation.
type Finding struct {
	// Code identifies the finding (see the Code constants).
	Code string
	// Severity is info, warning or fatal. Fatal stops every command.
	Severity Severity
	// Message says what was found.
	Message string
	// Fix says what to do about it.
	Fix string
	// BlocksApply makes the finding fatal for mutating commands only
	// (apply and the TUI Run step), for example missing Linux prerequisites.
	BlocksApply bool
}

// FatalFor reports whether the finding stops cmd.
func (f Finding) FatalFor(cmd Command) bool {
	return f.Severity == SeverityFatal || (f.BlocksApply && cmd.Mutates())
}

// HomebrewInfo is where (and whether) Homebrew was found.
type HomebrewInfo struct {
	// Found is true when a brew executable exists on PATH or at a known prefix.
	Found bool
	// Path is the brew executable.
	Path string
	// Prefix is the Homebrew prefix (/opt/homebrew, /usr/local, /home/linuxbrew/.linuxbrew).
	Prefix string
	// OnPATH is false when brew was found only at a known prefix.
	OnPATH bool
	// PrefixOwnedByOther is true when the prefix belongs to another user.
	PrefixOwnedByOther bool
	// Version is brew's version, "" when unknown.
	Version string
	// InheritedEnv are the sorted names (never the values) of the
	// HOMEBREW_* variables every brew command inherits from the user's
	// environment, the tap-trust bypass excepted (it is stripped).
	InheritedEnv []string
}

// NodeInfo is the Node.js runtime found during bootstrap.
type NodeInfo struct {
	// Found is true when node is on PATH.
	Found bool
	// Version is the node version, "" when unknown.
	Version string
	// Provenance is where node came from (brew, nvm, fnm, asdf, volta, os, ...).
	// It holds a catalog.Provenance value; it is a string here because
	// platform sits below catalog in the domain.
	Provenance string
	// Satisfies is true when Version is at least 24.11.
	Satisfies bool
	// NPM is true when npm is on PATH.
	NPM bool
}

// BootstrapReport is the result of the startup bootstrap check.
type BootstrapReport struct {
	// Platform holds the platform facts.
	Platform Platform
	// CLT is the Xcode Command Line Tools state on macOS; nil elsewhere.
	CLT *bool
	// LinuxPrereqsMissing lists missing Homebrew prerequisites (gcc, file, ...).
	LinuxPrereqsMissing []string
	// PrereqCommand is the exact apt/dnf command that installs them.
	PrereqCommand []string
	// Homebrew is where brew was found.
	Homebrew HomebrewInfo
	// ShellEnv holds the variables from `brew shellenv`.
	ShellEnv map[string]string
	// Node is the Node.js runtime found.
	Node NodeInfo
	// Findings are the observations, in check order.
	Findings []Finding
}

// Fatal returns the findings that stop cmd, in order.
func (r BootstrapReport) Fatal(cmd Command) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.FatalFor(cmd) {
			out = append(out, f)
		}
	}
	return out
}

// HasFatal reports whether any finding stops cmd.
func (r BootstrapReport) HasFatal(cmd Command) bool { return len(r.Fatal(cmd)) > 0 }
