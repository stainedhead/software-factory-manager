package bootstrap

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/domain/version"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

type probe struct {
	p   platform.Platform
	err error
}

func (f probe) Probe(context.Context) (platform.Platform, error) { return f.p, f.err }

type locator struct {
	info   platform.HomebrewInfo
	err    error
	env    map[string]string
	envErr error
}

func (f locator) Locate(context.Context) (platform.HomebrewInfo, error) { return f.info, f.err }
func (f locator) ShellEnv(context.Context, string) (map[string]string, error) {
	return f.env, f.envErr
}

// runner answers by argv[0] and records calls.
type runner struct {
	out   map[string]process.Result
	calls []process.Cmd
}

func (r *runner) Run(_ context.Context, c process.Cmd) (process.Result, error) {
	r.calls = append(r.calls, c)
	if res, ok := r.out[c.Argv[0]]; ok {
		return res, nil
	}
	return process.Result{ExitCode: -1}, process.ErrNotFound
}

type look map[string]string

func (l look) LookPath(name string) (string, error) {
	if p, ok := l[name]; ok {
		return p, nil
	}
	return "", process.ErrNotFound
}

func glibc(s string) *version.Version {
	v, _ := version.Parse(s)
	return &v
}

var (
	allPrereqs = look{"gcc": "/usr/bin/gcc", "ps": "/usr/bin/ps", "curl": "/usr/bin/curl", "file": "/usr/bin/file", "git": "/usr/bin/git"}
	brewOK     = locator{info: platform.HomebrewInfo{Found: true, Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew", OnPATH: true},
		env: map[string]string{"HOMEBREW_PREFIX": "/opt/homebrew", "PATH": "/opt/homebrew/bin:/usr/bin"}}
	darwin = platform.Platform{OS: "darwin", Arch: "arm64", Display: true, UID: 501}
	ubuntu = platform.Platform{OS: "linux", Arch: "amd64", Display: true, UID: 1000, Glibc: glibc("2.39"),
		Distro: platform.Distro{ID: "ubuntu", PkgManager: platform.PkgManagerApt}}
)

func codes(r platform.BootstrapReport) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.Code)
	}
	return out
}

func finding(r platform.BootstrapReport, code string) platform.Finding {
	for _, f := range r.Findings {
		if f.Code == code {
			return f
		}
	}
	return platform.Finding{}
}

func with(p platform.Platform, f func(*platform.Platform)) platform.Platform {
	f(&p)
	return p
}

