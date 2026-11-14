package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/domain/version"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// probeTimeout bounds xcode-select -p and node --version.
const probeTimeout = 5 * time.Second

// NodeMinVersion is the Node.js version that satisfies the check (FR-052).
const NodeMinVersion = "24.11"

// NodeKeg is the keg-only Node formula whose bin goes first on the child
// PATH instead of being linked (D-3).
const NodeKeg = "node@24"

// GlibcMin is the glibc Homebrew bottles need on Linux.
const GlibcMin = "2.39"

// Service runs the startup bootstrap check (FR-050).
type Service struct {
	// Probe reports the platform.
	Probe PlatformProbe
	// Homebrew finds brew and reads shellenv.
	Homebrew HomebrewLocator
	// Runner runs xcode-select -p and node --version.
	Runner CommandRunner
	// Look finds prerequisite programs, node and npm on the base PATH.
	Look PathLookup
	// Home is the user's home directory (Node provenance).
	Home string
}

// linuxPrereqs maps each program Homebrew needs to the apt and dnf
// packages that provide it.
var linuxPrereqs = []struct{ program, apt, dnf string }{
	{"gcc", "build-essential", "gcc"},
	{"ps", "procps", "procps-ng"},
	{"curl", "curl", "curl"},
	{"file", "file", "file"},
	{"git", "git", "git"},
}

// Check builds the BootstrapReport: platform, UID (0 is fatal and stops
// the check), Xcode CLT on macOS, Linux prerequisites, glibc and display,
// Homebrew (location, prefix owner, shellenv) and Node/npm. It never
// installs anything. Only a platform probe or Homebrew lookup that fails
// outright is an error (bootstrap kind, exit 4).
func (s *Service) Check(ctx context.Context) (platform.BootstrapReport, error) {
	p, err := s.Probe.Probe(ctx)
	if err != nil {
		return platform.BootstrapReport{}, failure.Bootstrap("probing the platform: %w", err)
	}
	r := platform.BootstrapReport{Platform: p}
	add := func(f platform.Finding) { r.Findings = append(r.Findings, f) }
	if p.UID == 0 {
		add(platform.Finding{Code: platform.CodeUID0, Severity: platform.SeverityFatal,
			Message: "facman is running as root (UID 0); Homebrew refuses to run as root",
			Fix:     "run facman as your normal user, without sudo; it asks for sudo itself only where a step needs it"})
		return r, nil
	}
	switch p.OS {
	case "darwin":
		s.checkCLT(ctx, &r)
	case "linux":
		s.checkLinux(&r)
	}
	if err := s.checkHomebrew(ctx, &r); err != nil {
		return r, err
	}
	s.checkNode(ctx, &r)
	return r, nil
}

func (s *Service) checkCLT(ctx context.Context, r *platform.BootstrapReport) {
	res, err := s.Runner.Run(ctx, process.Cmd{Argv: []string{"xcode-select", "-p"}, Timeout: probeTimeout})
	ok := err == nil && res.Success()
	r.CLT = &ok
	if !ok {
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeCLTMissing, Severity: platform.SeverityWarning,
			Message: "Xcode Command Line Tools are not installed; /usr/bin/git is not used",
			Fix:     "the Homebrew installer installs them; otherwise run xcode-select --install (bottled installs work without them)"})
	}
}

func (s *Service) checkLinux(r *platform.BootstrapReport) {
	p := r.Platform
	for _, pre := range linuxPrereqs {
		if _, err := s.Look.LookPath(pre.program); err != nil {
			r.LinuxPrereqsMissing = append(r.LinuxPrereqsMissing, pre.program)
		}
	}
	if len(r.LinuxPrereqsMissing) > 0 {
		f := platform.Finding{Code: platform.CodeLinuxPrereqs, Severity: platform.SeverityWarning, BlocksApply: true,
			Message: "Homebrew needs these programs, which are missing: " + strings.Join(r.LinuxPrereqsMissing, ", ")}
		switch p.Distro.PkgManager {
		case platform.PkgManagerApt:
			r.PrereqCommand = []string{"sudo", "apt-get", "install", "-y"}
			for _, pre := range linuxPrereqs {
				r.PrereqCommand = append(r.PrereqCommand, pre.apt)
			}
			f.Fix = "run: sudo apt-get update && " + strings.Join(r.PrereqCommand, " ")
		case platform.PkgManagerDnf:
			r.PrereqCommand = []string{"sudo", "dnf", "install", "-y"}
			for _, pre := range linuxPrereqs {
				r.PrereqCommand = append(r.PrereqCommand, pre.dnf)
			}
			f.Fix = "run: " + strings.Join(r.PrereqCommand, " ")
		default:
			f.Fix = "install a C compiler, procps, curl, file and git with your distribution's package manager, then run facman again"
		}
		r.Findings = append(r.Findings, f)
	}
	switch {
	case p.Glibc == nil:
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeGlibcUnknown, Severity: platform.SeverityWarning,
			Message: "the glibc version could not be determined (musl or an unusual system); Homebrew bottles may not work",
			Fix:     "use a glibc-based distribution with glibc " + GlibcMin + " or newer"})
	case p.GlibcBelow(GlibcMin):
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeGlibcOld, Severity: platform.SeverityWarning,
			Message: fmt.Sprintf("glibc %s is older than %s; Homebrew may build from source or fail", p.Glibc.Raw, GlibcMin),
			Fix:     "upgrade to a distribution with glibc " + GlibcMin + " or newer (for example Ubuntu 24.04)"})
	}
	if !p.WSL && !p.Display {
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeNoDisplay, Severity: platform.SeverityInfo,
			Message: "no display: GUI apps are skipped on this machine"})
	}
}

