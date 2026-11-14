// Package catalog holds the catalog model (packages facman knows how to
// detect and install), profiles, catalog validation, per-platform install
// selection and profile resolution into a desired set.
package catalog

import "github.com/stainedhead/software-factory-manager/internal/domain/platform"

// APIVersion is the only catalog and profile apiVersion this build reads.
const APIVersion = "facman/v1"

// Kind drives WSL inheritance and display rules.
type Kind string

// Kinds.
const (
	KindCLI Kind = "cli"
	KindGUI Kind = "gui"
)

// BackendID names an install backend.
type BackendID string

// Backends. npm parses but has no adapter in Increment 1; report and
// host-report are pseudo-backends that never mutate.
const (
	BackendBrewFormula BackendID = "brew-formula"
	BackendBrewCask    BackendID = "brew-cask"
	BackendNPM         BackendID = "npm"
	BackendReport      BackendID = "report"
	BackendHostReport  BackendID = "host-report"
)

// Known reports whether b is a backend the catalog format defines.
func (b BackendID) Known() bool {
	switch b {
	case BackendBrewFormula, BackendBrewCask, BackendNPM, BackendReport, BackendHostReport:
		return true
	}
	return false
}

// ReportOnly reports whether b only reports (never installs).
func (b BackendID) ReportOnly() bool { return b == BackendReport || b == BackendHostReport }

// IsBrew reports whether b is a Homebrew backend.
func (b BackendID) IsBrew() bool { return b == BackendBrewFormula || b == BackendBrewCask }

// VersionPolicy is parsed and stored; its behaviour is deferred (FR-019).
type VersionPolicy string

// Version policies.
const (
	PolicyTrack    VersionPolicy = "track"
	PolicyPinned   VersionPolicy = "pinned"
	PolicyDelegate VersionPolicy = "delegate"
)

// Provenance says where a detected install came from. It is declared here
// because catalog detect specs map provenances to path globs; package
// ownership uses the same type.
type Provenance string

// Provenances.
const (
	ProvBrew      Provenance = "brew"
	ProvBrewCask  Provenance = "brew-cask"
	ProvNative    Provenance = "native"
	ProvNPM       Provenance = "npm"
	ProvNVM       Provenance = "nvm"
	ProvFNM       Provenance = "fnm"
	ProvASDF      Provenance = "asdf"
	ProvVolta     Provenance = "volta"
	ProvScript    Provenance = "script"
	ProvAppBundle Provenance = "app-bundle"
	ProvOS        Provenance = "os"
	ProvUnknown   Provenance = "unknown"
)

// TrustItem is the only accepted value of InstallStep.Trust.
const TrustItem = "item"

// VersionProbe is the argv that prints a tool's version and the regex whose
// first group extracts it.
type VersionProbe struct {
	// Cmd is the argv, for example ["claude", "--version"].
	Cmd []string
	// Regex has one capture group holding the version.
	Regex string
}

// DetectSpec says how to find a package.
type DetectSpec struct {
	// Commands are executable names looked up on PATH.
	Commands []string
	// BundleIDs are macOS app bundle identifiers.
	BundleIDs []string
	// WingetID is the Windows-host package (WSL hints).
	WingetID string
	// Version is the version probe, if any.
	Version *VersionProbe
	// Provenance maps a provenance to a path glob with {brew_prefix},
	// {npm_prefix} and ~ templates.
	Provenance map[Provenance]string
	// NPMPackage is the global npm package name, if any.
	NPMPackage string
}

// InstallStep is one backend option in a platform's install block.
type InstallStep struct {
	// Backend names the backend.
	Backend BackendID
	// ID is the formula, cask or npm name (full tap path for tap items).
	ID string
	// Tap is the third-party tap the item lives in, if any.
	Tap string
	// Trust is "item" (item-level tap trust) or empty.
	Trust string
	// Version is an optional pin.
	Version string
	// Hint is shown for report backends and unavailable items.
	Hint string
	// PostInstall is parsed but unused in Increment 1.
	PostInstall []string
}

// Requirement is a dependency on another catalog package.
type Requirement struct {
	// Package is the required catalog ID.
	Package string
	// MinVersion is an optional minimum version.
	MinVersion string
}

// Satisfaction makes any provenance at or above MinVersion count as present.
type Satisfaction struct {
	// MinVersion is the minimum satisfying version.
	MinVersion string
}

