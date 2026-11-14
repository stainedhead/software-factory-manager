// Package conflicts runs the link-path pre-flight for brew items: it asks
// each backend which paths an install would write and asks the path probe
// what already sits there and who owns it. This file declares the ports it
// consumes; the use case lands in workstream E (T-E4).
package conflicts

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

// Backend is the subset of an install backend the pre-flight uses.
// Implemented by adapter/brew (brew-formula, brew-cask).
type Backend interface {
	ID() catalog.BackendID
	// LinkPaths returns the paths installing step would write: cask binary
	// and app targets, or a formula's detect.commands under {prefix}/bin.
	LinkPaths(ctx context.Context, entry catalog.Entry, step catalog.InstallStep) ([]string, error)
}

// PathProbe inspects a path on the filesystem. Implemented by adapter/fsprobe.
type PathProbe interface {
	// Inspect returns nil evidence when nothing is at path.
	Inspect(ctx context.Context, path string) (*plan.ConflictEvidence, error)
	// Owner classifies who owns the file at path, with a detail such as
	// "brew formula gemini-cli".
	Owner(ctx context.Context, path string, entry catalog.Entry) (plan.OwnerClass, string, error)
}
