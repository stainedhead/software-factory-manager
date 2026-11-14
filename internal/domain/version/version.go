// Package version parses and compares the version strings that tools print.
// It is deliberately lenient (tools do not follow semver) and safe: an unknown
// or unparsable version never satisfies a minimum.
package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalid is wrapped by every Parse error.
var ErrInvalid = errors.New("invalid version")

// Version is a parsed tool version. The zero value means "unknown".
type Version struct {
	// Raw is the text as the tool printed it (after any catalog regex).
	Raw string
	// Parts are the numeric dot-separated components.
	Parts []int
	// Pre is any non-numeric suffix (pre-release or build), without its separator.
	Pre string
}

// Parse reads a version such as "2.1.4", "v24.11.0" or "1.7.1-rc1".
// The string must start with a digit (after an optional "v").
func Parse(s string) (Version, error) {
	raw := s
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	end := 0
	for end < len(s) && (s[end] == '.' || (s[end] >= '0' && s[end] <= '9')) {
		end++
	}
	numeric, rest := s[:end], s[end:]
	if numeric == "" || strings.HasSuffix(numeric, ".") || strings.HasPrefix(numeric, ".") {
		return Version{}, fmt.Errorf("%w: %q", ErrInvalid, raw)
	}
	fields := strings.Split(numeric, ".")
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return Version{}, fmt.Errorf("%w: %q: %w", ErrInvalid, raw, err)
		}
		parts = append(parts, n)
	}
	pre := strings.TrimLeft(rest, "-+~_")
	return Version{Raw: raw, Parts: parts, Pre: pre}, nil
}

// Known reports whether v holds a parsed version.
func (v Version) Known() bool { return len(v.Parts) > 0 }

// String returns the raw text, or "unknown" for the zero value.
func (v Version) String() string {
	if !v.Known() {
		return "unknown"
	}
	return v.Raw
}

// Compare returns -1, 0 or 1. Missing components count as zero, and a
// version with a pre-release suffix sorts before the same version without one.
func Compare(a, b Version) int {
	n := max(len(a.Parts), len(b.Parts))
	for i := range n {
		x, y := at(a.Parts, i), at(b.Parts, i)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	case a.Pre < b.Pre:
		return -1
	default:
		return 1
	}
}

// AtLeast reports whether v >= minimum. Unknown versions never satisfy, and an
// unknown minimum is never satisfied.
func AtLeast(v, minimum Version) bool {
	if !v.Known() || !minimum.Known() {
		return false
	}
	return Compare(v, minimum) >= 0
}

// Satisfies parses raw and minimum and reports AtLeast. Parse errors mean false.
func Satisfies(raw, minimum string) bool {
	v, err := Parse(raw)
	if err != nil {
		return false
	}
	m, err := Parse(minimum)
	if err != nil {
		return false
	}
	return AtLeast(v, m)
}

func at(p []int, i int) int {
	if i < len(p) {
		return p[i]
	}
	return 0
}
