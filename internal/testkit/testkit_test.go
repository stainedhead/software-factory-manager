package testkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

func TestMain(m *testing.M) {
	code := m.Run()
	RemoveBuild()
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, f *Fake, env []string, name string, args ...string) result {
	t.Helper()
	cmd := exec.Command(f.Path(name), args...)
	cmd.Env = env
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", name, err)
	}
	return result{out.String(), errOut.String(), code}
}

func brewWorld(prefix string) fakeworld.World {
	return fakeworld.World{
		Rules: []fakeworld.Rule{
			{Cmd: "getconf", Args: []string{"GNU_LIBC_VERSION"}, Stdout: "glibc 2.39\n"},
			{Cmd: "xcode-select", Args: []string{"-p"}, Exit: 2, Stderr: "xcode-select: error: unable to get active developer directory\n"},
			{Cmd: "brew", ArgsPrefix: []string{"install", "slow"}, SleepMS: 10, Builtin: true},
			{Cmd: "brew", Args: []string{"install", "broken"}, Exit: 1, Stderr: "Error: broken\n"},
		},
		Brew: &fakeworld.Brew{
			Prefix: prefix,
			Formulae: map[string]fakeworld.Formula{
				"jq":   {Version: "1.7.1", Files: []string{"Cellar/jq/1.7.1/bin/jq"}, Symlinks: map[string]string{"bin/jq": "Cellar/jq/1.7.1/bin/jq"}},
				"slow": {Version: "1.0"},
			},
			Casks: map[string]fakeworld.Cask{
				"claude-code":        {Version: "2.1.4", Artifacts: []map[string]any{{"binary": []any{"claude"}}}},
				"stablyai/orca/orca": {Version: "1.0.0", Artifacts: []map[string]any{{"app": []any{"Orca.app"}}}},
			},
		},
	}
}

func TestFakeBrewInfoAndInstall(t *testing.T) {
	prefix := t.TempDir()
	f := NewFake(t, brewWorld(prefix))
	env := append(f.Env(t.TempDir()), "SECRET_TOKEN=x")

	r := run(t, f, env, "brew", "info", "--json=v2", "--installed")
	if r.code != 0 || strings.TrimSpace(r.stdout) != `{"formulae":[],"casks":[]}` {
		t.Fatalf("empty info = %+v", r)
	}
	if r := run(t, f, env, "brew", "install", "jq"); r.code != 0 {
		t.Fatalf("install jq: %+v", r)
	}
	r = run(t, f, env, "brew", "info", "--json=v2", "--installed")
	var doc struct {
		Formulae []struct {
			Name      string `json:"name"`
			Installed []struct {
				Version string `json:"version"`
			} `json:"installed"`
			LinkedKeg *string `json:"linked_keg"`
		} `json:"formulae"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatalf("decoding %q: %v", r.stdout, err)
	}
	if len(doc.Formulae) != 1 || doc.Formulae[0].Name != "jq" || doc.Formulae[0].Installed[0].Version != "1.7.1" || *doc.Formulae[0].LinkedKeg != "1.7.1" {
		t.Errorf("after install: %+v", doc)
	}
	if got := f.World(t).Brew.Installed.Formulae["jq"]; got != "1.7.1" {
		t.Errorf("state jq = %q", got)
	}
	// the install created the link and the keg file, which prints its version
	out, err := exec.Command(filepath.Join(prefix, "bin", "jq")).Output()
	if err != nil || strings.TrimSpace(string(out)) != "1.7.1" {
		t.Errorf("installed jq: %q, %v", out, err)
	}

	log := f.Log(t)
	if len(log) != 3 {
		t.Fatalf("log has %d entries: %+v", len(log), log)
	}
	if !slices.Equal(log[1].Argv, []string{"brew", "install", "jq"}) {
		t.Errorf("log[1].Argv = %v", log[1].Argv)
	}
	if !slices.Contains(log[0].EnvNames, "SECRET_TOKEN") || log[0].Cwd == "" {
		t.Errorf("log[0] = %+v", log[0])
	}
	for _, inv := range log {
		for _, n := range inv.EnvNames {
			if strings.Contains(n, "=") {
				t.Errorf("env name %q contains a value", n)
			}
		}
	}
	AssertSafeInvocations(t, log)
}

func TestFakeRules(t *testing.T) {
	f := NewFake(t, brewWorld(t.TempDir()))
	env := f.Env(t.TempDir())
	tests := []struct {
		name       string
		prog       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"stdout rule", "getconf", []string{"GNU_LIBC_VERSION"}, 0, "glibc 2.39", ""},
		{"exit and stderr", "xcode-select", []string{"-p"}, 2, "", "unable to get active developer directory"},
		{"no rule", "mdfind", []string{"x"}, 127, "", "no rule for mdfind x"},
		{"failing install rule", "brew", []string{"install", "broken"}, 1, "", "Error: broken"},
		{"sleep then builtin", "brew", []string{"install", "slow"}, 0, "Installed slow", ""},
		{"prefix", "brew", []string{"--prefix"}, 0, "", ""},
		{"version", "brew", []string{"--version"}, 0, "Homebrew 6.0.0", ""},
		{"shellenv", "brew", []string{"shellenv"}, 0, `export HOMEBREW_PREFIX="`, ""},
		{"cask info", "brew", []string{"info", "--json=v2", "--cask", "claude-code"}, 0, `"binary":["claude"]`, ""},
		{"tap item info before tap", "brew", []string{"info", "--json=v2", "--cask", "stablyai/orca/orca"}, 1, "", "No available formula or cask"},
		{"tap item install before tap", "brew", []string{"install", "--cask", "stablyai/orca/orca"}, 1, "", "Tap stablyai/orca first"},
		{"unsupported", "brew", []string{"doctor"}, 1, "", "unsupported brew command"},
		{"no args", "brew", nil, 1, "", "Example usage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := run(t, f, env, tt.prog, tt.args...)
			if r.code != tt.wantCode || !strings.Contains(r.stdout, tt.wantStdout) || !strings.Contains(r.stderr, tt.wantStderr) {
				t.Errorf("got %+v, want code %d stdout %q stderr %q", r, tt.wantCode, tt.wantStdout, tt.wantStderr)
			}
		})
	}
}

