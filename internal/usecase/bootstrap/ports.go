// Package bootstrap builds the startup BootstrapReport (platform, UID, Xcode
// CLT, Linux prerequisites, Homebrew location and owner, shellenv, Node) and
// decides which findings are fatal for a command. This file declares the
// ports it consumes; the use case itself lands in workstream E (T-E2).
package bootstrap

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// CommandRunner runs read-only probes (xcode-select -p, node --version).
// Implemented by adapter/execrunner.
type CommandRunner interface {
	Run(ctx context.Context, cmd process.Cmd) (process.Result, error)
}

// PlatformProbe reports OS, arch, WSL, display, glibc, distro and UID.
// Implemented by adapter/platform.
type PlatformProbe interface {
	Probe(ctx context.Context) (platform.Platform, error)
}

// HomebrewLocator finds brew on PATH or at a known prefix and evaluates
// `brew shellenv` without a shell. Implemented by adapter/brew.
type HomebrewLocator interface {
	Locate(ctx context.Context) (platform.HomebrewInfo, error)
	ShellEnv(ctx context.Context, brewPath string) (map[string]string, error)
}

// PathLookup finds an executable on the child PATH (gcc, file, curl, git,
// node, npm). Implemented by adapter/execrunner.
type PathLookup interface {
	LookPath(name string) (string, error)
}
