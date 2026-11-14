package catalog

import (
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

var increment1 = BackendSet{BackendBrewFormula: true, BackendBrewCask: true}

func TestSelectInstall(t *testing.T) {
	darwin := platform.Platform{OS: "darwin", Display: true}
	linux := platform.Platform{OS: "linux", Display: true}
	headless := platform.Platform{OS: "linux"}
	wsl := platform.Platform{OS: "linux", WSL: true}

	cask := func(id string) InstallStep { return InstallStep{Backend: BackendBrewCask, ID: id} }
	claudeCode := Entry{ID: "claude-code", Kind: KindCLI, Install: map[platform.Key][]InstallStep{
		platform.Darwin: {cask("claude-code"), {Backend: BackendNPM, ID: "@anthropic-ai/claude-code"}},
		platform.Linux:  {cask("claude-code")},
	}}
	vscode := Entry{ID: "visual-studio-code", Kind: KindGUI,
		Detect: DetectSpec{WingetID: "Microsoft.VisualStudioCode"},
		Install: map[platform.Key][]InstallStep{
			platform.Darwin: {cask("visual-studio-code")},
			platform.Linux:  {{Backend: BackendReport, Hint: "Install from Microsoft's apt/rpm repository"}},
		}}
	copilotApp := Entry{ID: "github-copilot-app", Kind: KindGUI, Detect: DetectSpec{WingetID: "GitHub.CopilotApp"},
		Install: map[platform.Key][]InstallStep{platform.Darwin: {cask("github-copilot-app")}, platform.Linux: {cask("github-copilot-app")}}}
	hostWithHint := Entry{ID: "claude-desktop", Kind: KindGUI, Install: map[platform.Key][]InstallStep{
		platform.WSL: {{Backend: BackendHostReport, Hint: "winget install Anthropic.Claude"}},
	}}
	hostNoHint := Entry{ID: "t3-code", Kind: KindGUI, Detect: DetectSpec{WingetID: "T3.Code"}, Install: map[platform.Key][]InstallStep{
		platform.WSL: {{Backend: BackendHostReport}},
	}}
	hostBare := Entry{ID: "orca", Kind: KindGUI, Install: map[platform.Key][]InstallStep{platform.Darwin: {cask("x")}}}
	iterm := Entry{ID: "iterm2", Kind: KindGUI, Platforms: []platform.Key{platform.Darwin},
		Install: map[platform.Key][]InstallStep{platform.Darwin: {cask("iterm2")}}}
	paperclip := Entry{ID: "paperclip", Kind: KindCLI, Install: map[platform.Key][]InstallStep{
		platform.Darwin: {{Backend: BackendNPM, ID: "paperclipai"}},
		platform.Linux:  {{Backend: BackendNPM, ID: "paperclipai"}},
	}}
	futureBackend := Entry{ID: "x", Kind: KindCLI, Install: map[platform.Key][]InstallStep{
		platform.Linux: {{Backend: "native-pkg", ID: "x"}},
	}}
	noBlocks := Entry{ID: "nothing", Kind: KindCLI}
	group := Entry{ID: "container-runtime", Choice: []ChoiceMember{{ID: "colima"}}}

	tests := []struct {
		name          string
		entry         Entry
		p             platform.Platform
		wantOffered   bool
		wantAvailable bool
		wantStepID    string
		wantReason    string
		wantHint      string
	}{
		{"darwin cask", claudeCode, darwin, true, true, "claude-code", "", ""},
		{"linux cask", claudeCode, linux, true, true, "claude-code", "", ""},
		{"wsl cli inherits linux", claudeCode, wsl, true, true, "claude-code", "", ""},
		{"wsl gui is a host item with winget hint", copilotApp, wsl, true, false, "", "Windows host", "winget install GitHub.CopilotApp"},
		{"wsl gui without winget id", hostBare, wsl, true, false, "", "Windows host", "Install it on the Windows host"},
		{"wsl host-report hint", hostWithHint, wsl, true, false, "", "Windows host", "winget install Anthropic.Claude"},
		{"wsl host-report falls back to winget id", hostNoHint, wsl, true, false, "", "Windows host", "winget install T3.Code"},
		{"linux gui without display", copilotApp, headless, true, false, "", "no display", ""},
		{"linux gui with display", copilotApp, linux, true, true, "github-copilot-app", "", ""},
		{"platforms darwin on linux not offered", iterm, linux, false, false, "", "", ""},
		{"platforms darwin on wsl not offered", iterm, wsl, false, false, "", "", ""},
		{"npm only is unavailable", paperclip, darwin, true, false, "", "needs the npm backend (Increment 2)", ""},
		{"npm only on wsl inherits linux", paperclip, wsl, true, false, "", "needs the npm backend (Increment 2)", ""},
		{"report backend", vscode, linux, true, false, "", "not installable through Homebrew", "apt/rpm"},
		{"report backend wins before display check", vscode, headless, true, false, "", "", ""},
		{"unknown future backend", futureBackend, linux, true, false, "", "native-pkg backend, which this version does not provide", ""},
		{"no install block not offered", noBlocks, darwin, false, false, "", "", ""},
		{"choice group offered", group, linux, true, true, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectInstall(tt.entry, tt.p, increment1)
			if got.Offered != tt.wantOffered || got.Available != tt.wantAvailable {
				t.Fatalf("got %+v, want offered=%v available=%v", got, tt.wantOffered, tt.wantAvailable)
			}
			if got.Unavailable() != (tt.wantOffered && !tt.wantAvailable) {
				t.Error("Unavailable() disagrees")
			}
			if tt.wantStepID != "" && (got.Step == nil || got.Step.ID != tt.wantStepID) {
				t.Errorf("step = %+v, want id %s", got.Step, tt.wantStepID)
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if !strings.Contains(got.Hint, tt.wantHint) {
				t.Errorf("hint = %q, want %q", got.Hint, tt.wantHint)
			}
		})
	}
}

func TestSelectInstallFallsBackToLaterBackend(t *testing.T) {
	e := Entry{ID: "codex", Kind: KindCLI, Install: map[platform.Key][]InstallStep{
		platform.Darwin: {{Backend: BackendNPM, ID: "@openai/codex"}, {Backend: BackendBrewCask, ID: "codex"}},
	}}
	got := SelectInstall(e, platform.Platform{OS: "darwin"}, increment1)
	if !got.Available || got.Step.Backend != BackendBrewCask {
		t.Errorf("got %+v, want the brew-cask step", got)
	}
}
