package catalog

import (
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// TestValidateValues covers the FR-001a allow-lists: every value that can
// reach argv, a file path or the terminal is checked at load time.
func TestValidateValues(t *testing.T) {
	brewStep := func(c *Catalog, s InstallStep) {
		c.Entries[0].Install = map[platform.Key][]InstallStep{platform.Darwin: {s}}
	}
	tests := []struct {
		name   string
		mutate func(c *Catalog)
		want   string
	}{
		{"package id option", func(c *Catalog) { c.Entries = append(c.Entries, Entry{ID: "-f", Kind: KindCLI}) }, `packages.-f: id "-f" is not allowed`},
		{"package id uppercase", func(c *Catalog) { c.Entries = append(c.Entries, Entry{ID: "Git", Kind: KindCLI}) }, `id "Git" is not allowed`},
		{"package id traversal", func(c *Catalog) { c.Entries = append(c.Entries, Entry{ID: "../../x", Kind: KindCLI}) }, `id "../../x" is not allowed`},
		{"package id slash", func(c *Catalog) { c.Entries = append(c.Entries, Entry{ID: "a/b", Kind: KindCLI}) }, `id "a/b" is not allowed`},
		{"package id control", func(c *Catalog) { c.Entries = append(c.Entries, Entry{ID: "x\x1b[2K", Kind: KindCLI}) }, `is not allowed`},
		{"brew id short option", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "-f"}) }, `install.darwin[0]: brew id "-f" is not allowed`},
		{"brew id long option", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "--HEAD"}) }, `brew id "--HEAD" is not allowed`},
		{"brew id force", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "--force"}) }, `brew id "--force" is not allowed`},
		{"brew id traversal", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "../../x"}) }, `brew id "../../x" is not allowed`},
		{"brew id too deep", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "a/b/c/d"}) }, `brew id "a/b/c/d" is not allowed`},
		{"brew id control", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "x\x1b[2K"}) }, `brew id "x\x1b[2K" is not allowed`},
		{"brew id space", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "git jq"}) }, `brew id "git jq" is not allowed`},
		{"brew id empty segment", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "a//b"}) }, `brew id "a//b" is not allowed`},
		{"tap option", func(c *Catalog) {
			brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "a/b/c", Tap: "--force-auto-update", Trust: TrustItem})
		}, `install.darwin[0]: tap "--force-auto-update" is not allowed`},
		{"tap short option", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "a/b/c", Tap: "-f"}) }, `tap "-f" is not allowed`},
		{"tap one part", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "a/b/c", Tap: "stablyai"}) }, `tap "stablyai" is not allowed`},
		{"tap three parts", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "a/b/c", Tap: "a/b/c"}) }, `tap "a/b/c" is not allowed`},
		{"tap traversal", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "a/b/c", Tap: "../x"}) }, `tap "../x" is not allowed`},
		{"tap control", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "a/b/c", Tap: "a/b\x1b"}) }, `is not allowed`},
		{"tap item outside tap", func(c *Catalog) {
			brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "other/tap/orca", Tap: "stablyai/orca", Trust: TrustItem})
		}, `install.darwin[0]: brew id "other/tap/orca" is not an item of tap "stablyai/orca"`},
		{"trusted item not fully qualified", func(c *Catalog) {
			brewStep(c, InstallStep{Backend: BackendBrewCask, ID: "orca", Tap: "stablyai/orca", Trust: TrustItem})
		}, `brew id "orca" is not an item of tap "stablyai/orca"`},
		{"npm id option", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendNPM, ID: "--global"}) }, `npm id "--global" is not allowed`},
		{"report id control", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendReport, ID: "x\x07"}) }, `install.darwin[0]: id has a control character`},
		{"hint control", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendReport, Hint: "ok\x1b[2Khidden"}) }, `install.darwin[0]: hint has a control character`},
		{"version pin control", func(c *Catalog) { brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "git", Version: "1\x00"}) }, `install.darwin[0]: version has a control character`},
		{"post install control", func(c *Catalog) {
			brewStep(c, InstallStep{Backend: BackendBrewFormula, ID: "git", PostInstall: []string{"a\nb"}})
		}, `install.darwin[0]: post_install has a control character`},
		{"detect command empty", func(c *Catalog) { c.Entries[0].Detect.Commands = []string{""} }, `packages.git: detect command "" is not allowed`},
		{"detect command slash", func(c *Catalog) { c.Entries[0].Detect.Commands = []string{"../bin/git"} }, `detect command "../bin/git" is not allowed`},
		{"detect command dotdot", func(c *Catalog) { c.Entries[0].Detect.Commands = []string{".."} }, `detect command ".." is not allowed`},
		{"detect command inner dotdot", func(c *Catalog) { c.Entries[0].Detect.Commands = []string{"a..b"} }, `detect command "a..b" is not allowed`},
		{"detect command control", func(c *Catalog) { c.Entries[0].Detect.Commands = []string{"x\x1b[2K"} }, `detect command "x\x1b[2K" is not allowed`},
		{"detect command option", func(c *Catalog) { c.Entries[0].Detect.Commands = []string{"-f"} }, `detect command "-f" is not allowed`},
		{"version probe other program", func(c *Catalog) {
			c.Entries[0].Detect.Commands = []string{"git"}
			c.Entries[0].Detect.Version = &VersionProbe{Cmd: []string{"rm", "-rf", "/"}, Regex: `(\S+)`}
		}, `packages.git: detect.version.cmd must start with a detect command, got "rm"`},
		{"version probe without commands", func(c *Catalog) {
			c.Entries[0].Detect.Version = &VersionProbe{Cmd: []string{"git", "--version"}, Regex: `(\S+)`}
		}, `detect.version.cmd must start with a detect command, got "git"`},
		{"version probe arg control", func(c *Catalog) {
			c.Entries[0].Detect.Commands = []string{"git"}
			c.Entries[0].Detect.Version = &VersionProbe{Cmd: []string{"git", "\x1b"}, Regex: `(\S+)`}
		}, `detect.version.cmd has a control character`},
		{"display control", func(c *Catalog) { c.Entries[0].Display = "Git\x1b[2K" }, `packages.git: display has a control character`},
		{"category control", func(c *Catalog) { c.Entries[0].Category = "a\tb" }, `packages.git: category has a control character`},
		{"bundle id control", func(c *Catalog) { c.Entries[0].Detect.BundleIDs = []string{"com.x\x1b"} }, `packages.git: detect.bundle_ids has a control character`},
		{"winget id control", func(c *Catalog) { c.Entries[0].Detect.WingetID = "x\x1b" }, `packages.git: detect.winget has a control character`},
		{"npm package control", func(c *Catalog) { c.Entries[0].Detect.NPMPackage = "x\x1b" }, `packages.git: detect.npm_package has a control character`},
		{"provenance control", func(c *Catalog) {
			c.Entries[0].Detect.Provenance = map[Provenance]string{ProvBrew: "{brew_prefix}/bin/git\x1b"}
		}, `packages.git: detect.provenance has a control character`},
		{"profile name", func(c *Catalog) {
			c.Profiles["-x"] = Profile{Name: "-x"}
		}, `profiles/-x: name "-x" is not allowed`},
		{"profile description control", func(c *Catalog) {
			p := c.Profiles["base-dev"]
			p.Description = "x\x1b[2K"
			c.Profiles["base-dev"] = p
		}, `profiles/base-dev: description has a control character`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validCatalog()
			tt.mutate(&c)
			assertValidation(t, c.Validate(), tt.want)
		})
	}
}

