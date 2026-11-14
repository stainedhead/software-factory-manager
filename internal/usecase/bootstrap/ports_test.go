package bootstrap

import (
	"context"

	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, process.Cmd) (process.Result, error) {
	return process.Result{}, nil
}

type fakeProbe struct{ p platform.Platform }

func (f fakeProbe) Probe(context.Context) (platform.Platform, error) { return f.p, nil }

type fakeLocator struct{ info platform.HomebrewInfo }

func (f fakeLocator) Locate(context.Context) (platform.HomebrewInfo, error) { return f.info, nil }
func (fakeLocator) ShellEnv(context.Context, string) (map[string]string, error) {
	return map[string]string{}, nil
}

type fakeLookup map[string]string

func (f fakeLookup) LookPath(name string) (string, error) { return f[name], nil }

var (
	_ CommandRunner   = fakeRunner{}
	_ PlatformProbe   = fakeProbe{}
	_ HomebrewLocator = fakeLocator{}
	_ PathLookup      = fakeLookup{}
)
