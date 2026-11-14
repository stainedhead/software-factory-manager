package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

// I1-2: scan --json reports every catalog package with state, provenance,
// path and version; it validates, runs only read-only commands and is fast.
func TestScanJSON(t *testing.T) {
	f := newFixture(t, machine{}, func(w *fakeworld.World) {
		w.Brew.Installed.Formulae = map[string]string{"jq": "1.7.1"} // the developer's own brew install
	})
	mustWrite(t, filepath.Join(f.prefix, "Cellar", "jq", "1.7.1", "bin", "jq"), "#!/bin/sh\necho jq-1.7.1\n")
	if err := os.Chmod(filepath.Join(f.prefix, "Cellar", "jq", "1.7.1", "bin", "jq"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.prefix, "Cellar", "jq", "1.7.1", "bin", "jq"), filepath.Join(f.prefix, "bin", "jq")); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	f.want(0, f.run("scan", "--json"))
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("scan took %s, want < 5s", d)
	}
	doc := f.doc()
	if got, want := len(doc["packages"].([]any)), len(f.cat.Entries); got != want {
		t.Errorf("packages = %d, want every catalog entry (%d)", got, want)
	}
	jq := pkg(t, doc, "jq")
	if jq["state"] != "foreign" || jq["provenance"] != "brew" || jq["path"] != filepath.Join(f.prefix, "bin", "jq") || jq["version"] == "" {
		t.Errorf("jq = %v", jq)
	}
	if pkg(t, doc, "gh")["state"] != "absent" {
		t.Errorf("gh = %v", pkg(t, doc, "gh"))
	}
	if m := f.mutating(); len(m) != 0 {
		t.Errorf("scan ran mutating commands: %v", m)
	}
	// text output names the state and provenance too.
	f.want(0, f.run("scan"))
	if !strings.Contains(f.stdout.String(), "jq") || !strings.Contains(f.stdout.String(), "foreign") {
		t.Errorf("text scan:\n%s", f.stdout.String())
	}
}

// I1-12 and FR-075: usage errors before any I/O, then 4 > 3 > 1.
func TestExitCodesAndUsage(t *testing.T) {
	catDir, _ := filepath.Abs("../../testdata/catalogs/e2e")
	tests := []struct {
		name string
		m    machine
		args []string
		code int
		msg  string
	}{
		{"plan without --profile", machine{}, []string{"plan"}, 64, "needs --profile"},
		{"apply without --profile", machine{}, []string{"apply"}, 64, "needs --profile"},
		{"--profile with no value", machine{}, []string{"plan", "--profile"}, 64, "needs an argument"},
		{"--profile blank", machine{}, []string{"plan", "--profile", "  "}, 64, "needs a value"},
		{"apply with non-terminal stdin", machine{}, []string{"apply", "--profile", "agentic-factory"}, 64, "stdin is not a terminal"},
		{"deferred command", machine{}, []string{"upgrade", "jq"}, 64, "later increment"},
		{"unknown command", machine{}, []string{"frobnicate"}, 64, "unknown command"},
		{"unknown flag", machine{}, []string{"scan", "--nope"}, 64, "unknown flag"},
		{"--set", machine{}, []string{"plan", "--profile", "agentic-factory", "--set", "a=b"}, 64, "--set"},
		{"-cl", machine{}, []string{"apply", "-cl", "list.yaml"}, 64, "configuration lists"},
		{"git catalog", machine{}, []string{"plan", "--profile", "x", "--catalog", "https://example.com/c.git"}, 64, "git catalogs"},
		{"usage beats bootstrap", machine{uid: -1}, []string{"plan", "--set", "a=b"}, 64, "--set"},
		{"UID 0", machine{uid: -1}, []string{"scan"}, 4, "root"},
		{"bootstrap beats validation", machine{uid: -1}, []string{"plan", "--profile", "nope"}, 4, "root"},
		{"unknown profile", machine{}, []string{"plan", "--profile", "nope"}, 3, "nope"},
		{"unknown --with", machine{}, []string{"plan", "--profile", "agentic-factory", "--with", "nope"}, 3, "nope"},
		{"choice group without a member", machine{}, []string{"plan", "--profile", "agentic-factory", "--with", "container-runtime"}, 3, "colima"},
		{"missing catalog dir", machine{}, []string{"plan", "--profile", "e2e", "--catalog", "/does/not/exist"}, 3, "/does/not/exist"},
		{"conflicting pins", machine{}, []string{"plan", "--profile", "e2e", "--profile", "other", "--catalog", catDir}, 3, "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.m.uid == -1
			tt.m.uid = 0
			f := newFixture(t, tt.m)
			if root {
				f.o.UID = func() int { return 0 }
			}
			f.want(tt.code, f.run(tt.args...))
			if !strings.Contains(f.stderr.String(), tt.msg) {
				t.Errorf("stderr = %q, want it to mention %q", f.stderr.String(), tt.msg)
			}
			if tt.code == 64 && len(f.fake.Log(t)) != 0 {
				t.Errorf("a usage error ran commands: %v", f.fake.Log(t))
			}
		})
	}
}

