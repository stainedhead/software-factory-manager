package cli

import (
	"strings"
	"testing"
)

// FuzzNormalize: the short-flag normalizer never panics, keeps every
// token, and never lets a short alias or a deferred flag through before
// "--" (review FR-014). The input is the argument list joined by NUL.
func FuzzNormalize(f *testing.F) {
	for _, s := range []string{"plan\x00--profile\x00base-dev", "-cl\x00x", "-nop=1", "-no", "--\x00-cl", "-ps=a=b", "scan\x00-hs"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, joined string) {
		args := strings.Split(joined, "\x00")
		out, err := Normalize(args)
		if err != nil {
			return
		}
		if len(out) != len(args) {
			t.Fatalf("Normalize(%q) = %q: token count changed", args, out)
		}
		for _, a := range out {
			if a == "--" {
				break
			}
			name, _, _ := strings.Cut(a, "=")
			if _, short := ShortFlags[name]; short {
				t.Errorf("short alias %q left in %q", a, out)
			}
			if _, deferred := DeferredFlags[name]; deferred {
				t.Errorf("deferred flag %q let through in %q", a, out)
			}
		}
	})
}
