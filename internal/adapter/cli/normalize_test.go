package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr string
	}{
		{"untouched", []string{"plan", "--profile", "agentic-factory", "--with", "colima"}, []string{"plan", "--profile", "agentic-factory", "--with", "colima"}, ""},
		{"-h stays help", []string{"-h"}, []string{"-h"}, ""},
		{"-o is not -os", []string{"-o", "x"}, []string{"-o", "x"}, ""},
		{"single letters unused", []string{"-s", "-p", "-t", "-n"}, []string{"-s", "-p", "-t", "-n"}, ""},
		{"clusters are not short flags", []string{"-clx", "-osx", "-nopx"}, []string{"-clx", "-osx", "-nopx"}, ""},
		{"after -- untouched", []string{"scan", "--", "-cl", "-nop=x"}, []string{"scan", "--", "-cl", "-nop=x"}, ""},
		{"-cl deferred", []string{"apply", "-cl", "list.yaml"}, nil, "configuration lists"},
		{"-cl=file deferred", []string{"apply", "-cl=list.yaml"}, nil, "configuration lists"},
		{"-c deferred", []string{"apply", "-c", "list.yaml"}, nil, "configuration lists"},
		{"-os deferred", []string{"apply", "-os=orca.yaml"}, nil, "--orca-setup"},
		{"-hs deferred", []string{"apply", "-hs", "h.yaml"}, nil, "--hermes-setup"},
		{"-ps deferred", []string{"apply", "-ps", "p.yaml"}, nil, "--paperclip-setup"},
		{"-ts deferred", []string{"apply", "-ts", "t.json"}, nil, "--t3-setup"},
		{"-nop deferred", []string{"plan", "-nop"}, nil, "Increment 2"},
		{"-nop=x usage", []string{"plan", "-nop=x"}, nil, "takes no value"},
		{"-no usage", []string{"plan", "-no"}, nil, "did you mean -nop"},
		{"--set deferred", []string{"plan", "--set", "a=b"}, nil, "--set arrives"},
		{"--set=x deferred", []string{"plan", "--set=a=b"}, nil, "--set arrives"},
		{"--check deferred", []string{"apply", "--check"}, nil, "--check"},
		{"long deferred", []string{"apply", "--configuration-list", "x"}, nil, "Increment 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || failure.ExitCode(err) != 64 {
					t.Fatalf("err = %v (exit %d), want usage error containing %q", err, failure.ExitCode(err), tt.wantErr)
				}
				return
			}
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("Normalize(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}

// TestShortFlagCollisions covers the full flag set (Appendix B): every
// multi-letter short flag maps to one long flag, none is a prefix of
// another's -x= form that could be misread, none equals a cobra shorthand
// of the real command tree, and the deliberately unused single letters are
// still unused.
func TestShortFlagCollisions(t *testing.T) {
	root := (&App{}).command(nil)
	root.InitDefaultHelpFlag()
	shorthands := map[string]string{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			if f.Shorthand != "" {
				shorthands["-"+f.Shorthand] = c.CommandPath() + " --" + f.Name
			}
			if _, deferred := DeferredFlags["--"+f.Name]; deferred {
				t.Errorf("%s registers --%s, which the normalizer treats as deferred", c.CommandPath(), f.Name)
			}
		}
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	for short, long := range ShortFlags {
		if !strings.HasPrefix(long, "--") || len(short) < 2 || short[0] != '-' || short[1] == '-' {
			t.Errorf("bad mapping %s -> %s", short, long)
		}
		if owner, ok := shorthands[short]; ok && short != "-c" {
			t.Errorf("%s collides with the shorthand of %s", short, owner)
		}
		if _, ok := DeferredFlags[long]; !ok {
			t.Errorf("%s -> %s: Increment 1 has no such flag, so it must be listed as deferred", short, long)
		}
	}
	for _, letter := range []string{"-s", "-p", "-t", "-n"} {
		if owner, ok := shorthands[letter]; ok {
			t.Errorf("%s is deliberately unused but belongs to %s", letter, owner)
		}
		if _, ok := ShortFlags[letter]; ok {
			t.Errorf("%s is deliberately unused but is a short flag", letter)
		}
	}
	if shorthands["-h"] == "" {
		t.Error("-h must stay help")
	}
	// -os must never swallow -o (or the reverse): rewriting is exact-token.
	for _, tok := range []string{"-o", "-osfile", "-c1", "-cl1", "-tsx"} {
		got, err := Normalize([]string{tok})
		if err != nil || got[0] != tok {
			t.Errorf("Normalize(%s) = %v, %v; want it untouched", tok, got, err)
		}
	}
}