// I1-4: --catalog replaces the embedded catalog; profiles repeat and union.
func TestCatalogDirAndProfileUnion(t *testing.T) {
	f := newFixture(t, machine{})
	dir, _ := filepath.Abs("../../testdata/catalogs/e2e")
	f.want(0, f.run("plan", "--json", "--profile", "e2e", "--profile", "tools", "--profile", "e2e", "--catalog", dir))
	doc := f.doc()
	if n := len(doc["packages"].([]any)); n != 2 {
		t.Errorf("packages = %d, want the 2 of the --catalog dir", n)
	}
	var got []string
	for _, a := range actions(doc) {
		got = append(got, argv(a, 0))
	}
	if !slices.Equal(got, []string{"brew install jq", "brew install gh"}) {
		t.Errorf("commands = %q", got)
	}
	if !strings.HasPrefix(doc["catalog"].(map[string]any)["revision"].(string), "dir:"+dir+"@") {
		t.Errorf("catalog = %v", doc["catalog"])
	}
	// the embedded catalog: agentic-factory includes base-dev, optional items off.
	f.want(0, f.run("plan", "--json", "--profile", "agentic-factory"))
	doc = f.doc()
	for _, id := range []string{"git", "gh", "tmux", "visual-studio-code", "iterm2", "sqlite", "node", "awscli", "claude-code", "codex", "uv", "jq", "ripgrep"} {
		if p := pkg(t, doc, id); p["selected"] != true || p["required"] != true {
			t.Errorf("%s = %v, want selected and required", id, p)
		}
	}
	for _, id := range []string{"gcloud-cli", "azure-cli", "session-manager-plugin", "t3-code"} {
		if p := pkg(t, doc, id); p["selected"] == true {
			t.Errorf("optional %s is selected without --with", id)
		}
	}
	f.want(0, f.run("plan", "--json", "--profile", "agentic-factory", "--with", "gcloud-cli"))
	if p := pkg(t, f.doc(), "gcloud-cli"); p["selected"] != true {
		t.Errorf("--with gcloud-cli: %v", p)
	}
}

// I1-6: plan prints the exact ordered commands, dependencies first;
// apply --dry-run prints the same and changes nothing.
func TestPlanAndDryRun(t *testing.T) {
	f := newFixture(t, machine{})
	f.want(0, f.run("plan", "--json", "--profile", "agentic-factory", "--with", "session-manager-plugin"))
	var order []string
	for _, a := range actions(f.doc()) {
		order = append(order, a["package_id"].(string))
	}
	aws, smp := slices.Index(order, "awscli"), slices.Index(order, "session-manager-plugin")
	if aws < 0 || smp < 0 || aws > smp {
		t.Errorf("order = %v, want awscli before session-manager-plugin", order)
	}
	f.want(0, f.run("plan", "--profile", "agentic-factory", "--with", "session-manager-plugin"))
	planText := f.stdout.String()
	f.want(0, f.run("apply", "--dry-run", "--profile", "agentic-factory", "--with", "session-manager-plugin"))
	if !strings.Contains(f.stdout.String(), "Dry run: nothing was changed") {
		t.Errorf("dry run output:\n%s", f.stdout.String())
	}
	for _, line := range strings.Split(planText, "\n") {
		if strings.Contains(line, "brew ") && !strings.Contains(f.stdout.String(), line) {
			t.Errorf("dry run is missing plan line %q", line)
		}
	}
	if strings.Contains(f.stderr.String(), "Proceed?") {
		t.Error("--dry-run prompted")
	}
	f.want(0, f.run("apply", "--dry-run", "--json", "--profile", "agentic-factory"))
	if doc := f.doc(); doc["consent_by"] != nil || doc["results"] != nil {
		t.Errorf("dry run report = consent %v results %v", doc["consent_by"], doc["results"])
	}
	if ops := f.operations(); len(ops) != 0 {
		t.Errorf("dry run wrote operations: %v", ops)
	}
	if recs := f.records(); len(recs) != 0 {
		t.Errorf("dry run wrote records: %v", recs)
	}
	if m := f.mutating(); len(m) != 0 {
		t.Errorf("dry run ran mutating commands: %v", m)
	}
}

