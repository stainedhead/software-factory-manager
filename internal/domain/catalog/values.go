package catalog

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Allow-lists for catalog values that reach argv, file paths or the terminal
// (review FR-001a). A catalog from --catalog is input, so every value is
// checked at load time; the plan invariant (plan.Validate) is the second line
// of defence.
var (
	// packageIDPattern matches catalog package IDs and profile names. They
	// name log files and appear in every output, so they are lowercase and
	// never start with an option dash.
	packageIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._+@-]*$`)
	// brewIDPattern matches a formula or cask name, optionally qualified by
	// its tap (user/repo/name).
	brewIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+@-]*(/[A-Za-z0-9._+@-]+){0,2}$`)
	// tapPattern matches a tap (user/repo).
	tapPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// npmIDPattern matches an npm package name, optionally scoped.
	npmIDPattern = regexp.MustCompile(`^(@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
	// commandPattern matches an executable name looked up on PATH.
	commandPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._+-]*$`)
)

// hasControl reports whether s has a control character (unicode.IsControl),
// which could hide or rewrite text on a terminal.
func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// safeLabel renders a key for an error detail without letting a control
// character reach the terminal.
func safeLabel(s string) string {
	if hasControl(s) {
		return strconv.Quote(s)
	}
	return s
}

// validCommand reports whether name is a bare executable name: no path
// separator, no "..", no control character and no leading dash or dot.
func validCommand(name string) bool {
	return commandPattern.MatchString(name) && !strings.Contains(name, "..")
}

// valueProblems checks the values of one entry against the allow-lists.
func valueProblems(e Entry) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if !packageIDPattern.MatchString(e.ID) {
		add("id %q is not allowed (want lowercase letters, digits and . _ + @ -, not starting with a symbol)", e.ID)
	}
	text := func(field, s string) {
		if hasControl(s) {
			add("%s has a control character", field)
		}
	}
	text("display", e.Display)
	text("category", e.Category)
	for _, c := range e.Detect.Commands {
		if !validCommand(c) {
			add("detect command %q is not allowed (want a bare executable name)", c)
		}
	}
	if v := e.Detect.Version; v != nil && len(v.Cmd) > 0 {
		if !slices.Contains(e.Detect.Commands, v.Cmd[0]) {
			add("detect.version.cmd must start with a detect command, got %q", v.Cmd[0])
		}
		for _, a := range v.Cmd {
			if hasControl(a) {
				add("detect.version.cmd has a control character")
				break
			}
		}
	}
	for _, b := range e.Detect.BundleIDs {
		text("detect.bundle_ids", b)
	}
	text("detect.winget", e.Detect.WingetID)
	text("detect.npm_package", e.Detect.NPMPackage)
	for _, g := range e.Detect.Provenance {
		text("detect.provenance", g)
	}
	keys := sortedInstallKeys(e)
	for _, k := range keys {
		for i, s := range e.Install[k] {
			for _, p := range stepProblems(s) {
				add("install.%s[%d]: %s", k, i, p)
			}
		}
	}
	return problems
}

// stepProblems checks one install step's values.
func stepProblems(s InstallStep) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	switch {
	case s.Backend.IsBrew():
		if s.ID != "" && !brewIDPattern.MatchString(s.ID) {
			add("brew id %q is not allowed (want name or user/repo/name)", s.ID)
		}
	case s.Backend == BackendNPM:
		if s.ID != "" && !npmIDPattern.MatchString(s.ID) {
			add("npm id %q is not allowed", s.ID)
		}
	default:
		if hasControl(s.ID) {
			add("id has a control character")
		}
	}
	if s.Tap != "" {
		switch {
		case !tapPattern.MatchString(s.Tap):
			add("tap %q is not allowed (want user/repo)", s.Tap)
		case s.Backend.IsBrew() && !strings.HasPrefix(s.ID, s.Tap+"/"):
			add("brew id %q is not an item of tap %q (want %s/<name>)", s.ID, s.Tap, s.Tap)
		}
	}
	if hasControl(s.Hint) {
		add("hint has a control character")
	}
	if hasControl(s.Version) {
		add("version has a control character")
	}
	for _, p := range s.PostInstall {
		if hasControl(p) {
			add("post_install has a control character")
			break
		}
	}
	return problems
}

// profileValueProblems checks a profile's name and text.
func profileValueProblems(key string, p Profile) []string {
	var problems []string
	if !packageIDPattern.MatchString(key) {
		problems = append(problems, fmt.Sprintf("profiles/%s: name %q is not allowed", safeLabel(key), key))
	}
	if hasControl(p.Description) {
		problems = append(problems, fmt.Sprintf("profiles/%s: description has a control character", safeLabel(key)))
	}
	return problems
}
