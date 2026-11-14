package appinfo

import (
	"strings"
	"testing"
)

func TestIdentityConstants(t *testing.T) {
	if Name != "facman" {
		t.Errorf("Name = %q, want facman", Name)
	}
	if Title != "Agentic Software Factory Manager" {
		t.Errorf("Title = %q, want Agentic Software Factory Manager", Title)
	}
	if Repository != "software-factory-manager" {
		t.Errorf("Repository = %q, want software-factory-manager", Repository)
	}
}

func TestVersionLine(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"default", "dev", "facman dev (Agentic Software Factory Manager)"},
		{"release", "v0.3.0", "facman v0.3.0 (Agentic Software Factory Manager)"},
		{"empty falls back to dev", "", "facman dev (Agentic Software Factory Manager)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VersionLine(tt.version); got != tt.want {
				t.Errorf("VersionLine(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestAbout(t *testing.T) {
	got := About("v0.3.0")
	for _, want := range []string{
		"facman — Agentic Software Factory Manager",
		"Version:     v0.3.0",
		"Repository:  software-factory-manager",
		Description,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("About() missing %q in:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("About() must end with a newline")
	}
}