// I1-7: after consent apply runs exactly the listed commands in order,
// records every operation with consent, and a second apply does nothing.
// Also I1-10 (tap and trust as separate steps, grant recorded) and FR-027.
func TestApplyAuditAndIdempotence(t *testing.T) {
	f := newFixture(t, machine{})
	f.want(0, f.run("plan", "--json", "--profile", "agentic-factory"))
	var planned [][]string
	acts := actions(f.doc())
	for _, a := range acts {
		for i := range a["commands"].([]any) {
			planned = append(planned, strings.Fields(argv(a, i)))
		}
	}
	tap, trust, install := -1, -1, -1
	for i, a := range acts {
		switch argv(a, 0) {
		case "brew tap stablyai/orca":
			tap = i
		case "brew trust --cask stablyai/orca/orca":
			trust = i
		case "brew install --cask stablyai/orca/orca":
			install = i
		}
	}
	if tap < 0 || trust != tap+1 || install != trust+1 {
		t.Errorf("orca steps at %d, %d, %d; want tap, trust, install in a row", tap, trust, install)
	}

	f.want(0, f.runTTY("y\n", "apply", "--profile", "agentic-factory"))
	if !strings.Contains(f.stderr.String(), "brew install jq") || !strings.Contains(f.stderr.String(), "Proceed? [y/N]") {
		t.Errorf("prompt (stderr):\n%s", f.stderr.String())
	}
	if i, j := strings.Index(f.stderr.String(), "brew install jq"), strings.Index(f.stderr.String(), "Proceed?"); i > j {
		t.Error("the prompt came before the command list")
	}
	var ran [][]string
	for _, m := range f.mutating() {
		ran = append(ran, append([]string{"brew"}, m[1:]...))
	}
	if !slices.EqualFunc(ran, planned, slices.Equal[[]string]) {
		t.Errorf("ran:\n%q\nplanned:\n%q", ran, planned)
	}
	ops := f.operations()
	if len(ops) != len(planned) {
		t.Fatalf("operations = %d, want %d", len(ops), len(planned))
	}
	for _, op := range ops {
		if op.ConsentBy != consent || op.Status != inventory.StatusSucceeded || op.ExitCode == nil || *op.ExitCode != 0 || op.LogPath == "" {
			t.Errorf("operation = %+v", op)
		}
		if _, err := os.Stat(op.LogPath); err != nil {
			t.Errorf("log %s: %v", op.LogPath, err)
		}
	}
	recs := f.records()
	for _, id := range []string{"jq", "gh", "claude-code", "orca", "node"} {
		if recs[id].State != inventory.StateManaged {
			t.Errorf("%s record = %+v, want managed", id, recs[id])
		}
	}
	grants, err := f.store().TapTrustGrants(t.Context())
	if err != nil || len(grants) != 1 || grants[0].Item != "stablyai/orca/orca" {
		t.Errorf("tap_trust = %+v, %v", grants, err)
	}
	if !strings.Contains(f.stdout.String(), "Claude Code (CLI): installed, sign-in required") {
		t.Errorf("summary:\n%s", f.stdout.String())
	}

	// second run: nothing to do, no prompt, exit 0 (FR-076).
	before := len(f.mutating())
	f.want(0, f.runTTY("", "apply", "--profile", "agentic-factory"))
	if !strings.Contains(f.stdout.String(), "Nothing to install") || strings.Contains(f.stderr.String(), "Proceed?") {
		t.Errorf("second apply:\n%s\n%s", f.stdout.String(), f.stderr.String())
	}
	if len(f.mutating()) != before || len(f.operations()) != len(ops) {
		t.Error("the second apply ran something")
	}
	f.want(0, f.run("status", "--json"))
	if p := pkg(t, f.doc(), "jq"); p["state"] != "managed" {
		t.Errorf("status jq = %v", p)
	}
}

