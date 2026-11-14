package brew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fuzz targets for the parsers of brew's output (review FR-014). Each is
// seeded from testdata and checks no panic plus one invariant.

func seedFiles(f *testing.F, pattern string) {
	f.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", pattern))
	if err != nil {
		f.Fatal(err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // test fixture
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
}

// FuzzParseInfo: every link target ParseInfo accepts is an absolute,
// clean path, so it can only ever be probed where brew said it is.
func FuzzParseInfo(f *testing.F) {
	seedFiles(f, "*.json")
	f.Fuzz(func(t *testing.T, data []byte) {
		info, err := ParseInfo(data)
		if err != nil {
			return
		}
		for _, c := range info.Casks {
			for _, l := range c.Links() {
				if l.Target != "" && (!filepath.IsAbs(l.Target) || filepath.Clean(l.Target) != l.Target || hasControlRune(l.Target)) {
					t.Errorf("cask %q link target %q is not an absolute clean path", c.Token, l.Target)
				}
			}
		}
	})
}

// FuzzParseShellEnv: every variable name ParseShellEnv returns is valid.
func FuzzParseShellEnv(f *testing.F) {
	f.Add("export HOMEBREW_PREFIX=\"/opt/homebrew\";\nexport PATH=\"/opt/homebrew/bin${PATH+:$PATH}\";\n")
	f.Add("export INFOPATH=\"/opt/homebrew/share/info:${INFOPATH:-}\";\n")
	f.Add("export X='a b'; export 1BAD=x\n")
	f.Fuzz(func(t *testing.T, out string) {
		for k := range ParseShellEnv(out, func(string) string { return "/usr/bin" }) {
			if !validName(k) {
				t.Errorf("ParseShellEnv returned invalid name %q", k)
			}
		}
	})
}

// FuzzParseTaps: every tap ParseTaps returns is one non-empty token.
func FuzzParseTaps(f *testing.F) {
	seedFiles(f, "*.txt")
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, parse := range []func(string) []string{ParseTaps, ParseTrusted} {
			for _, tap := range parse(string(data)) {
				if tap == "" || strings.ContainsAny(tap, " \t\r\n") {
					t.Errorf("parsed tap %q is not one token", tap)
				}
			}
		}
	})
}

func hasControlRune(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
