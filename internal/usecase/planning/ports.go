// Package planning turns the desired set, the package statuses and the
// bootstrap report into a Plan: the Homebrew installer first when brew is
// missing, then tap, trust and install actions in dependency order, with
// exclusions and their reasons. This file declares the ports it consumes;
// the use case lands in workstream E (T-E5).
package planning

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

// Caps describes what a backend may need.
type Caps struct {
	// NeedsSudo is true when some installs through this backend may prompt
	// for sudo (cask pkg/installer artifacts).
	NeedsSudo bool
}

// Backend builds the install actions for a catalog entry. Implemented by
// adapter/brew (brew-formula, brew-cask).
type Backend interface {
	ID() catalog.BackendID
	// Available reports whether the backend can run here (brew located).
	Available(ctx context.Context) bool
	// InstallActions returns the actions that install step, in order and
	// without IDs or dependencies (the planner assigns those): for a tap
	// item, a tap action and an item-trust action (each with TapTrust set,
	// omitted when TapState says it is already in place), then the install
	// action. Commands are exact argv with NeedsSudo set for casks with pkg
	// or installer artifacts, and never contain --force or --overwrite.
	InstallActions(ctx context.Context, entry catalog.Entry, step catalog.InstallStep) ([]plan.Action, error)
	// LinkPaths returns the paths installing step would write.
	LinkPaths(ctx context.Context, entry catalog.Entry, step catalog.InstallStep) ([]string, error)
	// TapState reports whether step's tap is tapped and its item trusted
	// (read-only; unknown output counts as not trusted).
	TapState(ctx context.Context, step catalog.InstallStep) (tapped, trusted bool, err error)
	Capabilities() Caps
}

// CatalogSource loads the catalog. Implemented by adapter/catalogyaml.
type CatalogSource interface {
	Load(ctx context.Context) (catalog.Catalog, error)
}
