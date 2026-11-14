package catalog

import (
	"slices"
	"strings"
	"testing"
)

func TestResolveIncludesBaseDev(t *testing.T) {
	d, err := Resolve(validCatalog(), []string{"agentic-factory"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(d.SelectedIDs(), ","); got != "git,node,awscli,jq,orca" {
		t.Errorf("selected = %s", got)
	}
	git, _ := d.Item("git")
	if !git.Required || !slices.Equal(git.FromProfiles, []string{"base-dev"}) {
		t.Errorf("git = %+v", git)
	}
	gc, ok := d.Item("gcloud-cli")
	if !ok || gc.Selected || gc.Required {
		t.Errorf("optional gcloud-cli = %+v, %v", gc, ok)
	}
	if _, ok := d.Item("nope"); ok {
		t.Error("Item(nope) found")
	}
	if !slices.Equal(d.Profiles, []string{"agentic-factory"}) {
		t.Errorf("profiles = %v", d.Profiles)
	}
}

func TestResolve(t *testing.T) {
	withProfiles := func(extra map[string]Profile) Catalog {
		c := validCatalog()
		for k, v := range extra {
			c.Profiles[k] = v
		}
		return c
	}
	tests := []struct {
		name     string
		catalog  Catalog
		profiles []string
		with     []string
		wantErr  []string
		check    func(t *testing.T, d DesiredSet)
	}{
		{
			name: "two profiles union and required beats optional",
			catalog: withProfiles(map[string]Profile{
				"cloud": {Name: "cloud", Packages: []ProfilePackage{{ID: "gcloud-cli"}, {ID: "jq", Optional: true}}},
			}),
			profiles: []string{"agentic-factory", "cloud", "cloud"},
			check: func(t *testing.T, d DesiredSet) {
				gc, _ := d.Item("gcloud-cli")
				if !gc.Required || !gc.Selected {
					t.Errorf("gcloud-cli = %+v", gc)
				}
				if !slices.Equal(gc.FromProfiles, []string{"agentic-factory", "cloud"}) {
					t.Errorf("FromProfiles = %v", gc.FromProfiles)
				}
				jq, _ := d.Item("jq")
				if !jq.Required {
					t.Errorf("jq = %+v", jq)
				}
				if !slices.Equal(d.Profiles, []string{"agentic-factory", "cloud"}) {
					t.Errorf("profiles not de-duplicated: %v", d.Profiles)
				}
			},
		},
		{
			name: "diamond include visits base-dev once",
			catalog: withProfiles(map[string]Profile{
				"cloud": {Name: "cloud", Include: []string{"base-dev"}, Packages: []ProfilePackage{{ID: "gcloud-cli"}}},
			}),
			profiles: []string{"agentic-factory", "cloud"},
			check: func(t *testing.T, d DesiredSet) {
				git, _ := d.Item("git")
				if !slices.Equal(git.FromProfiles, []string{"base-dev"}) {
					t.Errorf("git = %+v", git)
				}
			},
		},
		{
			name: "same pin twice is fine",
			catalog: withProfiles(map[string]Profile{
				"a": {Name: "a", Packages: []ProfilePackage{{ID: "jq", Version: "1.7"}}},
				"b": {Name: "b", Packages: []ProfilePackage{{ID: "jq", Version: "1.7"}}},
			}),
			profiles: []string{"a", "b"},
			check: func(t *testing.T, d DesiredSet) {
				jq, _ := d.Item("jq")
				if jq.Version != "1.7" {
					t.Errorf("jq = %+v", jq)
				}
			},
		},
		{
			name: "different pins name both profiles",
			catalog: withProfiles(map[string]Profile{
				"a": {Name: "a", Packages: []ProfilePackage{{ID: "jq", Version: "1.7"}}},
				"b": {Name: "b", Packages: []ProfilePackage{{ID: "jq", Version: "1.6"}}},
			}),
			profiles: []string{"a", "b"},
			wantErr:  []string{`jq: conflicting version pins "1.7" (profile a) and "1.6" (profile b)`},
		},
		{
			name: "include cycle",
			catalog: withProfiles(map[string]Profile{
				"x": {Name: "x", Include: []string{"y"}},
				"y": {Name: "y", Include: []string{"x"}},
			}),
			profiles: []string{"x"},
			wantErr:  []string{"profile include cycle: x -> y -> x"},
		},
		{
			name:     "unknown profile names known ones",
			catalog:  validCatalog(),
			profiles: []string{"nope"},
			wantErr:  []string{`unknown profile "nope" (known profiles: agentic-factory, base-dev)`},
		},
		{
			name:    "no profile",
			catalog: validCatalog(),
			wantErr: []string{"no profile selected"},
		},
		{
			name: "unknown package in profile",
			catalog: withProfiles(map[string]Profile{
				"bad": {Name: "bad", Packages: []ProfilePackage{{ID: "hermes"}}},
			}),
			profiles: []string{"bad"},
			wantErr:  []string{`profile bad: unknown package "hermes"`},
		},
		{
			name:     "--with optional selects",
			catalog:  validCatalog(),
			profiles: []string{"agentic-factory"},
			with:     []string{"gcloud-cli", "gcloud-cli"},
			check: func(t *testing.T, d DesiredSet) {
				gc, _ := d.Item("gcloud-cli")
				if !gc.Selected || gc.Required {
					t.Errorf("gcloud-cli = %+v", gc)
				}
			},
		},
		{
			name:     "--with of a required package has no effect",
			catalog:  validCatalog(),
			profiles: []string{"agentic-factory"},
			with:     []string{"jq"},
			check: func(t *testing.T, d DesiredSet) {
				jq, _ := d.Item("jq")
				if !jq.Required || !jq.Selected {
					t.Errorf("jq = %+v", jq)
				}
			},
		},
		{
			name:     "--with unknown",
			catalog:  validCatalog(),
			profiles: []string{"agentic-factory"},
			with:     []string{"nope"},
			wantErr:  []string{"--with nope: unknown package (optional items: session-manager-plugin, gcloud-cli, orbstack, colima)"},
		},
		{
			name:     "--with not in profile",
			catalog:  validCatalog(),
			profiles: []string{"base-dev"},
			with:     []string{"jq"},
			wantErr:  []string{"--with jq: not in the selected profiles"},
		},
		{
			name:     "--with choice group lists members",
			catalog:  validCatalog(),
			profiles: []string{"agentic-factory"},
			with:     []string{"container-runtime"},
			wantErr:  []string{"name one member: orbstack, colima"},
		},
		{
			name:     "--with member picks it and pulls its requires",
			catalog:  validCatalog(),
			profiles: []string{"agentic-factory"},
			with:     []string{"colima"},
			check: func(t *testing.T, d DesiredSet) {
				g, _ := d.Item("container-runtime")
				if g.ChoicePick != "colima" || !g.Selected {
					t.Errorf("group = %+v", g)
				}
				col, _ := d.Item("colima")
				if !col.Selected || !slices.Equal(col.RequiredBy, []string{"container-runtime"}) {
					t.Errorf("colima = %+v", col)
				}
				dock, _ := d.Item("docker")
				if !dock.Selected || !slices.Equal(dock.RequiredBy, []string{"colima"}) {
					t.Errorf("docker = %+v", dock)
				}
				ids := d.SelectedIDs()
				if slices.Index(ids, "docker") > slices.Index(ids, "colima") {
					t.Errorf("not in catalog order: %v", ids)
				}
			},
		},
		{
			name:     "two members of one group",
			catalog:  validCatalog(),
			profiles: []string{"agentic-factory"},
			with:     []string{"colima", "orbstack"},
			wantErr:  []string{"choice group container-runtime already has colima picked"},
		},
		{
			name:     "requires of an optional selected item",
			catalog:  validCatalog(),
			profiles: []string{"agentic-factory"},
			with:     []string{"session-manager-plugin"},
			check: func(t *testing.T, d DesiredSet) {
				aws, _ := d.Item("awscli")
				if !slices.Equal(aws.RequiredBy, []string{"session-manager-plugin"}) || !aws.Required {
					t.Errorf("awscli = %+v", aws)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Resolve(tt.catalog, tt.profiles, tt.with)
			if len(tt.wantErr) > 0 {
				assertValidation(t, err, tt.wantErr...)
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			tt.check(t, d)
		})
	}
}

func TestResolveSkipsUnknownRequires(t *testing.T) {
	c := validCatalog()
	c.Entries[0].Requires = []Requirement{{Package: "ghost"}}
	d, err := Resolve(c, []string{"base-dev"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Item("ghost"); ok {
		t.Error("an unknown requires must not be added")
	}
}

func TestResolveSelectionWithout(t *testing.T) {
	tests := []struct {
		name    string
		sel     Selection
		wantErr string
		check   func(t *testing.T, d DesiredSet)
	}{
		{
			name: "unticking a required item deselects it but keeps it in the set",
			sel:  Selection{Profiles: []string{"agentic-factory"}, Without: []string{"jq", "jq"}},
			check: func(t *testing.T, d DesiredSet) {
				jq, ok := d.Item("jq")
				if !ok || !jq.Required || jq.Selected {
					t.Errorf("jq = %+v, %v", jq, ok)
				}
				if got := strings.Join(d.SelectedIDs(), ","); got != "git,node,awscli,orca" {
					t.Errorf("selected = %s", got)
				}
			},
		},
		{
			name: "a requires edge re-selects an unticked item",
			sel:  Selection{Profiles: []string{"agentic-factory"}, With: []string{"session-manager-plugin"}, Without: []string{"awscli"}},
			check: func(t *testing.T, d DesiredSet) {
				aws, _ := d.Item("awscli")
				if !aws.Selected || !slices.Equal(aws.RequiredBy, []string{"session-manager-plugin"}) {
					t.Errorf("awscli = %+v", aws)
				}
			},
		},
		{
			name: "without a picked member clears the pick",
			sel:  Selection{Profiles: []string{"agentic-factory"}, With: []string{"colima"}, Without: []string{"colima"}},
			check: func(t *testing.T, d DesiredSet) {
				g, _ := d.Item("container-runtime")
				if g.ChoicePick != "" {
					t.Errorf("group = %+v", g)
				}
				if _, ok := d.Item("docker"); ok {
					t.Error("docker still pulled in")
				}
			},
		},
		{
			name:    "without an item outside the profiles",
			sel:     Selection{Profiles: []string{"base-dev"}, Without: []string{"jq"}},
			wantErr: "--without jq: not in the selected profiles",
		},
		{
			name:    "without a choice group",
			sel:     Selection{Profiles: []string{"agentic-factory"}, Without: []string{"container-runtime"}},
			wantErr: "--without container-runtime: a choice group",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := ResolveSelection(validCatalog(), tc.sel)
			if tc.wantErr != "" {
				assertValidation(t, err, tc.wantErr)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, d)
		})
	}
}
