package main

import (
	"fmt"
	"os"
	"testing"
)

// skipOrFailer is the part of testing.TB that requireOrSkip uses, so its
// own test can record the call instead of ending a real test.
type skipOrFailer interface {
	Helper()
	Skipf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// requireOrSkip ends the test when a precondition (Homebrew installed, not
// root) is missing: it skips normally, but fails when the mutating run was
// asked for with FACMAN_INTEGRATION_MUTATE=1, so a CI job that is meant to
// prove I1-14 cannot pass by skipping (review FR-004).
func requireOrSkip(t skipOrFailer, getenv func(string) string, format string, args ...any) {
	t.Helper()
	if getenv(mutateEnv) == "1" {
		t.Fatalf("%s=1 but %s", mutateEnv, fmt.Sprintf(format, args...))
		return
	}
	t.Skipf(format, args...)
}

// requireOrSkipEnv is requireOrSkip with the process environment.
func requireOrSkipEnv(t *testing.T, format string, args ...any) {
	t.Helper()
	requireOrSkip(t, os.Getenv, format, args...)
}

// mutateEnv opts in to the one integration test that changes the machine
// (installs and then removes GNU hello). CI sets it on macOS only (I1-14).
const mutateEnv = "FACMAN_INTEGRATION_MUTATE"

type recordTB struct{ skipped, failed string }

func (r *recordTB) Helper()                   {}
func (r *recordTB) Skipf(f string, a ...any)  { r.skipped = fmt.Sprintf(f, a...) }
func (r *recordTB) Fatalf(f string, a ...any) { r.failed = fmt.Sprintf(f, a...) }

func TestRequireOrSkip(t *testing.T) {
	tests := []struct {
		name, mutate       string
		wantSkip, wantFail string
	}{
		{"skips without the opt-in", "", "Homebrew not found", ""},
		{"skips with another value", "0", "Homebrew not found", ""},
		{"fails with the opt-in", "1", "", mutateEnv + "=1 but Homebrew not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r recordTB
			env := func(k string) string {
				if k == mutateEnv {
					return tt.mutate
				}
				return ""
			}
			requireOrSkip(&r, env, "Homebrew %s", "not found")
			if r.skipped != tt.wantSkip || r.failed != tt.wantFail {
				t.Errorf("skipped=%q failed=%q, want %q %q", r.skipped, r.failed, tt.wantSkip, tt.wantFail)
			}
		})
	}
}
