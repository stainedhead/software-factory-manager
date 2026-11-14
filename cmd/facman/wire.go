package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/adapter/brew"
	"github.com/stainedhead/software-factory-manager/internal/adapter/catalogyaml"
	"github.com/stainedhead/software-factory-manager/internal/adapter/cli"
	"github.com/stainedhead/software-factory-manager/internal/adapter/detect"
	"github.com/stainedhead/software-factory-manager/internal/adapter/execrunner"
	"github.com/stainedhead/software-factory-manager/internal/adapter/fsprobe"
	"github.com/stainedhead/software-factory-manager/internal/adapter/installer"
	platformprobe "github.com/stainedhead/software-factory-manager/internal/adapter/platform"
	"github.com/stainedhead/software-factory-manager/internal/adapter/session"
	"github.com/stainedhead/software-factory-manager/internal/adapter/sqlite"
	"github.com/stainedhead/software-factory-manager/internal/adapter/statedir"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/usecase/bootstrap"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

// options are the environment facts and streams facman runs with. main
// uses defaultOptions(); the end-to-end tests change them to fake a
// machine (R-3, AD-15). They are never set from flags or variables.
type options struct {
	// Stdin, Stdout and Stderr are the process streams.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Environ is the base environment (os.Environ()).
	Environ []string
	// GOOS and GOARCH name the platform (runtime values).
	GOOS, GOARCH string
	// UID is the invoking user's UID.
	UID func() int
	// OwnerUID returns the owner of a path (Homebrew prefix check); nil
	// means the file system's.
	OwnerUID func(path string) (int, error)
	// KnownPrefixes are where Homebrew is looked for off PATH.
	KnownPrefixes []string
	// AppDirs are scanned for app bundles when Spotlight finds nothing.
	AppDirs []string
	// AppInstallDir is where casks install apps (/Applications).
	AppInstallDir string
	// ShimDir holds the macOS CLT shims (/usr/bin).
	ShimDir string
	// ProcVersionPath and OSReleasePath are the Linux platform files.
	ProcVersionPath, OSReleasePath string
	// InstallerURL is the Homebrew installer; HTTPClient fetches it.
	InstallerURL string
	HTTPClient   *http.Client
	// TempDir holds the downloaded installer.
	TempDir string
	// Now is the clock; User names the OS user.
	Now  func() time.Time
	User func() string
	// Signals delivers SIGINT/SIGTERM during apply.
	Signals func() (<-chan os.Signal, func())
	// DetectorTimeout bounds each detector (zero: 10 s).
	DetectorTimeout time.Duration
	// TUI opens the TUI; nil until workstream G.
	TUI cli.TUIRunner
}

// defaultOptions are the real machine.
func defaultOptions() options {
	return options{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Environ: os.Environ(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, UID: os.Getuid,
		Now:  time.Now,
		User: execrunner.SystemUser{}.Username,
		Signals: func() (<-chan os.Signal, func()) {
			ch := make(chan os.Signal, 4)
			signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
			return ch, func() { signal.Stop(ch) }
		},
	}
}

// getenv reads a variable from a KEY=VALUE list.
func getenv(env []string) func(string) string {
	return func(key string) string {
		for i := len(env) - 1; i >= 0; i-- {
			if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
				return v
			}
		}
		return ""
	}
}

type clock func() time.Time

func (c clock) Now() time.Time { return c() }

type userName func() string

func (u userName) Username() string { return u() }

// newApp builds every adapter from o, injects them into the use cases (via
// the session) and returns the CLI. Wiring only: no logic.
func newApp(o options) *cli.App {
	env := getenv(o.Environ)
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.User == nil {
		o.User = execrunner.SystemUser{Getenv: env}.Username
	}
	if o.GOOS == "" {
		o.GOOS, o.GOARCH = runtime.GOOS, runtime.GOARCH
	}
	if o.UID == nil {
		o.UID = os.Getuid
	}
	if o.KnownPrefixes == nil {
		o.KnownPrefixes = brew.DefaultPrefixes(o.GOOS)
	}
	home := env("HOME")
	runner := execrunner.New(execrunner.Options{BaseEnv: o.Environ, Stdin: o.Stdin, Stdout: o.Stdout, Stderr: o.Stderr})
	machine := &session.Machine{Home: home}
	target := func() brew.Target {
		b := machine.Brew()
		return brew.Target{Path: b.Path, Prefix: b.Prefix, Env: machine.Env()}
	}
	formula := brew.NewFormula(runner, brew.BackendOptions{Target: target, AppDir: o.AppInstallDir})
	cask := brew.NewCask(runner, brew.BackendOptions{Target: target, AppDir: o.AppInstallDir})
	boot := &bootstrap.Service{
		Probe: platformprobe.New(runner, platformprobe.Options{GOOS: o.GOOS, GOARCH: o.GOARCH, ProcVersionPath: o.ProcVersionPath,
			OSReleasePath: o.OSReleasePath, Getenv: env, UID: o.UID}),
		Homebrew: brew.NewLocator(runner, brew.LocatorOptions{KnownPrefixes: o.KnownPrefixes, Getenv: env, UID: o.UID, OwnerUID: o.OwnerUID}),
		Runner:   runner, Look: runner, Home: home,
	}
	clk := clock(o.Now)
	sess := session.New(session.Deps{
		Bootstrap: boot,
		OpenState: func(ctx context.Context) (*session.State, error) {
			root, err := statedir.Resolve(o.GOOS, env)
			if err != nil {
				return nil, err
			}
			dir, err := statedir.Open(root)
			if err != nil {
				return nil, err
			}
			store, err := sqlite.Open(ctx, dir.DBPath(), o.Now)
			if err != nil {
				return nil, err
			}
			return &session.State{Dir: dir, Inventory: store, Close: store.Close}, nil
		},
		LoadCatalog: func(ctx context.Context, dir string) (catalog.Catalog, error) {
			if dir == "" {
				return catalogyaml.Embedded(version).Load(ctx)
			}
			src, err := catalogyaml.Dir(dir)
			if err != nil {
				return catalog.Catalog{}, err
			}
			return src.Load(ctx)
		},
		Detectors: []scan.Detector{
			detect.NewPathDetector(runner, detect.PathOptions{ShimDir: o.ShimDir}),
			brew.NewDetector(runner),
			detect.NewBundleDetector(runner, detect.BundleOptions{AppDirs: o.AppDirs}),
			detect.NewNPMDetector(runner),
		},
		Backends:  map[catalog.BackendID]session.Backend{catalog.BackendBrewFormula: formula, catalog.BackendBrewCask: cask},
		Probe:     fsprobe.New(runner, fsprobe.Options{Locations: machine.Locations, Env: machine.Env}),
		Runner:    runner,
		Installer: installer.New(installer.Options{URL: o.InstallerURL, Client: o.HTTPClient, TempDir: o.TempDir}),
		Clock:     clk, User: userName(o.User), Machine: machine, Home: home, BasePath: env("PATH"),
		DetectorTimeout: o.DetectorTimeout,
	})
	return &cli.App{Engine: sess, TUI: o.TUI, Stdio: cli.Stdio{In: o.Stdin, Out: o.Stdout, Err: o.Stderr},
		Version: version, Now: o.Now, Signals: o.Signals}
}
