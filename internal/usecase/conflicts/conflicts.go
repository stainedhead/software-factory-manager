package conflicts

import (
	"context"
	"fmt"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

// Service runs the link-path pre-flight (FR-015 to FR-017).
type Service struct {
	// Backends by ID; steps whose backend is missing are not checked.
	Backends map[catalog.BackendID]Backend
	// Probe inspects paths.
	Probe PathProbe
}

// Item is one package to check with the step it would be installed by.
type Item struct {
	// Entry is the catalog entry.
	Entry catalog.Entry
	// Step is the chosen install step.
	Step catalog.InstallStep
}

// Result is the pre-flight outcome for several items.
type Result struct {
	// Conflicts by package ID.
	Conflicts map[string]*plan.Conflict
	// Failed holds packages that could not be checked, with the reason;
	// they must not be planned (detection incomplete).
	Failed map[string]string
}

// Check returns the first conflict for installing step, or nil. Each path
// the backend would write is inspected; a path held by this package is
// fine, anything else is a conflict with its evidence, owner, the command
// that would fail and a manual fix hint. facman never overwrites it.
func (s *Service) Check(ctx context.Context, entry catalog.Entry, step catalog.InstallStep) (*plan.Conflict, error) {
	b, ok := s.Backends[step.Backend]
	if !ok {
		return nil, nil
	}
	paths, err := b.LinkPaths(ctx, entry, step)
	if err != nil {
		return nil, fmt.Errorf("listing the paths %s would write: %w", entry.ID, err)
	}
	for _, p := range paths {
		ev, err := s.Probe.Inspect(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("inspecting %s: %w", p, err)
		}
		if ev == nil {
			continue
		}
		owner, detail, err := s.Probe.Owner(ctx, p, entry)
		if err != nil {
			return nil, fmt.Errorf("finding the owner of %s: %w", p, err)
		}
		if owner == plan.OwnerThisPackage {
			continue
		}
		return &plan.Conflict{
			PackageID: entry.ID, Path: p, Owner: owner, OwnerDetail: detail, Evidence: *ev,
			FailingCommand: InstallArgv(step), FixHint: FixHint(p, owner, detail),
		}, nil
	}
	return nil, nil
}

// CheckAll checks every item; an item that cannot be checked is reported
// in Failed instead of failing the others.
func (s *Service) CheckAll(ctx context.Context, items []Item) Result {
	res := Result{Conflicts: map[string]*plan.Conflict{}, Failed: map[string]string{}}
	for _, it := range items {
		c, err := s.Check(ctx, it.Entry, it.Step)
		switch {
		case err != nil:
			res.Failed[it.Entry.ID] = err.Error()
		case c != nil:
			res.Conflicts[it.Entry.ID] = c
		}
	}
	return res
}

// InstallArgv is the brew command that would fail for step.
func InstallArgv(step catalog.InstallStep) []string {
	if step.Backend == catalog.BackendBrewCask {
		return []string{"brew", "install", "--cask", step.ID}
	}
	return []string{"brew", "install", step.ID}
}

// FixHint is the manual fix shown with a conflict. facman does not move
// or delete the file in Increment 1 (move-aside arrives later).
func FixHint(path string, owner plan.OwnerClass, detail string) string {
	switch owner {
	case plan.OwnerNPM:
		pkg := strings.TrimPrefix(detail, "npm ")
		return fmt.Sprintf("%s belongs to the global npm package %s: if you no longer need it, run npm uninstall -g %s, then run facman again", path, pkg, pkg)
	case plan.OwnerOtherBrew:
		return fmt.Sprintf("%s belongs to %s: uninstall or unlink that item yourself if you want this one instead, then run facman again", path, detail)
	case plan.OwnerOtherInstaller:
		return fmt.Sprintf("%s belongs to %s: remove that install with its own uninstaller if you want the Homebrew one, then run facman again", path, detail)
	default:
		return fmt.Sprintf("nothing owns %s: check what it is, then move it aside or delete it yourself and run facman again (facman never overwrites it)", path)
	}
}
