package brew

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/adapter/execrunner"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/testkit"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

func formulaEntry(id, formula string, commands ...string) catalog.Entry {
	return catalog.Entry{ID: id, Kind: catalog.KindCLI, Detect: catalog.DetectSpec{Commands: commands},
		Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewFormula, ID: formula}}, platform.Linux: {{Backend: catalog.BackendBrewFormula, ID: formula}}}}
}

func caskEntry(id, cask string, commands ...string) catalog.Entry {
	return catalog.Entry{ID: id, Kind: catalog.KindCLI, Detect: catalog.DetectSpec{Commands: commands},
		Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: cask}}}}
}

var orcaStep = catalog.InstallStep{Backend: catalog.BackendBrewCask, ID: "stablyai/orca/orca", Tap: "stablyai/orca", Trust: catalog.TrustItem}

func orcaEntry() catalog.Entry {
	return catalog.Entry{ID: "orca", Kind: catalog.KindGUI, Detect: catalog.DetectSpec{Commands: []string{"orca"}},
		Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {orcaStep}}}
}

func testWorld(prefix string) fakeworld.World {
	return fakeworld.World{Brew: &fakeworld.Brew{
		Prefix: prefix,
		Formulae: map[string]fakeworld.Formula{
			"jq":      {Version: "1.7.1"},
			"sqlite":  {Version: "3.47.0", KegOnly: true},
			"node@24": {Version: "24.11.1", KegOnly: true},
			"gh":      {Version: "2.62.0"},
		},
		Casks: map[string]fakeworld.Cask{
			// Installed casks report each link's absolute path in a "target"
			// key next to the stanza (Homebrew 7.0.11, captured on CI).
			"claude-code": {Version: "2.1.4", Artifacts: []map[string]any{{"binary": []any{"claude"}, "target": prefix + "/bin/claude"}}},
			"firefox": {Version: "159.0", Artifacts: []map[string]any{
				{"uninstall": []any{map[string]any{"quit": "org.mozilla.firefox"}}},
				{"app": []any{"Firefox.app"}, "target": "/Applications/Firefox.app"},
				{"command_wrapper": []any{"firefox", map[string]any{"executable": "/Applications/Firefox.app/Contents/MacOS/firefox"}}, "target": prefix + "/bin/firefox"},
			}},
			// Not installed: no target keys, so link paths are derived.
			"wrapped":                {Version: "1.0", Artifacts: []map[string]any{{"command_wrapper": []any{"wrapped", map[string]any{"executable": "/Apps/W.app/w"}}}}},
			"iterm2":                 {Version: "3.5", Artifacts: []map[string]any{{"app": []any{"iTerm.app", map[string]any{"target": "iTerm2.app"}}}}},
			"session-manager-plugin": {Version: "1.2", Artifacts: []map[string]any{{"pkg": []any{"SessionManagerPlugin.pkg"}}}},
			"stablyai/orca/orca":     {Version: "1.4.0", Artifacts: []map[string]any{{"app": []any{"Orca.app"}}}},
		},
		Installed: fakeworld.Installed{
			Formulae: map[string]string{"jq": "1.7.1", "node@24": "24.11.1"},
			Casks:    map[string]string{"claude-code": "2.1.4", "firefox": "159.0", "stablyai/orca/orca": "1.4.0"},
		},
	}}
}

func newFake(t *testing.T, w fakeworld.World) (*testkit.Fake, *execrunner.Runner) {
	t.Helper()
	f := testkit.NewFake(t, w, "brew")
	return f, execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + f.Dir}})
}

