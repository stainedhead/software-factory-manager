package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

// helperEnv names the config file of the crash-test helper mode of this
// test binary. The shipped binary has no such mode (D-11, AD-15).
const helperEnv = "FACMAN_E2E_CRASH_HELPER"

// helperConfig is the part of options the child process needs.
type helperConfig struct {
	Args            []string
	Environ         []string
	GOOS            string
	UID             int
	KnownPrefixes   []string
	AppDirs         []string
	AppInstallDir   string
	ShimDir         string
	ProcVersionPath string
	OSReleasePath   string
}

// crashHelper runs facman in this child process with the config named by
// helperEnv, on the inherited terminal.
func crashHelper() int {
	b, err := os.ReadFile(os.Getenv(helperEnv))
	if err != nil {
		return 125
	}
	var c helperConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return 125
	}
	o := defaultOptions()
	o.Environ, o.GOOS, o.KnownPrefixes, o.AppDirs, o.AppInstallDir = c.Environ, c.GOOS, c.KnownPrefixes, c.AppDirs, c.AppInstallDir
	o.ShimDir, o.ProcVersionPath, o.OSReleasePath = c.ShimDir, c.ProcVersionPath, c.OSReleasePath
	o.UID = func() int { return c.UID }
	o.OwnerUID = func(string) (int, error) { return c.UID, nil }
	o.Now = func() time.Time { return fixedNow }
	o.User = func() string { return testUser }
	return run(c.Args, o)
}

// I1-11: a run killed after an in_progress row is reconciled on the next
// launch (row interrupted, package re-detected) and the next apply
// finishes the work.
func TestCrashRecovery(t *testing.T) {
	f := newFixture(t, machine{}, func(w *fakeworld.World) {
		w.Rules = append(w.Rules, fakeworld.Rule{Cmd: "brew", Args: []string{"install", "jq"}, SleepMS: 1500})
	})
	dir, _ := filepath.Abs("../../testdata/catalogs/e2e")
	args := []string{"apply", "--profile", "e2e", "--catalog", dir}
	cfg, _ := json.Marshal(helperConfig{Args: args, Environ: f.o.Environ, GOOS: f.o.GOOS, UID: testUID, KnownPrefixes: f.o.KnownPrefixes,
		AppDirs: f.o.AppDirs, AppInstallDir: f.o.AppInstallDir, ShimDir: f.o.ShimDir, ProcVersionPath: f.o.ProcVersionPath, OSReleasePath: f.o.OSReleasePath})
	cfgPath := filepath.Join(t.TempDir(), "helper.json")
	mustWrite(t, cfgPath, string(cfg))

	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = ptmx.Close(); _ = tty.Close() }()
	go func() { _, _ = io.Copy(io.Discard, ptmx) }()
	child := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^$") //nolint:gosec // the test binary itself
	child.Env = append(os.Environ(), helperEnv+"="+cfgPath)
	child.Stdin, child.Stdout, child.Stderr = tty, io.Discard, io.Discard
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := ptmx.WriteString("y\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		// the log is opened right after the in_progress row is written, so
		// the DB is not opened while the child may still be migrating it.
		logs, _ := os.ReadDir(filepath.Join(f.stateDir(), "logs"))
		if len(logs) > 0 {
			if ops := f.operations(); len(ops) == 1 && ops[0].Status == inventory.StatusInProgress {
				break
			}
		}
		if time.Now().After(deadline) {
			_ = child.Process.Kill()
			t.Fatal("no in_progress row appeared")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()

	// the next launch reconciles; jq was never installed, so it stays absent.
	w := f.fake.World(t)
	w.Rules = w.Rules[:len(w.Rules)-1]
	f.fake.SetWorld(t, w)
	f.want(0, f.run("scan", "--catalog", dir))
	ops := f.operations()
	if len(ops) != 1 || ops[0].Status != inventory.StatusInterrupted {
		t.Fatalf("after relaunch: %+v", ops)
	}
	f.want(0, f.runTTY("y\n", args...))
	if f.records()["jq"].State != inventory.StateManaged || !strings.Contains(f.stdout.String(), "jq: installed") {
		t.Errorf("the next apply did not finish the work:\n%s\n%s", f.stdout.String(), f.stderr.String())
	}
	time.Sleep(1500 * time.Millisecond) // let the orphaned fake brew of the killed run exit before cleanup
}
