package summary

import (
	"slices"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

func cmd(argv ...string) []plan.Command { return []plan.Command{{Argv: argv}} }

var testCatalog = catalog.Catalog{Entries: []catalog.Entry{
	{ID: "claude-code", Display: "Claude Code", SignInRequired: true, Detect: catalog.DetectSpec{Commands: []string{"claude"}}},
	{ID: "codex", SignInRequired: true},
	{ID: "node", Display: "Node.js"},
	{ID: "gh", Display: "GitHub CLI"},
	{ID: "orca", Display: "Orca"},
	{ID: "jq"},
	{ID: "awscli"},
	{ID: "paperclip"},
	{ID: "docker"},
}}

func fullRun() Input {
	return Input{
		Catalog: testCatalog,
		Plan: plan.Plan{
			Actions: []plan.Action{
				{ID: 1, Kind: plan.ActionBootstrapHomebrew, PackageID: "homebrew", Commands: cmd("/bin/bash", "/tmp/i.sh")},
				{ID: 2, Kind: plan.ActionInstall, PackageID: "node", Commands: cmd("brew", "install", "node@24")},
				{ID: 3, Kind: plan.ActionInstall, PackageID: "claude-code", Commands: cmd("brew", "install", "--cask", "claude-code")},
				{ID: 4, Kind: plan.ActionInstall, PackageID: "codex", Commands: cmd("brew", "install", "--cask", "codex")},
				{ID: 5, Kind: plan.ActionTap, PackageID: "orca", Commands: cmd("brew", "tap", "stablyai/orca")},
				{ID: 6, Kind: plan.ActionInstall, PackageID: "orca", Commands: cmd("brew", "install", "--cask", "stablyai/orca/orca")},
				{ID: 7, Kind: plan.ActionInstall, PackageID: "jq", Commands: cmd("brew", "install", "jq")},
				{ID: 8, Kind: plan.ActionInstall, PackageID: "gh", Commands: cmd("brew", "install", "gh")},
				{ID: 9, Kind: plan.ActionInstall, PackageID: "awscli", Commands: cmd("brew", "install", "awscli")},
			},
			Excluded: []plan.Exclusion{
				{PackageID: "gh", Reason: plan.ExcludedConflict, Detail: "remove /opt/homebrew/bin/gh"},
				{PackageID: "paperclip", Reason: plan.ExcludedDependency, Detail: "depends on gh (conflict)"},
				{PackageID: "docker", Reason: plan.ExcludedUnavailableHere, Detail: "needs the npm backend"},
				{PackageID: "x", Reason: plan.ExcludedDetectionIncomplete, Detail: "timeout"},
				{PackageID: "y", Reason: plan.ExcludedNotSelected},
				{PackageID: "z", Reason: "odd"},
			},
		},
		Results: []Result{
			{ActionID: 1, PackageID: "homebrew", Status: StatusSucceeded, LogPath: "/l/1.log"},
			{ActionID: 2, PackageID: "node", Status: StatusSucceeded},
			{ActionID: 3, PackageID: "claude-code", Status: StatusSucceeded},
			{ActionID: 4, PackageID: "codex", Status: StatusAlreadyPresent, Detail: "found present"},
			{ActionID: 5, PackageID: "orca", Status: StatusSucceeded},
			{ActionID: 6, PackageID: "orca", Status: StatusFailed, Detail: "brew install exited 1", LogPath: "/l/6.log"},
			{ActionID: 7, PackageID: "jq", Status: StatusSkipped, Detail: "depends on action 6, which failed"},
			{ActionID: 8, PackageID: "gh", Status: StatusSkipped, Detail: "excluded after Homebrew was installed"},
			{ActionID: 9, PackageID: "awscli", Status: StatusNotStarted, Detail: "the run was cancelled"},
		},
		BrewPrefix:    "/opt/homebrew",
		BrewInstalled: true,
	}
}

func TestBuild(t *testing.T) {
	s := Build(fullRun())
	var got []string
	for _, l := range s.Lines {
		got = append(got, l.String())
	}
	want := []string{
		"Homebrew: installed",
		"Node.js: installed",
		"Claude Code: installed, sign-in required",
		"codex: already present",
		"Orca: not installed: failed (brew install exited 1)",
		"jq: not installed: dependency failed (depends on action 6, which failed)",
		"GitHub CLI: not installed: conflict (remove /opt/homebrew/bin/gh)",
		"awscli: not installed: cancelled",
		"paperclip: not installed: dependency failed (depends on gh (conflict))",
		"docker: not installed: unavailable here (needs the npm backend)",
		"x: not installed: detection incomplete (timeout)",
		"y: not installed: not selected",
		"z: not installed: odd",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("lines:\n%q\nwant\n%q", got, want)
	}
	if s.Lines[4].LogPath != "/l/6.log" || s.Lines[0].LogPath != "/l/1.log" || !s.Lines[0].Installed || s.Lines[4].Installed {
		t.Errorf("log paths / installed = %+v %+v", s.Lines[0], s.Lines[4])
	}
	if !slices.Equal(s.NextSteps, []string{"Sign in to Claude Code: run claude and follow its prompts"}) {
		t.Errorf("next steps = %q", s.NextSteps)
	}
	if !slices.Equal(s.ProfileLines, []string{`eval "$(/opt/homebrew/bin/brew shellenv)"`, `export PATH="/opt/homebrew/opt/node@24/bin:$PATH"`}) {
		t.Errorf("profile = %q", s.ProfileLines)
	}
}

func TestProfileLines(t *testing.T) {
	linuxbrew := platform.HomebrewInfo{Found: true, Prefix: "/home/linuxbrew/.linuxbrew", OnPATH: true}
	nodeRun := Input{
		Plan:    plan.Plan{Actions: []plan.Action{{ID: 1, Kind: plan.ActionInstall, PackageID: "node", Commands: cmd("brew", "install", "node@24")}}},
		Results: []Result{{ActionID: 1, PackageID: "node", Status: StatusSucceeded}},
	}
	tests := []struct {
		name string
		in   Input
		want []string
	}{
		{"brew on PATH, nothing new", Input{Homebrew: linuxbrew}, nil},
		{"brew not on PATH", Input{Homebrew: platform.HomebrewInfo{Found: true, Prefix: "/usr/local"}}, []string{`eval "$(/usr/local/bin/brew shellenv)"`}},
		{"node installed", func() Input { in := nodeRun; in.Homebrew = linuxbrew; return in }(), []string{`export PATH="/home/linuxbrew/.linuxbrew/opt/node@24/bin:$PATH"`}},
		{"no prefix known", Input{BrewInstalled: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Build(tt.in).ProfileLines; !slices.Equal(got, tt.want) {
				t.Errorf("profile = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSignInWithoutCommand(t *testing.T) {
	in := Input{
		Catalog: catalog.Catalog{Entries: []catalog.Entry{{ID: "codex", SignInRequired: true}}},
		Plan:    plan.Plan{Actions: []plan.Action{{ID: 1, Kind: plan.ActionInstall, PackageID: "codex", Commands: cmd("brew", "install", "--cask", "codex")}}},
		Results: []Result{{ActionID: 1, PackageID: "codex", Status: StatusSucceeded}, {ActionID: 2, PackageID: "odd", Status: "weird"}},
	}
	s := Build(in)
	if !slices.Equal(s.NextSteps, []string{"Sign in to codex: open it and follow its prompts"}) || s.Lines[1].Text != "not installed: weird" {
		t.Errorf("summary = %+v", s)
	}
}
