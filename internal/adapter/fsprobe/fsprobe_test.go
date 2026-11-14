package fsprobe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/adapter/execrunner"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/testkit"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testkit.RemoveBuild()
	os.Exit(code)
}

func write(t *testing.T, p string, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Fatal(err)
	}
}

var claudeEntry = catalog.Entry{ID: "claude-code", Kind: catalog.KindCLI,
	Detect:  catalog.DetectSpec{Commands: []string{"claude"}, Provenance: map[catalog.Provenance]string{catalog.ProvNative: "~/.local/share/claude/**"}},
	Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: "claude-code"}}}}

var ghEntry = catalog.Entry{ID: "gh", Kind: catalog.KindCLI,
	Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewFormula, ID: "gh"}}}}

func TestProbe(t *testing.T) {
	root := t.TempDir()
	prefix := filepath.Join(root, "brew")
	npm := filepath.Join(root, "npm")
	home := filepath.Join(root, "home")
	bin := filepath.Join(prefix, "bin")
	loc := catalog.Locations{BrewPrefix: prefix, NPMPrefix: npm, Home: home}

	link(t, filepath.Join(prefix, "lib", "node_modules", "@anthropic-ai", "claude-code", "cli.js"), filepath.Join(bin, "dangling"))
	write(t, filepath.Join(bin, "plain"), "not a program", 0o644)
	write(t, filepath.Join(bin, "empty"), "", 0o755)
	write(t, filepath.Join(prefix, "Caskroom", "gemini", "1.0", "gemini"), "#!/bin/sh\n", 0o755)
	link(t, filepath.Join(prefix, "Caskroom", "gemini", "1.0", "gemini"), filepath.Join(bin, "other-cask"))
	write(t, filepath.Join(prefix, "Caskroom", "claude-code", "2.1.4", "claude"), "#!/bin/sh\n", 0o755)
	link(t, filepath.Join(prefix, "Caskroom", "claude-code", "2.1.4", "claude"), filepath.Join(bin, "own-cask"))
	write(t, filepath.Join(prefix, "Cellar", "gh", "2.62.0", "bin", "gh"), "#!/bin/sh\n", 0o755)
	link(t, "../Cellar/gh/2.62.0/bin/gh", filepath.Join(bin, "own-formula"))
	write(t, filepath.Join(prefix, "Cellar", "gemini-cli", "1", "bin", "gemini"), "#!/bin/sh\n", 0o755)
	link(t, filepath.Join(prefix, "Cellar", "gemini-cli", "1", "bin", "gemini"), filepath.Join(bin, "other-formula"))
	write(t, filepath.Join(prefix, "lib", "node_modules", "@anthropic-ai", "claude-code", "cli2.js"), "x", 0o755)
	link(t, filepath.Join(prefix, "lib", "node_modules", "@anthropic-ai", "claude-code", "cli2.js"), filepath.Join(bin, "npm-scoped"))
	write(t, filepath.Join(npm, "lib", "node_modules", "paperclipai", "bin.js"), "x", 0o755)
	link(t, filepath.Join(npm, "lib", "node_modules", "paperclipai", "bin.js"), filepath.Join(bin, "npm-plain"))
	write(t, filepath.Join(home, ".local", "share", "claude", "versions", "2.1.4"), "x", 0o755)
	link(t, filepath.Join(home, ".local", "share", "claude", "versions", "2.1.4"), filepath.Join(bin, "native"))
	if err := os.MkdirAll(filepath.Join(bin, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := New(execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + t.TempDir()}}), Options{Locations: func() catalog.Locations { return loc }})
	ctx := context.Background()
	tests := []struct {
		name       string
		entry      catalog.Entry
		owner      plan.OwnerClass
		detail     string
		symlink    bool
		dangling   bool
		executable bool
		fileType   string
	}{
		{"dangling", claudeEntry, plan.OwnerOrphan, "dangling symlink", true, true, false, "broken symbolic link to "},
		{"plain", claudeEntry, plan.OwnerOrphan, "no package manager", false, false, false, "regular file"},
		{"empty", claudeEntry, plan.OwnerOrphan, "no package manager", false, false, true, "empty"},
		{"other-cask", claudeEntry, plan.OwnerOtherBrew, "brew cask gemini", true, false, true, "symbolic link to "},
		{"own-cask", claudeEntry, plan.OwnerThisPackage, "brew cask claude-code", true, false, true, ""},
		{"own-formula", ghEntry, plan.OwnerThisPackage, "brew formula gh", true, false, true, ""},
		{"other-formula", claudeEntry, plan.OwnerOtherBrew, "brew formula gemini-cli", true, false, true, ""},
		{"npm-scoped", claudeEntry, plan.OwnerNPM, "npm @anthropic-ai/claude-code", true, false, true, ""},
		{"npm-plain", claudeEntry, plan.OwnerNPM, "npm paperclipai", true, false, true, ""},
		{"native", claudeEntry, plan.OwnerOtherInstaller, "native install", true, false, true, ""},
		{"adir", claudeEntry, plan.OwnerOrphan, "", false, false, false, "directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(bin, tt.name)
			ev, err := p.Inspect(ctx, path)
			if err != nil || ev == nil {
				t.Fatalf("Inspect = %+v, %v", ev, err)
			}
			if ev.IsSymlink != tt.symlink || ev.Dangling != tt.dangling || ev.Executable != tt.executable {
				t.Errorf("evidence = %+v", ev)
			}
			if tt.fileType != "" && !strings.HasPrefix(ev.FileType, tt.fileType) {
				t.Errorf("file type = %q, want %q", ev.FileType, tt.fileType)
			}
			if !strings.Contains(ev.LsLine, path) || ev.ModTime.IsZero() {
				t.Errorf("ls line = %q", ev.LsLine)
			}
			if tt.symlink && (!strings.HasPrefix(ev.LsLine, "l") || !strings.HasSuffix(ev.LsLine, " -> "+ev.SymlinkTarget) || ev.SymlinkTarget == "") {
				t.Errorf("symlink ls line = %q", ev.LsLine)
			}
			owner, detail, err := p.Owner(ctx, path, tt.entry)
			if err != nil {
				t.Fatal(err)
			}
			if owner != tt.owner || !strings.Contains(detail, tt.detail) {
				t.Errorf("owner = %s %q, want %s %q", owner, detail, tt.owner, tt.detail)
			}
		})
	}
	if ev, err := p.Inspect(ctx, filepath.Join(bin, "missing")); ev != nil || err != nil {
		t.Errorf("missing path = %+v, %v", ev, err)
	}
	if _, _, err := p.Owner(ctx, filepath.Join(bin, "missing"), claudeEntry); err == nil {
		t.Error("owner of a missing path is an error")
	}
	ev, _ := p.Inspect(ctx, filepath.Join(bin, "plain"))
	if ev.Size != int64(len("not a program")) || !strings.HasPrefix(ev.LsLine, "-rw-r--r-- 1 ") || !strings.Contains(ev.LsLine, " 13 ") {
		t.Errorf("plain = %+v", ev)
	}
	if time.Since(ev.ModTime) > time.Hour {
		t.Errorf("mod time = %s", ev.ModTime)
	}
}

