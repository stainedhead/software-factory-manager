package plan

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
)

// OrderPackages returns ids in a stable topological order: each package's
// requires (that are also in ids) come first, and ties keep catalog order
// (FR-028). Unknown IDs sort last in the order given. A cycle is an error.
func OrderPackages(c catalog.Catalog, ids []string) ([]string, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	sorted := append([]string(nil), ids...)
	sort.SliceStable(sorted, func(i, j int) bool { return rank(c, sorted[i]) < rank(c, sorted[j]) })

	var out []string
	state := map[string]int{} // 1 visiting, 2 done
	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch state[id] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("requires cycle: %s", strings.Join(append(path, id), " -> "))
		}
		state[id] = 1
		e, _ := c.Entry(id)
		deps := make([]string, 0, len(e.Requires))
		for _, r := range e.Requires {
			if want[r.Package] {
				deps = append(deps, r.Package)
			}
		}
		sort.SliceStable(deps, func(i, j int) bool { return rank(c, deps[i]) < rank(c, deps[j]) })
		for _, d := range deps {
			if err := visit(d, append(path, id)); err != nil {
				return err
			}
		}
		state[id] = 2
		out = append(out, id)
		return nil
	}
	for _, id := range sorted {
		if err := visit(id, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rank(c catalog.Catalog, id string) int {
	if i := c.Index(id); i >= 0 {
		return i
	}
	return len(c.Entries)
}

// ExcludeWithDependents returns a copy of p without the actions of pkgID and
// without every action that transitively depends on them. pkgID is recorded
// with reason and detail; each dependent package is recorded as
// dependency-excluded.
func ExcludeWithDependents(p Plan, pkgID string, reason ExclusionReason, detail string) Plan {
	removed := map[int]bool{}
	removedPkgs := []string{pkgID}
	for _, a := range p.Actions {
		if a.PackageID == pkgID {
			removed[a.ID] = true
		}
	}
	changed := true
	for changed {
		changed = false
		for _, a := range p.Actions {
			if removed[a.ID] {
				continue
			}
			for _, d := range a.DependsOn {
				if removed[d] {
					removed[a.ID] = true
					changed = true
					if !slices.Contains(removedPkgs, a.PackageID) {
						removedPkgs = append(removedPkgs, a.PackageID)
					}
					break
				}
			}
		}
	}
	out := p
	out.Actions = nil
	for _, a := range p.Actions {
		if !removed[a.ID] {
			out.Actions = append(out.Actions, a)
		}
	}
	out.Excluded = append([]Exclusion(nil), p.Excluded...)
	for i, id := range removedPkgs {
		if hasExclusion(out.Excluded, id) {
			continue
		}
		ex := Exclusion{PackageID: id, Reason: reason, Detail: detail}
		if i > 0 {
			ex = Exclusion{PackageID: id, Reason: ExcludedDependency, Detail: fmt.Sprintf("depends on %s (%s)", pkgID, reason)}
		}
		out.Excluded = append(out.Excluded, ex)
	}
	return out
}

func hasExclusion(ex []Exclusion, id string) bool {
	for _, e := range ex {
		if e.PackageID == id {
			return true
		}
	}
	return false
}
