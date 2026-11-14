package brew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/adapter/execrunner"
	"github.com/stainedhead/software-factory-manager/internal/testkit"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testkit.RemoveBuild()
	os.Exit(code)
}

// fakePrefix builds <tmp>/prefix/bin -> a fake bin dir holding brew, so
// <tmp>/prefix is a Homebrew prefix that is not on PATH.
func fakePrefix(t *testing.T, w fakeworld.World) (prefix string, f *testkit.Fake) {
	t.Helper()
	f = testkit.NewFake(t, w, "brew")
	prefix = filepath.Join(t.TempDir(), "prefix")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.Dir, filepath.Join(prefix, "bin")); err != nil {
		t.Fatal(err)
	}
	return prefix, f
}

func envFunc(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLocate(t *testing.T) {
	world := fakeworld.World{Brew: &fakeworld.Brew{Prefix: "/x", Version: "6.0.2"}}
	onPath := testkit.NewFake(t, world, "brew")
	prefix, _ := fakePrefix(t, world)
	empty := t.TempDir()
	me := os.Getuid()

	tests := []struct {
		name      string
		path      string
		prefixes  []string
		owner     int
		ownerErr  error
		want      bool
		wantOnP   bool
		wantPath  string
		wantPfx   string
		wantOther bool
		wantErr   bool
	}{
		{name: "on PATH", path: onPath.Dir, prefixes: []string{prefix}, owner: me, want: true, wantOnP: true,
			wantPath: onPath.Path("brew"), wantPfx: filepath.Dir(onPath.Dir)},
		{name: "known prefix not on PATH", path: empty, prefixes: []string{"/nonexistent", prefix}, owner: me, want: true,
			wantPath: filepath.Join(prefix, "bin", "brew"), wantPfx: prefix},
		{name: "not found", path: ":" + empty, prefixes: []string{"/nonexistent"}, owner: me},
		{name: "prefix owned by another user", path: empty, prefixes: []string{prefix}, owner: me + 1, want: true,
			wantPath: filepath.Join(prefix, "bin", "brew"), wantPfx: prefix, wantOther: true},
		{name: "owner lookup fails", path: empty, prefixes: []string{prefix}, ownerErr: errors.New("boom"), want: true,
			wantPath: filepath.Join(prefix, "bin", "brew"), wantPfx: prefix, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + tt.path}})
			l := NewLocator(r, LocatorOptions{
				KnownPrefixes: tt.prefixes,
				Getenv:        envFunc(map[string]string{"PATH": tt.path}),
				UID:           func() int { return me },
				OwnerUID:      func(string) (int, error) { return tt.owner, tt.ownerErr },
			})
			info, err := l.Locate(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if info.Found != tt.want || info.OnPATH != tt.wantOnP || info.Path != tt.wantPath || info.Prefix != tt.wantPfx || info.PrefixOwnedByOther != tt.wantOther {
				t.Errorf("info = %+v", info)
			}
			if tt.want && !tt.wantErr && info.Version != "6.0.2" {
				t.Errorf("version = %q", info.Version)
			}
		})
	}
}

// Review FR-002: a brew in a relative PATH entry (the current directory of a
// cloned repo) is never located, so it is never run.
func TestLocateSkipsRelativePathEntries(t *testing.T) {
	world := fakeworld.World{Brew: &fakeworld.Brew{Prefix: "/x", Version: "6.0.2"}}
	cwdFake := testkit.NewFake(t, world, "brew")
	t.Chdir(cwdFake.Dir)
	for _, path := range []string{".", "", "./"} {
		r := execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + path}})
		l := NewLocator(r, LocatorOptions{
			KnownPrefixes: []string{"/nonexistent"},
			Getenv:        envFunc(map[string]string{"PATH": path}),
			UID:           os.Getuid,
			OwnerUID:      func(string) (int, error) { return os.Getuid(), nil },
		})
		info, err := l.Locate(context.Background())
		if err != nil || info.Found {
			t.Errorf("PATH=%q: info = %+v, err = %v; want not found", path, info, err)
		}
	}
	if n := len(cwdFake.Log(t)); n != 0 {
		t.Errorf("the brew in the current directory ran %d time(s)", n)
	}
}

