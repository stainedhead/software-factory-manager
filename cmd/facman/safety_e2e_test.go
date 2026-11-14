package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

// writeCatalog writes a one-package catalog dir with the given package
// block and an "evil" profile that wants it, and returns the dir.
func writeCatalog(t *testing.T, id, block string) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "catalog.yaml"), "apiVersion: facman/v1\npackages:\n  "+id+":\n"+block)
	mustMkdir(t, filepath.Join(dir, "profiles"))
	mustWrite(t, filepath.Join(dir, "profiles", "evil.yaml"), "apiVersion: facman/v1\nprofile: evil\npackages:\n  - "+id+"\n")
	return dir
}

// assertOnlyLocateProbes fails if any brew command ran other than the
// catalog-independent locate probes (brew --version, brew shellenv), which
// the bootstrap check runs before the catalog loads (exit 4 beats exit 3).
func assertOnlyLocateProbes(t *testing.T, f *fixture) {
	t.Helper()
	for _, inv := range f.fake.Log(t) {
		if filepath.Base(inv.Argv[0]) != "brew" {
			continue
		}
		if len(inv.Argv) > 1 && (inv.Argv[1] == "--version" || inv.Argv[1] == "shellenv") {
			continue
		}
		t.Errorf("brew ran: %v", inv.Argv)
	}
}

// Review FR-001a: a --catalog value that would reach brew argv as an option
// fails at load time with exit 3, and no brew command runs.
func TestCatalogValuesRejectedAtLoad(t *testing.T) {
	tests := []struct {
		name, id, block, msg string
	}{
		{"brew id short force", "evil", `    kind: cli
    detect: { commands: [evil] }
    install:
      darwin: [{ backend: brew-formula, id: "-f" }]
      linux: [{ backend: brew-formula, id: "-f" }]
`, `brew id "-f" is not allowed`},
		{"tap option", "evil", `    kind: gui
    install:
      darwin: [{ backend: brew-cask, id: "a/b/c", tap: "--force-auto-update", trust: item }]
      linux: [{ backend: brew-cask, id: "a/b/c", tap: "--force-auto-update", trust: item }]
`, `tap "--force-auto-update" is not allowed`},
		{"detect command traversal", "evil", `    kind: cli
    detect: { commands: ["../../bin/sh"] }
`, `detect command "../../bin/sh" is not allowed`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, machine{})
			dir := writeCatalog(t, tt.id, tt.block)
			f.want(3, f.run("plan", "--profile", "evil", "--catalog", dir))
			if !strings.Contains(f.stderr.String(), tt.msg) {
				t.Errorf("stderr = %q, want %q", f.stderr.String(), tt.msg)
			}
			if strings.Contains(f.stdout.String(), "brew install") {
				t.Errorf("a plan was printed: %q", f.stdout.String())
			}
			assertOnlyLocateProbes(t, f)
		})
	}
}

// Review FR-002: with "." first in PATH, a node (or brew) in the current
// directory is never run, and scan warns about the relative entry.
func TestRelativePathEntryNeverRuns(t *testing.T) {
	f := newFixture(t, machine{})
	// Planted programs in the current directory leave a marker when run.
	cwd, marker := t.TempDir(), filepath.Join(t.TempDir(), "ran")
	for _, name := range []string{"node", "brew", "git", "jq"} {
		mustWrite(t, filepath.Join(cwd, name), "#!/bin/sh\necho \"$0 $*\" >> "+marker+"\necho v99.0.0\n")
		if err := os.Chmod(filepath.Join(cwd, name), 0o755); err != nil { //nolint:gosec // test executable
			t.Fatal(err)
		}
	}
	t.Chdir(cwd)
	for i, kv := range f.o.Environ {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			f.o.Environ[i] = "PATH=." + string(os.PathListSeparator) + v
		}
	}
	f.want(0, f.run("scan", "--json"))
	if b, err := os.ReadFile(marker); err == nil {
		t.Errorf("programs ran from the current directory:\n%s", b)
	}
	doc := f.doc()
	if p, _ := doc["bootstrap"].(map[string]any)["homebrew"].(map[string]any)["path"].(string); !filepath.IsAbs(p) {
		t.Errorf("located Homebrew at %q, want an absolute path outside the current directory", p)
	}
	for _, x := range doc["packages"].([]any) {
		dets, _ := x.(map[string]any)["detections"].([]any)
		for _, d := range dets {
			if p, ok := d.(map[string]any)["path"].(string); ok && !filepath.IsAbs(p) {
				t.Errorf("%v detected at relative path %q", x.(map[string]any)["id"], p)
			}
		}
	}
	found := false
	for _, x := range doc["findings"].([]any) {
		fd := x.(map[string]any)
		if fd["code"] == "path-relative-entry" && fd["severity"] == "warning" {
			found = true
		}
	}
	if !found {
		t.Errorf("no path-relative-entry warning in %s", f.stdout.String())
	}
}

// Review FR-016: Paperclip's unavailable reason is printed once, in plan
// and scan text, on Linux and macOS.
func TestPaperclipReasonOnce(t *testing.T) {
	const want = "paperclip: unavailable-here; needs the npm backend (Increment 2)\n"
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			f := newFixture(t, machine{goos: goos})
			f.want(0, f.run("plan", "--profile", "agentic-factory"))
			out := f.stdout.String()
			if !strings.Contains(out, "  "+want) || strings.Count(out, "Increment 2") != 1 {
				t.Errorf("plan text:\n%s\nwant the line %q once", out, want)
			}
		})
	}
}

// Review FR-007: a brew install killed by SIGTERM is recorded with exit
// 143 and its signal, and its log ends with the outcome footer.
func TestSignalKilledInstallIsRecorded(t *testing.T) {
	f := newFixture(t, machine{}, func(w *fakeworld.World) {
		w.Rules = append([]fakeworld.Rule{{Cmd: "brew", Args: []string{"install", "jq"}, Stdout: "==> Downloading jq\n", KillSelf: int(syscall.SIGTERM)}}, w.Rules...)
	})
	f.want(1, f.runTTY("y\n", "apply", "--profile", "e2e", "--catalog", "../../testdata/catalogs/e2e"))
	ops := f.operations()
	if len(ops) != 1 || ops[0].ExitCode == nil || *ops[0].ExitCode != 128+int(syscall.SIGTERM) {
		t.Fatalf("operations = %+v", ops)
	}
	if !strings.Contains(f.stdout.String()+f.stderr.String(), "killed by signal SIGTERM") {
		t.Errorf("output does not name the signal:\n%s\n%s", f.stdout.String(), f.stderr.String())
	}
	b, err := os.ReadFile(ops[0].LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "# result: killed by signal SIGTERM (exit 143)\n# time: started ") {
		t.Errorf("log:\n%s", b)
	}
}