// Declining (n, empty line, EOF) and Ctrl+C at the prompt change nothing.
func TestApplyDecline(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", "no\n", "nope\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			f := newFixture(t, machine{})
			f.want(0, f.runTTY(answer, "apply", "--profile", "agentic-factory"))
			if len(f.operations()) != 0 || len(f.mutating()) != 0 {
				t.Error("a decline changed something")
			}
			if !strings.Contains(f.stdout.String(), "Nothing was changed") {
				t.Errorf("stdout:\n%s", f.stdout.String())
			}
		})
	}
	t.Run("ctrl-c", func(t *testing.T) {
		f := newFixture(t, machine{})
		go func() {
			time.Sleep(200 * time.Millisecond)
			f.sigs <- syscall.SIGINT
		}()
		f.want(0, f.runTTY("", "apply", "--profile", "agentic-factory"))
		if len(f.operations()) != 0 || len(f.mutating()) != 0 {
			t.Error("Ctrl+C at the prompt changed something")
		}
	})
}

// R-8: apply --json keeps the prompt and command list on stderr.
func TestApplyJSONKeepsPromptOnStderr(t *testing.T) {
	f := newFixture(t, machine{})
	dir, _ := filepath.Abs("../../testdata/catalogs/e2e")
	f.want(0, f.runTTY("y\n", "apply", "--json", "--profile", "e2e", "--catalog", dir))
	doc := f.doc()
	if doc["consent_by"] != consent || len(doc["results"].([]any)) != 1 {
		t.Errorf("report = %v", doc)
	}
	if !strings.Contains(f.stderr.String(), "Proceed? [y/N]") || !strings.Contains(f.stderr.String(), "brew install jq") {
		t.Errorf("stderr:\n%s", f.stderr.String())
	}
}

// I1-9: a dangling symlink at {prefix}/bin/claude makes claude-code a
// conflict (orphan) with evidence; the rest installs; exit 1; doctor 2.
func TestConflictRefusal(t *testing.T) {
	f := newFixture(t, machine{})
	if err := os.Symlink(filepath.Join(f.home, ".local", "share", "claude", "gone"), filepath.Join(f.prefix, "bin", "claude")); err != nil {
		t.Fatal(err)
	}
	f.want(0, f.run("plan", "--json", "--profile", "agentic-factory"))
	doc := f.doc()
	c, _ := pkg(t, doc, "claude-code")["conflict"].(map[string]any)
	if c == nil || c["owner"] != "orphan" || c["evidence"].(map[string]any)["dangling"] != true || c["fix_hint"] == "" {
		t.Fatalf("claude-code = %v", pkg(t, doc, "claude-code"))
	}
	for _, a := range actions(doc) {
		if a["package_id"] == "claude-code" {
			t.Error("claude-code got a command")
		}
	}
	f.want(1, f.runTTY("y\n", "apply", "--profile", "agentic-factory"))
	if !strings.Contains(f.stdout.String(), "not installed: conflict") {
		t.Errorf("summary:\n%s", f.stdout.String())
	}
	if f.records()["jq"].State != inventory.StateManaged {
		t.Error("the other packages were not installed")
	}
	for _, m := range f.mutating() {
		if slices.Contains(m, "claude-code") {
			t.Errorf("ran %v", m)
		}
	}
	f.want(2, f.run("doctor"))
	if !strings.Contains(f.stdout.String(), "claude-code") || !strings.Contains(f.stdout.String(), "orphan") {
		t.Errorf("doctor:\n%s", f.stdout.String())
	}
	f.want(2, f.run("doctor", "--json"))
	if f.doc()["exit_code"] != float64(2) {
		t.Error("doctor --json exit_code")
	}
}

// R-2: doctor on a clean machine is 0; an untrusted tapped Orca is 2 with
// the exact fix.
func TestDoctorTapTrust(t *testing.T) {
	f := newFixture(t, machine{})
	f.want(0, f.run("doctor"))
	if !strings.Contains(f.stdout.String(), "No problems found") {
		t.Errorf("doctor:\n%s", f.stdout.String())
	}
	f.want(0, f.run("doctor", "--json"))
	f.doc()
	w := f.fake.World(t)
	w.Brew.Taps = []string{"stablyai/orca"}
	f.fake.SetWorld(t, w)
	f.want(2, f.run("doctor"))
	if !strings.Contains(f.stdout.String(), "brew trust --cask stablyai/orca/orca") {
		t.Errorf("doctor:\n%s", f.stdout.String())
	}
}

