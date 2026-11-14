package detect

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/adapter/execrunner"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/testkit"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testkit.RemoveBuild()
	os.Exit(code)
}

// recorder wraps a runner and records every argv.
type recorder struct {
	mu    sync.Mutex
	inner Runner
	calls [][]string
}

func (r *recorder) Run(ctx context.Context, c process.Cmd) (process.Result, error) {
	r.mu.Lock()
	r.calls = append(r.calls, c.Argv)
	r.mu.Unlock()
	return r.inner.Run(ctx, c)
}

func newRecorder(path ...string) *recorder {
	return &recorder{inner: execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + strings.Join(path, ":")}})}
}

// script writes an executable shell script printing out.
func script(t *testing.T, path, out string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho '"+out+"'\n"), 0o755); err != nil { //nolint:gosec // test executable
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func cli(id string, cmds []string, version *catalog.VersionProbe, prov map[catalog.Provenance]string) catalog.Entry {
	return catalog.Entry{ID: id, Kind: catalog.KindCLI, Detect: catalog.DetectSpec{Commands: cmds, Version: version, Provenance: prov}}
}

func TestPathDetector(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	prefix := filepath.Join(root, "brew")
	npmPrefix := filepath.Join(root, "npm")
	osBin := filepath.Join(root, "usr", "bin")
	brewBin := filepath.Join(prefix, "bin")
	localBin := filepath.Join(home, ".local", "bin")
	npmBin := filepath.Join(npmPrefix, "bin")
	nvmBin := filepath.Join(home, ".nvm", "versions", "node", "v22.1.0", "bin")

	// claude: native first on PATH, then the brew cask.
	script(t, filepath.Join(home, ".local", "share", "claude", "versions", "2.1.4"), "2.1.4 (Claude Code)")
	symlink(t, filepath.Join(home, ".local", "share", "claude", "versions", "2.1.4"), filepath.Join(localBin, "claude"))
	script(t, filepath.Join(prefix, "Caskroom", "claude-code", "2.1.3", "claude"), "2.1.3 (Claude Code)")
	symlink(t, filepath.Join(prefix, "Caskroom", "claude-code", "2.1.3", "claude"), filepath.Join(brewBin, "claude"))
	// codex through npm.
	script(t, filepath.Join(npmPrefix, "lib", "node_modules", "@openai", "codex", "bin", "codex.js"), "codex-cli 0.58.0")
	symlink(t, "../lib/node_modules/@openai/codex/bin/codex.js", filepath.Join(npmBin, "codex"))
	// node through nvm, jq from the OS, aws printing garbage, tmux hanging.
	script(t, filepath.Join(nvmBin, "node"), "v22.1.0")
	script(t, filepath.Join(osBin, "jq"), "jq-1.7.1")
	script(t, filepath.Join(localBin, "aws"), "something went wrong")
	if err := os.WriteFile(filepath.Join(localBin, "tmux"), []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil { //nolint:gosec // test executable
		t.Fatal(err)
	}
	// a non-executable file and a directory named like commands are ignored.
	if err := os.WriteFile(filepath.Join(brewBin, "gh"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(brewBin, "uv"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries := []catalog.Entry{
		cli("claude-code", []string{"claude"}, &catalog.VersionProbe{Cmd: []string{"claude", "--version"}, Regex: `^(\S+)`}, map[catalog.Provenance]string{
			catalog.ProvNative: "~/.local/share/claude/**", catalog.ProvBrewCask: "{brew_prefix}/Caskroom/claude-code*/**"}),
		cli("codex", []string{"codex"}, &catalog.VersionProbe{Cmd: []string{"codex", "--version"}, Regex: `(\d+\.\d+\.\d+\S*)`}, nil),
		cli("node", []string{"node"}, &catalog.VersionProbe{Cmd: []string{"node", "--version"}, Regex: `v(\S+)`}, nil),
		cli("jq", []string{"jq"}, &catalog.VersionProbe{Cmd: []string{"jq", "--version"}, Regex: `jq-(\S+)`}, nil),
		cli("awscli", []string{"aws"}, &catalog.VersionProbe{Cmd: []string{"aws", "--version"}, Regex: `aws-cli/(\S+)`}, nil),
		cli("tmux", []string{"tmux"}, &catalog.VersionProbe{Cmd: []string{"tmux", "-V"}, Regex: `tmux (\S+)`}, nil),
		cli("gh", []string{"gh"}, nil, nil),
		cli("uv", []string{"uv"}, nil, nil),
		{ID: "iterm2", Kind: catalog.KindGUI, Detect: catalog.DetectSpec{BundleIDs: []string{"com.googlecode.iterm2"}}},
	}
	env := scan.Env{
		Platform: platform.Platform{OS: "linux"}, Home: home, BrewPrefix: prefix, NPMPrefix: npmPrefix,
		Path: []string{localBin, "", brewBin, npmBin, nvmBin, osBin, localBin},
	}
	r := newRecorder("/usr/bin", "/bin")
	d := NewPathDetector(r, PathOptions{ShimDir: osBin, VersionTimeout: 300 * time.Millisecond})
	if d.ID() != ownership.DetectorPath {
		t.Error("ID")
	}
	start := time.Now()
	got, err := d.Detect(context.Background(), entries, env)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("hanging probe took %s", elapsed)
	}
	byPkg := map[string][]ownership.Detection{}
	for _, g := range got {
		if g.Detector != ownership.DetectorPath || g.Status != ownership.DetectionOK {
			t.Errorf("bad detection %+v", g)
		}
		byPkg[g.PackageID] = append(byPkg[g.PackageID], g)
	}
	type want struct {
		path    string
		prov    catalog.Provenance
		version string
		winner  bool
	}
	wants := map[string][]want{
		"claude-code": {{filepath.Join(localBin, "claude"), catalog.ProvNative, "2.1.4", true}, {filepath.Join(brewBin, "claude"), catalog.ProvBrewCask, "2.1.3", false}},
		"codex":       {{filepath.Join(npmBin, "codex"), catalog.ProvNPM, "0.58.0", true}},
		"node":        {{filepath.Join(nvmBin, "node"), catalog.ProvNVM, "22.1.0", true}},
		"jq":          {{filepath.Join(osBin, "jq"), catalog.ProvUnknown, "1.7.1", true}},
		"awscli":      {{filepath.Join(localBin, "aws"), catalog.ProvUnknown, "", true}},
		"tmux":        {{filepath.Join(localBin, "tmux"), catalog.ProvUnknown, "", true}},
	}
	for id, ws := range wants {
		ds := byPkg[id]
		if len(ds) != len(ws) {
			t.Errorf("%s: %d detections, want %d: %+v", id, len(ds), len(ws), ds)
			continue
		}
		for i, w := range ws {
			d := ds[i]
			if !d.Found || d.Path != w.path || d.Provenance != w.prov || d.Version != w.version || d.PathWinner != w.winner {
				t.Errorf("%s[%d] = %+v, want %+v", id, i, d, w)
			}
		}
	}
	if ds := byPkg["claude-code"]; len(ds) > 0 && !strings.HasSuffix(ds[0].ResolvedPath, filepath.Join("versions", "2.1.4")) {
		t.Errorf("resolved path = %s", ds[0].ResolvedPath)
	}
	for _, id := range []string{"gh", "uv"} {
		if ds := byPkg[id]; len(ds) != 1 || ds[0].Found {
			t.Errorf("%s = %+v, want one not-found detection", id, ds)
		}
	}
	if _, ok := byPkg["iterm2"]; ok {
		t.Error("entries without commands get no PATH detection")
	}
}

func TestPathDetectorOSProvenanceAndCLT(t *testing.T) {
	root := t.TempDir()
	// A fake git in the shim dir; the fake world answers its version.
	f := testkit.NewFake(t, fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "git", Stdout: "git version 2.39.5 (Apple Git-154)\n"}}}, "git")
	shimDir := f.Dir
	entries := []catalog.Entry{cli("git", []string{"git"}, &catalog.VersionProbe{Cmd: []string{"git", "--version"}, Regex: `git version (\S+)`}, nil)}
	yes, no := true, false
	tests := []struct {
		name      string
		plat      string
		clt       *bool
		wantFound bool
	}{
		{"darwin without CLT never runs the shim", "darwin", &no, false},
		{"darwin with CLT unknown never runs the shim", "darwin", nil, false},
		{"darwin with CLT runs git", "darwin", &yes, true},
		{"linux runs git", "linux", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(f.Log(t))
			r := newRecorder(root)
			got, err := NewPathDetector(r, PathOptions{ShimDir: shimDir}).Detect(context.Background(), entries,
				scan.Env{Platform: platform.Platform{OS: tt.plat}, CLT: tt.clt, Path: []string{shimDir}})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].Found != tt.wantFound {
				t.Fatalf("got %+v", got)
			}
			ran := len(f.Log(t)) > before
			if ran != tt.wantFound {
				t.Errorf("git executed = %v, want %v (fakecmd log)", ran, tt.wantFound)
			}
			if tt.wantFound && got[0].Version != "2.39.5" {
				t.Errorf("version = %q", got[0].Version)
			}
		})
	}
	// default shim dir; /usr/bin paths are OS provenance.
	if d := NewPathDetector(nil, PathOptions{}); d.opts.ShimDir != "/usr/bin" || d.opts.VersionTimeout != VersionTimeout {
		t.Error("default shim dir")
	}
}

func TestPathDetectorCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewPathDetector(newRecorder(), PathOptions{}).Detect(ctx, []catalog.Entry{cli("jq", []string{"jq"}, nil, nil)}, scan.Env{})
	if err == nil {
		t.Error("cancelled scan must be an error")
	}
}

func TestProbeVersionEdgeCases(t *testing.T) {
	bin := t.TempDir()
	script(t, filepath.Join(bin, "tool"), "tool 1.2.3")
	r := newRecorder(bin)
	ctx := context.Background()
	tests := []struct {
		name  string
		probe *catalog.VersionProbe
		want  string
	}{
		{"no probe", nil, ""},
		{"bad regex", &catalog.VersionProbe{Cmd: []string{"tool"}, Regex: `(`}, ""},
		{"no group", &catalog.VersionProbe{Cmd: []string{"tool"}, Regex: `nomatch (\S+)`}, ""},
		{"other program on PATH", &catalog.VersionProbe{Cmd: []string{"tool", "-v"}, Regex: `tool (\S+)`}, "1.2.3"},
		{"missing program", &catalog.VersionProbe{Cmd: []string{"nope"}, Regex: `(.*)`}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := cli("x", []string{"x"}, tt.probe, nil)
			if got := probeVersion(ctx, r, e, "x", "/nonexistent/x", nil, VersionTimeout); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	// stderr is read when stdout has no version.
	if err := os.WriteFile(filepath.Join(bin, "err"), []byte("#!/bin/sh\necho 'aws-cli/2.17.0 Python/3' >&2\n"), 0o755); err != nil { //nolint:gosec // test executable
		t.Fatal(err)
	}
	e := cli("awscli", []string{"aws"}, &catalog.VersionProbe{Cmd: []string{"aws", "--version"}, Regex: `aws-cli/(\S+)`}, nil)
	if got := probeVersion(ctx, r, e, "aws", filepath.Join(bin, "err"), nil, VersionTimeout); got != "2.17.0" {
		t.Errorf("stderr version = %q", got)
	}
}

func TestBundleDetector(t *testing.T) {
	apps := t.TempDir()
	for _, a := range []string{"T3 Code (Beta).app", "iTerm.app", "Notes.txt", "Other.app"} {
		if err := os.MkdirAll(filepath.Join(apps, a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	plist := func(app string) string { return filepath.Join(apps, app, "Contents", "Info.plist") }
	world := fakeworld.World{Rules: []fakeworld.Rule{
		{Cmd: "mdfind", Args: []string{"kMDItemCFBundleIdentifier == 'com.microsoft.VSCode'"}, Stdout: "/Applications/Visual Studio Code.app\n"},
		{Cmd: "mdfind", Args: []string{"kMDItemCFBundleIdentifier == 'com.broken'"}, Exit: 1},
		{Cmd: "mdfind", Stdout: ""},
		{Cmd: "plutil", Args: []string{"-extract", "CFBundleShortVersionString", "raw", "-o", "-", "/Applications/Visual Studio Code.app/Contents/Info.plist"}, Stdout: "1.95.3\n"},
		{Cmd: "plutil", Args: []string{"-extract", "CFBundleIdentifier", "raw", "-o", "-", plist("T3 Code (Beta).app")}, Stdout: "com.t3tools.t3code\n"},
		{Cmd: "plutil", Args: []string{"-extract", "CFBundleShortVersionString", "raw", "-o", "-", plist("T3 Code (Beta).app")}, Stdout: "0.9.1\n"},
		{Cmd: "plutil", Args: []string{"-extract", "CFBundleIdentifier", "raw", "-o", "-", plist("iTerm.app")}, Stdout: "com.googlecode.iterm2\n"},
		{Cmd: "plutil", Exit: 1, Stderr: "no such key"},
	}}
	f := testkit.NewFake(t, world, "mdfind", "plutil")
	r := newRecorder(f.Dir)
	gui := func(id string, bundles ...string) catalog.Entry {
		return catalog.Entry{ID: id, Kind: catalog.KindGUI, Detect: catalog.DetectSpec{BundleIDs: bundles}}
	}
	entries := []catalog.Entry{
		gui("visual-studio-code", "com.microsoft.VSCode"),
		gui("t3-code", "com.t3tools.t3code"),
		gui("iterm2", "com.broken", "com.googlecode.iterm2"),
		gui("orca", "dev.orca.app", "bad'id"),
		cli("jq", []string{"jq"}, nil, nil),
	}
	d := NewBundleDetector(r, BundleOptions{AppDirs: []string{filepath.Join(t.TempDir(), "missing"), apps}})
	if d.ID() != ownership.DetectorBundle {
		t.Error("ID")
	}
	got, err := d.Detect(context.Background(), entries, scan.Env{Platform: platform.Platform{OS: "darwin"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]ownership.Detection{
		"visual-studio-code": {Found: true, Path: "/Applications/Visual Studio Code.app", ResolvedPath: "/Applications/Visual Studio Code.app", Provenance: catalog.ProvAppBundle, Version: "1.95.3"},
		"t3-code":            {Found: true, Path: filepath.Join(apps, "T3 Code (Beta).app"), ResolvedPath: filepath.Join(apps, "T3 Code (Beta).app"), Provenance: catalog.ProvAppBundle, Version: "0.9.1"},
		"iterm2":             {Found: true, Path: filepath.Join(apps, "iTerm.app"), ResolvedPath: filepath.Join(apps, "iTerm.app"), Provenance: catalog.ProvAppBundle},
		"orca":               {},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, g := range got {
		w := want[g.PackageID]
		w.PackageID, w.Detector, w.Status = g.PackageID, ownership.DetectorBundle, ownership.DetectionOK
		if g != w {
			t.Errorf("%s = %+v, want %+v", g.PackageID, g, w)
		}
	}
	// the fallback scan runs once and reads each .app once.
	count := 0
	for _, inv := range f.Log(t) {
		if inv.Argv[0] == "plutil" && inv.Argv[2] == "CFBundleIdentifier" {
			count++
		}
	}
	if count != 3 {
		t.Errorf("fallback scan read %d Info.plist files, want 3", count)
	}
	testkit.AssertSafeInvocations(t, f.Log(t))

	if got, err := d.Detect(context.Background(), entries, scan.Env{Platform: platform.Platform{OS: "linux"}}); got != nil || err != nil {
		t.Errorf("linux: %v %v", got, err)
	}
}

type errRunner struct{ err error }

func (e errRunner) Run(context.Context, process.Cmd) (process.Result, error) {
	return process.Result{ExitCode: -1}, e.err
}

func TestBundleDetectorFailures(t *testing.T) {
	entries := []catalog.Entry{{ID: "x", Kind: catalog.KindGUI, Detect: catalog.DetectSpec{BundleIDs: []string{"com.x"}}}}
	darwin := scan.Env{Platform: platform.Platform{OS: "darwin"}, Home: t.TempDir()}
	if _, err := NewBundleDetector(errRunner{process.ErrTimeout}, BundleOptions{}).Detect(context.Background(), entries, darwin); err == nil {
		t.Error("mdfind timeout must be an error")
	}
	got, err := NewBundleDetector(errRunner{process.ErrNotFound}, BundleOptions{AppDirs: []string{}}).Detect(context.Background(), entries, darwin)
	if err != nil || len(got) != 1 || got[0].Found {
		t.Errorf("no mdfind falls back: %+v %v", got, err)
	}
	d := NewBundleDetector(errRunner{process.ErrNotFound}, BundleOptions{})
	if d.opts.Mdfind != "mdfind" || d.opts.Plutil != "plutil" {
		t.Error("defaults")
	}
	if _, err := d.Detect(context.Background(), entries, darwin); err != nil {
		t.Errorf("default app dirs: %v", err)
	}
	apps := t.TempDir()
	if err := os.MkdirAll(filepath.Join(apps, "A.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewBundleDetector(errRunner{process.ErrNotFound}, BundleOptions{AppDirs: []string{apps}}).Detect(ctx, entries, darwin); err == nil {
		t.Error("cancelled fallback scan must be an error")
	}
}

func TestNPMDetector(t *testing.T) {
	list := `{"dependencies":{"paperclipai":{"version":"0.4.0"},"@anthropic-ai/claude-code":{"version":"2.1.0"},"@openai/codex":{"version":"0.58.0"},"@google/gemini-cli":{"version":"0.9.0"}}}`
	npmEntry := func(id, pkg string) catalog.Entry {
		return catalog.Entry{ID: id, Kind: catalog.KindCLI, Detect: catalog.DetectSpec{NPMPackage: pkg}}
	}
	entries := []catalog.Entry{
		npmEntry("paperclip", "paperclipai"), npmEntry("claude-code", "@anthropic-ai/claude-code"), npmEntry("codex", "@openai/codex"),
		npmEntry("gemini-cli", "@google/gemini-cli"), npmEntry("copilot-cli", "@github/copilot"), cli("jq", []string{"jq"}, nil, nil),
	}
	f := testkit.NewFake(t, fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "npm", Args: []string{"ls", "-g", "--json", "--depth=0"}, Stdout: list, Exit: 1}}}, "npm")
	r := newRecorder(f.Dir)
	d := NewNPMDetector(r)
	if d.ID() != ownership.DetectorNPM {
		t.Error("ID")
	}
	got, err := d.Detect(context.Background(), entries, scan.Env{Path: []string{"", f.Dir}, NPMPrefix: "/usr/local"})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]ownership.Detection{}
	for _, g := range got {
		found[g.PackageID] = g
	}
	if len(got) != 5 {
		t.Fatalf("got %+v", got)
	}
	cc := found["claude-code"]
	if !cc.Found || cc.Version != "2.1.0" || cc.Provenance != catalog.ProvNPM || cc.BackendRef != "npm:@anthropic-ai/claude-code" ||
		cc.Path != "/usr/local/lib/node_modules/@anthropic-ai/claude-code" {
		t.Errorf("claude-code = %+v", cc)
	}
	for _, id := range []string{"paperclip", "codex", "gemini-cli"} {
		if !found[id].Found {
			t.Errorf("%s not found", id)
		}
	}
	if found["copilot-cli"].Found {
		t.Error("copilot-cli is not installed")
	}
	if log := f.Log(t); len(log) != 1 || !slices.Equal(log[0].Argv, []string{"npm", "ls", "-g", "--json", "--depth=0"}) {
		t.Errorf("log = %+v", log)
	}

	if got, err := d.Detect(context.Background(), entries, scan.Env{Path: []string{t.TempDir()}}); got != nil || err != nil {
		t.Errorf("npm absent: %v %v", got, err)
	}
	bad := testkit.NewFake(t, fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "npm", Stdout: "npm ERR! oops"}}}, "npm")
	if _, err := NewNPMDetector(newRecorder(bad.Dir)).Detect(context.Background(), entries, scan.Env{Path: []string{bad.Dir}}); err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("bad JSON err = %v", err)
	}
	if _, err := NewNPMDetector(errRunner{process.ErrTimeout}).Detect(context.Background(), entries, scan.Env{Path: []string{bad.Dir}}); err == nil {
		t.Error("timeout must be an error")
	}
	noPrefix, _ := NewNPMDetector(r).Detect(context.Background(), entries, scan.Env{Path: []string{f.Dir}})
	if noPrefix[1].Path != "" {
		t.Errorf("no npm prefix gives no path: %+v", noPrefix[1])
	}
}