func TestValidateValuesAccepted(t *testing.T) {
	tests := []struct {
		name string
		step InstallStep
		cmds []string
	}{
		{"versioned formula", InstallStep{Backend: BackendBrewFormula, ID: "node@24"}, []string{"node"}},
		{"plus in name", InstallStep{Backend: BackendBrewFormula, ID: "gtk+3"}, []string{"gtk+3"}},
		{"tap item", InstallStep{Backend: BackendBrewCask, ID: "stablyai/orca/orca", Tap: "stablyai/orca", Trust: TrustItem}, []string{"orca"}},
		{"untrusted tap item", InstallStep{Backend: BackendBrewFormula, ID: "hashicorp/tap/terraform", Tap: "hashicorp/tap"}, []string{"terraform"}},
		{"core qualified", InstallStep{Backend: BackendBrewFormula, ID: "homebrew/core/git"}, []string{"git"}},
		{"scoped npm", InstallStep{Backend: BackendNPM, ID: "@anthropic-ai/claude-code"}, []string{"claude"}},
		{"report with hint", InstallStep{Backend: BackendReport, Hint: "Install from Microsoft's apt/rpm repository"}, []string{"docker-compose"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validCatalog()
			c.Entries[0].Install = map[platform.Key][]InstallStep{platform.Darwin: {tt.step}}
			c.Entries[0].Detect.Commands = tt.cmds
			c.Entries[0].Detect.Version = &VersionProbe{Cmd: []string{tt.cmds[0], "--version"}, Regex: `(\S+)`}
			if err := c.Validate(); err != nil {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}

func TestHasControl(t *testing.T) {
	for _, s := range []string{"\x00", "\x1b", "\n", "\t", "\u0085", "\u009b"} {
		if !hasControl("a" + s) {
			t.Errorf("hasControl(%q) = false", s)
		}
	}
	if hasControl("plain text, with punctuation: ok") || hasControl(strings.Repeat("é", 3)) {
		t.Error("hasControl reports printable text")
	}
}