func TestDetector(t *testing.T) {
	f, r := newFake(t, testWorld("/opt/homebrew"))
	entries := []catalog.Entry{
		formulaEntry("jq", "jq", "jq"),
		formulaEntry("node", "node@24", "node"),
		formulaEntry("gh", "gh", "gh"),
		caskEntry("claude-code", "claude-code", "claude"),
		{ID: "firefox", Kind: catalog.KindGUI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: "firefox"}}}},
		orcaEntry(),
		{ID: "paperclip", Kind: catalog.KindCLI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendNPM, ID: "paperclipai"}}}},
		{ID: "bare-orca", Kind: catalog.KindGUI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: "orca"}}}},
	}
	d := NewDetector(r)
	if d.ID() != ownership.DetectorBrew {
		t.Error("ID")
	}
	got, err := d.Detect(context.Background(), entries, scan.Env{BrewPath: f.Path("brew"), Vars: map[string]string{"HOMEBREW_PREFIX": "/opt/homebrew"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]ownership.Detection{
		"jq":   {Found: true, Provenance: catalog.ProvBrew, Version: "1.7.1", BackendRef: "brew-formula:jq"},
		"node": {Found: true, Provenance: catalog.ProvBrew, Version: "24.11.1", BackendRef: "brew-formula:node@24"},
		"gh":   {},
		// The path Homebrew reports for the link is the detection's path.
		"claude-code": {Found: true, Provenance: catalog.ProvBrewCask, Version: "2.1.4", BackendRef: "brew-cask:claude-code", Path: "/opt/homebrew/bin/claude"},
		"firefox":     {Found: true, Provenance: catalog.ProvBrewCask, Version: "159.0", BackendRef: "brew-cask:firefox", Path: "/opt/homebrew/bin/firefox"},
		"orca":        {Found: true, Provenance: catalog.ProvBrewCask, Version: "1.4.0", BackendRef: "brew-cask:stablyai/orca/orca"},
		"bare-orca":   {},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d detections: %+v", len(got), got)
	}
	for _, g := range got {
		w, ok := want[g.PackageID]
		if !ok {
			t.Errorf("unexpected %s", g.PackageID)
			continue
		}
		w.PackageID, w.Detector, w.Status = g.PackageID, ownership.DetectorBrew, ownership.DetectionOK
		if g != w {
			t.Errorf("%s = %+v, want %+v", g.PackageID, g, w)
		}
	}
	log := f.Log(t)
	if len(log) != 1 || !slices.Equal(log[0].Argv[1:], []string{"info", "--json=v2", "--installed"}) {
		t.Fatalf("want exactly one brew info call, got %+v", log)
	}
	if !slices.Contains(log[0].EnvNames, "HOMEBREW_NO_AUTO_UPDATE") || !slices.Contains(log[0].EnvNames, "HOMEBREW_NO_ENV_HINTS") || !slices.Contains(log[0].EnvNames, "HOMEBREW_PREFIX") {
		t.Errorf("env names = %v", log[0].EnvNames)
	}
	testkit.AssertSafeInvocations(t, log)
}

func TestDetectorFailures(t *testing.T) {
	entries := []catalog.Entry{formulaEntry("jq", "jq", "jq")}
	tests := []struct {
		name  string
		world fakeworld.World
		want  string
	}{
		{"invalid JSON", fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "brew", Stdout: "Warning: {broken"}}}, "invalid JSON"},
		{"non-zero exit", fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "brew", Exit: 1, Stderr: "Error: Homebrew is broken"}}}, "exited 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, r := newFake(t, tt.world)
			_, err := NewDetector(r).Detect(context.Background(), entries, scan.Env{BrewPath: f.Path("brew")})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
	got, err := NewDetector(nil).Detect(context.Background(), entries, scan.Env{})
	if err != nil || got != nil {
		t.Errorf("brew missing: %v, %v", got, err)
	}
	if _, err := NewDetector(execrunner.New(execrunner.Options{})).Detect(context.Background(), entries, scan.Env{BrewPath: "/nonexistent/brew"}); err == nil {
		t.Error("missing brew binary must be an error")
	}
}

// slowRunner checks the detector passes the 10 s timeout.
type timeoutRunner struct{ got time.Duration }

func (r *timeoutRunner) Run(_ context.Context, c process.Cmd) (process.Result, error) {
	r.got = c.Timeout
	return process.Result{}, process.ErrTimeout
}

