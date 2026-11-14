package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
