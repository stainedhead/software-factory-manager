package catalog

import (
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

func brewFormula(id string) map[platform.Key][]InstallStep {
	s := []InstallStep{{Backend: BackendBrewFormula, ID: id}}
	return map[platform.Key][]InstallStep{platform.Darwin: s, platform.Linux: s}
}

func validCatalog() Catalog {
	return Catalog{
		APIVersion: APIVersion,
		Revision:   "test",
		Entries: []Entry{
			{ID: "git", Kind: KindCLI, Install: brewFormula("git")},
			{ID: "node", Kind: KindCLI, Install: brewFormula("node@24")},
			{ID: "awscli", Kind: KindCLI, Install: brewFormula("awscli")},
			{ID: "session-manager-plugin", Kind: KindCLI, Requires: []Requirement{{Package: "awscli"}},
				Install: map[platform.Key][]InstallStep{platform.Darwin: {{Backend: BackendBrewCask, ID: "session-manager-plugin"}}}},
			{ID: "jq", Kind: KindCLI, Install: brewFormula("jq")},
			{ID: "gcloud-cli", Kind: KindCLI, Install: brewFormula("gcloud-cli")},
			{ID: "orca", Kind: KindGUI, Install: map[platform.Key][]InstallStep{
				platform.Darwin: {{Backend: BackendBrewCask, ID: "stablyai/orca/orca", Tap: "stablyai/orca", Trust: TrustItem}},
			}},
			{ID: "docker", Kind: KindCLI, Install: brewFormula("docker")},
			{ID: "colima", Kind: KindCLI, Requires: []Requirement{{Package: "docker"}}, Install: brewFormula("colima")},
			{ID: "orbstack", Kind: KindGUI, Platforms: []platform.Key{platform.Darwin}},
			{ID: "container-runtime", Choice: []ChoiceMember{
				{ID: "orbstack", Platforms: []platform.Key{platform.Darwin}},
				{ID: "colima", Platforms: []platform.Key{platform.Darwin, platform.Linux}},
			}, SatisfiedByAnyPresent: true},
		},
		Profiles: map[string]Profile{
			"base-dev": {Name: "base-dev", Packages: []ProfilePackage{{ID: "git"}, {ID: "node"}, {ID: "awscli"}}},
			"agentic-factory": {Name: "agentic-factory", Include: []string{"base-dev"}, Packages: []ProfilePackage{
				{ID: "jq"},
				{ID: "orca"},
				{ID: "gcloud-cli", Optional: true},
				{ID: "session-manager-plugin", Optional: true},
				{ID: "container-runtime", Optional: true},
			}},
		},
	}
}
