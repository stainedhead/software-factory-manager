// Package platform probes the machine: OS and arch, WSL, display, glibc
// version, Linux distribution and UID. Every file path and command name is
// a constructor option, so tests run against fixtures and the fake command.
package platform

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	domain "github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/domain/version"
	"github.com/stainedhead/software-factory-manager/internal/usecase/bootstrap"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// probeTimeout bounds each getconf/ldd call.
const probeTimeout = 5 * time.Second

// Options configure a Probe. Zero values take the real defaults.
type Options struct {
	// GOOS and GOARCH default to runtime.GOOS and runtime.GOARCH.
	GOOS, GOARCH string
	// ProcVersionPath defaults to /proc/version.
	ProcVersionPath string
	// OSReleasePath defaults to /etc/os-release.
	OSReleasePath string
	// GetconfCmd and LddCmd default to getconf and ldd.
	GetconfCmd, LddCmd string
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
	// UID defaults to os.Getuid.
	UID func() int
	// ReadFile defaults to os.ReadFile.
	ReadFile func(string) ([]byte, error)
}

// Probe implements bootstrap.PlatformProbe.
type Probe struct {
	opts   Options
	runner bootstrap.CommandRunner
}

var _ bootstrap.PlatformProbe = (*Probe)(nil)

// New returns a Probe that runs getconf and ldd through runner.
func New(runner bootstrap.CommandRunner, opts Options) *Probe {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	if opts.ProcVersionPath == "" {
		opts.ProcVersionPath = "/proc/version"
	}
	if opts.OSReleasePath == "" {
		opts.OSReleasePath = "/etc/os-release"
	}
	if opts.GetconfCmd == "" {
		opts.GetconfCmd = "getconf"
	}
	if opts.LddCmd == "" {
		opts.LddCmd = "ldd"
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.UID == nil {
		opts.UID = os.Getuid
	}
	if opts.ReadFile == nil {
		opts.ReadFile = os.ReadFile
	}
	return &Probe{opts: opts, runner: runner}
}

// Probe reports the platform facts. Missing files and commands mean
// "unknown" (no WSL, unknown distro, unknown glibc), never an error; an
// unreadable file that exists is an error.
func (p *Probe) Probe(ctx context.Context) (domain.Platform, error) {
	o := p.opts
	pl := domain.Platform{OS: o.GOOS, Arch: o.GOARCH, UID: o.UID()}
	if o.GOOS == "darwin" {
		pl.Display = true
		return pl, nil
	}
	procVersion, err := p.read(o.ProcVersionPath)
	if err != nil {
		return pl, err
	}
	pl.WSL = o.Getenv("WSL_DISTRO_NAME") != "" || strings.Contains(strings.ToLower(string(procVersion)), "microsoft")
	pl.Display = o.Getenv("DISPLAY") != "" || o.Getenv("WAYLAND_DISPLAY") != ""
	osRelease, err := p.read(o.OSReleasePath)
	if err != nil {
		return pl, err
	}
	pl.Distro = ParseOSRelease(osRelease)
	pl.Glibc = p.glibc(ctx)
	return pl, nil
}

func (p *Probe) read(path string) ([]byte, error) {
	b, err := p.opts.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return b, nil
}

// glibc asks getconf, then ldd; nil when neither answers (musl, missing).
func (p *Probe) glibc(ctx context.Context) *version.Version {
	if out, ok := p.run(ctx, p.opts.GetconfCmd, "GNU_LIBC_VERSION"); ok {
		// "glibc 2.39"
		if f := strings.Fields(out); len(f) == 2 && f[0] == "glibc" {
			if v, err := version.Parse(f[1]); err == nil {
				return &v
			}
		}
	}
	if out, ok := p.run(ctx, p.opts.LddCmd, "--version"); ok {
		// "ldd (Ubuntu GLIBC 2.39-0ubuntu8) 2.39"
		first, _, _ := strings.Cut(out, "\n")
		lower := strings.ToLower(first)
		if strings.Contains(lower, "glibc") || strings.Contains(lower, "gnu libc") {
			if f := strings.Fields(first); len(f) > 0 {
				if v, err := version.Parse(f[len(f)-1]); err == nil {
					return &v
				}
			}
		}
	}
	return nil
}

func (p *Probe) run(ctx context.Context, argv ...string) (string, bool) {
	res, err := p.runner.Run(ctx, process.Cmd{Argv: argv, Timeout: probeTimeout})
	if err != nil || !res.Success() {
		return "", false
	}
	return strings.TrimSpace(res.Stdout), true
}

// ParseOSRelease reads ID and ID_LIKE from /etc/os-release content and
// derives the package manager: debian or ubuntu → apt; fedora, rhel or
// centos → dnf; anything else → "" (unknown).
func ParseOSRelease(b []byte) domain.Distro {
	var d domain.Distro
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			d.ID = strings.ToLower(v)
		case "ID_LIKE":
			d.IDLike = strings.Fields(strings.ToLower(v))
		}
	}
	family := append([]string{d.ID}, d.IDLike...)
	switch {
	case slices.Contains(family, "debian") || slices.Contains(family, "ubuntu"):
		d.PkgManager = domain.PkgManagerApt
	case slices.Contains(family, "fedora") || slices.Contains(family, "rhel") || slices.Contains(family, "centos"):
		d.PkgManager = domain.PkgManagerDnf
	}
	return d
}
