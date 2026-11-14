package plan

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
)

func brew(args ...string) Command {
	return Command{Argv: append([]string{"brew"}, args...), Mutating: true}
}

func allAbsent(string) bool { return true }

func TestConsentString(t *testing.T) {
	at := time.Date(2026, 11, 14, 14, 0, 0, 0, time.FixedZone("EST", -5*3600))
	if got := NewConsent("j", at).String(); got != "j@2026-11-14T19:00:00Z" {
		t.Errorf("Consent.String() = %q", got)
	}
}

func TestCommandString(t *testing.T) {
	c := Command{Argv: []string{"/bin/bash", "/tmp/install file.sh", "", "a$b"}}
	if got := c.String(); got != `/bin/bash "/tmp/install file.sh" "" "a$b"` {
		t.Errorf("String() = %s", got)
	}
}

// Review FR-001c: every control character is escaped, metacharacters or not,
// so data can never hide or rewrite text on the consent screen.
func TestCommandStringEscapesControls(t *testing.T) {
	tests := []struct {
		arg, want string
	}{
		{"a\x1bb", `"a\x1bb"`},
		{"x\x1b[2K", `"x\x1b[2K"`},
		{"line\nbreak", `"line\nbreak"`},
		{"tab\tx", `"tab\tx"`},
		{"nul\x00", `"nul\x00"`},
		{"c1\u009b2K", `"c1\u009b2K"`},
		{"bidi\u202eevil", `"bidi\u202eevil"`},
		{"bad\xffutf8", `"bad\xffutf8"`},
		{"plain", "plain"},
		{"node@24", "node@24"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := Command{Argv: []string{"brew", "install", tt.arg}}.String()
			if got != "brew install "+tt.want {
				t.Errorf("String() = %s, want brew install %s", got, tt.want)
			}
			for _, r := range got {
				if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
					t.Errorf("String() has a raw control character %U", r)
				}
			}
		})
	}
}

func TestPlanHelpers(t *testing.T) {
	p := Plan{Actions: []Action{
		{ID: 1, PackageID: "jq", Commands: []Command{brew("install", "jq")}},
		{ID: 2, PackageID: "gh", Commands: []Command{brew("install", "gh")}},
	}}
	if p.Empty() || len(p.Commands()) != 2 {
		t.Error("helpers wrong")
	}
	if a, ok := p.Action(2); !ok || a.PackageID != "gh" {
		t.Error("Action(2)")
	}
	if _, ok := p.Action(9); ok {
		t.Error("Action(9) found")
	}
	if !(Plan{}).Empty() {
		t.Error("zero plan not empty")
	}
}