func TestCheck(t *testing.T) {
	cltOK := map[string]process.Result{"xcode-select": {Stdout: "/Library/Developer/CommandLineTools\n"}}
	cltMissing := map[string]process.Result{"xcode-select": {ExitCode: 2, Stderr: "error: unable to get active developer directory"}}
	tests := []struct {
		name      string
		platform  platform.Platform
		brew      locator
		look      look
		out       map[string]process.Result
		wantCodes []string
		check     func(t *testing.T, r platform.BootstrapReport)
	}{
		{name: "uid 0 is fatal and stops", platform: with(darwin, func(p *platform.Platform) { p.UID = 0 }), brew: brewOK,
			wantCodes: []string{platform.CodeUID0},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if !r.HasFatal(platform.CommandScan) || finding(r, platform.CodeUID0).Fix == "" || r.Homebrew.Found {
					t.Errorf("uid0 report = %+v", r)
				}
			}},
		{name: "darwin clean", platform: darwin, brew: brewOK, look: look{}, out: cltOK,
			check: func(t *testing.T, r platform.BootstrapReport) {
				if r.CLT == nil || !*r.CLT || r.ShellEnv["HOMEBREW_PREFIX"] != "/opt/homebrew" {
					t.Errorf("report = %+v", r)
				}
			}},
		{name: "darwin CLT missing warns", platform: darwin, brew: brewOK, look: look{}, out: cltMissing,
			wantCodes: []string{platform.CodeCLTMissing},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if r.CLT == nil || *r.CLT || r.HasFatal(platform.CommandApply) {
					t.Error("CLT missing is a warning only")
				}
			}},
		{name: "brew missing warns, never fatal", platform: darwin, brew: locator{}, look: look{}, out: cltOK,
			wantCodes: []string{platform.CodeBrewMissing},
			check: func(t *testing.T, r platform.BootstrapReport) {
				for _, c := range []platform.Command{platform.CommandScan, platform.CommandPlan, platform.CommandApply} {
					if r.HasFatal(c) {
						t.Errorf("brew missing fatal for %s", c)
					}
				}
			}},
		{name: "brew at prefix not on PATH uses shellenv", platform: darwin, out: cltOK, look: look{},
			brew:      locator{info: platform.HomebrewInfo{Found: true, Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew"}, env: map[string]string{"PATH": "/opt/homebrew/bin"}},
			wantCodes: []string{platform.CodeBrewNotOnPath},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if r.ShellEnv["PATH"] != "/opt/homebrew/bin" {
					t.Errorf("shellenv = %v", r.ShellEnv)
				}
			}},
		{name: "shellenv failure warns", platform: darwin, out: cltOK, look: look{},
			brew:      locator{info: brewOK.info, envErr: errors.New("boom")},
			wantCodes: []string{platform.CodeBrewShellenv}},
		{name: "prefix owned by another user blocks apply", platform: darwin, out: cltOK, look: look{},
			brew:      locator{info: platform.HomebrewInfo{Found: true, Path: "/opt/homebrew/bin/brew", Prefix: "/opt/homebrew", OnPATH: true, PrefixOwnedByOther: true}},
			wantCodes: []string{platform.CodeBrewPrefixOwner},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if r.HasFatal(platform.CommandScan) || !r.HasFatal(platform.CommandApply) || !r.HasFatal(platform.CommandTUIRun) {
					t.Error("prefix owner: fatal for apply only")
				}
			}},
		{name: "ubuntu clean", platform: ubuntu, brew: brewOK, look: allPrereqs},
		{name: "ubuntu prerequisites missing", platform: ubuntu, brew: brewOK, look: look{"curl": "/usr/bin/curl", "git": "/usr/bin/git"},
			wantCodes: []string{platform.CodeLinuxPrereqs},
			check: func(t *testing.T, r platform.BootstrapReport) {
				f := finding(r, platform.CodeLinuxPrereqs)
				if f.Fix != "run: sudo apt-get update && sudo apt-get install -y build-essential procps curl file git" {
					t.Errorf("fix = %q", f.Fix)
				}
				if !slices.Equal(r.LinuxPrereqsMissing, []string{"gcc", "ps", "file"}) || r.PrereqCommand[1] != "apt-get" {
					t.Errorf("missing = %v, cmd = %v", r.LinuxPrereqsMissing, r.PrereqCommand)
				}
				if r.HasFatal(platform.CommandScan) || !r.HasFatal(platform.CommandApply) {
					t.Error("prereqs: fatal for apply only")
				}
			}},
		{name: "fedora uses dnf", platform: with(ubuntu, func(p *platform.Platform) {
			p.Distro = platform.Distro{ID: "fedora", PkgManager: platform.PkgManagerDnf}
		}),
			brew: brewOK, look: look{}, wantCodes: []string{platform.CodeLinuxPrereqs},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if f := finding(r, platform.CodeLinuxPrereqs); f.Fix != "run: sudo dnf install -y gcc procps-ng curl file git" {
					t.Errorf("fix = %q", f.Fix)
				}
			}},
		{name: "unknown distro with missing prereqs blocks apply", platform: with(ubuntu, func(p *platform.Platform) { p.Distro = platform.Distro{ID: "alpine"} }),
			brew: brewOK, look: look{}, wantCodes: []string{platform.CodeLinuxPrereqs},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if !r.HasFatal(platform.CommandApply) || r.PrereqCommand != nil || !strings.Contains(finding(r, platform.CodeLinuxPrereqs).Fix, "package manager") {
					t.Errorf("report = %+v", r)
				}
			}},
		{name: "glibc 2.35 warns", platform: with(ubuntu, func(p *platform.Platform) { p.Glibc = glibc("2.35") }), brew: brewOK, look: allPrereqs,
			wantCodes: []string{platform.CodeGlibcOld}},
		{name: "glibc unknown warns", platform: with(ubuntu, func(p *platform.Platform) { p.Glibc = nil }), brew: brewOK, look: allPrereqs,
			wantCodes: []string{platform.CodeGlibcUnknown}},
		{name: "headless linux is info", platform: with(ubuntu, func(p *platform.Platform) { p.Display = false }), brew: brewOK, look: allPrereqs,
			wantCodes: []string{platform.CodeNoDisplay}},
		{name: "wsl without display has no display finding", platform: with(ubuntu, func(p *platform.Platform) { p.Display, p.WSL = false, true }), brew: brewOK, look: allPrereqs},
		{name: "node 24.11 via nvm satisfies", platform: darwin, brew: brewOK, out: map[string]process.Result{"xcode-select": {}, "/Users/j/.nvm/versions/node/v24.11.0/bin/node": {Stdout: "v24.11.0\n"}},
			look: look{"node": "/Users/j/.nvm/versions/node/v24.11.0/bin/node", "npm": "/Users/j/.nvm/versions/node/v24.11.0/bin/npm"},
			check: func(t *testing.T, r platform.BootstrapReport) {
				want := platform.NodeInfo{Found: true, Version: "24.11.0", Provenance: "nvm", Satisfies: true, NPM: true}
				if r.Node != want {
					t.Errorf("node = %+v, want %+v", r.Node, want)
				}
			}},
		{name: "node 22 is info", platform: darwin, brew: brewOK, out: map[string]process.Result{"xcode-select": {}, "/usr/local/bin/node": {Stdout: "v22.1.0\n"}},
			look: look{"node": "/usr/local/bin/node"}, wantCodes: []string{platform.CodeNodeOld},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if r.Node.Satisfies || r.Node.NPM || r.Node.Provenance != "unknown" {
					t.Errorf("node = %+v", r.Node)
				}
			}},
		{name: "keg-only node@24 found under the prefix", platform: darwin, brew: brewOK,
			out:  map[string]process.Result{"xcode-select": {}, "/opt/homebrew/opt/node@24/bin/node": {Stdout: "v24.11.1\n"}},
			look: look{"/opt/homebrew/opt/node@24/bin/node": "/opt/homebrew/opt/node@24/bin/node", "/opt/homebrew/opt/node@24/bin/npm": "/opt/homebrew/opt/node@24/bin/npm"},
			check: func(t *testing.T, r platform.BootstrapReport) {
				if !r.Node.Satisfies || r.Node.Provenance != "brew" || !r.Node.NPM {
					t.Errorf("node = %+v", r.Node)
				}
			}},
		{name: "node version unreadable", platform: darwin, brew: brewOK, out: map[string]process.Result{"xcode-select": {}, "/x/node": {ExitCode: 1}},
			look: look{"node": "/x/node"}, wantCodes: []string{platform.CodeNodeOld}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runner{out: tt.out}
			s := &Service{Probe: probe{p: tt.platform}, Homebrew: tt.brew, Runner: r, Look: tt.look, Home: "/Users/j"}
			rep, err := s.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got := codes(rep); !slices.Equal(got, tt.wantCodes) {
				t.Errorf("codes = %v, want %v", got, tt.wantCodes)
			}
			if tt.check != nil {
				tt.check(t, rep)
			}
			for _, c := range r.calls {
				if c.Timeout == 0 {
					t.Errorf("%v has no timeout", c.Argv)
				}
			}
		})
	}
}

