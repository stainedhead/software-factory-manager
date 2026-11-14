package brew

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseInfoInstalled(t *testing.T) {
	info, err := ParseInfo(fixture(t, "info-installed.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id, version string
		kegOnly     bool
		linked      bool
	}{
		{"jq", "1.7.1", false, true},
		{"node@24", "24.11.1", true, false},
		{"gh", "2.62.0", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			f, ok := info.Formula(tt.id)
			if !ok {
				t.Fatal("not found")
			}
			if f.Version() != tt.version || f.KegOnly != tt.kegOnly || (f.LinkedKeg != nil) != tt.linked {
				t.Errorf("got %+v", f)
			}
		})
	}
	casks := []struct {
		id, version string
		apps, bins  []string
	}{
		{"claude-code", "2.1.4", nil, []string{"claude"}},
		{"stablyai/orca/orca", "1.4.0", []string{"Orca.app"}, []string{"orca"}},
		{"visual-studio-code", "1.95.3", []string{"Visual Studio Code.app"}, []string{"code"}},
	}
	for _, tt := range casks {
		t.Run(tt.id, func(t *testing.T) {
			c, ok := info.Cask(tt.id)
			if !ok {
				t.Fatal("not found")
			}
			if c.Version() != tt.version || !slices.Equal(c.Apps(), tt.apps) || !slices.Equal(c.Binaries(), tt.bins) || c.NeedsSudo() {
				t.Errorf("got %+v apps=%v bins=%v", c, c.Apps(), c.Binaries())
			}
		})
	}
	if _, ok := info.Cask("orca"); ok {
		t.Error("a tap cask must be matched by its full token only when the id has a tap")
	}
	if _, ok := info.Cask("missing"); ok {
		t.Error("missing cask found")
	}
	if _, ok := info.Formula("missing"); ok {
		t.Error("missing formula found")
	}
}

func TestParseInfoArtifacts(t *testing.T) {
	info, err := ParseInfo(fixture(t, "info-cask-pkg.json"))
	if err != nil {
		t.Fatal(err)
	}
	smp, _ := info.Cask("session-manager-plugin")
	gcloud, _ := info.Cask("gcloud-cli")
	iterm, _ := info.Cask("iterm2")
	if !smp.NeedsSudo() || !gcloud.NeedsSudo() || iterm.NeedsSudo() {
		t.Error("NeedsSudo: pkg and installer need sudo, app does not")
	}
	if smp.Version() != "" || !slices.Equal(smp.Binaries(), []string{"session-manager-plugin"}) {
		t.Errorf("smp = %+v", smp)
	}
	if !slices.Equal(iterm.Apps(), []string{"iTerm2.app"}) {
		t.Errorf("iterm apps = %v", iterm.Apps())
	}
	odd := Cask{Artifacts: nil}
	if odd.Apps() != nil {
		t.Error("no artifacts")
	}
	bad, err := ParseInfo([]byte(`{"casks":[{"token":"x","artifacts":[{"app":"notalist"},{"app":[1]},{"binary":[]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := bad.Casks[0].Apps(); got != nil {
		t.Errorf("malformed artifacts gave %v", got)
	}
	keg, err := ParseInfo(fixture(t, "info-formula-kegonly.json"))
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := keg.Formula("sqlite"); !f.KegOnly || f.Version() != "" {
		t.Errorf("sqlite = %+v", f)
	}
}

func TestParseInfoEmptyAndInvalid(t *testing.T) {
	info, err := ParseInfo(fixture(t, "info-empty.json"))
	if err != nil || len(info.Formulae)+len(info.Casks) != 0 {
		t.Errorf("empty: %+v, %v", info, err)
	}
	_, err = ParseInfo([]byte("Error: something {not json"))
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("invalid JSON err = %v", err)
	}
}

func TestParseTapsAndTrusted(t *testing.T) {
	if got := ParseTaps(string(fixture(t, "tap-list.txt"))); !slices.Equal(got, []string{"homebrew/cask", "stablyai/orca"}) {
		t.Errorf("taps = %v", got)
	}
	if got := ParseTaps("Warning: something odd\n\n"); got != nil {
		t.Errorf("odd taps = %v", got)
	}
	tests := []struct {
		out  string
		want []string
	}{
		{string(fixture(t, "trust-list.txt")), []string{"stablyai/orca/orca"}},
		{"Trusted casks:\n  stablyai/orca/orca, foo/bar/baz\n", []string{"stablyai/orca/orca", "foo/bar/baz"}},
		{"stablyai/orca\n", nil},
		{"/usr/local/bin/x\n", nil},
		{"", nil},
	}
	for _, tt := range tests {
		if got := ParseTrusted(tt.out); !slices.Equal(got, tt.want) {
			t.Errorf("ParseTrusted(%q) = %v, want %v", tt.out, got, tt.want)
		}
	}
}
