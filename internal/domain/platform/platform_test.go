package platform

import (
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/version"
)

func TestKey(t *testing.T) {
	tests := []struct {
		name string
		p    Platform
		want Key
	}{
		{"darwin", Platform{OS: "darwin"}, Darwin},
		{"linux", Platform{OS: "linux"}, Linux},
		{"wsl", Platform{OS: "linux", WSL: true}, WSL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Key(); got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGlibcBelow(t *testing.T) {
	v := func(s string) *version.Version {
		t.Helper()
		parsed, err := version.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return &parsed
	}
	tests := []struct {
		name  string
		glibc *version.Version
		want  bool
	}{
		{"2.35 is below", v("2.35"), true},
		{"2.39 is not below", v("2.39"), false},
		{"2.40 is not below", v("2.40"), false},
		{"unknown is not below", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Platform{OS: "linux", Glibc: tt.glibc}
			if got := p.GlibcBelow("2.39"); got != tt.want {
				t.Errorf("GlibcBelow = %v, want %v", got, tt.want)
			}
		})
	}
	if (Platform{Glibc: v("2.35")}).GlibcBelow("bad") {
		t.Error("an unparsable minimum must not report below")
	}
}

func TestKeyValid(t *testing.T) {
	for _, k := range []Key{Darwin, Linux, WSL} {
		if !k.Valid() {
			t.Errorf("%q should be valid", k)
		}
	}
	if Key("windows").Valid() {
		t.Error("windows should not be valid")
	}
}

func TestCommandMutates(t *testing.T) {
	tests := map[Command]bool{
		CommandScan: false, CommandPlan: false, CommandStatus: false, CommandDoctor: false,
		CommandApply: true, CommandTUIRun: true,
	}
	for c, want := range tests {
		if got := c.Mutates(); got != want {
			t.Errorf("%s.Mutates() = %v, want %v", c, got, want)
		}
	}
}

func TestBootstrapReportFatal(t *testing.T) {
	uid0 := Finding{Code: CodeUID0, Severity: SeverityFatal, Message: "running as root", Fix: "run as your user"}
	prereqs := Finding{Code: CodeLinuxPrereqs, Severity: SeverityWarning, BlocksApply: true}
	owner := Finding{Code: CodeBrewPrefixOwner, Severity: SeverityWarning, BlocksApply: true}
	glibc := Finding{Code: CodeGlibcOld, Severity: SeverityWarning}
	brew := Finding{Code: CodeBrewMissing, Severity: SeverityWarning}

	tests := []struct {
		name     string
		findings []Finding
		cmd      Command
		want     []string
	}{
		{"uid0 fatal for scan", []Finding{uid0, glibc}, CommandScan, []string{CodeUID0}},
		{"uid0 fatal for apply", []Finding{uid0}, CommandApply, []string{CodeUID0}},
		{"prereqs not fatal for scan", []Finding{prereqs, owner}, CommandScan, nil},
		{"prereqs not fatal for doctor", []Finding{prereqs, owner}, CommandDoctor, nil},
		{"prereqs fatal for apply", []Finding{prereqs, owner, glibc}, CommandApply, []string{CodeLinuxPrereqs, CodeBrewPrefixOwner}},
		{"owner fatal for tui run", []Finding{owner}, CommandTUIRun, []string{CodeBrewPrefixOwner}},
		{"brew missing never fatal", []Finding{brew}, CommandApply, nil},
		{"no findings", nil, CommandApply, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := BootstrapReport{Findings: tt.findings}
			got := r.Fatal(tt.cmd)
			if len(got) != len(tt.want) {
				t.Fatalf("Fatal(%s) = %v, want codes %v", tt.cmd, got, tt.want)
			}
			for i, f := range got {
				if f.Code != tt.want[i] {
					t.Errorf("Fatal[%d] = %s, want %s", i, f.Code, tt.want[i])
				}
			}
			if r.HasFatal(tt.cmd) != (len(tt.want) > 0) {
				t.Error("HasFatal disagrees with Fatal")
			}
		})
	}
}

func TestFindingFatalFor(t *testing.T) {
	f := Finding{Severity: SeverityInfo}
	if f.FatalFor(CommandApply) {
		t.Error("info finding must not be fatal")
	}
}
