package cli

import (
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
)

// ShortFlags are the multi-letter short flags of PRD Appendix B. The
// normalizer rewrites them, as exact tokens only, to their long forms
// before cobra parses (FR-005).
var ShortFlags = map[string]string{
	"-cl":  "--configuration-list",
	"-c":   "--configuration-list",
	"-hs":  "--hermes-setup",
	"-ps":  "--paperclip-setup",
	"-os":  "--orca-setup",
	"-ts":  "--t3-setup",
	"-nop": "--no-paperclip",
}

// valueless are the short flags that take no value: -nop=<x> is a usage error.
var valueless = map[string]bool{"-nop": true}

// rejected are near-misses of the short flags that are usage errors
// instead of being read as single-letter clusters.
var rejected = map[string]string{"-no": "did you mean -nop (--no-paperclip)?"}

// DeferredFlags are long flags of the full product that Increment 1 does
// not have yet. Each is a usage error naming its increment.
var DeferredFlags = map[string]string{
	"--configuration-list": "configuration lists (-cl) arrive in Increment 3",
	"--hermes-setup":       "setup flags (-hs) arrive in Increment 5",
	"--paperclip-setup":    "setup flags (-ps) arrive in Increment 5",
	"--orca-setup":         "setup flags (-os) arrive in Increment 5",
	"--t3-setup":           "setup flags (-ts) arrive in Increment 5",
	"--no-paperclip":       "-nop arrives in Increment 2 with the npm backend (Paperclip is not offered yet)",
	"--set":                "--set arrives in Increment 3",
	"--check":              "--check arrives in Increment 3 with configuration lists",
	"--review":             "--review arrives in Increment 5 with setup flags",
}

// Normalize rewrites the exact tokens in ShortFlags (and -x=<value> for
// the value flags) to their long forms. Everything after "--" is left
// alone. Other tokens, including -h, -o and any single-letter cluster, are
// untouched. -nop=<x> and -no are usage errors. A deferred long flag,
// written short or long, is a usage error (exit 64) naming the increment
// that brings it.
func Normalize(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			return append(out, args[i:]...), nil
		}
		name, value, hasValue := strings.Cut(a, "=")
		if hint, ok := rejected[name]; ok {
			return nil, failure.Usage("unknown flag %s: %s", a, hint)
		}
		if long, ok := ShortFlags[name]; ok {
			if hasValue && valueless[name] {
				return nil, failure.Usage("%s takes no value (got %s)", name, a)
			}
			if hasValue {
				a = long + "=" + value
			} else {
				a = long
			}
			name = long
		}
		if why, ok := DeferredFlags[name]; ok {
			return nil, failure.Usage("%s is not available yet: %s", name, why)
		}
		out = append(out, a)
	}
	return out, nil
}
