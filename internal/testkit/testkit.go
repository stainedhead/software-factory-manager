// Package testkit is test-only support: it builds testdata/fakecmd, links
// it into a temporary bin directory under fake program names, seeds the
// fake world, reads the invocation log, and asserts that recorded calls are
// safe. Only _test.go files import it.
package testkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

// DefaultNames are the programs a Fake provides.
var DefaultNames = []string{"brew", "xcode-select", "mdfind", "plutil", "npm", "getconf", "ldd", "file"}

var (
	buildOnce sync.Once
	buildPath string
	buildErr  error
)

// FakecmdBinary builds testdata/fakecmd once per test process and returns
// the binary path. The binary lives in a process-wide temp directory that
// RemoveBuild deletes (call it from TestMain if you want it gone).
func FakecmdBinary(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "facman-fakecmd-")
		if err != nil {
			buildErr = fmt.Errorf("creating build dir: %w", err)
			return
		}
		buildPath = filepath.Join(dir, "fakecmd")
		cmd := exec.Command("go", "build", "-o", buildPath, "./testdata/fakecmd")
		cmd.Dir = moduleRoot()
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("building fakecmd: %w: %s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildPath
}

// RemoveBuild deletes the fakecmd build directory.
func RemoveBuild() {
	if buildPath != "" {
		_ = os.RemoveAll(filepath.Dir(buildPath))
	}
}

func moduleRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// Fake is a temporary bin directory of fakecmd links plus its world.
type Fake struct {
	// Dir holds the links, the world file and the log; put it on PATH.
	Dir string
}

// NewFake creates a bin directory with fakecmd linked as each name (as
// DefaultNames when none are given) and writes w as the world.
func NewFake(t testing.TB, w fakeworld.World, names ...string) *Fake {
	t.Helper()
	bin := FakecmdBinary(t)
	if len(names) == 0 {
		names = DefaultNames
	}
	dir := t.TempDir()
	for _, n := range names {
		if err := linkOrCopy(bin, filepath.Join(dir, n)); err != nil {
			t.Fatalf("linking %s: %v", n, err)
		}
	}
	f := &Fake{Dir: dir}
	f.SetWorld(t, w)
	return f
}

func linkOrCopy(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil { //nolint:gosec // test executable
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	return nil
}

// Path returns the absolute path of a fake program.
func (f *Fake) Path(name string) string { return filepath.Join(f.Dir, name) }

// SetWorld replaces the world.
func (f *Fake) SetWorld(t testing.TB, w fakeworld.World) {
	t.Helper()
	if err := fakeworld.Save(filepath.Join(f.Dir, fakeworld.WorldFile), w); err != nil {
		t.Fatal(err)
	}
}

// World returns the current world (after any emulated installs).
func (f *Fake) World(t testing.TB) fakeworld.World {
	t.Helper()
	w, err := fakeworld.Load(filepath.Join(f.Dir, fakeworld.WorldFile))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Log returns every recorded invocation in call order.
func (f *Fake) Log(t testing.TB) []fakeworld.Invocation {
	t.Helper()
	log, err := fakeworld.ReadLog(filepath.Join(f.Dir, fakeworld.LogFile))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// forbiddenBrew are brew subcommands facman must never run in Increment 1.
var forbiddenBrew = []string{"upgrade", "uninstall", "remove", "rm", "reinstall", "untap", "untrust", "link", "unlink", "cleanup", "autoremove"}

// UnsafeReasons returns why an invocation is unsafe, or nil. It flags
// --force/--overwrite in any argv, brew upgrade/uninstall/untap/untrust/link
// and similar, whole-tap trust, and HOMEBREW_NO_REQUIRE_TAP_TRUST* in env.
func UnsafeReasons(inv fakeworld.Invocation) []string {
	var reasons []string
	for _, a := range inv.Argv {
		if strings.HasPrefix(a, "--force") || strings.HasPrefix(a, "--overwrite") {
			reasons = append(reasons, "forbidden flag "+a)
		}
	}
	for _, n := range inv.EnvNames {
		if strings.HasPrefix(n, "HOMEBREW_NO_REQUIRE_TAP_TRUST") {
			reasons = append(reasons, "environment contains "+n)
		}
	}
	if len(inv.Argv) >= 2 && filepath.Base(inv.Argv[0]) == "brew" {
		sub := inv.Argv[1]
		if slices.Contains(forbiddenBrew, sub) {
			reasons = append(reasons, "forbidden brew subcommand "+sub)
		}
		if sub == "trust" {
			for _, a := range inv.Argv[2:] {
				if !strings.HasPrefix(a, "-") && strings.Count(a, "/") != 2 {
					reasons = append(reasons, "whole-tap trust of "+a)
				}
			}
		}
	}
	return reasons
}

// AssertSafeInvocations fails t for every unsafe invocation in log (I1-8,
// I1-9, I1-10).
func AssertSafeInvocations(t testing.TB, log []fakeworld.Invocation) {
	t.Helper()
	for _, inv := range log {
		for _, r := range UnsafeReasons(inv) {
			t.Errorf("unsafe invocation %q: %s", strings.Join(inv.Argv, " "), r)
		}
	}
}

// Env returns an environment for running programs against the fake: PATH is
// only the fake dir (plus extra dirs), and HOME is home.
func (f *Fake) Env(home string, extraPath ...string) []string {
	path := strings.Join(append([]string{f.Dir}, extraPath...), string(os.PathListSeparator))
	return []string{"PATH=" + path, "HOME=" + home}
}
