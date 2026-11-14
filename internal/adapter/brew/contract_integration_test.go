//go:build integration

package brew

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// captureEnv names a directory where the contract test writes every output
// it reads from the real Homebrew, so the hand-written fixtures in testdata/
// can be replaced by captured ones (review FR-003, user-docs/field-check.md).
const captureEnv = "FACMAN_CAPTURE_DIR"

// contractBrew returns the real brew, or skips: as root (Homebrew refuses
// UID 0) or when Homebrew is not installed. Where CI is meant to prove the
// contract (FACMAN_INTEGRATION_MUTATE=1, macOS) those cases fail instead
// (review FR-004).
func contractBrew(t *testing.T) string {
	t.Helper()
	skip := t.Skip
	if os.Getenv("FACMAN_INTEGRATION_MUTATE") == "1" {
		skip = func(args ...any) { t.Fatal(append([]any{"FACMAN_INTEGRATION_MUTATE=1 but"}, args...)...) }
	}
	if os.Getuid() == 0 {
		skip("running as root: Homebrew refuses UID 0")
	}
	if p, err := exec.LookPath("brew"); err == nil && filepath.IsAbs(p) {
		return p
	}
	for _, prefix := range DefaultPrefixes(runtime.GOOS) {
		p := filepath.Join(prefix, "bin", "brew")
		if isExecutable(p) {
			return p
		}
	}
	skip("Homebrew not found on PATH or at the known prefixes")
	return ""
}

// readBrew runs a read-only brew command, logs and optionally captures its
// output, and returns stdout and the exit error.
func readBrew(t *testing.T, brewPath, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, brewPath, args...) //nolint:gosec // fixed read-only argv
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	t.Logf("brew %s: err=%v\n--- stdout ---\n%s\n--- stderr ---\n%s", strings.Join(args, " "), err, out, stderr.String())
	if dir := os.Getenv(captureEnv); dir != "" {
		if werr := os.WriteFile(filepath.Join(dir, name), out, 0o600); werr != nil {
			t.Errorf("capturing %s: %v", name, werr)
		}
	}
	return string(out), err
}

// TestIntegrationBrewContract checks the real `brew info --json=v2`,
// `brew tap` and `brew trust` output against what ParseInfo, ParseTaps and
// ParseTrusted expect: every field facman reads must have the expected JSON
// type (review FR-003 AC 2). It runs only read-only commands.
func TestIntegrationBrewContract(t *testing.T) {
	brewPath := contractBrew(t)
	if v, err := readBrew(t, brewPath, "version.txt", "--version"); err == nil {
		t.Logf("Homebrew version: %s", strings.SplitN(v, "\n", 2)[0])
	}
	queries := []struct {
		file string
		args []string
		goos string // "" means every OS
	}{
		{"info-installed.json", []string{"info", "--json=v2", "--installed"}, ""},
		{"info-formula-hello.json", []string{"info", "--json=v2", "--formula", "hello"}, ""},
		{"info-formula-kegonly.json", []string{"info", "--json=v2", "--formula", "sqlite"}, "darwin"},
		{"info-cask-pkg.json", []string{"info", "--json=v2", "--cask", "zoom"}, "darwin"},
	}
	for _, q := range queries {
		t.Run(q.file, func(t *testing.T) {
			if q.goos != "" && q.goos != runtime.GOOS {
				t.Skipf("%s only", q.goos)
			}
			out, err := readBrew(t, brewPath, q.file, q.args...)
			if err != nil {
				t.Fatalf("brew %s: %v", strings.Join(q.args, " "), err)
			}
			if problems := infoShapeProblems([]byte(out)); len(problems) > 0 {
				t.Fatalf("brew info shape differs from what facman parses:\n  %s", strings.Join(problems, "\n  "))
			}
			info, err := ParseInfo([]byte(out))
			if err != nil {
				t.Fatal(err)
			}
			switch q.file {
			case "info-formula-hello.json":
				if f, ok := info.Formula("hello"); !ok || f.KegOnly {
					t.Errorf("hello = %+v, %v", f, ok)
				}
			case "info-formula-kegonly.json":
				if f, ok := info.Formula("sqlite"); !ok || !f.KegOnly {
					t.Errorf("sqlite = %+v, %v; want keg-only on macOS", f, ok)
				}
			case "info-cask-pkg.json":
				c, ok := info.Cask("zoom")
				if !ok || len(c.Artifacts) == 0 {
					t.Fatalf("zoom = %+v, %v", c, ok)
				}
				t.Logf("zoom: apps=%v binaries=%v needs sudo=%v", c.Apps(), c.Binaries(), c.NeedsSudo())
			}
		})
	}
	t.Run("tap", func(t *testing.T) {
		out, err := readBrew(t, brewPath, "tap-list.txt", "tap")
		if err != nil {
			t.Fatalf("brew tap: %v", err)
		}
		var lines int
		for _, l := range strings.Split(out, "\n") {
			if strings.TrimSpace(l) != "" {
				lines++
			}
		}
		if got := len(ParseTaps(out)); got != lines {
			t.Errorf("ParseTaps read %d taps from %d lines", got, lines)
		}
	})
	t.Run("trust", func(t *testing.T) {
		out, err := readBrew(t, brewPath, "trust-list.txt", "trust")
		if err != nil {
			// R-OQ-1: the listing format is unconfirmed; facman treats an
			// unreadable listing as "not trusted", which only adds a step.
			t.Skipf("brew trust with no arguments failed (%v); format still open", err)
		}
		t.Logf("ParseTrusted = %v", ParseTrusted(out))
	})
}
