// Package scan runs the detectors in parallel with timeouts, reconciles the
// inventory (interrupted operations, drift, AD-8), classifies every catalog
// entry and persists the detection snapshot. This file declares the ports
// it consumes and the detection environment; the use case lands in
// workstream E (T-E3).
package scan

import (
	"context"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// DetectorTimeout is the per-detector timeout (FR-010).
const DetectorTimeout = 10 * time.Second

// Env is what detectors need to know about the machine.
type Env struct {
	// Platform holds the platform facts.
	Platform platform.Platform
	// Home is the user's home directory (expands ~ in provenance globs).
	Home string
	// Path is the child PATH, split into directories, in order.
	Path []string
	// BrewPrefix is the Homebrew prefix ("" when Homebrew is missing).
	BrewPrefix string
	// BrewPath is the brew executable ("" when Homebrew is missing).
	BrewPath string
	// NPMPrefix is the global npm prefix ("" when npm is missing).
	NPMPrefix string
	// CLT is the Xcode CLT state on darwin (nil elsewhere); when false,
	// /usr/bin/git must never be executed (FR-020).
	CLT *bool
	// Vars are the environment overrides every detector command gets
	// (shellenv, HOMEBREW_NO_AUTO_UPDATE=1, HOMEBREW_NO_ENV_HINTS=1).
	Vars map[string]string
}

// Detector finds catalog packages one way (PATH, brew, app bundle, npm).
// It returns detections for the entries it covers; on timeout or failure it
// returns an error, and scan records ownership.DetectionUnknown for them.
// Implemented by adapter/detect (path, bundle, npm) and adapter/brew (brew).
type Detector interface {
	ID() ownership.DetectorID
	Detect(ctx context.Context, entries []catalog.Entry, env Env) ([]ownership.Detection, error)
}

// Inventory is the subset of the inventory store scan uses. Implemented by
// adapter/sqlite.
type Inventory interface {
	Records(ctx context.Context) ([]inventory.Record, error)
	UpsertRecord(ctx context.Context, r inventory.Record) error
	MarkInterrupted(ctx context.Context) ([]inventory.Operation, error)
	SaveDetections(ctx context.Context, snaps []inventory.DetectionSnapshot) error
}

// CatalogSource loads the embedded catalog or a --catalog directory.
// Implemented by adapter/catalogyaml.
type CatalogSource interface {
	Load(ctx context.Context) (catalog.Catalog, error)
}

// StateDir is the subset of the state directory scan uses: the lock decides
// whether scan may write (reconcile and snapshots). Implemented by
// adapter/statedir.
type StateDir interface {
	// TryLock takes the exclusive lock without waiting. When another
	// process holds it, unlock is nil and holder is that process's PID.
	TryLock() (unlock func(), holder int, err error)
}

// CommandRunner runs version probes and other read-only commands.
// Implemented by adapter/execrunner.
type CommandRunner interface {
	Run(ctx context.Context, cmd process.Cmd) (process.Result, error)
}
