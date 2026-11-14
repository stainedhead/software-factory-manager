package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"version", []string{"--version"}, 0, "facman dev (Agentic Software Factory Manager)"},
		{"about", []string{"--about"}, 0, "facman — Agentic Software Factory Manager"},
		{"no args prints about when stdout is not a terminal", nil, 0, "Agentic Software Factory Manager"},
		{"unknown flag is a usage error", []string{"--nope"}, 64, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			o := defaultOptions()
			o.Stdout, o.Stderr = &out, &errOut
			if got := run(tt.args, o); got != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got, tt.wantCode, errOut.String())
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}
