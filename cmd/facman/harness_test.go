package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/stainedhead/software-factory-manager/internal/adapter/catalogyaml"
	"github.com/stainedhead/software-factory-manager/internal/adapter/report"
	"github.com/stainedhead/software-factory-manager/internal/adapter/sqlite"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/testkit"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) != "" {
		os.Exit(crashHelper())
	}
	code := m.Run()
	testkit.RemoveBuild()
	os.Exit(code)
}

// fixedNow is the e2e clock: consent strings are j@2026-11-14T19:00:00Z.
var fixedNow = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

const (
	testUser = "j"
	testUID  = 501
	consent  = "j@2026-11-14T19:00:00Z"
)

// brewState says where the fake Homebrew is.
type brewState int

const (
	brewOnPath  brewState = iota // prefix/bin on PATH
	brewOffPath                  // only at a known prefix (linuxbrew style)
	brewMissing                  // not installed; the fake installer creates it
)

// machine describes a fake machine.
type machine struct {
	goos    string // darwin or linux
	wsl     bool
	display bool
	glibc   string // linux; default 2.39
	brew    brewState
	uid     int // default testUID
}

// fixture is one fake machine: a temp HOME, a Homebrew prefix, fake
// programs on PATH, and the options that point facman at them (R-3).
type fixture struct {
	t      *testing.T
	m      machine
	fake   *testkit.Fake
	home   string
	prefix string
	apps   string
	o      options
	stdout bytes.Buffer
	stderr bytes.Buffer
	sigs   chan os.Signal
	cat    catalog.Catalog
	srv    *httptest.Server
}

func newFixture(t *testing.T, m machine, edit ...func(*fakeworld.World)) *fixture {
	t.Helper()
	if m.goos == "" {
		m.goos = "darwin"
	}
	if m.glibc == "" {
		m.glibc = "2.39"
	}
	if m.uid == 0 {
		m.uid = testUID
	}
	root := t.TempDir()
	f := &fixture{t: t, m: m, home: filepath.Join(root, "home"), prefix: filepath.Join(root, "brew"), apps: filepath.Join(root, "Applications"), sigs: make(chan os.Signal, 4)}
	for _, d := range []string{f.home, f.prefix, f.apps} {
		mustMkdir(t, d)
	}
	cat, err := catalogyaml.Embedded("dev").Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.cat = cat
	w := fakeworld.World{Brew: brewWorld(cat, f.platform(), f.prefix), Rules: []fakeworld.Rule{
		{Cmd: "xcode-select", Args: []string{"-p"}, Stdout: "/Library/Developer/CommandLineTools\n"},
		{Cmd: "mdfind"},
		{Cmd: "getconf", Args: []string{"GNU_LIBC_VERSION"}, Stdout: "glibc " + m.glibc + "\n"},
		{Cmd: "file", Stdout: "symbolic link\n"},
	}}
	for _, e := range edit {
		e(&w)
	}
	names := []string{"xcode-select", "mdfind", "plutil", "getconf", "ldd", "file"}
	if m.goos == "linux" {
		names = append(names, "gcc", "ps", "curl", "git") // Homebrew's Linux prerequisites
	}
	f.fake = testkit.NewFake(t, w, names...)
	if m.brew != brewMissing {
		f.linkBrew()
	}
	path := f.fake.Dir
	if m.brew == brewOnPath {
		path = filepath.Join(f.prefix, "bin") + string(os.PathListSeparator) + path
	}
	env := []string{"PATH=" + path, "HOME=" + f.home, "USER=" + testUser, fakeworld.DirEnv + "=" + f.fake.Dir}
	if m.display {
		env = append(env, "DISPLAY=:0")
	}
	procVersion := filepath.Join(root, "proc-version")
	pv := "Linux version 6.8.0 (buildd@ubuntu) #1 SMP"
	if m.wsl {
		pv = "Linux version 5.15.153.1-microsoft-standard-WSL2"
	}
	mustWrite(t, procVersion, pv+"\n")
	osRelease := filepath.Join(root, "os-release")
	mustWrite(t, osRelease, "ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\n")
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "#!/bin/bash\n/bin/mkdir -p %q && /bin/cp %q %q\n", filepath.Join(f.prefix, "bin"), testkit.FakecmdBinary(t), filepath.Join(f.prefix, "bin", "brew"))
	}))
	t.Cleanup(f.srv.Close)
	f.o = options{
		Stdin: strings.NewReader(""), Stdout: &f.stdout, Stderr: &f.stderr,
		Environ: env, GOOS: m.goos, GOARCH: "arm64", UID: func() int { return m.uid },
		OwnerUID:      func(string) (int, error) { return m.uid, nil },
		KnownPrefixes: []string{f.prefix}, AppDirs: []string{f.apps}, AppInstallDir: f.apps,
		ShimDir: filepath.Join(root, "usr-bin"), ProcVersionPath: procVersion, OSReleasePath: osRelease,
		InstallerURL: f.srv.URL, TempDir: t.TempDir(),
		Now: func() time.Time { return fixedNow }, User: func() string { return testUser },
		Signals:         func() (<-chan os.Signal, func()) { return f.sigs, func() {} },
		DetectorTimeout: 10 * time.Second,
	}
	t.Cleanup(func() { testkit.AssertSafeInvocations(t, f.fake.Log(t)) }) // I1-8, I1-9, I1-10
	return f
}

