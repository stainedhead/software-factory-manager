package plan

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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

// allowedBrewOptions are the only options a plan may pass after a brew
// subcommand (review FR-001b). Everything else that starts with "-" could
// change the reviewed command (-f, -s, --HEAD, --ignore-dependencies ...),
// and Homebrew accepts abbreviated long options, so the list is exact.
var allowedBrewOptions = map[string][]string{
	"install": {"--cask", "--formula"},
	"trust":   {"--cask", "--formula"},
}

// forceLike reports whether a is any short or long form of --force or
// --overwrite: a long option whose name (case-insensitive, before any "=")
// starts with, or abbreviates, force or overwrite; or a short-option
// cluster that contains f (-f, -fsSL).
func forceLike(a string) bool {
	if name, ok := strings.CutPrefix(a, "--"); ok {
		name, _, _ = strings.Cut(strings.ToLower(name), "=")
		for _, w := range []string{"force", "overwrite"} {
			if strings.HasPrefix(name, w) || (name != "" && strings.HasPrefix(w, name)) {
				return true
			}
		}
		return false
	}
	if letters, ok := strings.CutPrefix(a, "-"); ok {
		return strings.ContainsRune(letters, 'f') && strings.Trim(letters, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") == ""
	}
	return false
}

// Validate checks the safety invariants (data-dictionary §2.8). absent
// reports whether a package is in the absent state; every action must be for
// an absent package. It rejects: any short or long form of --force or
// --overwrite in any argv; any option after a brew subcommand other than
// --cask or --formula on install and trust (FR-001b); brew
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
		if forceLike(a) {
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
	for _, a := range c.Argv[2:] {
		if strings.HasPrefix(a, "-") && !slices.Contains(allowedBrewOptions[sub], a) {
			problems = append(problems, fmt.Sprintf("forbidden option %s (no option from data reaches brew)", a))
		}
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