// Rejection makes a detection below VersionLT not the wanted tool.
type Rejection struct {
	// VersionLT rejects versions below this.
	VersionLT string
}

// ChoiceMember is one alternative of a choice group.
type ChoiceMember struct {
	// ID is the member's catalog ID.
	ID string
	// Platforms restricts where the member is offered; empty means all.
	Platforms []platform.Key
	// HostOn lists platforms where the member lives on the host (report only).
	HostOn []platform.Key
}

// ServiceSpec is kept opaque in Increment 1 (Paperclip).
type ServiceSpec struct {
	// Raw is the decoded service block.
	Raw map[string]any
}

// Entry is one catalog package (data-dictionary §2.3).
type Entry struct {
	// ID is the kebab-case unique ID.
	ID string
	// Display is the human name.
	Display string
	// Category groups entries in the TUI.
	Category string
	// Kind is cli or gui; empty only for choice groups.
	Kind Kind
	// Platforms restricts the entry; empty means all.
	Platforms []platform.Key
	// Detect says how to find the package.
	Detect DetectSpec
	// Install maps platform keys to ordered backend options.
	Install map[platform.Key][]InstallStep
	// VersionPolicy defaults to track.
	VersionPolicy VersionPolicy
	// Requires lists dependencies, planned first.
	Requires []Requirement
	// SatisfiesIf makes another provenance count as present.
	SatisfiesIf *Satisfaction
	// RejectIf marks old versions as not the wanted tool.
	RejectIf *Rejection
	// Choice is non-empty for a choice group.
	Choice []ChoiceMember
	// SatisfiedByAnyPresent marks a choice group satisfied by any present member.
	SatisfiedByAnyPresent bool
	// SignInRequired drives the Summary's sign-in next steps (FR-112).
	SignInRequired bool
	// Service is parsed and kept opaque.
	Service *ServiceSpec
	// SupersededBy names the catalog entry that replaces this one. A
	// superseded entry is detection-only: it has no install block, no
	// profile may want it, and when it is found scan reports it as foreign
	// with an info finding (Gemini CLI, superseded by Antigravity CLI).
	SupersededBy string
}

// IsChoice reports whether the entry is a choice group.
func (e Entry) IsChoice() bool { return len(e.Choice) > 0 }

// OfferedOn reports whether the entry's platforms list allows key.
func (e Entry) OfferedOn(key platform.Key) bool { return keyIn(e.Platforms, key) }

// Member returns the choice member with the given ID.
func (e Entry) Member(id string) (ChoiceMember, bool) {
	for _, m := range e.Choice {
		if m.ID == id {
			return m, true
		}
	}
	return ChoiceMember{}, false
}

// MemberIDs lists the choice member IDs in order.
func (e Entry) MemberIDs() []string {
	ids := make([]string, 0, len(e.Choice))
	for _, m := range e.Choice {
		ids = append(ids, m.ID)
	}
	return ids
}

// ProfilePackage is one package line in a profile.
type ProfilePackage struct {
	// ID is the catalog ID.
	ID string
	// Optional entries are unselected unless --with names them.
	Optional bool
	// Version is an optional pin.
	Version string
}

// Profile is a named set of packages a factory needs.
type Profile struct {
	// Name is the profile name.
	Name string
	// Description says what the profile is for.
	Description string
	// Include lists other profiles whose packages are added.
	Include []string
	// Packages lists the profile's own packages in order.
	Packages []ProfilePackage
	// Policy is parsed and unused in Increment 1.
	Policy map[string]any
}

// Catalog is the full set of packages and profiles (data-dictionary §2.4).
type Catalog struct {
	// APIVersion is facman/v1.
	APIVersion string
	// Entries are ordered as in the file (the planner's tie-break order).
	Entries []Entry
	// Profiles are keyed by name.
	Profiles map[string]Profile
	// Revision is embedded:<version> or dir:<abs path>@<sha256>.
	Revision string
}

// Entry returns the entry with the given ID.
func (c Catalog) Entry(id string) (Entry, bool) {
	if i := c.Index(id); i >= 0 {
		return c.Entries[i], true
	}
	return Entry{}, false
}

// Index returns the entry's position in catalog order, or -1.
func (c Catalog) Index(id string) int {
	for i, e := range c.Entries {
		if e.ID == id {
			return i
		}
	}
	return -1
}

func keyIn(keys []platform.Key, key platform.Key) bool {
	if len(keys) == 0 {
		return true
	}
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}
