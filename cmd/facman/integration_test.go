//go:build integration

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/adapter/brew"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
)

// mutateEnv opts in to the one integration test that changes the machine
// (installs and then removes GNU hello). CI sets it on macOS only (I1-14).
const mutateEnv = "FACMAN_INTEGRATION_MUTATE"

// realBrew returns the path of the machine's Homebrew, or skips the test
// when there is none or when running as root (Homebrew refuses root, and so
// does facman).
func realBrew(t *testing.T) string {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("running as root: Homebrew and facman refuse UID 0")
	}
	if p, err := exec.LookPath("brew"); err == nil {
		return p
	}
	for _, prefix := range brew.DefaultPrefixes(runtime.GOOS) {
		p := filepath.Join(prefix, "bin", "brew")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	t.Skip("Homebrew not found on PATH or at the known prefixes")
	return ""
}

// realFixture runs facman in-process against the real machine, with HOME
// (and so the state directory) in a temp dir. Nothing else is faked.
func realFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, m: machine{goos: runtime.GOOS}, home: t.TempDir()}
	env := []string{"HOME=" + f.home}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != "HOME" && k != "XDG_STATE_HOME" {
			env = append(env, kv)
		}
	}
	o := defaultOptions()
	o.Stdin, o.Stdout, o.Stderr = strings.NewReader(""), &f.stdout, &f.stderr
	o.Environ = env
	f.o = o
	return f
}

// I1-14, read-only half: scan, plan and doctor work against the real
// Homebrew and write state only into the temp state directory.
func TestIntegrationReadOnly(t *testing.T) {
	realBrew(t)
	f := realFixture(t)

	f.want(0, f.run("scan", "--json"))
	doc := f.doc()
	if bs, _ := doc["bootstrap"].(map[string]any); bs == nil {
		t.Fatalf("scan report has no bootstrap section:\n%s", f.stdout.String())
	} else if hb, _ := bs["homebrew"].(map[string]any); hb == nil || hb["found"] != true {
		t.Errorf("scan did not locate Homebrew: %v", bs["homebrew"])
	}
	if n := len(doc["packages"].([]any)); n == 0 {
		t.Error("scan reported no packages")
	}

	f.want(0, f.run("plan", "--json", "--profile", "agentic-factory"))
	_ = f.doc()

	switch code := f.run("doctor", "--json"); code {
	case 0, 2:
		_ = f.doc()
	default:
		t.Fatalf("doctor exit = %d, want 0 or 2\nstdout:\n%s\nstderr:\n%s", code, f.stdout.String(), f.stderr.String())
	}

	if _, err := os.Stat(filepath.Join(f.stateDir(), "state.db")); err != nil {
		t.Errorf("no state DB in the temp state directory: %v", err)
	}
}

// I1-14, mutating half: apply installs GNU hello through a one-entry
// --catalog and records it managed. Runs only with FACMAN_INTEGRATION_MUTATE=1
// on macOS, when hello is not installed yet and the prefix is ours.
func TestIntegrationApplyHello(t *testing.T) {
	if os.Getenv(mutateEnv) != "1" {
		t.Skipf("%s is not 1: the install test is opt-in", mutateEnv)
	}
	if runtime.GOOS != "darwin" {
		t.Skip("the install test runs on macOS only")
	}
	brewPath := realBrew(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, brewPath, "--prefix").Output()
	if err != nil {
		t.Skipf("brew --prefix failed: %v", err)
	}
	prefix := strings.TrimSpace(string(out))
	if fi, err := os.Stat(prefix); err != nil {
		t.Skipf("Homebrew prefix %s: %v", prefix, err)
	} else if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
		t.Skipf("Homebrew prefix %s is not owned by this user: unsafe to change", prefix)
	}
	if _, err := os.Stat(filepath.Join(prefix, "Cellar", "hello")); err == nil {
		t.Skip("hello is already installed; the test only installs what it can remove")
	}

	f := realFixture(t)
	dir, err := filepath.Abs("../../testdata/catalogs/integration")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd := exec.CommandContext(context.Background(), brewPath, "uninstall", "hello") //nolint:gosec // fixed argv
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Logf("cleanup: brew uninstall hello: %v\n%s", err, b)
		}
	})

	f.want(0, f.runTTY("y\n", "apply", "--profile", "integration", "--catalog", dir))
	if !strings.Contains(f.stderr.String(), "brew install hello") {
		t.Errorf("the prompt did not list the install:\n%s", f.stderr.String())
	}
	rec, ok := f.records()["hello"]
	if !ok {
		t.Fatalf("no packages row for hello; records = %v", f.records())
	}
	if rec.State != inventory.StateManaged || rec.Backend != catalog.BackendBrewFormula {
		t.Errorf("hello record = %+v, want managed through brew-formula", rec)
	}
	if _, err := os.Stat(filepath.Join(prefix, "Cellar", "hello")); err != nil {
		t.Errorf("hello is not in the Cellar after apply: %v", err)
	}

	// A second run proposes nothing (FR-076).
	f.want(0, f.run("plan", "--json", "--profile", "integration", "--catalog", dir))
	if n := len(actions(f.doc())); n != 0 {
		t.Errorf("second plan has %d actions, want none", n)
	}
}