func TestFakeTapTrustFlow(t *testing.T) {
	f := NewFake(t, brewWorld(t.TempDir()), "brew")
	env := f.Env(t.TempDir())
	steps := []struct {
		args     []string
		wantCode int
		want     string
	}{
		{[]string{"tap"}, 0, ""},
		{[]string{"tap", "stablyai/orca"}, 0, "Tapped stablyai/orca"},
		{[]string{"tap"}, 0, "stablyai/orca"},
		{[]string{"install", "--cask", "stablyai/orca/orca"}, 1, "untrusted tap"},
		{[]string{"trust", "--cask", "stablyai/orca/orca"}, 0, "Trusted"},
		{[]string{"trust"}, 0, "stablyai/orca/orca"},
		{[]string{"install", "--cask", "stablyai/orca/orca"}, 0, "Installed"},
		{[]string{"info", "--json=v2", "--cask", "stablyai/orca/orca"}, 0, `"installed":"1.0.0"`},
	}
	for _, s := range steps {
		r := run(t, f, env, "brew", s.args...)
		if r.code != s.wantCode || !strings.Contains(r.stdout+r.stderr, s.want) {
			t.Fatalf("brew %v = %+v, want code %d and %q", s.args, r, s.wantCode, s.want)
		}
	}
	AssertSafeInvocations(t, f.Log(t))
}

// recorder captures Errorf calls so assertion failures can be tested.
type recorder struct {
	testing.TB
	errors []string
}

func (r *recorder) Helper() {}
func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, strings.TrimSpace(strings.ReplaceAll(format, "%q", "")))
	_ = args
}

func TestAssertSafeInvocations(t *testing.T) {
	inv := func(env []string, argv ...string) fakeworld.Invocation {
		return fakeworld.Invocation{Argv: argv, EnvNames: env}
	}
	tests := []struct {
		name string
		inv  fakeworld.Invocation
		want int
	}{
		{"safe install", inv(nil, "brew", "install", "--cask", "claude-code"), 0},
		{"safe item trust", inv(nil, "brew", "trust", "--cask", "stablyai/orca/orca"), 0},
		{"safe read", inv(nil, "brew", "info", "--json=v2", "--installed"), 0},
		{"force", inv(nil, "brew", "install", "--force", "jq"), 1},
		{"overwrite", inv(nil, "brew", "link", "--overwrite", "jq"), 2},
		{"upgrade", inv(nil, "brew", "upgrade"), 1},
		{"uninstall", inv(nil, "/opt/homebrew/bin/brew", "uninstall", "jq"), 1},
		{"whole tap trust", inv(nil, "brew", "trust", "stablyai/orca"), 1},
		{"tap trust env", inv([]string{"HOME", "HOMEBREW_NO_REQUIRE_TAP_TRUST"}, "brew", "install", "jq"), 1},
		{"tap trust env variant", inv([]string{"HOMEBREW_NO_REQUIRE_TAP_TRUST_FOR"}, "npm", "ls"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(UnsafeReasons(tt.inv)); got != tt.want {
				t.Errorf("UnsafeReasons = %v, want %d", UnsafeReasons(tt.inv), tt.want)
			}
			rec := &recorder{TB: t}
			AssertSafeInvocations(rec, []fakeworld.Invocation{tt.inv})
			if (len(rec.errors) > 0) != (tt.want > 0) {
				t.Errorf("AssertSafeInvocations recorded %v", rec.errors)
			}
		})
	}
}

func TestFakeCopyFallbackAndDirEnv(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "brew")
	if err := linkOrCopy(FakecmdBinary(t), dst); err != nil {
		t.Fatal(err)
	}
	if err := linkOrCopy(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("copying a missing file should fail")
	}
	// FAKECMD_DIR points a program anywhere at a world
	f := NewFake(t, brewWorld(t.TempDir()), "getconf")
	other := t.TempDir()
	if err := fakeworld.Save(filepath.Join(other, fakeworld.WorldFile), fakeworld.World{Rules: []fakeworld.Rule{{Cmd: "getconf", Stdout: "other\n"}}}); err != nil {
		t.Fatal(err)
	}
	r := run(t, f, append(f.Env(t.TempDir()), fakeworld.DirEnv+"="+other), "getconf", "X")
	if strings.TrimSpace(r.stdout) != "other" {
		t.Errorf("FAKECMD_DIR ignored: %+v", r)
	}
}
