package catalogyaml

import (
	"slices"
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

const smallCatalog = `apiVersion: facman/v1
packages:
  zeta:
    display: Zeta
    kind: cli
    detect:
      commands: [zeta]
      version: { cmd: "zeta --version", regex: 'zeta (\S+)' }
      provenance:
        brew: "{brew_prefix}/Cellar/zeta/**"
    install:
      darwin:
        - backend: brew-formula
          id: zeta
          version: "1.2"
    requires:
      - alpha
      - { package: alpha, min_version: 2 }
    satisfies_if: { min_version: "24.11" }
    reject_if: { version_lt: "2.0" }
  alpha:
    kind: gui
    platforms: [darwin]
    sign_in_required: true
    detect:
      bundle_ids: [com.example.alpha]
      winget_id: Example.Alpha
      npm_package: "@x/alpha"
      version: { cmd: [alpha, -v], regex: '(.*)' }
    install:
      darwin:
        - backend: brew-cask
          id: x/y/alpha
          tap: x/y
          trust: item
          hint: h
          post_install: ["alpha init"]
    version_policy: delegate
    service:
      manager: auto
      anything: { nested: [1, 2] }
  old:
    kind: cli
    superseded_by: zeta
    detect: { commands: [old] }
  group:
    choice:
      - { id: alpha, platforms: [darwin], host_on: [wsl] }
      - id: zeta
    satisfied_by_any_present: true
`

func TestDecodeCatalog(t *testing.T) {
	api, entries, err := decodeCatalog("catalog.yaml", []byte(smallCatalog))
	if err != nil {
		t.Fatal(err)
	}
	if api != "facman/v1" {
		t.Errorf("api = %q", api)
	}
	ids := []string{}
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(ids, []string{"zeta", "alpha", "old", "group"}) {
		t.Errorf("order not preserved: %v", ids)
	}
	z, a, g := entries[0], entries[1], entries[3]
	if entries[2].SupersededBy != "zeta" {
		t.Errorf("superseded_by = %q", entries[2].SupersededBy)
	}
	if z.Kind != catalog.KindCLI || z.Display != "Zeta" || z.VersionPolicy != catalog.PolicyTrack {
		t.Errorf("zeta = %+v", z)
	}
	if !slices.Equal(z.Detect.Version.Cmd, []string{"zeta", "--version"}) || z.Detect.Version.Regex != `zeta (\S+)` {
		t.Errorf("version probe = %+v", z.Detect.Version)
	}
	if z.Detect.Provenance[catalog.ProvBrew] != "{brew_prefix}/Cellar/zeta/**" {
		t.Errorf("provenance = %v", z.Detect.Provenance)
	}
	if got := z.Install[platform.Darwin]; len(got) != 1 || got[0].Version != "1.2" || got[0].Backend != catalog.BackendBrewFormula {
		t.Errorf("install = %+v", got)
	}
	if !slices.Equal(z.Requires, []catalog.Requirement{{Package: "alpha"}, {Package: "alpha", MinVersion: "2"}}) {
		t.Errorf("requires = %+v", z.Requires)
	}
	if z.SatisfiesIf.MinVersion != "24.11" || z.RejectIf.VersionLT != "2.0" {
		t.Errorf("satisfies/reject = %+v %+v", z.SatisfiesIf, z.RejectIf)
	}
	if !a.SignInRequired || a.VersionPolicy != catalog.PolicyDelegate || !slices.Equal(a.Platforms, []platform.Key{platform.Darwin}) {
		t.Errorf("alpha = %+v", a)
	}
	step := a.Install[platform.Darwin][0]
	if step.Tap != "x/y" || step.Trust != "item" || step.Hint != "h" || !slices.Equal(step.PostInstall, []string{"alpha init"}) {
		t.Errorf("step = %+v", step)
	}
	if a.Detect.WingetID != "Example.Alpha" || a.Detect.NPMPackage != "@x/alpha" || a.Detect.BundleIDs[0] != "com.example.alpha" {
		t.Errorf("detect = %+v", a.Detect)
	}
	if a.Service == nil || a.Service.Raw["manager"] != "auto" {
		t.Errorf("service = %+v", a.Service)
	}
	if !g.IsChoice() || !g.SatisfiedByAnyPresent || !slices.Equal(g.Choice[0].HostOn, []platform.Key{platform.WSL}) {
		t.Errorf("group = %+v", g)
	}
}

func TestDecodeCatalogErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{"unknown key", "apiVersion: facman/v1\npackages:\n  jq:\n    kind: cli\n    detect:\n      detekt: [jq]\n", []string{"catalog.yaml:6:7 packages.jq.detect.detekt: unknown field"}},
		{"unknown entry key", "apiVersion: facman/v1\npackages:\n  jq:\n    knd: cli\n", []string{"catalog.yaml:4:5 packages.jq.knd: unknown field"}},
		{"unknown top key", "apiVersion: facman/v1\npackages: {}\nextra: 1\n", []string{"catalog.yaml:3:1 extra: unknown field"}},
		{"wrong type list", "apiVersion: facman/v1\npackages:\n  jq:\n    detect:\n      commands: jq\n", []string{`catalog.yaml:5:17 packages.jq.detect.commands: expected a list, got "jq"`}},
		{"wrong type string", "apiVersion: facman/v1\npackages:\n  jq:\n    kind: [cli]\n", []string{"catalog.yaml:4:11 packages.jq.kind: expected a string, got a list"}},
		{"wrong type bool", "apiVersion: facman/v1\npackages:\n  jq:\n    sign_in_required: yes please\n", []string{"packages.jq.sign_in_required: expected true or false"}},
		{"null string", "apiVersion: facman/v1\npackages:\n  jq:\n    kind:\n", []string{"packages.jq.kind: expected a string, got null"}},
		{"packages not a mapping", "apiVersion: facman/v1\npackages: [jq]\n", []string{"packages: expected a mapping, got a list"}},
		{"install not a list", "apiVersion: facman/v1\npackages:\n  jq:\n    install:\n      darwin: brew\n", []string{"packages.jq.install.darwin: expected a list of install steps"}},
		{"link force", "apiVersion: facman/v1\npackages:\n  node:\n    install:\n      darwin:\n        - { backend: brew-formula, id: node@24, link: force }\n", []string{"install.darwin[0].link: not supported"}},
		{"unknown step key", "apiVersion: facman/v1\npackages:\n  jq:\n    install:\n      darwin:\n        - { backend: brew-formula, idd: jq }\n", []string{"packages.jq.install.darwin[0].idd: unknown field"}},
		{"bad satisfies", "apiVersion: facman/v1\npackages:\n  jq:\n    satisfies_if: { min: 1 }\n    reject_if: { lt: 1 }\n", []string{"satisfies_if.min: unknown field", "reject_if.lt: unknown field"}},
		{"bad version probe", "apiVersion: facman/v1\npackages:\n  jq:\n    detect:\n      version: { run: x }\n", []string{"detect.version.run: unknown field"}},
		{"bad requires", "apiVersion: facman/v1\npackages:\n  jq:\n    requires: { a: b }\n", []string{"packages.jq.requires: expected a list"}},
		{"bad requires item", "apiVersion: facman/v1\npackages:\n  jq:\n    requires: [{ pkg: a }]\n", []string{"requires[0].pkg: unknown field"}},
		{"bad choice", "apiVersion: facman/v1\npackages:\n  g:\n    choice: x\n", []string{"packages.g.choice: expected a list"}},
		{"bad choice item", "apiVersion: facman/v1\npackages:\n  g:\n    choice: [{ name: x }]\n", []string{"choice[0].name: unknown field"}},
		{"bad service", "apiVersion: facman/v1\npackages:\n  g:\n    service: [1]\n", []string{"packages.g.service: expected a mapping"}},
		{"duplicate key", "apiVersion: facman/v1\npackages:\n  jq:\n    kind: cli\n    kind: gui\n", []string{"packages.jq.kind: duplicate key"}},
		{"missing packages", "apiVersion: facman/v1\n", []string{"missing packages"}},
		{"not a mapping", "- a\n", []string{"catalog.yaml:1:1 expected a mapping at the top level"}},
		{"empty", "", []string{"catalog.yaml:1:1 empty file"}},
		{"syntax", "apiVersion: [\n", []string{"catalog.yaml: "}},
		{"several together", "apiVersion: facman/v1\npackages:\n  a:\n    x: 1\n  b:\n    y: 2\n", []string{"packages.a.x: unknown field", "packages.b.y: unknown field"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := decodeCatalog("catalog.yaml", []byte(tt.yaml))
			if err == nil {
				t.Fatal("want an error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestDecodeAliases(t *testing.T) {
	y := "apiVersion: facman/v1\npackages:\n  jq:\n    kind: cli\n    detect:\n      commands: &cmds [jq]\n  jq2:\n    kind: cli\n    detect:\n      commands: *cmds\n"
	_, entries, err := decodeCatalog("catalog.yaml", []byte(y))
	if err != nil || entries[1].Detect.Commands[0] != "jq" {
		t.Errorf("alias: %+v, %v", entries, err)
	}
}

func TestDecodeProfile(t *testing.T) {
	y := `apiVersion: facman/v1
profile: agentic-factory
description: Baseline
include: [base-dev]
packages:
  - jq
  - gcloud-cli: { optional: true }
  - node: { version: "24" }
policy:
  allow_adopt: true
`
	api, p, err := decodeProfile("profiles/agentic-factory.yaml", []byte(y))
	if err != nil {
		t.Fatal(err)
	}
	want := []catalog.ProfilePackage{{ID: "jq"}, {ID: "gcloud-cli", Optional: true}, {ID: "node", Version: "24"}}
	if api != "facman/v1" || p.Name != "agentic-factory" || p.Description != "Baseline" || !slices.Equal(p.Include, []string{"base-dev"}) || !slices.Equal(p.Packages, want) {
		t.Errorf("profile = %+v", p)
	}
	if p.Policy["allow_adopt"] != true {
		t.Errorf("policy = %v", p.Policy)
	}
}

func TestDecodeProfileErrors(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"unknown key", "apiVersion: facman/v1\nprofile: a\npkgs: []\n", "profiles/a.yaml:3:1 pkgs: unknown field"},
		{"missing name", "apiVersion: facman/v1\n", "missing profile name"},
		{"packages not list", "profile: a\npackages: {jq: {}}\n", "packages: expected a list"},
		{"two ids", "profile: a\npackages:\n  - { jq: {}, gh: {} }\n", "packages[0]: expected one package id"},
		{"bad option", "profile: a\npackages:\n  - jq: { opt: true }\n", "packages[0].jq.opt: unknown field"},
		{"bad optional type", "profile: a\npackages:\n  - jq: { optional: maybe }\n", "packages[0].jq.optional: expected true or false"},
		{"empty", "", "empty file"},
		{"item not a mapping", "profile: a\npackages:\n  - [jq]\n", "packages[0]: expected a mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := decodeProfile("profiles/a.yaml", []byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