func TestLocateDefaultsAndRealOwner(t *testing.T) {
	l := NewLocator(execrunner.New(execrunner.Options{}), LocatorOptions{})
	if len(l.opts.KnownPrefixes) == 0 || l.opts.Getenv == nil || l.opts.UID == nil {
		t.Error("defaults not set")
	}
	if got := DefaultPrefixes("darwin"); len(got) != 2 || got[0] != "/opt/homebrew" {
		t.Errorf("darwin prefixes = %v", got)
	}
	if got := DefaultPrefixes("linux"); len(got) != 1 || got[0] != "/home/linuxbrew/.linuxbrew" {
		t.Errorf("linux prefixes = %v", got)
	}
	uid, err := ownerUID(t.TempDir())
	if err != nil || uid != os.Getuid() {
		t.Errorf("ownerUID = %d, %v", uid, err)
	}
	if _, err := ownerUID(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing path has no owner")
	}
}

func TestShellEnv(t *testing.T) {
	f := testkit.NewFake(t, fakeworld.World{Brew: &fakeworld.Brew{Prefix: "/opt/homebrew"}}, "brew")
	r := execrunner.New(execrunner.Options{BaseEnv: []string{"PATH=" + f.Dir}})
	l := NewLocator(r, LocatorOptions{Getenv: envFunc(map[string]string{"PATH": "/usr/bin:/bin"})})
	env, err := l.ShellEnv(context.Background(), f.Path("brew"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"HOMEBREW_PREFIX":     "/opt/homebrew",
		"HOMEBREW_CELLAR":     "/opt/homebrew/Cellar",
		"HOMEBREW_REPOSITORY": "/opt/homebrew",
		"PATH":                "/opt/homebrew/bin:/opt/homebrew/sbin:/usr/bin:/bin",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	log := f.Log(t)
	if len(log) != 1 || log[0].Argv[1] != "shellenv" {
		t.Errorf("log = %+v", log)
	}
	testkit.AssertSafeInvocations(t, log)

	failing := testkit.NewFake(t, fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "brew", Exit: 1, Stderr: "Error: broken\n"}}}, "brew")
	if _, err := l.ShellEnv(context.Background(), failing.Path("brew")); err == nil {
		t.Error("failing shellenv must be an error")
	}
	silent := testkit.NewFake(t, fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "brew", Stdout: "echo hi\n"}}}, "brew")
	if _, err := l.ShellEnv(context.Background(), silent.Path("brew")); err == nil {
		t.Error("no exports must be an error")
	}
	if _, err := l.ShellEnv(context.Background(), filepath.Join(t.TempDir(), "brew")); err == nil {
		t.Error("missing brew must be an error")
	}
}

func TestParseShellEnv(t *testing.T) {
	base := envFunc(map[string]string{"PATH": "/usr/bin", "INFOPATH": "/info", "MANPATH": ":/man"})
	out := `export HOMEBREW_PREFIX="/opt/homebrew";
export HOMEBREW_CELLAR="$HOMEBREW_PREFIX/Cellar";
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin${PATH+:$PATH}";
[ -z "${MANPATH-}" ] || export MANPATH=":${MANPATH#:}";
export INFOPATH="/opt/homebrew/share/info:${INFOPATH:-}";
export EMPTY="${NOPE:-fallback}";
export DASH="${NOPE-dflt}";
export SET="${PATH-dflt}";
export PLUS="${NOPE+x}";
export RAW='single';
export BAD="$(whoami)";
export TICK="` + "`id`" + `";
export UNCLOSED="${PATH";
export WEIRD="${:-x}";
export ODD="${PATH%%x}";
export 1BAD=x;
export noeq;
export TRAIL="cost$";
export DOLLAR="a$ b";
eval something
`
	env := ParseShellEnv(out, base)
	want := map[string]string{
		"HOMEBREW_PREFIX": "/opt/homebrew",
		"HOMEBREW_CELLAR": "/opt/homebrew/Cellar",
		"PATH":            "/opt/homebrew/bin:/opt/homebrew/sbin:/usr/bin",
		"INFOPATH":        "/opt/homebrew/share/info:/info",
		"EMPTY":           "fallback",
		"DASH":            "dflt",
		"SET":             "/opt/homebrew/bin:/opt/homebrew/sbin:/usr/bin",
		"PLUS":            "",
		"RAW":             "single",
		"TRAIL":           "cost$",
		"DOLLAR":          "a$ b",
	}
	for k, v := range want {
		if got, ok := env[k]; !ok || got != v {
			t.Errorf("%s = %q (set %v), want %q", k, got, ok, v)
		}
	}
	for _, k := range []string{"BAD", "TICK", "UNCLOSED", "WEIRD", "ODD", "1BAD", "noeq", "MANPATH"} {
		if _, ok := env[k]; ok {
			t.Errorf("%s must be skipped, got %q", k, env[k])
		}
	}
}