func TestDetectorTimeout(t *testing.T) {
	r := &timeoutRunner{}
	_, err := NewDetector(r).Detect(context.Background(), []catalog.Entry{formulaEntry("jq", "jq")}, scan.Env{BrewPath: "/b/brew"})
	if err == nil || r.got != scan.DetectorTimeout {
		t.Errorf("err = %v, timeout = %s", err, r.got)
	}
}

func backends(t *testing.T, w fakeworld.World, prefix string) (*testkit.Fake, *Backend, *Backend) {
	t.Helper()
	f, r := newFake(t, w)
	target := func() Target {
		return Target{Path: f.Path("brew"), Prefix: prefix, Env: map[string]string{"HOMEBREW_PREFIX": prefix}}
	}
	return f, NewFormula(r, BackendOptions{Target: target}), NewCask(r, BackendOptions{Target: target, AppDir: "/Apps"})
}

func argvs(actions []plan.Action) [][]string {
	var out [][]string
	for _, a := range actions {
		for _, c := range a.Commands {
			out = append(out, c.Argv)
		}
	}
	return out
}

func TestInstallActions(t *testing.T) {
	w := testWorld("/opt/homebrew")
	ctx := context.Background()
	tests := []struct {
		name      string
		taps      []string
		trusted   []string
		entry     catalog.Entry
		cask      bool
		want      [][]string
		wantKinds []plan.ActionKind
		wantSudo  bool
	}{
		{name: "formula", entry: formulaEntry("jq", "jq"), want: [][]string{{"brew", "install", "jq"}}, wantKinds: []plan.ActionKind{plan.ActionInstall}},
		{name: "cask", cask: true, entry: caskEntry("claude-code", "claude-code"), want: [][]string{{"brew", "install", "--cask", "claude-code"}}, wantKinds: []plan.ActionKind{plan.ActionInstall}},
		{name: "cask with pkg needs sudo", cask: true, entry: caskEntry("session-manager-plugin", "session-manager-plugin"),
			want: [][]string{{"brew", "install", "--cask", "session-manager-plugin"}}, wantKinds: []plan.ActionKind{plan.ActionInstall}, wantSudo: true},
		{name: "unknown cask info needs sudo", cask: true, entry: caskEntry("mystery", "mystery"),
			want: [][]string{{"brew", "install", "--cask", "mystery"}}, wantKinds: []plan.ActionKind{plan.ActionInstall}, wantSudo: true},
		{name: "orca untapped", cask: true, entry: orcaEntry(),
			want:      [][]string{{"brew", "tap", "stablyai/orca"}, {"brew", "trust", "--cask", "stablyai/orca/orca"}, {"brew", "install", "--cask", "stablyai/orca/orca"}},
			wantKinds: []plan.ActionKind{plan.ActionTap, plan.ActionTrust, plan.ActionInstall}, wantSudo: true},
		{name: "orca tapped not trusted", cask: true, taps: []string{"stablyai/orca"}, entry: orcaEntry(),
			want:      [][]string{{"brew", "trust", "--cask", "stablyai/orca/orca"}, {"brew", "install", "--cask", "stablyai/orca/orca"}},
			wantKinds: []plan.ActionKind{plan.ActionTrust, plan.ActionInstall}},
		{name: "orca whole tap trusted is not item trust", cask: true, taps: []string{"stablyai/orca"}, trusted: []string{"stablyai/orca"}, entry: orcaEntry(),
			want:      [][]string{{"brew", "trust", "--cask", "stablyai/orca/orca"}, {"brew", "install", "--cask", "stablyai/orca/orca"}},
			wantKinds: []plan.ActionKind{plan.ActionTrust, plan.ActionInstall}},
		{name: "orca tapped and trusted", cask: true, taps: []string{"stablyai/orca"}, trusted: []string{"stablyai/orca/orca"}, entry: orcaEntry(),
			want: [][]string{{"brew", "install", "--cask", "stablyai/orca/orca"}}, wantKinds: []plan.ActionKind{plan.ActionInstall}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w.Brew.Taps, w.Brew.Trusted = tt.taps, tt.trusted
			f, formula, cask := backends(t, w, "/opt/homebrew")
			b := formula
			if tt.cask {
				b = cask
			}
			step := tt.entry.Install[platform.Darwin][0]
			actions, err := b.InstallActions(ctx, tt.entry, step)
			if err != nil {
				t.Fatal(err)
			}
			if got := argvs(actions); !slices.EqualFunc(got, tt.want, slices.Equal[[]string]) {
				t.Errorf("argv = %v, want %v", got, tt.want)
			}
			for i, a := range actions {
				if a.Kind != tt.wantKinds[i] || a.PackageID != tt.entry.ID || a.ID != 0 || a.DependsOn != nil {
					t.Errorf("action %d = %+v", i, a)
				}
				if (a.Kind == plan.ActionTap || a.Kind == plan.ActionTrust) &&
					(a.TapTrust == nil || *a.TapTrust != plan.TapTrustStep{Tap: "stablyai/orca", ItemKind: plan.ItemCask, Item: "stablyai/orca/orca", PackageID: "orca"}) {
					t.Errorf("tap trust = %+v", a.TapTrust)
				}
				for _, c := range a.Commands {
					if !c.Mutating {
						t.Error("plan commands are mutating")
					}
					if a.Kind == plan.ActionInstall && (c.NeedsSudo != tt.wantSudo || c.Interactive != tt.wantSudo) {
						t.Errorf("sudo = %v interactive = %v, want %v", c.NeedsSudo, c.Interactive, tt.wantSudo)
					}
				}
			}
			p := plan.Plan{Actions: actions}
			for i := range p.Actions {
				p.Actions[i].ID = i + 1
			}
			if err := p.Validate(func(string) bool { return true }); err != nil {
				t.Errorf("plan invalid: %v", err)
			}
			for _, inv := range f.Log(t) {
				if inv.Argv[1] == "install" || (inv.Argv[1] == "tap" || inv.Argv[1] == "trust") && len(inv.Argv) > 2 {
					t.Errorf("planning must not mutate: %v", inv.Argv)
				}
			}
			testkit.AssertSafeInvocations(t, f.Log(t))
		})
	}
	_, formula, _ := backends(t, w, "/opt/homebrew")
	if _, err := formula.InstallActions(ctx, caskEntry("x", "x"), catalog.InstallStep{Backend: catalog.BackendBrewCask, ID: "x"}); err == nil {
		t.Error("formula backend must refuse a cask step")
	}
}