// I1-5: no Homebrew. scan and plan report it and install nothing; apply
// lists the installer first (temp file, unverified); a decline changes
// nothing; consent installs it (homebrew record managed, bootstrap=1) and
// later commands get brew shellenv; only consented commands run.
func TestNoHomebrew(t *testing.T) {
	f := newFixture(t, machine{goos: "linux", brew: brewMissing})
	f.want(0, f.run("scan", "--json"))
	if hb := f.doc()["bootstrap"].(map[string]any)["homebrew"].(map[string]any); hb["found"] != false {
		t.Errorf("homebrew = %v", hb)
	}
	f.want(0, f.run("plan", "--json", "--profile", "e2e", "--catalog", "../../testdata/catalogs/e2e"))
	acts := actions(f.doc())
	if len(acts) != 2 || acts[0]["kind"] != "bootstrap-homebrew" || argv(acts[0], 0) != "/bin/bash <temp file>" {
		t.Fatalf("actions = %v", acts)
	}
	if cmd := acts[0]["commands"].([]any)[0].(map[string]any); cmd["unverified"] != true || cmd["needs_sudo"] != true {
		t.Errorf("installer command = %v", cmd)
	}
	f.want(0, f.runTTY("n\n", "apply", "--profile", "e2e", "--catalog", "../../testdata/catalogs/e2e"))
	if len(f.operations()) != 0 || len(f.fake.Log(t)) != 0 && len(f.mutating()) != 0 {
		t.Error("declining changed something")
	}
	if _, err := os.Stat(filepath.Join(f.prefix, "bin", "brew")); err == nil {
		t.Fatal("declining installed Homebrew")
	}

	f.want(0, f.runTTY("y\n", "apply", "--profile", "e2e", "--catalog", "../../testdata/catalogs/e2e"))
	recs := f.records()
	if hb := recs["homebrew"]; hb.State != inventory.StateManaged || !hb.Bootstrap {
		t.Errorf("homebrew record = %+v", hb)
	}
	if recs["jq"].State != inventory.StateManaged {
		t.Errorf("jq record = %+v", recs["jq"])
	}
	var install *fakeworld.Invocation
	log := f.fake.Log(t)
	for i := range log {
		if strings.Join(log[i].Argv[1:], " ") == "install jq" {
			install = &log[i]
		}
	}
	if install == nil || !slices.Contains(install.EnvNames, "HOMEBREW_PREFIX") {
		t.Errorf("brew install jq ran without brew shellenv: %+v", install)
	}
	if m := f.mutating(); len(m) != 1 {
		t.Errorf("mutating commands = %v, want only the consented brew install jq", m)
	}
	if !strings.Contains(f.stdout.String(), "shellenv") {
		t.Errorf("summary misses the shellenv line:\n%s", f.stdout.String())
	}
}

// FR-056: Homebrew at a known prefix off PATH is found; glibc below 2.39
// warns in --dry-run; UID 0 is exit 4 (in TestExitCodesAndUsage).
func TestLinuxbrewOffPathAndGlibc(t *testing.T) {
	f := newFixture(t, machine{goos: "linux", brew: brewOffPath, glibc: "2.35"})
	f.want(0, f.run("scan", "--json"))
	hb := f.doc()["bootstrap"].(map[string]any)["homebrew"].(map[string]any)
	if hb["found"] != true || hb["on_path"] != false || hb["prefix"] != f.prefix {
		t.Errorf("homebrew = %v", hb)
	}
	f.want(0, f.run("apply", "--dry-run", "--profile", "agentic-factory"))
	if !strings.Contains(f.stdout.String(), "glibc 2.35 is older than 2.39") {
		t.Errorf("dry run:\n%s", f.stdout.String())
	}
}

// R-9, FR-044, FR-045: GUI items are unavailable-here on Linux without a
// display; on WSL they carry the winget hint.
func TestHeadlessLinuxAndWSL(t *testing.T) {
	f := newFixture(t, machine{goos: "linux"})
	f.want(0, f.run("plan", "--json", "--profile", "agentic-factory"))
	doc := f.doc()
	for _, e := range f.cat.Entries {
		if e.Kind != "gui" {
			continue
		}
		if p := pkg(t, doc, e.ID); p["offered"] == true && p["state"] != "unavailable-here" {
			t.Errorf("%s = %v, want unavailable-here", e.ID, p["state"])
		}
	}
	if p := pkg(t, doc, "visual-studio-code"); p["state"] != "unavailable-here" {
		t.Errorf("visual-studio-code = %v", p)
	}
	var excluded []string
	for _, ex := range doc["plan"].(map[string]any)["excluded"].([]any) {
		if ex.(map[string]any)["reason"] == "unavailable-here" {
			excluded = append(excluded, ex.(map[string]any)["package_id"].(string))
		}
	}
	if !slices.Contains(excluded, "visual-studio-code") {
		t.Errorf("excluded = %v", excluded)
	}

	w := newFixture(t, machine{goos: "linux", wsl: true})
	w.want(0, w.run("plan", "--json", "--profile", "agentic-factory"))
	p := pkg(t, w.doc(), "claude-desktop")
	if p["state"] != "unavailable-here" || !strings.Contains(p["hint"].(string)+p["unavailable_reason"].(string), "winget") {
		t.Errorf("WSL claude-desktop = %v", p)
	}
}