func TestCheckErrors(t *testing.T) {
	s := &Service{Probe: probe{err: errors.New("no /proc")}}
	if _, err := s.Check(context.Background()); failure.ExitCode(err) != 4 {
		t.Errorf("probe error = %v", err)
	}
	s = &Service{Probe: probe{p: darwin}, Homebrew: locator{err: errors.New("stat failed")}, Runner: &runner{}, Look: look{}}
	if _, err := s.Check(context.Background()); failure.ExitCode(err) != 4 {
		t.Errorf("locate error = %v", err)
	}
}

func TestChildEnv(t *testing.T) {
	tests := []struct {
		name string
		r    platform.BootstrapReport
		base string
		want map[string]string
	}{
		{"nothing", platform.BootstrapReport{}, "", nil},
		{"base path only", platform.BootstrapReport{}, "/usr/bin", map[string]string{"PATH": "/usr/bin"}},
		{"shellenv and node keg", platform.BootstrapReport{Homebrew: platform.HomebrewInfo{Prefix: "/opt/homebrew"},
			ShellEnv: map[string]string{"HOMEBREW_PREFIX": "/opt/homebrew", "PATH": "/opt/homebrew/bin:/usr/bin"}}, "/ignored",
			map[string]string{"HOMEBREW_PREFIX": "/opt/homebrew", "PATH": "/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin"}},
		{"prefix without shellenv", platform.BootstrapReport{Homebrew: platform.HomebrewInfo{Prefix: "/p"}}, "",
			map[string]string{"PATH": "/p/opt/node@24/bin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChildEnv(tt.r, tt.base)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// TestInheritedHomebrewEnv: the names (never the values) of inherited
// HOMEBREW_* variables are reported, and HOMEBREW_CASK_OPTS with
// --no-quarantine is a warning (review FR-013).
func TestInheritedHomebrewEnv(t *testing.T) {
	tests := []struct {
		name     string
		environ  []string
		wantEnv  []string
		wantWarn bool
	}{
		{"none", []string{"PATH=/usr/bin", "HOME=/Users/j"}, nil, false},
		{"names sorted, tap-trust bypass left out", []string{"HOMEBREW_NO_INSTALL_FROM_API=1", "HOMEBREW_CASK_OPTS=--appdir=~/Apps", "HOMEBREW_NO_REQUIRE_TAP_TRUST=1", "PATH=/usr/bin"},
			[]string{"HOMEBREW_CASK_OPTS", "HOMEBREW_NO_INSTALL_FROM_API"}, false},
		{"no-quarantine", []string{"HOMEBREW_CASK_OPTS=--appdir=/Applications --no-quarantine"}, []string{"HOMEBREW_CASK_OPTS"}, true},
		{"no-quarantine alone", []string{"HOMEBREW_CASK_OPTS=--no-quarantine"}, []string{"HOMEBREW_CASK_OPTS"}, true},
		{"other variable mentions it", []string{"HOMEBREW_GITHUB_API_TOKEN=--no-quarantine"}, []string{"HOMEBREW_GITHUB_API_TOKEN"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{Probe: probe{p: darwin}, Homebrew: brewOK, Runner: &runner{}, Look: look{}, Home: "/Users/j",
				Environ: func() []string { return tt.environ }}
			r, err := s.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(r.Homebrew.InheritedEnv, tt.wantEnv) {
				t.Errorf("InheritedEnv = %v, want %v", r.Homebrew.InheritedEnv, tt.wantEnv)
			}
			var warn *platform.Finding
			for i, f := range r.Findings {
				if f.Code == platform.CodeHomebrewCaskOpts {
					warn = &r.Findings[i]
				}
			}
			if (warn != nil) != tt.wantWarn {
				t.Fatalf("homebrew-cask-opts finding = %+v, want %v", warn, tt.wantWarn)
			}
			if warn != nil && (warn.Severity != platform.SeverityWarning || !strings.Contains(warn.Message, "HOMEBREW_CASK_OPTS") || !strings.Contains(warn.Fix, "unset HOMEBREW_CASK_OPTS")) {
				t.Errorf("finding = %+v", warn)
			}
			for _, f := range r.Findings {
				if strings.Contains(f.Message+f.Fix, "~/Apps") || strings.Contains(f.Message+f.Fix, "/Applications") {
					t.Errorf("a finding shows a value: %+v", f)
				}
			}
		})
	}
}