func TestValidate(t *testing.T) {
	orcaTap := Action{ID: 1, Kind: ActionTap, PackageID: "orca", Commands: []Command{brew("tap", "stablyai/orca")}}
	tests := []struct {
		name    string
		actions []Action
		absent  func(string) bool
		want    string
	}{
		{"valid", []Action{
			{ID: 1, Kind: ActionBootstrapHomebrew, PackageID: HomebrewPackageID, Commands: []Command{{Argv: []string{"/bin/bash", "/tmp/x"}, Mutating: true, NeedsSudo: true, Unverified: true}}},
			{ID: 2, Kind: ActionTap, PackageID: "orca", Commands: []Command{brew("tap", "stablyai/orca")}, DependsOn: []int{1}},
			{ID: 3, Kind: ActionTrust, PackageID: "orca", Commands: []Command{brew("trust", "--cask", "stablyai/orca/orca")}, DependsOn: []int{1, 2}},
			{ID: 4, Kind: ActionInstall, PackageID: "orca", Commands: []Command{brew("install", "--cask", "stablyai/orca/orca")}, DependsOn: []int{3}},
		}, allAbsent, ""},
		{"force", []Action{{ID: 1, PackageID: "jq", Commands: []Command{brew("install", "--force", "jq")}}}, allAbsent, "forbidden flag --force"},
		{"overwrite", []Action{{ID: 1, PackageID: "jq", Commands: []Command{brew("link", "--overwrite", "jq")}}}, allAbsent, "forbidden flag --overwrite"},
		{"force on any program", []Action{{ID: 1, PackageID: "x", Commands: []Command{{Argv: []string{"npm", "i", "--force"}, Mutating: true}}}}, allAbsent, "forbidden flag"},
		{"whole tap trust", []Action{{ID: 1, PackageID: "orca", Commands: []Command{brew("trust", "stablyai/orca")}}}, allAbsent, "not a full item path"},
		{"trust without item", []Action{{ID: 1, PackageID: "orca", Commands: []Command{brew("trust", "--cask")}}}, allAbsent, "without an item"},
		{"upgrade", []Action{{ID: 1, PackageID: "jq", Commands: []Command{brew("upgrade", "jq")}}}, allAbsent, "forbidden brew subcommand upgrade"},
		{"uninstall", []Action{{ID: 1, PackageID: "jq", Commands: []Command{{Argv: []string{"/opt/homebrew/bin/brew", "uninstall", "jq"}, Mutating: true}}}}, allAbsent, "forbidden brew subcommand uninstall"},
		{"untap", []Action{{ID: 1, PackageID: "orca", Commands: []Command{brew("untap", "stablyai/orca")}}}, allAbsent, "forbidden brew subcommand untap"},
		{"untrust", []Action{{ID: 1, PackageID: "orca", Commands: []Command{brew("untrust", "--cask", "stablyai/orca/orca")}}}, allAbsent, "forbidden brew subcommand untrust"},
		{"bare brew", []Action{{ID: 1, PackageID: "jq", Commands: []Command{brew()}}}, allAbsent, "brew without a subcommand"},
		{"not absent", []Action{{ID: 1, PackageID: "jq", Commands: []Command{brew("install", "jq")}}}, func(id string) bool { return id != "jq" }, "package jq is not absent"},
		{"tap trust env", []Action{{ID: 1, PackageID: "jq", Commands: []Command{{Argv: []string{"brew", "install", "jq"}, Mutating: true, EnvNames: []string{"HOMEBREW_NO_REQUIRE_TAP_TRUST"}}}}}, allAbsent, "forbidden environment variable"},
		{"not mutating", []Action{{ID: 1, PackageID: "jq", Commands: []Command{{Argv: []string{"brew", "install", "jq"}}}}}, allAbsent, "not marked mutating"},
		{"empty argv", []Action{{ID: 1, PackageID: "jq", Commands: []Command{{Mutating: true}}}}, allAbsent, "empty argv"},
		{"no commands", []Action{{ID: 1, PackageID: "jq"}}, allAbsent, "no commands"},
		{"duplicate id", []Action{orcaTap, orcaTap}, allAbsent, "duplicate id"},
		{"forward dependency", []Action{{ID: 1, PackageID: "jq", Commands: []Command{brew("install", "jq")}, DependsOn: []int{2}}}, allAbsent, "depends on 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Plan{Actions: tt.actions}.Validate(tt.absent)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !errors.Is(err, ErrInvariant) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate = %v, want %q", err, tt.want)
			}
		})
	}
}

// Review FR-001b: no option from data reaches brew argv. Only --cask and
// --formula on install and trust are allowed after the subcommand, and no
// program gets any short or long form of --force or --overwrite.
func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string // "" means valid
	}{
		{"install formula", []string{"brew", "install", "jq"}, ""},
		{"install cask", []string{"brew", "install", "--cask", "foo"}, ""},
		{"install formula flag", []string{"brew", "install", "--formula", "foo"}, ""},
		{"trust cask item", []string{"brew", "trust", "--cask", "a/b/c"}, ""},
		{"trust formula item", []string{"brew", "trust", "--formula", "a/b/c"}, ""},
		{"tap", []string{"brew", "tap", "a/b"}, ""},
		{"absolute brew", []string{"/opt/homebrew/bin/brew", "install", "--cask", "foo"}, ""},
		{"installer", []string{"/bin/bash", "/tmp/facman-install-123.sh"}, ""},
		{"short force", []string{"brew", "install", "-f", "jq"}, "forbidden option -f"},
		{"short force after item", []string{"brew", "install", "jq", "-f"}, "forbidden option -f"},
		{"long force", []string{"brew", "install", "--force", "jq"}, "forbidden"},
		{"force bottle", []string{"brew", "install", "--force-bottle", "jq"}, "forbidden"},
		{"abbreviated force", []string{"brew", "install", "--forc", "jq"}, "forbidden option --forc"},
		{"build from source", []string{"brew", "install", "-s", "jq"}, "forbidden option -s"},
		{"head", []string{"brew", "install", "--HEAD", "jq"}, "forbidden option --HEAD"},
		{"overwrite", []string{"brew", "install", "--overwrite", "jq"}, "forbidden"},
		{"ignore dependencies", []string{"brew", "install", "--ignore-dependencies", "jq"}, "forbidden option --ignore-dependencies"},
		{"cask with value", []string{"brew", "install", "--cask=x"}, "forbidden option --cask=x"},
		{"cask on tap", []string{"brew", "tap", "--cask", "a/b"}, "forbidden option --cask"},
		{"tap force auto update", []string{"brew", "tap", "--force-auto-update", "a/b"}, "forbidden"},
		{"trust with tap flag", []string{"brew", "trust", "--tap", "a/b"}, "forbidden option --tap"},
		{"option before subcommand", []string{"brew", "-f", "install", "jq"}, "forbidden"},
		{"end of options marker", []string{"brew", "install", "--", "jq"}, "forbidden option --"},
		{"bare dash", []string{"brew", "install", "-", "jq"}, "forbidden option -"},
		{"non-brew short force", []string{"/bin/bash", "-f", "/tmp/x"}, "forbidden flag -f"},
		{"non-brew short cluster", []string{"/usr/bin/curl", "-fsSL", "/tmp/x"}, "forbidden flag -fsSL"},
		{"non-brew abbreviated force", []string{"npm", "i", "--forc"}, "forbidden flag --forc"},
		{"non-brew abbreviated overwrite", []string{"npm", "i", "--overw"}, "forbidden flag --overw"},
		{"non-brew force uppercase", []string{"npm", "i", "--FORCE"}, "forbidden flag --FORCE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Plan{Actions: []Action{{ID: 1, PackageID: "x", Commands: []Command{{Argv: tt.argv, Mutating: true}}}}}
			err := p.Validate(allAbsent)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate(%q) = %v", tt.argv, err)
				}
				return
			}
			if err == nil || !errors.Is(err, ErrInvariant) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate(%q) = %v, want %q", tt.argv, err, tt.want)
			}
		})
	}
}