func (f *fixture) platform() platform.Platform {
	return platform.Platform{OS: f.m.goos, Arch: "arm64", WSL: f.m.wsl, Display: f.m.goos == "darwin" || f.m.display, UID: f.m.uid}
}

func (f *fixture) linkBrew() {
	f.t.Helper()
	mustMkdir(f.t, filepath.Join(f.prefix, "bin"))
	b, err := os.ReadFile(testkit.FakecmdBinary(f.t))
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.prefix, "bin", "brew"), b, 0o755); err != nil { //nolint:gosec // test executable
		f.t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, d string) {
	t.Helper()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

// brewWorld makes every brew item the catalog offers here installable by
// the fake brew: formulae link their commands from the Cellar, casks from
// the Caskroom (as binary artifacts), node@24 is keg-only.
func brewWorld(cat catalog.Catalog, p platform.Platform, prefix string) *fakeworld.Brew {
	b := &fakeworld.Brew{Prefix: prefix, Formulae: map[string]fakeworld.Formula{}, Casks: map[string]fakeworld.Cask{}}
	impl := catalog.BackendSet{catalog.BackendBrewFormula: true, catalog.BackendBrewCask: true}
	for _, e := range cat.Entries {
		var steps []catalog.InstallStep
		for _, ss := range e.Install {
			steps = append(steps, ss...)
		}
		if a := catalog.SelectInstall(e, p, impl); a.Step != nil {
			steps = append([]catalog.InstallStep{*a.Step}, steps...)
		}
		for _, s := range steps {
			ver := "1.0.0"
			if strings.HasPrefix(s.ID, "node") {
				ver = "24.11.0"
			}
			token := s.ID[strings.LastIndex(s.ID, "/")+1:]
			switch s.Backend {
			case catalog.BackendBrewFormula:
				if _, ok := b.Formulae[s.ID]; ok {
					continue
				}
				fm := fakeworld.Formula{Version: ver, Symlinks: map[string]string{}}
				keg := filepath.Join("Cellar", token, ver)
				for _, c := range e.Detect.Commands {
					fm.Files = append(fm.Files, filepath.Join(keg, "bin", c))
					if s.ID == "node@24" {
						continue
					}
					fm.Symlinks[filepath.Join("bin", c)] = filepath.Join(keg, "bin", c)
				}
				if s.ID == "node@24" {
					fm.KegOnly = true
					fm.Symlinks[filepath.Join("opt", "node@24")] = keg
				}
				b.Formulae[s.ID] = fm
			case catalog.BackendBrewCask:
				if _, ok := b.Casks[s.ID]; ok {
					continue
				}
				ck := fakeworld.Cask{Version: ver, Symlinks: map[string]string{}}
				room := filepath.Join("Caskroom", token, ver)
				for _, c := range e.Detect.Commands {
					ck.Artifacts = append(ck.Artifacts, map[string]any{"binary": []any{filepath.Join(prefix, room, c)}})
					ck.Files = append(ck.Files, filepath.Join(room, c))
					ck.Symlinks[filepath.Join("bin", c)] = filepath.Join(room, c)
				}
				b.Casks[s.ID] = ck
			}
		}
	}
	return b
}

// run runs facman in-process with stdin that is not a terminal.
func (f *fixture) run(args ...string) int {
	f.t.Helper()
	f.stdout.Reset()
	f.stderr.Reset()
	return run(args, f.o)
}

// runTTY runs facman with a pseudo-terminal as stdin, typing answer.
func (f *fixture) runTTY(answer string, args ...string) int {
	f.t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		f.t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = ptmx.Close(); _ = tty.Close() }()
	go func() { _, _ = io.Copy(io.Discard, ptmx) }() // drain echo
	if answer != "" {
		if _, err := ptmx.WriteString(answer); err != nil {
			f.t.Fatal(err)
		}
	}
	o := f.o
	o.Stdin = tty
	f.stdout.Reset()
	f.stderr.Reset()
	return run(args, o)
}

