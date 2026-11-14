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
		{"no args prints about", nil, 0, "Agentic Software Factory Manager"},
		{"unknown flag", []string{"--nope"}, 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run(tt.args, &out, &errOut); got != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got, tt.wantCode, errOut.String())
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}
