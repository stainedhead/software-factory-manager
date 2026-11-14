package catalog

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// Validate checks the whole catalog and returns one failure.Validation error
// listing every problem found, or nil. It checks: apiVersion, duplicate IDs,
// missing or unknown kind, unknown backends, trust other than "item", unknown
// platform keys, unknown requires, requires cycles, unknown or nested choice
// members, unknown profile packages and includes, and include cycles.
func (c Catalog) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if c.APIVersion != APIVersion {
		add("apiVersion is %q, want %q", c.APIVersion, APIVersion)
	}
	seen := map[string]bool{}
	for _, e := range c.Entries {
		if e.ID == "" {
			add("an entry has no id")
			continue
		}
		if seen[e.ID] {
			add("packages.%s: duplicate id", e.ID)
		}
		seen[e.ID] = true
	}
	for _, e := range c.Entries {
		problems = append(problems, c.entryProblems(e, seen)...)
	}
	if cycle := c.requiresCycle(); cycle != nil {
		add("requires cycle: %s", strings.Join(cycle, " -> "))
	}
	problems = append(problems, c.profileProblems(seen)...)

	if len(problems) == 0 {
		return nil
	}
	return failure.Validation("catalog %s is invalid", c.revisionLabel()).
		WithDetails(problems...).
		WithFix("fix the catalog entries listed and run again")
}

func (c Catalog) revisionLabel() string {
	if c.Revision == "" {
		return "(unnamed)"
	}
	return c.Revision
}

func (c Catalog) entryProblems(e Entry, ids map[string]bool) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("packages.%s: ", e.ID)+fmt.Sprintf(format, args...))
	}
	if e.IsChoice() {
		if e.Kind != "" && e.Kind != KindCLI && e.Kind != KindGUI {
			add("unknown kind %q", e.Kind)
		}
		for _, m := range e.Choice {
			member, ok := c.Entry(m.ID)
			switch {
			case !ok:
				add("unknown choice member %q", m.ID)
			case member.IsChoice():
				add("choice member %q is itself a choice group", m.ID)
			}
			problems = append(problems, keyProblems("packages."+e.ID+".choice."+m.ID+".platforms", m.Platforms)...)
			problems = append(problems, keyProblems("packages."+e.ID+".choice."+m.ID+".host_on", m.HostOn)...)
		}
	} else {
		switch e.Kind {
		case KindCLI, KindGUI:
		case "":
			add("missing kind (cli or gui)")
		default:
			add("unknown kind %q", e.Kind)
		}
	}
	problems = append(problems, keyProblems("packages."+e.ID+".platforms", e.Platforms)...)
	keys := make([]string, 0, len(e.Install))
	for k := range e.Install {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !platform.Key(k).Valid() {
			add("unknown install platform %q", k)
		}
		for i, s := range e.Install[platform.Key(k)] {
			if !s.Backend.Known() {
				add("install.%s[%d]: unknown backend %q", k, i, s.Backend)
			}
			if s.Trust != "" && s.Trust != TrustItem {
				add("install.%s[%d]: trust must be %q, got %q (whole-tap trust is never used)", k, i, TrustItem, s.Trust)
			}
			if s.Trust != "" && s.Tap == "" {
				add("install.%s[%d]: trust without tap", k, i)
			}
			if !s.Backend.ReportOnly() && s.Backend.Known() && s.ID == "" {
				add("install.%s[%d]: missing id", k, i)
			}
		}
	}
	switch e.VersionPolicy {
	case "", PolicyTrack, PolicyPinned, PolicyDelegate:
	default:
		add("unknown version_policy %q", e.VersionPolicy)
	}
	for _, r := range e.Requires {
		if !ids[r.Package] {
			add("unknown requires %q", r.Package)
		}
	}
	if e.SupersededBy != "" {
		if !ids[e.SupersededBy] {
			add("unknown superseded_by %q", e.SupersededBy)
		}
		if len(e.Install) > 0 {
			add("a superseded entry is detection-only and must not have an install block")
		}
	}
	return problems
}

func keyProblems(field string, keys []platform.Key) []string {
	var problems []string
	for _, k := range keys {
		if !k.Valid() {
			problems = append(problems, fmt.Sprintf("%s: unknown platform %q", field, k))
		}
	}
	return problems
}

// requiresCycle returns the first requires cycle found, or nil.
func (c Catalog) requiresCycle() []string {
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var cycle []string
	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		e, _ := c.Entry(id)
		for _, r := range e.Requires {
			if _, ok := c.Entry(r.Package); !ok {
				continue
			}
			switch color[r.Package] {
			case grey:
				start := slices.Index(stack, r.Package)
				cycle = append(append([]string{}, stack[start:]...), r.Package)
				return true
			case white:
				if visit(r.Package) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}
	for _, e := range c.Entries {
		if color[e.ID] == white && visit(e.ID) {
			return cycle
		}
	}
	return nil
}

func (c Catalog) profileProblems(ids map[string]bool) []string {
	var problems []string
	for _, name := range c.ProfileNames() {
		p := c.Profiles[name]
		for _, inc := range p.Include {
			if _, ok := c.Profiles[inc]; !ok {
				problems = append(problems, fmt.Sprintf("profiles/%s: unknown include %q", name, inc))
			}
		}
		for _, pkg := range p.Packages {
			if !ids[pkg.ID] {
				problems = append(problems, fmt.Sprintf("profiles/%s: unknown package %q", name, pkg.ID))
				continue
			}
			if e, _ := c.Entry(pkg.ID); e.SupersededBy != "" {
				problems = append(problems, fmt.Sprintf("profiles/%s: package %q is superseded by %q and cannot be wanted", name, pkg.ID, e.SupersededBy))
			}
		}
		if cycle := c.includeCycle(name, nil); cycle != nil {
			problems = append(problems, fmt.Sprintf("profiles/%s: include cycle: %s", name, strings.Join(cycle, " -> ")))
		}
	}
	return problems
}

// includeCycle returns a cycle reachable from name, or nil.
func (c Catalog) includeCycle(name string, path []string) []string {
	if i := slices.Index(path, name); i >= 0 {
		return append(append([]string{}, path[i:]...), name)
	}
	p, ok := c.Profiles[name]
	if !ok {
		return nil
	}
	path = append(path, name)
	for _, inc := range p.Include {
		if cycle := c.includeCycle(inc, path); cycle != nil {
			return cycle
		}
	}
	return nil
}

// ProfileNames returns the profile names sorted.
func (c Catalog) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
