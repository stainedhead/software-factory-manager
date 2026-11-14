package plan

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrInvariant is wrapped by every Validate error. A violation is a bug in
// facman, never a user error.
var ErrInvariant = errors.New("plan invariant violated")

// TapTrustEnvPrefix is the environment prefix that disables Homebrew tap
// trust; it is never set for, and is stripped from, every child process.
const TapTrustEnvPrefix = "HOMEBREW_NO_REQUIRE_TAP_TRUST"

// allowedBrewSubcommands are the only brew subcommands a plan may run.
var allowedBrewSubcommands = map[string]bool{"install": true, "tap": true, "trust": true}

// Validate checks the safety invariants (data-dictionary §2.8). absent
// reports whether a package is in the absent state; every action must be for
// an absent package. It rejects: --force or --overwrite in any argv; brew
// subcommands other than install, tap and trust (so no upgrade, uninstall,
// untap, untrust or link); a brew trust whose target is not a full item path
// (whole-tap trust); HOMEBREW_NO_REQUIRE_TAP_TRUST* in any command's env;
// non-mutating commands; duplicate or unordered IDs; and dependencies on
// later or missing actions.
func (p Plan) Validate(absent func(packageID string) bool) error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	seen := map[int]bool{}
	last := 0
	for _, a := range p.Actions {
		if seen[a.ID] {
			add("action %d: duplicate id", a.ID)
		}
		if a.ID <= last {
			add("action %d: ids are not increasing", a.ID)
		}
		seen[a.ID], last = true, a.ID
		for _, d := range a.DependsOn {
			if !seen[d] || d == a.ID {
				add("action %d: depends on %d, which is not an earlier action", a.ID, d)
			}
		}
		if !absent(a.PackageID) {
			add("action %d: package %s is not absent", a.ID, a.PackageID)
		}
		if len(a.Commands) == 0 {
			add("action %d: no commands", a.ID)
		}
		for _, c := range a.Commands {
			for _, msg := range commandProblems(c) {
				add("action %d (%s): %s", a.ID, c.String(), msg)
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvariant, strings.Join(problems, "; "))
}

func commandProblems(c Command) []string {
	var problems []string
	if len(c.Argv) == 0 {
		return []string{"empty argv"}
	}
	if !c.Mutating {
		problems = append(problems, "plan command not marked mutating")
	}
	for _, a := range c.Argv {
		if strings.HasPrefix(a, "--force") || strings.HasPrefix(a, "--overwrite") {
			problems = append(problems, fmt.Sprintf("forbidden flag %s", a))
		}
	}
	for _, n := range c.EnvNames {
		if strings.HasPrefix(n, TapTrustEnvPrefix) {
			problems = append(problems, fmt.Sprintf("forbidden environment variable %s", n))
		}
	}
	if filepath.Base(c.Argv[0]) != "brew" {
		return problems
	}
	if len(c.Argv) < 2 {
		return append(problems, "brew without a subcommand")
	}
	sub := c.Argv[1]
	if !allowedBrewSubcommands[sub] {
		return append(problems, fmt.Sprintf("forbidden brew subcommand %s", sub))
	}
	if sub == "trust" {
		items := 0
		for _, a := range c.Argv[2:] {
			if strings.HasPrefix(a, "-") {
				continue
			}
			items++
			if strings.Count(a, "/") != 2 {
				problems = append(problems, fmt.Sprintf("brew trust target %s is not a full item path (whole-tap trust is never used)", a))
			}
		}
		if items == 0 {
			problems = append(problems, "brew trust without an item")
		}
	}
	return problems
}