func TestInstallActionsBrewMissing(t *testing.T) {
	cask := NewCask(nil, BackendOptions{Target: func() Target { return Target{} }})
	actions, err := cask.InstallActions(context.Background(), orcaEntry(), orcaStep)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 3 || !actions[2].Commands[0].NeedsSudo {
		t.Errorf("actions = %+v", actions)
	}
	if cask.Available(context.Background()) {
		t.Error("not available without brew")
	}
	if p, err := cask.LinkPaths(context.Background(), orcaEntry(), orcaStep); p != nil || err != nil {
		t.Errorf("link paths without brew = %v, %v", p, err)
	}
}

func TestLinkPaths(t *testing.T) {
	_, formula, cask := backends(t, testWorld("/opt/homebrew"), "/opt/homebrew")
	ctx := context.Background()
	tests := []struct {
		name  string
		b     *Backend
		entry catalog.Entry
		want  []string
	}{
		{"formula commands", formula, formulaEntry("gh", "gh", "gh"), []string{"/opt/homebrew/bin/gh"}},
		{"keg-only formula links nothing", formula, formulaEntry("sqlite", "sqlite", "sqlite3"), nil},
		{"cask binary", cask, caskEntry("claude-code", "claude-code", "claude"), []string{"/opt/homebrew/bin/claude"}},
		{"cask app with target", cask, catalog.Entry{ID: "iterm2", Kind: catalog.KindGUI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: "iterm2"}}}}, []string{"/Apps/iTerm2.app"}},
		{"tap item before tap uses detect.commands (D-9)", cask, orcaEntry(), []string{"/opt/homebrew/bin/orca"}},
		// Homebrew's own target is authoritative (an installed cask, doctor's whole-catalog check).
		{"reported targets", cask, catalog.Entry{ID: "firefox", Kind: catalog.KindGUI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: "firefox"}}}},
			[]string{"/Applications/Firefox.app", "/opt/homebrew/bin/firefox"}},
		{"command wrapper without target", cask, caskEntry("wrapped", "wrapped"), []string{"/opt/homebrew/bin/wrapped"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.b.LinkPaths(ctx, tt.entry, tt.entry.Install[platform.Darwin][0])
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
	if _, err := cask.LinkPaths(ctx, caskEntry("nope", "nope"), catalog.InstallStep{Backend: catalog.BackendBrewCask, ID: "nope"}); err == nil {
		t.Error("unknown cask must be an error")
	}
	w := testWorld("/opt/homebrew")
	w.Brew.Taps = []string{"stablyai/orca"}
	_, _, tapped := backends(t, w, "/opt/homebrew")
	if got, _ := tapped.LinkPaths(ctx, orcaEntry(), orcaStep); !slices.Equal(got, []string{"/Apps/Orca.app"}) {
		t.Errorf("tapped orca = %v", got)
	}
	odd := fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "brew", Stdout: `{"formulae":[],"casks":[]}`}}}
	_, _, oddCask := backends(t, odd, "/p")
	if got, _ := oddCask.LinkPaths(ctx, caskEntry("x", "x", "x"), catalog.InstallStep{Backend: catalog.BackendBrewCask, ID: "x"}); !slices.Equal(got, []string{"/p/bin/x"}) {
		t.Errorf("cask missing from info = %v", got)
	}
}