// stateDir is where facman keeps its state on the fixture.
func (f *fixture) stateDir() string {
	if f.m.goos == "darwin" {
		return filepath.Join(f.home, "Library", "Application Support", "facman")
	}
	return filepath.Join(f.home, ".local", "state", "facman")
}

// store opens the state DB for inspection.
func (f *fixture) store() *sqlite.Store {
	f.t.Helper()
	s, err := sqlite.Open(context.Background(), filepath.Join(f.stateDir(), "state.db"), nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = s.Close() })
	return s
}

func (f *fixture) operations() []inventory.Operation {
	f.t.Helper()
	if _, err := os.Stat(filepath.Join(f.stateDir(), "state.db")); err != nil {
		return nil
	}
	ops, err := f.store().Operations(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return ops
}

func (f *fixture) records() map[string]inventory.Record {
	f.t.Helper()
	recs, err := f.store().Records(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]inventory.Record{}
	for _, r := range recs {
		out[r.ID] = r
	}
	return out
}

// mutating returns the recorded brew invocations that change the machine.
func (f *fixture) mutating() [][]string {
	var out [][]string
	for _, inv := range f.fake.Log(f.t) {
		if filepath.Base(inv.Argv[0]) == "brew" && len(inv.Argv) > 1 && slices.Contains([]string{"install", "tap", "trust"}, inv.Argv[1]) && len(inv.Argv) > 2 {
			out = append(out, inv.Argv)
		}
	}
	return out
}

var (
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
)

// doc validates stdout against report schema v1 and decodes it.
func (f *fixture) doc() map[string]any {
	f.t.Helper()
	schemaOnce.Do(func() {
		var raw any
		raw, schemaErr = jsonschema.UnmarshalJSON(bytes.NewReader(report.Schema))
		if schemaErr != nil {
			return
		}
		c := jsonschema.NewCompiler()
		if schemaErr = c.AddResource("report.v1.json", raw); schemaErr == nil {
			schema, schemaErr = c.Compile("report.v1.json")
		}
	})
	if schemaErr != nil {
		f.t.Fatal(schemaErr)
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(f.stdout.Bytes()))
	if err != nil {
		f.t.Fatalf("stdout is not JSON: %v\n%s", err, f.stdout.String())
	}
	if err := schema.Validate(v); err != nil {
		f.t.Fatalf("report does not validate: %v\n%s", err, f.stdout.String())
	}
	var m map[string]any
	if err := json.Unmarshal(f.stdout.Bytes(), &m); err != nil {
		f.t.Fatal(err)
	}
	return m
}

// pkg returns the package with id from a report.
func pkg(t *testing.T, doc map[string]any, id string) map[string]any {
	t.Helper()
	list, _ := doc["packages"].([]any)
	for _, p := range list {
		if m := p.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("package %s not in report", id)
	return nil
}

// actions returns the plan actions from a report.
func actions(doc map[string]any) []map[string]any {
	p, _ := doc["plan"].(map[string]any)
	list, _ := p["actions"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, a := range list {
		out = append(out, a.(map[string]any))
	}
	return out
}

func argv(a map[string]any, i int) string {
	cmds := a["commands"].([]any)
	parts := cmds[i].(map[string]any)["argv"].([]any)
	s := make([]string, len(parts))
	for j, p := range parts {
		s[j] = p.(string)
	}
	return strings.Join(s, " ")
}

func (f *fixture) want(code, got int) {
	f.t.Helper()
	if got != code {
		f.t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", got, code, f.stdout.String(), f.stderr.String())
	}
}
