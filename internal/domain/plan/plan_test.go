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
