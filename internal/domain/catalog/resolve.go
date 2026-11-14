package catalog

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
)

// DesiredItem is one package the selected profiles want (data-dictionary §2.5).
type DesiredItem struct {
	// ID is the catalog ID.
	ID string
	// Required is true when any selected profile lists it without optional.
	Required bool
	// Selected is true when Required, named by --with, picked as a choice
	// member, or required by another selected item.
	Selected bool
	// Version is the pin, if any profile pins it.
	Version string
	// FromProfiles lists the profiles that ask for it ("which profile asked").
	FromProfiles []string
	// ChoicePick is the member picked for a choice group.
	ChoicePick string
	// RequiredBy lists the selected items that pulled this one in: the choice
	// group for a picked member, or dependents through requires.
	RequiredBy []string
}

// DesiredSet is the union of the selected profiles, in catalog order.
type DesiredSet struct {
	// Profiles are the selected profile names, de-duplicated, in given order.
	Profiles []string
	// Items are ordered by catalog order.
	Items []DesiredItem
}

// Item returns the desired item with the given ID.
func (d DesiredSet) Item(id string) (DesiredItem, bool) {
	for _, it := range d.Items {
		if it.ID == id {
			return it, true
		}
	}
	return DesiredItem{}, false
}

// SelectedIDs lists the selected items' IDs in catalog order.
func (d DesiredSet) SelectedIDs() []string {
	var ids []string
	for _, it := range d.Items {
		if it.Selected {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

type resolver struct {
	c        Catalog
	items    map[string]*DesiredItem
	pinnedBy map[string]string
	problems []string
}

func (r *resolver) addf(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

// Resolve unions the named profiles (with their includes) into a desired
// set and applies --with selections (FR-079): required beats optional, the
// same pin twice is fine, different pins are an error naming both profiles,
// --with selects an optional item or picks a choice-group member, and
// requires of selected items are selected too. Every problem is returned
// together as one failure.Validation error.
func Resolve(c Catalog, profiles []string, with []string) (DesiredSet, error) {
	r := &resolver{c: c, items: map[string]*DesiredItem{}, pinnedBy: map[string]string{}}
	names := dedupe(profiles)
	if len(names) == 0 {
		r.addf("no profile selected (known profiles: %s)", strings.Join(c.ProfileNames(), ", "))
	}
	visited := map[string]bool{}
	for _, name := range names {
		r.visitProfile(name, nil, visited)
	}
	r.applyWith(dedupe(with))
	r.expandSelections()
	if len(r.problems) > 0 {
		return DesiredSet{}, failure.Validation("cannot resolve profiles %s", strings.Join(names, ", ")).
			WithDetails(r.problems...).
			WithFix("check the profile names, --with IDs and version pins")
	}
	out := DesiredSet{Profiles: names}
	for _, it := range r.items {
		out.Items = append(out.Items, *it)
	}
	sort.SliceStable(out.Items, func(i, j int) bool { return c.Index(out.Items[i].ID) < c.Index(out.Items[j].ID) })
	return out, nil
}

func (r *resolver) visitProfile(name string, path []string, visited map[string]bool) {
	if i := slices.Index(path, name); i >= 0 {
		r.addf("profile include cycle: %s", strings.Join(append(append([]string{}, path[i:]...), name), " -> "))
		return
	}
	p, ok := r.c.Profiles[name]
	if !ok {
		r.addf("unknown profile %q (known profiles: %s)", name, strings.Join(r.c.ProfileNames(), ", "))
		return
	}
	if visited[name] {
		return
	}
	path = append(path, name)
	for _, inc := range p.Include {
		r.visitProfile(inc, path, visited)
	}
	visited[name] = true
	for _, pkg := range p.Packages {
		r.addPackage(name, pkg)
	}
}

func (r *resolver) addPackage(profile string, pkg ProfilePackage) {
	if _, ok := r.c.Entry(pkg.ID); !ok {
		r.addf("profile %s: unknown package %q", profile, pkg.ID)
		return
	}
	it := r.items[pkg.ID]
	if it == nil {
		it = &DesiredItem{ID: pkg.ID}
		r.items[pkg.ID] = it
	}
	if !pkg.Optional {
		it.Required, it.Selected = true, true
	}
	if !slices.Contains(it.FromProfiles, profile) {
		it.FromProfiles = append(it.FromProfiles, profile)
	}
	if pkg.Version == "" {
		return
	}
	switch {
	case it.Version == "":
		it.Version = pkg.Version
		r.pinnedBy[pkg.ID] = profile
	case it.Version != pkg.Version:
		r.addf("%s: conflicting version pins %q (profile %s) and %q (profile %s)",
			pkg.ID, it.Version, r.pinnedBy[pkg.ID], pkg.Version, profile)
	}
}

func (r *resolver) applyWith(with []string) {
	for _, id := range with {
		if it, ok := r.items[id]; ok {
			e, _ := r.c.Entry(id)
			if e.IsChoice() {
				r.addf("--with %s: %s is a choice group; name one member: %s", id, id, strings.Join(e.MemberIDs(), ", "))
				continue
			}
			it.Selected = true
			continue
		}
		if group := r.groupWithMember(id); group != nil {
			if group.ChoicePick != "" && group.ChoicePick != id {
				r.addf("--with %s: choice group %s already has %s picked; pick only one", id, group.ID, group.ChoicePick)
				continue
			}
			group.ChoicePick, group.Selected = id, true
			continue
		}
		if _, ok := r.c.Entry(id); ok {
			r.addf("--with %s: not in the selected profiles (optional items: %s)", id, strings.Join(r.optionalIDs(), ", "))
			continue
		}
		r.addf("--with %s: unknown package (optional items: %s)", id, strings.Join(r.optionalIDs(), ", "))
	}
}

func (r *resolver) groupWithMember(member string) *DesiredItem {
	for _, e := range r.c.Entries {
		if _, ok := e.Member(member); ok {
			if it := r.items[e.ID]; it != nil {
				return it
			}
		}
	}
	return nil
}

func (r *resolver) optionalIDs() []string {
	var ids []string
	for _, e := range r.c.Entries {
		it := r.items[e.ID]
		if it == nil || it.Required {
			continue
		}
		if e.IsChoice() {
			ids = append(ids, e.MemberIDs()...)
			continue
		}
		ids = append(ids, e.ID)
	}
	return ids
}

// expandSelections adds picked choice members and the transitive requires of
// every selected item.
func (r *resolver) expandSelections() {
	var queue []string
	for _, e := range r.c.Entries {
		if it := r.items[e.ID]; it != nil && it.Selected {
			queue = append(queue, e.ID)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		it := r.items[id]
		e, _ := r.c.Entry(id)
		var deps []string
		if it.ChoicePick != "" {
			deps = append(deps, it.ChoicePick)
		}
		for _, req := range e.Requires {
			deps = append(deps, req.Package)
		}
		for _, dep := range deps {
			if _, ok := r.c.Entry(dep); !ok {
				continue // reported by Catalog.Validate
			}
			d := r.items[dep]
			if d == nil {
				d = &DesiredItem{ID: dep}
				r.items[dep] = d
			}
			if !slices.Contains(d.RequiredBy, id) {
				d.RequiredBy = append(d.RequiredBy, id)
			}
			if !d.Selected {
				d.Selected = true
				queue = append(queue, dep)
			}
		}
	}
}

func dedupe(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