func (s *Service) checkHomebrew(ctx context.Context, r *platform.BootstrapReport) error {
	info, err := s.Homebrew.Locate(ctx)
	if err != nil {
		return failure.Bootstrap("locating Homebrew: %w", err)
	}
	r.Homebrew = info
	if !info.Found {
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeBrewMissing, Severity: platform.SeverityWarning,
			Message: "Homebrew is not installed",
			Fix:     "facman apply (or the TUI) installs it with the official installer as the first reviewed step, after you consent"})
		return nil
	}
	if !info.OnPATH {
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeBrewNotOnPath, Severity: platform.SeverityInfo,
			Message: "Homebrew was found at " + info.Prefix + " but is not on PATH; facman uses it through brew shellenv",
			Fix:     fmt.Sprintf(`add eval "$(%s shellenv)" to your shell profile`, info.Path)})
	}
	if info.PrefixOwnedByOther {
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeBrewPrefixOwner, Severity: platform.SeverityWarning, BlocksApply: true,
			Message: "the Homebrew prefix " + info.Prefix + " belongs to another user",
			Fix:     "run facman as the user who owns " + info.Prefix + "; multi-user Homebrew is not supported"})
	}
	env, err := s.Homebrew.ShellEnv(ctx, info.Path)
	if err != nil {
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeBrewShellenv, Severity: platform.SeverityWarning,
			Message: "brew shellenv could not be read: " + err.Error(),
			Fix:     "run " + info.Path + " shellenv yourself to see the error"})
		return nil
	}
	r.ShellEnv = env
	return nil
}

var nodeVersionRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+\S*)`)

func (s *Service) checkNode(ctx context.Context, r *platform.BootstrapReport) {
	lookup := func(name string) string {
		if p, err := s.Look.LookPath(name); err == nil {
			return p
		}
		if r.Homebrew.Prefix != "" {
			if p, err := s.Look.LookPath(filepath.Join(r.Homebrew.Prefix, "opt", NodeKeg, "bin", name)); err == nil {
				return p
			}
		}
		return ""
	}
	r.Node.NPM = lookup("npm") != ""
	node := lookup("node")
	if node == "" {
		return
	}
	r.Node.Found = true
	r.Node.Provenance = string(catalog.GenericProvenance(node, catalog.Locations{BrewPrefix: r.Homebrew.Prefix, Home: s.Home}))
	res, err := s.Runner.Run(ctx, process.Cmd{Argv: []string{node, "--version"}, Env: ChildEnv(*r, ""), Timeout: probeTimeout})
	if err == nil && res.Success() {
		if m := nodeVersionRe.FindStringSubmatch(res.Stdout); m != nil {
			r.Node.Version = m[1]
		}
	}
	r.Node.Satisfies = version.Satisfies(r.Node.Version, NodeMinVersion)
	if !r.Node.Satisfies {
		r.Findings = append(r.Findings, platform.Finding{Code: platform.CodeNodeOld, Severity: platform.SeverityInfo,
			Message: fmt.Sprintf("Node.js %s (%s) is below %s; npm-backend packages need %s or newer", orUnknown(r.Node.Version), r.Node.Provenance, NodeMinVersion, NodeMinVersion),
			Fix:     "the base-dev profile installs " + NodeKeg + " through Homebrew; facman does not replace a Node it did not install"})
	}
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown version"
	}
	return v
}

// ChildEnv returns the environment overrides every child process gets:
// the brew shellenv variables, with {prefix}/opt/node@24/bin put first on
// PATH (D-3). basePath is used when shellenv set no PATH. Nil when there
// is nothing to override.
func ChildEnv(r platform.BootstrapReport, basePath string) map[string]string {
	env := map[string]string{}
	for k, v := range r.ShellEnv {
		env[k] = v
	}
	path := env["PATH"]
	if path == "" {
		path = basePath
	}
	if r.Homebrew.Prefix != "" {
		keg := filepath.Join(r.Homebrew.Prefix, "opt", NodeKeg, "bin")
		if path == "" {
			path = keg
		} else {
			path = keg + string(filepath.ListSeparator) + path
		}
	}
	if path != "" {
		env["PATH"] = path
	}
	if len(env) == 0 {
		return nil
	}
	return env
}