// --version and --about keep working; no command without a terminal
// prints the about text and a hint (AD-14).
func TestVersionAboutAndNoCommand(t *testing.T) {
	f := newFixture(t, machine{})
	f.want(0, f.run("--version"))
	if f.stdout.String() != "facman dev (Agentic Software Factory Manager)\n" {
		t.Errorf("--version = %q", f.stdout.String())
	}
	f.want(0, f.run("--about"))
	if !strings.Contains(f.stdout.String(), "Agentic Software Factory Manager") {
		t.Errorf("--about = %q", f.stdout.String())
	}
	f.want(0, f.run())
	if !strings.Contains(f.stdout.String(), "not a terminal") {
		t.Errorf("no command = %q", f.stdout.String())
	}
	if len(f.fake.Log(t)) != 0 {
		t.Error("--version/--about/no-command ran commands")
	}
}

// AD-9: a second mutating run while one holds the lock exits 4 naming the PID.
func TestSecondMutatingRunIsRefused(t *testing.T) {
	f := newFixture(t, machine{})
	f.want(0, f.run("scan")) // creates the state directory
	lock, err := os.OpenFile(filepath.Join(f.stateDir(), "facman.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // fd fits
		t.Fatal(err)
	}
	f.want(4, f.runTTY("y\n", "apply", "--profile", "agentic-factory"))
	if !strings.Contains(f.stderr.String(), "another facman") {
		t.Errorf("stderr = %s", f.stderr.String())
	}
	// read-only commands still work, with a warning.
	f.want(0, f.run("scan"))
	if !strings.Contains(f.stdout.String(), "does not save its results") {
		t.Errorf("scan with the lock held:\n%s", f.stdout.String())
	}
}

// Ctrl+C during Run: the current command finishes and is recorded, the
// rest is not started, exit 1.
func TestSignalDuringRunStopsAfterCurrentCommand(t *testing.T) {
	f := newFixture(t, machine{}, func(w *fakeworld.World) {
		w.Rules = append(w.Rules, fakeworld.Rule{Cmd: "brew", Args: []string{"install", "jq"}, SleepMS: 800, Builtin: true})
	})
	dir, _ := filepath.Abs("../../testdata/catalogs/e2e")
	go func() {
		for range 400 {
			for _, inv := range f.fake.Log(t) {
				if strings.Join(inv.Argv, " ") == "brew install jq" {
					f.sigs <- syscall.SIGINT
					f.sigs <- syscall.SIGINT // a second signal is ignored
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	f.want(1, f.runTTY("y\n", "apply", "--profile", "e2e", "--profile", "tools", "--catalog", dir))
	ops := f.operations()
	if len(ops) != 1 || ops[0].PackageID != "jq" || ops[0].Status != inventory.StatusSucceeded {
		t.Fatalf("operations = %+v, want jq finished and gh not started", ops)
	}
	if !strings.Contains(f.stdout.String(), "GitHub CLI: not installed: cancelled") {
		t.Errorf("summary:\n%s", f.stdout.String())
	}
}

// The installer download fails 3 times: exit 4 naming the URL, nothing else runs.
func TestInstallerDownloadFailure(t *testing.T) {
	f := newFixture(t, machine{goos: "linux", brew: brewMissing})
	f.srv.Close()
	f.o.InstallerURL = "http://127.0.0.1:1/install.sh"
	f.want(4, f.runTTY("y\n", "apply", "--profile", "e2e", "--catalog", "../../testdata/catalogs/e2e"))
	if !strings.Contains(f.stderr.String(), "127.0.0.1:1") {
		t.Errorf("stderr:\n%s", f.stderr.String())
	}
	if len(f.mutating()) != 0 {
		t.Error("something ran after the download failed")
	}
}
