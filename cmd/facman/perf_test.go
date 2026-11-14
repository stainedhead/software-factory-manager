package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestVersionIsFast is the T-F6 smoke test for startup: --version builds the
// whole app (wire.go) and answers well inside the budgets. The limits are
// generous to avoid flakes on busy CI runners; the 100 ms product target is
// measured by hand (implementation-notes I-41).
func TestVersionIsFast(t *testing.T) {
	t.Run("in process", func(t *testing.T) {
		o := defaultOptions()
		var out bytes.Buffer
		o.Stdout, o.Stderr = &out, &out
		start := time.Now()
		if code := run([]string{"--version"}, o); code != 0 {
			t.Fatalf("exit %d: %s", code, out.String())
		}
		if d := time.Since(start); d > 50*time.Millisecond {
			t.Errorf("--version took %v in process, want < 50ms", d)
		}
		t.Logf("--version in process: %v", time.Since(start))
	})
	t.Run("built binary", func(t *testing.T) {
		if testing.Short() {
			t.Skip("builds the binary")
		}
		bin := filepath.Join(t.TempDir(), "facman")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
		start := time.Now()
		out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput() //nolint:gosec // bin is the binary this test just built
		if err != nil {
			t.Fatalf("facman --version: %v\n%s", err, out)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("built facman --version took %v, want < 1s", d)
		}
		t.Logf("built binary --version: %v", time.Since(start))
	})
}