func TestTapStateAndCaps(t *testing.T) {
	ctx := context.Background()
	w := testWorld("/opt/homebrew")
	w.Brew.Taps, w.Brew.Trusted = []string{"stablyai/orca"}, []string{"stablyai/orca/orca"}
	_, formula, cask := backends(t, w, "/opt/homebrew")
	if tapped, trusted, err := cask.TapState(ctx, orcaStep); !tapped || !trusted || err != nil {
		t.Errorf("tap state = %v %v %v", tapped, trusted, err)
	}
	if tapped, trusted, err := cask.TapState(ctx, catalog.InstallStep{ID: "jq"}); !tapped || !trusted || err != nil {
		t.Error("an official item needs no tap")
	}
	broken := fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "brew", Exit: 1, Stderr: "boom"}}}
	_, _, bc := backends(t, broken, "/p")
	if _, _, err := bc.TapState(ctx, orcaStep); err == nil {
		t.Error("failing brew tap must be an error")
	}
	trustFails := fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "brew", Args: []string{"tap"}, Stdout: "stablyai/orca\n"}, {Cmd: "brew", Exit: 1}}}
	_, _, tc := backends(t, trustFails, "/p")
	if tapped, trusted, err := tc.TapState(ctx, orcaStep); !tapped || trusted || err == nil {
		t.Errorf("trust listing fails: %v %v %v", tapped, trusted, err)
	}
	acts, err := bc.InstallActions(ctx, orcaEntry(), orcaStep)
	if err != nil || len(acts) != 3 {
		t.Errorf("tap state unknown must plan tap and trust: %v %v", acts, err)
	}
	if formula.ID() != catalog.BackendBrewFormula || cask.ID() != catalog.BackendBrewCask {
		t.Error("IDs")
	}
	if formula.Capabilities().NeedsSudo || !cask.Capabilities().NeedsSudo {
		t.Error("caps")
	}
	if !formula.Available(ctx) {
		t.Error("available")
	}
}