func TestProbeOwnerOSAndNoPrefix(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "x"), "x", 0o755)
	p := New(nil, Options{Locations: func() catalog.Locations { return catalog.Locations{} }})
	if owner, _, _ := p.Owner(context.Background(), filepath.Join(root, "x"), claudeEntry); owner != plan.OwnerOrphan {
		t.Errorf("no locations = %s", owner)
	}
	// /bin/sh exists on every test machine and is an OS file.
	if owner, detail, err := p.Owner(context.Background(), "/bin/sh", claudeEntry); err != nil || owner != plan.OwnerOtherInstaller || !strings.Contains(detail, "operating system") {
		if resolved, _ := filepath.EvalSymlinks("/bin/sh"); filepath.Dir(resolved) == "/bin" || filepath.Dir(resolved) == "/usr/bin" {
			t.Errorf("/bin/sh = %s %q %v", owner, detail, err)
		}
	}
	if got, ok := npmPackage("/p/lib/node_modules/@scope", "/p"); !ok || got != "@scope" {
		t.Errorf("bare scope = %q %v", got, ok)
	}
	if _, ok := firstSegment("/p/", "/p"); ok {
		t.Error("no segment")
	}
}

func TestProbeFileCommand(t *testing.T) {
	f := testkit.NewFake(t, fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "file", Stdout: "Mach-O 64-bit executable arm64\n"}}}, "file")
	root := t.TempDir()
	write(t, filepath.Join(root, "x"), "x", 0o755)
	p := New(execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + f.Dir}}), Options{Locations: func() catalog.Locations { return catalog.Locations{} }})
	ev, err := p.Inspect(context.Background(), filepath.Join(root, "x"))
	if err != nil || ev.FileType != "Mach-O 64-bit executable arm64" {
		t.Errorf("file type = %+v, %v", ev, err)
	}
	log := f.Log(t)
	if len(log) != 1 || log[0].Argv[1] != "-b" {
		t.Errorf("log = %+v", log)
	}
	if modeString(os.ModeDir|0o755) != "drwxr-xr-x" {
		t.Error("dir mode")
	}
	if userName(4294967294) == "" || groupName(4294967294) == "" {
		t.Error("unknown ids fall back to numbers")
	}
}

func TestInspectUnreadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read anything")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	p := New(nil, Options{Locations: func() catalog.Locations { return catalog.Locations{} }})
	if _, err := p.Inspect(context.Background(), filepath.Join(locked, "x")); err == nil {
		t.Error("unreadable directory must be an error")
	}
}
