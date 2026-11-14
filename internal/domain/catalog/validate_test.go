package catalog

import (
	"errors"
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

func TestValidateValid(t *testing.T) {
	if err := validCatalog().Validate(); err != nil {
		t.Fatalf("valid catalog: %v", err)
	}
}

func TestValidateProblems(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Catalog)
		want   []string
	}{
		{"api version", func(c *Catalog) { c.APIVersion = "v0" }, []string{`apiVersion is "v0"`}},
		{"duplicate id", func(c *Catalog) { c.Entries = append(c.Entries, Entry{ID: "jq", Kind: KindCLI}) }, []string{"packages.jq: duplicate id"}},
		{"empty id", func(c *Catalog) { c.Entries = append(c.Entries, Entry{Kind: KindCLI}) }, []string{"an entry has no id"}},
		{"unknown requires", func(c *Catalog) { c.Entries[0].Requires = []Requirement{{Package: "nope"}} }, []string{`packages.git: unknown requires "nope"`}},
		{"unknown choice member", func(c *Catalog) {
			c.Entries[10].Choice = append(c.Entries[10].Choice, ChoiceMember{ID: "podman"})
		}, []string{`unknown choice member "podman"`}},
		{"nested choice", func(c *Catalog) {
			c.Entries = append(c.Entries, Entry{ID: "meta", Choice: []ChoiceMember{{ID: "container-runtime"}}})
		}, []string{`choice member "container-runtime" is itself a choice group`}},
		{"missing kind", func(c *Catalog) { c.Entries[0].Kind = "" }, []string{"packages.git: missing kind"}},
		{"unknown kind", func(c *Catalog) { c.Entries[0].Kind = "tui" }, []string{`packages.git: unknown kind "tui"`}},
		{"choice unknown kind", func(c *Catalog) { c.Entries[10].Kind = "tui" }, []string{`packages.container-runtime: unknown kind "tui"`}},
		{"unknown backend", func(c *Catalog) {
			c.Entries[0].Install[platform.Darwin] = []InstallStep{{Backend: "apt", ID: "git"}}
		}, []string{`install.darwin[0]: unknown backend "apt"`}},
		{"missing id", func(c *Catalog) {
			c.Entries[0].Install[platform.Darwin] = []InstallStep{{Backend: BackendBrewFormula}}
		}, []string{`install.darwin[0]: missing id`}},
		{"trust not item", func(c *Catalog) { c.Entries[6].Install[platform.Darwin][0].Trust = "tap" }, []string{`trust must be "item"`}},
		{"trust without tap", func(c *Catalog) {
			c.Entries[6].Install[platform.Darwin] = []InstallStep{{Backend: BackendBrewCask, ID: "x", Trust: TrustItem}}
		}, []string{"trust without tap"}},
		{"unknown install platform", func(c *Catalog) {
			c.Entries[0].Install = map[platform.Key][]InstallStep{"windows": {{Backend: BackendReport}}}
		}, []string{`unknown install platform "windows"`}},
		{"unknown platforms key", func(c *Catalog) { c.Entries[0].Platforms = []platform.Key{"bsd"} }, []string{`packages.git.platforms: unknown platform "bsd"`}},
		{"unknown member platform", func(c *Catalog) { c.Entries[10].Choice[0].HostOn = []platform.Key{"win"} }, []string{`host_on: unknown platform "win"`}},
		{"version policy", func(c *Catalog) { c.Entries[0].VersionPolicy = "latest" }, []string{`unknown version_policy "latest"`}},
		{"requires cycle", func(c *Catalog) {
			c.Entries = append(c.Entries, Entry{ID: "a", Kind: KindCLI, Requires: []Requirement{{Package: "b"}}},
				Entry{ID: "b", Kind: KindCLI, Requires: []Requirement{{Package: "a"}}})
		}, []string{"requires cycle: a -> b -> a"}},
		{"self cycle", func(c *Catalog) { c.Entries[0].Requires = []Requirement{{Package: "git"}} }, []string{"requires cycle: git -> git"}},
		{"unknown profile package", func(c *Catalog) {
			p := c.Profiles["base-dev"]
			p.Packages = append(p.Packages, ProfilePackage{ID: "hermes"})
			c.Profiles["base-dev"] = p
		}, []string{`profiles/base-dev: unknown package "hermes"`}},
		{"unknown include", func(c *Catalog) {
			p := c.Profiles["base-dev"]
			p.Include = []string{"missing"}
			c.Profiles["base-dev"] = p
		}, []string{`profiles/base-dev: unknown include "missing"`}},
		{"superseded by unknown", func(c *Catalog) {
			c.Entries = append(c.Entries, Entry{ID: "gemini-cli", Kind: KindCLI, SupersededBy: "nope"})
		}, []string{`packages.gemini-cli: unknown superseded_by "nope"`}},
		{"superseded installable", func(c *Catalog) {
			c.Entries = append(c.Entries, Entry{ID: "gemini-cli", Kind: KindCLI, SupersededBy: "git",
				Install: map[platform.Key][]InstallStep{platform.Darwin: {{Backend: BackendBrewFormula, ID: "gemini-cli"}}}})
		}, []string{"packages.gemini-cli: a superseded entry is detection-only and must not have an install block"}},
		{"superseded in profile", func(c *Catalog) {
			c.Entries = append(c.Entries, Entry{ID: "gemini-cli", Kind: KindCLI, SupersededBy: "git"})
			p := c.Profiles["base-dev"]
			p.Packages = append(p.Packages, ProfilePackage{ID: "gemini-cli"})
			c.Profiles["base-dev"] = p
		}, []string{`profiles/base-dev: package "gemini-cli" is superseded by "git" and cannot be wanted`}},
		{"include cycle", func(c *Catalog) {
			p := c.Profiles["base-dev"]
			p.Include = []string{"agentic-factory"}
			c.Profiles["base-dev"] = p
		}, []string{"include cycle: agentic-factory -> base-dev -> agentic-factory"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validCatalog()
			tt.mutate(&c)
			err := c.Validate()
			assertValidation(t, err, tt.want...)
		})
	}
}