func orderCatalog() catalog.Catalog {
	return catalog.Catalog{Entries: []catalog.Entry{
		{ID: "git"},
		{ID: "session-manager-plugin", Requires: []catalog.Requirement{{Package: "awscli"}}},
		{ID: "awscli"},
		{ID: "colima", Requires: []catalog.Requirement{{Package: "docker-compose"}, {Package: "docker"}}},
		{ID: "docker"},
		{ID: "docker-compose"},
		{ID: "a", Requires: []catalog.Requirement{{Package: "b"}}},
		{ID: "b", Requires: []catalog.Requirement{{Package: "a"}}},
	}}
}

func TestOrderPackages(t *testing.T) {
	c := orderCatalog()
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"requires first", []string{"session-manager-plugin", "awscli", "git"}, []string{"git", "awscli", "session-manager-plugin"}},
		{"ties by catalog order", []string{"docker-compose", "docker", "colima"}, []string{"docker", "docker-compose", "colima"}},
		{"dependency not in set is ignored", []string{"session-manager-plugin"}, []string{"session-manager-plugin"}},
		{"unknown last", []string{"zzz", "git"}, []string{"git", "zzz"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 3 { // deterministic
				got, err := OrderPackages(c, tt.in)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(got, tt.want) {
					t.Fatalf("OrderPackages = %v, want %v", got, tt.want)
				}
			}
		})
	}
	if _, err := OrderPackages(c, []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cycle err = %v", err)
	}
}

func TestExcludeWithDependents(t *testing.T) {
	p := Plan{
		Excluded: []Exclusion{{PackageID: "paperclip", Reason: ExcludedUnavailableHere}},
		Actions: []Action{
			{ID: 1, PackageID: "awscli", Commands: []Command{brew("install", "awscli")}},
			{ID: 2, PackageID: "session-manager-plugin", Commands: []Command{brew("install", "--cask", "session-manager-plugin")}, DependsOn: []int{1}},
			{ID: 3, PackageID: "x", Commands: []Command{brew("install", "x")}, DependsOn: []int{2}},
			{ID: 4, PackageID: "jq", Commands: []Command{brew("install", "jq")}},
		},
	}
	got := ExcludeWithDependents(p, "awscli", ExcludedConflict, "/opt/homebrew/bin/aws exists")
	if len(got.Actions) != 1 || got.Actions[0].PackageID != "jq" {
		t.Fatalf("actions = %+v", got.Actions)
	}
	want := []Exclusion{
		{PackageID: "paperclip", Reason: ExcludedUnavailableHere},
		{PackageID: "awscli", Reason: ExcludedConflict, Detail: "/opt/homebrew/bin/aws exists"},
		{PackageID: "session-manager-plugin", Reason: ExcludedDependency, Detail: "depends on awscli (conflict)"},
		{PackageID: "x", Reason: ExcludedDependency, Detail: "depends on awscli (conflict)"},
	}
	if !slices.Equal(got.Excluded, want) {
		t.Errorf("excluded = %+v", got.Excluded)
	}
	if len(p.Actions) != 4 || len(p.Excluded) != 1 {
		t.Error("input plan mutated")
	}
	again := ExcludeWithDependents(got, "paperclip", ExcludedConflict, "")
	if len(again.Excluded) != 4 {
		t.Errorf("an already excluded package was recorded twice: %+v", again.Excluded)
	}
}