func TestValidateReportsAllTogether(t *testing.T) {
	c := validCatalog()
	c.Entries = append(c.Entries,
		Entry{ID: "jq", Kind: KindCLI},
		Entry{ID: "x", Requires: []Requirement{{Package: "nope"}}},
	)
	c.Entries[6].Install[platform.Darwin][0].Trust = "all"
	err := c.Validate()
	assertValidation(t, err, "duplicate id", "packages.x: missing kind", `unknown requires "nope"`, `trust must be "item"`)
	var fe *failure.Error
	if errors.As(err, &fe) && len(fe.Details) != 4 {
		t.Errorf("want 4 details, got %d: %v", len(fe.Details), fe.Details)
	}
}

func assertValidation(t *testing.T, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("Validate() = nil, want a validation error")
	}
	if failure.ExitCode(err) != 3 {
		t.Errorf("exit code = %d, want 3", failure.ExitCode(err))
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not mention %q", err.Error(), w)
		}
	}
}

func TestRevisionLabel(t *testing.T) {
	c := validCatalog()
	c.Revision = ""
	c.APIVersion = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "(unnamed)") {
		t.Errorf("err = %v", err)
	}
}

func TestEntryHelpers(t *testing.T) {
	c := validCatalog()
	group, ok := c.Entry("container-runtime")
	if !ok || !group.IsChoice() {
		t.Fatal("container-runtime should be a choice group")
	}
	if got := strings.Join(group.MemberIDs(), ","); got != "orbstack,colima" {
		t.Errorf("MemberIDs = %s", got)
	}
	if _, ok := group.Member("colima"); !ok {
		t.Error("Member(colima) not found")
	}
	if _, ok := group.Member("podman"); ok {
		t.Error("Member(podman) found")
	}
	if _, ok := c.Entry("nope"); ok {
		t.Error("Entry(nope) found")
	}
	orb, _ := c.Entry("orbstack")
	if orb.OfferedOn(platform.Linux) || !orb.OfferedOn(platform.Darwin) {
		t.Error("OfferedOn wrong for orbstack")
	}
	if !BackendNPM.Known() || BackendID("x").Known() || !BackendHostReport.ReportOnly() || !BackendBrewCask.IsBrew() || BackendNPM.IsBrew() {
		t.Error("backend helpers wrong")
	}
	if got := strings.Join(c.ProfileNames(), ","); got != "agentic-factory,base-dev" {
		t.Errorf("ProfileNames = %s", got)
	}
}
